package protect

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/auth"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/resolve"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/sessionttl"
	"github.com/spf13/cobra"
)

// EmailProtection is the CLI-facing view of an egg's email-allowlist state.
type EmailProtection struct {
	Enabled          bool
	Emails           []string
	Domains          []string
	SessionTTL       int // effective magic-link session lifetime, seconds (h-pbhn)
	MailerConfigured bool
}

// EmailAPIClient is the email-protection surface of the Hatch API.
// SetEmailProtection's sessionTTL is nil to keep the egg's session lifetime,
// 0 to reset it to the default, or seconds (h-pbhn).
type EmailAPIClient interface {
	SetEmailProtection(slug string, emails, domains []string, sessionTTL *int) (*EmailProtection, error)
	GetEmailProtection(slug string) (*EmailProtection, error)
	DeleteEmailProtection(slug string) error
}

// EmailDeps holds injectable dependencies for testing (cmd/webhook pattern).
type EmailDeps struct {
	GetToken     func() (string, error)
	GetCwd       func() (string, error)
	NewAPIClient func(token string) EmailAPIClient
}

var emailDeps = defaultEmailDeps()

func defaultEmailDeps() *EmailDeps {
	return &EmailDeps{
		GetToken: auth.GetToken,
		GetCwd:   os.Getwd,
		NewAPIClient: func(token string) EmailAPIClient {
			return &realEmailAPIClient{client: api.NewClient(token)}
		},
	}
}

// realEmailAPIClient adapts *api.Client to the EmailAPIClient surface.
type realEmailAPIClient struct{ client *api.Client }

func cliEmailProtection(ep api.EmailProtection) EmailProtection {
	return EmailProtection{
		Enabled:          ep.EmailProtected,
		Emails:           ep.Emails,
		Domains:          ep.Domains,
		SessionTTL:       ep.SessionTTLSeconds,
		MailerConfigured: ep.MailerConfigured,
	}
}

func (r *realEmailAPIClient) SetEmailProtection(slug string, emails, domains []string, sessionTTL *int) (*EmailProtection, error) {
	ep, err := r.client.SetEmailProtection(slug, emails, domains, sessionTTL)
	if err != nil {
		return nil, err
	}
	c := cliEmailProtection(*ep)
	return &c, nil
}

func (r *realEmailAPIClient) GetEmailProtection(slug string) (*EmailProtection, error) {
	ep, err := r.client.GetEmailProtection(slug)
	if err != nil {
		return nil, err
	}
	c := cliEmailProtection(*ep)
	return &c, nil
}

func (r *realEmailAPIClient) DeleteEmailProtection(slug string) error {
	return r.client.DeleteEmailProtection(slug)
}

// NewEmailCmd returns the `hatch protect email` command group.
func NewEmailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "email",
		Short: "Manage email-allowlist protection for this app",
	}
	emailEnableCmd.Flags().StringSlice("email", nil, "Exact email address(es) to allow")
	emailEnableCmd.Flags().StringSlice("domain", nil, "Email domain(s) to allow (with or without a leading @)")
	emailEnableCmd.Flags().String("session-ttl", "", "How long a magic-link sign-in lasts: 1h to 30d (e.g. 12h, 30d), or \"default\" (12h); unchanged if omitted")
	cmd.AddCommand(emailEnableCmd, emailDisableCmd, emailListCmd, emailAddCmd, emailRemoveCmd, emailSessionTTLCmd)
	return cmd
}

// Every subcommand declares Args: these resolve the egg from the current
// directory, so a stray positional must fail rather than be silently
// ignored while the cwd egg is mutated (h-abmr F1).
var emailEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable email-allowlist protection (replaces the current lists)",
	Args:  cobra.NoArgs,
	RunE:  runEmailEnable,
}

var emailDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable email-allowlist protection",
	Args:  cobra.NoArgs,
	RunE:  runEmailDisable,
}

var emailListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show the current email allowlist",
	Args:  cobra.NoArgs,
	RunE:  runEmailList,
}

var emailSessionTTLCmd = &cobra.Command{
	Use:   "session-ttl <duration|default>",
	Short: "Set how long a magic-link sign-in lasts (1h to 30d, default 12h)",
	Long: "Set how long a visitor stays signed in after clicking a magic link: " +
		"a duration between 1h and 30d (e.g. 12h, 7d, 30d), or \"default\" for 12h. " +
		"Applies to new sign-ins; existing sessions keep their original expiry " +
		"(use 'hatch protect sessions revoke' to sign everyone out).",
	Args: cobra.ExactArgs(1),
	RunE: runEmailSessionTTL,
}

var emailAddCmd = &cobra.Command{
	Use:   "add <email-or-@domain>...",
	Short: "Add email(s) or @domain(s) to the allowlist",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runEmailAdd,
}

var emailRemoveCmd = &cobra.Command{
	Use:   "remove <email-or-@domain>...",
	Short: "Remove email(s) or @domain(s) from the allowlist",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runEmailRemove,
}

