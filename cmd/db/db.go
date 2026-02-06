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
	"github.com/EscapeVelocityOperations/hatch-cli/internal/ui"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

var (
	port    int
	host    string
	launchPsql bool
)

// Deps holds injectable dependencies for testing.
type Deps struct {
	GetToken     func() (string, error)
	HasRemote    func(name string) bool
	GetRemoteURL func(name string) (string, error)
	DialWS       func(url string, header http.Header) (*websocket.Conn, *http.Response, error)
	Listen       func(network, address string) (net.Listener, error)
	RunPsql      func(host string, port int) error
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
		Listen:  net.Listen,
		RunPsql: runPsql,
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
	return cmd
}

func newConnectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect [slug]",
		Short: "Open a local TCP proxy to your nugget's database",
		Long: `Opens a WebSocket tunnel to your nugget's PostgreSQL database and starts a
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

	ui.Info(fmt.Sprintf("Database proxy for %s listening on %s", ui.Bold(slug), ui.Bold(addr)))
	ui.Info("Connect with: psql -h " + host + " -p " + fmt.Sprint(port))
	fmt.Println()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if launchPsql {
		go func() {
			if err := deps.RunPsql(host, port); err != nil {
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
	return "wss://" + api.DefaultHost[len("https://"):] + "/api/v1/apps/" + slug + "/db/tunnel"
}

func resolveSlug(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	if !deps.HasRemote("hatch") {
		return "", fmt.Errorf("no nugget specified and no hatch git remote found. Usage: hatch db connect <slug>")
	}
	url, err := deps.GetRemoteURL("hatch")
	if err != nil {
		return "", fmt.Errorf("reading hatch remote: %w", err)
	}
	return api.SlugFromRemote(url)
}

func runPsql(host string, port int) error {
	cmd := exec.Command("psql", "-h", host, "-p", fmt.Sprint(port))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
