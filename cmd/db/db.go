package db

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/git"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/resolve"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

var (
	port       int
	host       string
	launchPsql bool
)

// dbCreds holds parsed database credentials for psql.
type dbCreds struct {
	User     string
	Password string
	DBName   string
}

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken     func() (string, error)
	HasRemote    func(name string) bool
	GetRemoteURL func(name string) (string, error)
	DialWS       func(url string, header http.Header) (*websocket.Conn, *http.Response, error)
	Listen       func(network, address string) (net.Listener, error)
	RunPsql      func(host string, port int, creds *dbCreds) error
}

func defaultDeps() *Deps {
	dialer := &websocket.Dialer{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	return &Deps{
		GetToken:     auth.GetToken,
		HasRemote:    git.HasRemote,
		GetRemoteURL: git.GetRemoteURL,
		DialWS: func(url string, header http.Header) (*websocket.Conn, *http.Response, error) {
			return dialer.Dial(url, header)
		},
		Listen: net.Listen,
		RunPsql: func(host string, port int, creds *dbCreds) error {
			return runPsql(host, port, creds)
		},
	}
}

var deps = defaultDeps()

// NewCmd returns the db command group.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database management commands",
	}
	cmd.AddCommand(newConnectCmd())
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newInfoCmd())
	return cmd
}

func newConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect [slug]",
		Short: "Open a local TCP proxy to your egg's database",
		Long: `Opens a WebSocket tunnel to your egg's PostgreSQL database and starts a
local TCP listener. Connect with any PostgreSQL client:

  psql -h localhost -p 15432

Or use --psql to auto-launch psql.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConnect,
	}
	cmd.Flags().IntVarP(&port, "port", "p", 15432, "local port to listen on")
	cmd.Flags().StringVar(&host, "host", "localhost", "local address to bind to")
	cmd.Flags().BoolVar(&launchPsql, "psql", false, "auto-launch psql after connecting")
	return cmd
}

func runConnect(cmd *cobra.Command, args []string) error {
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	slug, err := resolveSlug(args)
	if err != nil {
		return err
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	listener, err := deps.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("binding %s: %w", addr, err)
	}
	defer listener.Close()

	// Warn if binding to non-loopback address
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		ui.Warn(fmt.Sprintf("Warning: binding to %s exposes the database proxy to the network. Use --host localhost for local-only access.", host))
	}

	// Fetch database credentials for psql connection
	client := api.NewClient(token)
	var creds *dbCreds
	dbURL, err := client.GetDatabaseURL(slug)
	if err == nil && dbURL != "" {
		creds = parseDBURL(dbURL)
	}

	ui.Info(fmt.Sprintf("Database proxy for %s listening on %s", ui.Bold(slug), ui.Bold(addr)))
	if creds != nil {
		ui.Info(fmt.Sprintf("Connect with: PGPASSWORD='...' psql -h %s -p %d -U %s %s", host, port, creds.User, creds.DBName))
	} else {
		ui.Info("Connect with: psql -h " + host + " -p " + fmt.Sprint(port))
	}
	fmt.Println()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if launchPsql {
		go func() {
			if err := deps.RunPsql(host, port, creds); err != nil {
				ui.Error(fmt.Sprintf("psql: %v", err))
			}
			// After psql exits, shut down the proxy
			sigCh <- syscall.SIGINT
		}()
	}

	// Accept connections until signal
	go func() {
		<-sigCh
		fmt.Println()
		ui.Info("Shutting down...")
		listener.Close()
	}()

	wsURL := wsURLForSlug(slug)
	header := http.Header{"Authorization": {"Bearer " + token}}

	for {
		conn, err := listener.Accept()
		if err != nil {
			// Listener closed (shutdown)
			return nil
		}
		go handleConn(conn, wsURL, header)
	}
}

func handleConn(tcpConn net.Conn, wsURL string, header http.Header) {
	defer tcpConn.Close()

	wsConn, _, err := deps.DialWS(wsURL, header)
	if err != nil {
		ui.Error(fmt.Sprintf("WebSocket dial: %v", err))
		return
	}
	defer wsConn.Close()

	ui.Info("Client connected, tunnel active")

	var wg sync.WaitGroup
	wg.Add(2)

	// TCP -> WebSocket
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := tcpConn.Read(buf)
			if err != nil {
				if err != io.EOF {
					ui.Error(fmt.Sprintf("TCP read: %v", err))
				}
				wsConn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if err := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	// WebSocket -> TCP
	go func() {
		defer wg.Done()
		for {
			_, data, err := wsConn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					// Only log if not a normal close
					if err != io.EOF {
						ui.Error(fmt.Sprintf("WebSocket read: %v", err))
					}
				}
				tcpConn.Close()
				return
			}
			if _, err := tcpConn.Write(data); err != nil {
				return
			}
		}
	}()

	wg.Wait()
	ui.Info("Client disconnected")
}

func wsURLForSlug(slug string) string {
	return "wss://" + api.DefaultHost[len("https://"):] + "/v1/apps/" + slug + "/db/tunnel"
}

func resolveSlug(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	// Check .hatch.toml
	if slug := resolve.SlugFromToml(); slug != "" {
		return slug, nil
	}
	if !deps.HasRemote("hatch") {
		return "", fmt.Errorf("no egg specified and no hatch git remote found. Usage: hatch db connect <slug>")
	}
	url, err := deps.GetRemoteURL("hatch")
	if err != nil {
		return "", fmt.Errorf("reading hatch remote: %w", err)
	}
	return api.SlugFromRemote(url)
}

func runPsql(host string, port int, creds *dbCreds) error {
	args := []string{"-h", host, "-p", fmt.Sprint(port)}
	if creds != nil {
		args = append(args, "-U", creds.User, creds.DBName)
	}
	cmd := exec.Command("psql", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if creds != nil && creds.Password != "" {
		cmd.Env = append(os.Environ(), "PGPASSWORD="+creds.Password)
	}
	return cmd.Run()
}

// parseDBURL extracts user, password, and dbname from a PostgreSQL URL.
func parseDBURL(dbURL string) *dbCreds {
	// postgresql://user:pass@host:port/dbname
	idx := len("postgresql://")
	if len(dbURL) <= idx {
		return nil
	}
	rest := dbURL[idx:] // user:pass@host:port/dbname

	atIdx := -1
	for i, c := range rest {
		if c == '@' {
			atIdx = i
			break
		}
	}
	if atIdx < 0 {
		return nil
	}

	userPass := rest[:atIdx]
	hostDB := rest[atIdx+1:]

	var user, pass string
	colonIdx := -1
	for i, c := range userPass {
		if c == ':' {
			colonIdx = i
			break
		}
	}
	if colonIdx >= 0 {
		user = userPass[:colonIdx]
		pass = userPass[colonIdx+1:]
	} else {
		user = userPass
	}

	// Extract dbname from host:port/dbname
	var dbName string
	slashIdx := -1
	for i, c := range hostDB {
		if c == '/' {
			slashIdx = i
			break
		}
	}
	if slashIdx >= 0 {
		dbName = hostDB[slashIdx+1:]
	}

	return &dbCreds{User: user, Password: pass, DBName: dbName}
}

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [slug]",
		Short: "Provision a PostgreSQL database for your egg",
		Long: `Provisions a managed PostgreSQL database and sets DATABASE_URL automatically.