// resolveEmailApp returns the email-protection API client and the current
// app slug, or a friendly error (not logged in / no app in this directory).
func resolveEmailApp() (EmailAPIClient, string, error) {
	token, err := emailDeps.GetToken()
	if err != nil || token == "" {
		return nil, "", errors.New("not logged in (run 'hatch login' first)")
	}
	if emailDeps.NewAPIClient == nil {
		return nil, "", errors.New("protect email commands are not yet wired to the API")
	}

	dir := "."
	if emailDeps.GetCwd != nil {
		d, err := emailDeps.GetCwd()
		if err != nil {
			return nil, "", fmt.Errorf("resolving working directory: %w", err)
		}
		dir = d
	}
	slug := resolve.SlugFromDir(dir)
	if slug == "" {
		return nil, "", errors.New("no app found here — run from an app directory " +
			"with a .hatch.toml (or 'hatch init' first)")
	}

	return emailDeps.NewAPIClient(token), slug, nil
}

func runEmailEnable(cmd *cobra.Command, args []string) error {
	client, slug, err := resolveEmailApp()
	if err != nil {
		return err
	}

	emails, _ := cmd.Flags().GetStringSlice("email")
	domains, _ := cmd.Flags().GetStringSlice("domain")
	if len(emails) == 0 && len(domains) == 0 {
		return errors.New("specify at least one --email or --domain")
	}
	// Normalize before send (mirrors the server's trim+lowercase, T-104) so
	// what the CLI echoes back matches what actually gets stored.
	for i, e := range emails {
		emails[i] = normalizeEmailArg(e)
	}
	for i, d := range domains {
		domains[i] = normalizeDomainArg(d)
	}
	var ttl *int
	if cmd.Flags().Changed("session-ttl") {
		raw, _ := cmd.Flags().GetString("session-ttl")
		secs, err := sessionttl.Parse(raw)
		if err != nil {
			return err
		}
		ttl = &secs
	}

	ep, err := client.SetEmailProtection(slug, emails, domains, ttl)
	if err != nil {
		return fmt.Errorf("enabling email protection: %w", err)
	}

	fmt.Printf("Email protection enabled for %s.\n", slug)
	printEmailProtection(ep)
	return nil
}

func runEmailDisable(cmd *cobra.Command, args []string) error {
	client, slug, err := resolveEmailApp()
	if err != nil {
		return err
	}

	if err := client.DeleteEmailProtection(slug); err != nil {
		return fmt.Errorf("disabling email protection: %w", err)
	}

	fmt.Printf("Email protection disabled for %s.\n", slug)
	warnUnservedPages(os.Stdout, client, slug)
	return nil
}

func runEmailList(cmd *cobra.Command, args []string) error {
	client, slug, err := resolveEmailApp()
	if err != nil {
		return err
	}

	ep, err := client.GetEmailProtection(slug)
	if err != nil {
		return fmt.Errorf("getting email protection: %w", err)
	}
	if !ep.Enabled {
		fmt.Println("Email protection is disabled for this app.")
		return nil
	}

	printEmailProtection(ep)
	return nil
}

// printEmailProtection renders the current lists (or a placeholder when
// both are empty — an enabled-but-empty allowlist blocks every visitor).
// When protection is enabled but the deployment has no mailer configured,
// no magic link can ever be sent — every allowed visitor is silently locked
// out. That warning goes to stderr, never stdout's parseable payload.
func printEmailProtection(ep *EmailProtection) {
	if len(ep.Emails) > 0 {
		fmt.Printf("Emails:  %s\n", strings.Join(ep.Emails, ", "))
	}
	if len(ep.Domains) > 0 {
		fmt.Printf("Domains: %s\n", strings.Join(ep.Domains, ", "))
	}
	if len(ep.Emails) == 0 && len(ep.Domains) == 0 {
		fmt.Println("(no emails or domains configured — this blocks every visitor)")
	}
	if ep.SessionTTL > 0 {
		fmt.Printf("Session: %s\n", sessionttl.Format(ep.SessionTTL))
	}
	if ep.Enabled && !ep.MailerConfigured {
		fmt.Fprintln(os.Stderr, "warning: magic-link mail is not configured on this deployment — allowed visitors will not receive sign-in links")
	}
}

func runEmailAdd(cmd *cobra.Command, args []string) error {
	client, slug, err := resolveEmailApp()
	if err != nil {
		return err
	}

	current, err := client.GetEmailProtection(slug)
	if err != nil {
		return fmt.Errorf("reading current allowlist: %w", err)
	}

	addEmails, addDomains, err := splitEmailArgs(args)
	if err != nil {
		return err
	}
	newEmails := mergeUnique(current.Emails, addEmails)
	newDomains := mergeUnique(current.Domains, addDomains)

	ep, err := client.SetEmailProtection(slug, newEmails, newDomains, nil)
	if err != nil {
		return fmt.Errorf("updating allowlist: %w", err)
	}

	fmt.Printf("Updated email allowlist for %s.\n", slug)
	printEmailProtection(ep)
	return nil
}

