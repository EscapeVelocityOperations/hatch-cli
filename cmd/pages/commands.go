package pages

import (
	"bufio"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
	"github.com/EscapeVelocityOperations/hatch-cli/internal/pagepack"
	"github.com/spf13/cobra"
)

func newPublishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish <egg> <file.html|folder>",
		Short: "Publish a page, or a new version of it",
		Long: "Publish an .html file (served as index.html) or a folder containing index.html at\n" +
			"/pages/<name>/ on <egg>. Publishing again at the same --at creates the next version;\n" +
			"the main link shows the latest and /pages/<name>/v<N>/ keeps each version.\n\n" +
			"--at defaults to a name derived from --title, else from the file or folder name.\n" +
			"Hidden files (.git, .env, .DS_Store…) and symlinks are left out.",
		Example: "  hatch pages publish portail-jbi rapport.html --at /pages/bilan/ --title \"Bilan\"\n" +
			"  hatch pages publish portail-jbi ./site/pages/bilan --as \"Claude (bosco)\"",
		Args: cobra.ExactArgs(2),
		RunE: runE(runPublish),
	}
	cmd.Flags().String("at", "", "Page path or name, e.g. /pages/bilan/ (default: from --title or the file name)")
	cmd.Flags().String("title", "", "Page title shown in the version bar and the /pages/ index")
	cmd.Flags().String("as", "", "Author label shown to visitors (default: your token's name)")
	cmd.Flags().Bool("owner", false, "Publish as the egg's owner rather than as an agent")
	cmd.Flags().String("date", "", "Display date YYYY-MM-DD (default: today)")
	cmd.Flags().Bool("json", false, "Print the result as JSON")
	cmd.MarkFlagsMutuallyExclusive("as", "owner")
	return cmd
}

// pageNameFor picks the page name: --at, else the title, else the file or
// folder name.
func pageNameFor(at, title, path string) (string, error) {
	if at != "" {
		return api.NormalizePageName(at)
	}
	if s := pagepack.Slugify(title); s != "" {
		return api.NormalizePageName(s)
	}
	base := filepath.Base(filepath.Clean(path))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if s := pagepack.Slugify(base); s != "" && s != "index" {
		return api.NormalizePageName(s)
	}
	return "", errors.New("cannot derive a page name: pass --at /pages/<name>/")
}

func runPublish(cmd *cobra.Command, args []string) error {
	slug, path := args[0], args[1]
	at, _ := cmd.Flags().GetString("at")
	title, _ := cmd.Flags().GetString("title")
	label, _ := cmd.Flags().GetString("as")
	owner, _ := cmd.Flags().GetBool("owner")
	date, _ := cmd.Flags().GetString("date")
	asJSON, _ := cmd.Flags().GetBool("json")

	name, err := pageNameFor(at, title, path)
	if err != nil {
		return err
	}
	if date != "" {
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			return fmt.Errorf("invalid --date %q: want YYYY-MM-DD", date)
		}
	}
	pack, err := pagepack.Build(path)
	if err != nil {
		return err
	}
	meta := api.PagePublishMeta{Name: name, Title: title, AuthorLabel: label, DisplayDate: date}
	if owner {
		meta.AuthorKind = "owner"
	}

	c, err := client()
	if err != nil {
		return err
	}
	res, err := c.PublishPage(slug, pack.Archive, meta)
	if err != nil {
		return apiErr("publishing "+name, slug, err)
	}
	if asJSON {
		skipped := pack.Skipped
		if skipped == nil {
			skipped = []string{}
		}
		return writeJSON(cmd.OutOrStdout(), map[string]any{
			"egg": slug, "name": res.Name, "title": res.Title, "version": res.Version,
			"url": res.URL, "version_url": res.VersionURL, "files": res.Files,
			"bytes": res.Bytes, "dedup_bytes": res.DedupBytes, "skipped": skipped,
		})
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Published %s v%d on %s (%d files, %s", res.Name, res.Version, slug, res.Files, humanBytes(res.Bytes))
	if res.DedupBytes > 0 {
		fmt.Fprintf(out, ", %s already stored", humanBytes(res.DedupBytes))
	}
	fmt.Fprintln(out, ")")
	fmt.Fprintf(out, "  Link:    %s\n", res.URL)
	fmt.Fprintf(out, "  Version: %s\n", res.VersionURL)
	if len(pack.Skipped) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "Left out (hidden or not a regular file): %s\n", strings.Join(pack.Skipped, ", "))
	}
	return nil
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list <egg>",
		Short: "List an egg's pages with their latest version",
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			asJSON, _ := cmd.Flags().GetBool("json")
			c, err := client()
			if err != nil {
				return err
			}
			l, err := c.ListPages(slug)
			if err != nil {
				return apiErr("listing pages", slug, err)
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), map[string]any{"egg": slug, "served": l.Gateway, "pages": l.Pages})
			}
			out := cmd.OutOrStdout()
			if !l.Gateway {
				fmt.Fprintf(cmd.ErrOrStderr(), "Note: %s is not protected, so its pages are not served (protect it to serve them).\n", slug)
			}
			if len(l.Pages) == 0 {
				fmt.Fprintf(out, "No pages on %s.\n", slug)
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "PAGE\tLATEST\tVERSIONS\tDATE\tAUTHOR\tTITLE\tLINK")
			for _, p := range l.Pages {
				author := ""
				if p.AuthorLabel != nil {
					author = *p.AuthorLabel
				}
				fmt.Fprintf(tw, "%s\tv%d\t%d\t%s\t%s\t%s\t%s\n", p.Name, p.LatestVersion, p.Versions,
					p.DisplayDate, oneLine(author), oneLine(p.Title), p.URL)
			}
			return tw.Flush()
		}),
	}
	cmd.Flags().Bool("json", false, "Print the pages as JSON")
	return cmd
}

func newVersionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "versions <egg> <page>",
		Short: "List the versions of a page, newest first",
		Args:  cobra.ExactArgs(2),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			asJSON, _ := cmd.Flags().GetBool("json")
			c, err := client()
			if err != nil {
				return err
			}
			v, err := c.ListPageVersions(slug, args[1])
			if err != nil {
				return apiErr("listing versions", slug, err)
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), map[string]any{"egg": slug, "page": v})
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s — %s\n", v.Name, oneLine(v.Title))
			fmt.Fprintf(out, "  Link: %s (shows v%d)\n\n", v.URL, v.LatestVersion)
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "VERSION\tDATE\tAUTHOR\tFILES\tSIZE\tTITLE\tLINK")
			for _, pv := range v.Versions {
				author := pv.AuthorEmail
				if pv.AuthorLabel != nil && *pv.AuthorLabel != "" {
					author = *pv.AuthorLabel
				}
				mark := ""
				if pv.Latest {
					mark = " (latest)"
				}
				fmt.Fprintf(tw, "v%d%s\t%s\t%s\t%d\t%s\t%s\t%s\n", pv.Version, mark, pv.DisplayDate, oneLine(author),
					pv.Files, humanBytes(pv.Bytes), oneLine(pv.Title), pv.URL)
			}
			return tw.Flush()
		}),
	}
	cmd.Flags().Bool("json", false, "Print the versions as JSON")
	return cmd
}

func newRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <egg> <page>",
		Short: "Remove a page (all versions) or one version of it",
		Long: "Remove <page> and all its versions, or only --version N. Version numbers are never\n" +
			"reused: publishing the same name again continues at the next number, so an old\n" +
			"/vN/ link never shows different content. --purge also forgets the name (numbering\n" +
			"restarts at v1). Without --yes you are asked to confirm; outside a terminal --yes\n" +
			"is required.",
		Args: cobra.ExactArgs(2),
		RunE: runE(runRemove),
	}
	cmd.Flags().Int("version", 0, "Remove only this version")
	cmd.Flags().Bool("purge", false, "Also forget the name so its numbering restarts")
	cmd.Flags().Bool("yes", false, "Do not ask for confirmation")
	cmd.Flags().Bool("json", false, "Print the result as JSON")
	cmd.MarkFlagsMutuallyExclusive("version", "purge")
	return cmd
}

func runRemove(cmd *cobra.Command, args []string) error {
	slug := args[0]
	name, err := api.NormalizePageName(args[1])
	if err != nil {
		return err
	}
	version, _ := cmd.Flags().GetInt("version")
	purge, _ := cmd.Flags().GetBool("purge")
	yes, _ := cmd.Flags().GetBool("yes")
	asJSON, _ := cmd.Flags().GetBool("json")
	if version < 0 {
		return errors.New("--version must be a positive number")
	}

	what := fmt.Sprintf("page %s (all versions) from %s", name, slug)
	if version > 0 {
		what = fmt.Sprintf("version v%d of page %s from %s", version, name, slug)
	} else if purge {
		what = fmt.Sprintf("page %s (all versions, and its name) from %s", name, slug)
	}
	if !yes {
		if !deps.StdinIsTerminal() {
			return fmt.Errorf("refusing to remove %s without --yes outside a terminal", what)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Remove %s? [y/N] ", what)
		line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
			return errors.New("removal cancelled")
		}
	}

	c, err := client()
	if err != nil {
		return err
	}
	res, err := c.RemovePage(slug, name, version, purge)
	if err != nil {
		return apiErr("removing "+name, slug, err)
	}
	if asJSON {
		return writeJSON(cmd.OutOrStdout(), map[string]any{"egg": slug, "result": res})
	}
	out := cmd.OutOrStdout()
	switch {
	case version > 0 && res.PageRemoved:
		fmt.Fprintf(out, "Removed v%d of %s on %s; it was the last version, so the page is gone.\n", version, name, slug)
	case version > 0:
		fmt.Fprintf(out, "Removed v%d of %s on %s.\n", version, name, slug)
	case purge:
		fmt.Fprintf(out, "Removed page %s and its name on %s.\n", name, slug)
	default:
		fmt.Fprintf(out, "Removed page %s on %s (republishing it continues the version numbers).\n", name, slug)
	}
	return nil
}

// oneLine keeps table cells on one line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