Free tier limits: 50 MB storage, 10,000 rows.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runAdd,
	}
	return cmd
}

func runAdd(cmd *cobra.Command, args []string) error {
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	slug, err := resolveSlug(args)
	if err != nil {
		return err
	}

	ui.Info(fmt.Sprintf("Provisioning PostgreSQL database for %s...", ui.Bold(slug)))

	client := api.NewClient(token)
	addon, err := client.AddAddon(slug, "postgresql")
	if err != nil {
		return fmt.Errorf("provisioning database: %w", err)
	}

	if addon.Status == "active" {
		ui.Success("Database ready!")
		if addon.DatabaseURL != "" {
			ui.Info("DATABASE_URL has been set automatically.")
		}
		ui.Info(fmt.Sprintf("Connect locally: hatch db connect %s", slug))
		fmt.Println()
		ui.Info("Limits: 50 MB storage, 10,000 rows (free tier)")
	} else {
		ui.Warn(fmt.Sprintf("Database status: %s", addon.Status))
	}

	return nil
}

func newInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info [slug]",
		Short: "Show database status and usage for your egg",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runInfo,
	}
	return cmd
}

func runInfo(cmd *cobra.Command, args []string) error {
	token, err := deps.GetToken()
	if err != nil {
		return fmt.Errorf("checking auth: %w", err)
	}
	if token == "" {
		return fmt.Errorf("not logged in. Run 'hatch login', set HATCH_TOKEN, or use --token")
	}

	slug, err := resolveSlug(args)
	if err != nil {
		return err
	}

	client := api.NewClient(token)
	addons, err := client.ListAddons(slug)
	if err != nil {
		return fmt.Errorf("fetching addons: %w", err)
	}

	// Find postgresql addon
	var pgAddon *api.Addon
	for i, a := range addons {
		if a.Type == "postgresql" {
			pgAddon = &addons[i]
			break
		}
	}

	if pgAddon == nil {
		return fmt.Errorf("no database provisioned for %s. Run: hatch db add %s", slug, slug)
	}

	fmt.Printf("Database for %s\n", ui.Bold(slug))
	fmt.Printf("  Status:  %s\n", pgAddon.Status)

	if pgAddon.PostgresBytesUsed != nil && pgAddon.PostgresLimitBytes != nil {
		used := *pgAddon.PostgresBytesUsed
		limit := *pgAddon.PostgresLimitBytes
		pct := float64(0)
		if limit > 0 {
			pct = float64(used) / float64(limit) * 100
		}
		fmt.Printf("  Size:    %s / %s (%.1f%%)\n", formatBytes(used), formatBytes(limit), pct)
	}

	if pgAddon.PostgresRowsUsed != nil && pgAddon.PostgresLimitRows != nil {
		used := *pgAddon.PostgresRowsUsed
		limit := *pgAddon.PostgresLimitRows
		if limit > 0 {
			pct := float64(used) / float64(limit) * 100
			fmt.Printf("  Rows:    %s / %s (%.1f%%)\n", formatCount(used), formatCount(limit), pct)
		} else {
			fmt.Printf("  Rows:    %s (unlimited)\n", formatCount(used))
		}
	}

	if pgAddon.WritesBlocked != nil {
		if *pgAddon.WritesBlocked {
			fmt.Printf("  Writes:  %s\n", ui.Bold("BLOCKED — quota exceeded"))
		} else {
			fmt.Printf("  Writes:  allowed\n")
		}
	}

	return nil
}

func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatCount(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