func runEmailRemove(cmd *cobra.Command, args []string) error {
	client, slug, err := resolveEmailApp()
	if err != nil {
		return err
	}

	current, err := client.GetEmailProtection(slug)
	if err != nil {
		return fmt.Errorf("reading current allowlist: %w", err)
	}

	removeEmails, removeDomains, err := splitEmailArgs(args)
	if err != nil {
		return err
	}
	newEmails, notFoundEmails := removeItems(current.Emails, removeEmails)
	newDomains, notFoundDomains := removeItems(current.Domains, removeDomains)
	if notFound := append(notFoundEmails, notFoundDomains...); len(notFound) > 0 {
		return fmt.Errorf("not on the allowlist: %s", strings.Join(notFound, ", "))
	}

	ep, err := client.SetEmailProtection(slug, newEmails, newDomains, nil)
	if err != nil {
		return fmt.Errorf("updating allowlist: %w", err)
	}

	fmt.Printf("Updated email allowlist for %s.\n", slug)
	printEmailProtection(ep)
	return nil
}

func runEmailSessionTTL(cmd *cobra.Command, args []string) error {
	secs, err := sessionttl.Parse(args[0])
	if err != nil {
		return err
	}
	client, slug, err := resolveEmailApp()
	if err != nil {
		return err
	}

	// Set-replace API: re-send the current lists unchanged with the new TTL.
	current, err := client.GetEmailProtection(slug)
	if err != nil {
		return fmt.Errorf("reading current allowlist: %w", err)
	}
	if !current.Enabled {
		return errors.New("email protection is disabled for this app — enable it first with 'hatch protect email enable'")
	}

	ep, err := client.SetEmailProtection(slug, current.Emails, current.Domains, &secs)
	if err != nil {
		return fmt.Errorf("setting session TTL: %w", err)
	}

	fmt.Printf("Magic-link sessions for %s now last %s.\n", slug, sessionttl.Format(ep.SessionTTL))
	return nil
}

// normalizeEmailArg trims and lowercases a single email argument so it
// compares equal to the server-normalized form (T-104's normalizeEmailList)
// regardless of how the user typed it.
func normalizeEmailArg(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// normalizeDomainArg trims, lowercases, and strips a leading "@" from a
// single domain argument — mirrors the server's normalizeDomainList so the
// CLI's local view (used to dedupe/diff against the current allowlist in
// mergeUnique/removeItems) matches what the server actually stored. Returns
// "" if nothing remains after stripping.
func normalizeDomainArg(s string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "@")
}

// splitEmailArgs partitions positional args into emails and domains: an
// "@"-prefixed arg is a domain (prefix stripped), everything else is an
// exact email address. Both are trimmed and lowercased (normalizeEmailArg /
// normalizeDomainArg) so mergeUnique/removeItems compare correctly against
// the server-normalized current list. A bare "@" (nothing left after
// stripping) is rejected rather than silently turned into an empty-string
// domain the server would drop without explanation.
func splitEmailArgs(args []string) (emails, domains []string, err error) {
	for _, a := range args {
		if strings.HasPrefix(strings.TrimSpace(a), "@") {
			d := normalizeDomainArg(a)
			if d == "" {
				return nil, nil, fmt.Errorf("%q is not a valid domain (nothing after \"@\")", a)
			}
			domains = append(domains, d)
		} else {
			emails = append(emails, normalizeEmailArg(a))
		}
	}
	return emails, domains, nil
}

// mergeUnique appends add to existing, skipping anything already present —
// the read-modify-write core of `protect email add`.
func mergeUnique(existing, add []string) []string {
	seen := make(map[string]struct{}, len(existing))
	out := make([]string, 0, len(existing)+len(add))
	for _, e := range existing {
		if _, ok := seen[e]; !ok {
			seen[e] = struct{}{}
			out = append(out, e)
		}
	}
	for _, a := range add {
		if _, ok := seen[a]; !ok {
			seen[a] = struct{}{}
			out = append(out, a)
		}
	}
	return out
}

// removeItems returns existing minus remove, plus any remove entries NOT
// found in existing as notFound — the caller errors on those instead of
// silently no-op'ing a typo (T-304: "remove unknown item errors cleanly").
func removeItems(existing, remove []string) (result, notFound []string) {
	existingSet := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		existingSet[e] = struct{}{}
	}
	removeSet := make(map[string]struct{}, len(remove))
	for _, r := range remove {
		if _, ok := existingSet[r]; !ok {
			notFound = append(notFound, r)
		}
		removeSet[r] = struct{}{}
	}
	for _, e := range existing {
		if _, ok := removeSet[e]; !ok {
			result = append(result, e)
		}
	}
	return result, notFound
}
