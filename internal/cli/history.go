package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/config"
)

func (a *app) releaseListCmd() *cobra.Command {
	var limit int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List releases, newest first, and where each is live",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.releaseList(cmd.Context(), limit, asJSON)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "how many releases (at most 200)")
	addJSONFlag(cmd, &asJSON)
	return cmd
}

func (a *app) releaseList(ctx context.Context, limit int, asJSON bool) error {
	if limit < 1 || limit > 200 {
		return errors.New("--limit is from 1 to 200")
	}
	p, err := a.loadProject(ctx, config.Need{})
	if err != nil {
		return err
	}
	releases, err := p.client.Releases(ctx, limit)
	if err != nil {
		return fmt.Errorf("list releases: %w", err)
	}
	if asJSON {
		return printJSON(a.stdout, releases)
	}
	if len(releases) == 0 {
		fmt.Fprintln(a.stderr, "No releases yet: sb release create makes the first.")
		return nil
	}
	envs, err := p.client.Environments(ctx)
	if err != nil {
		return fmt.Errorf("list environments: %w", err)
	}
	live := map[string][]string{}
	for _, e := range envs {
		if e.Release != nil {
			live[e.Release.ID] = append(live[e.Release.ID], e.Name)
		}
	}
	t := newTable(a.stdout, "VERSION", "STATUS", "KEYS", "LOCALES", "CREATED", "LIVE IN", "NOTE")
	for _, r := range releases {
		sort.Strings(live[r.ID])
		t.row(fmt.Sprintf("v%d", r.Version), r.Status, strconv.Itoa(r.KeyCount), strconv.Itoa(len(r.Locales)),
			formatTime(r.CreatedAt), orDash(strings.Join(live[r.ID], ", ")), orDash(shorten(r.Note, 40)))
	}
	t.flush()
	return nil
}

func (a *app) releaseDiffCmd() *cobra.Command {
	var against string
	var stat, asJSON bool
	cmd := &cobra.Command{
		Use:   "diff <release>",
		Short: "Show what changed between two releases",
		Long: `Show the keys added, changed and removed between two releases, per locale.

A release is a version (12, v12), an id, or an environment name for the
release it serves. --against defaults to the release before. Check what a
promotion would change before running it:

  sb release diff staging --against production`,
		Example: `  sb release diff v13
  sb release diff staging --against production --stat`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.releaseDiff(cmd.Context(), args[0], against, stat, asJSON)
		},
	}
	cmd.Flags().StringVar(&against, "against", "", "release to compare with (default: the one before)")
	cmd.Flags().BoolVar(&stat, "stat", false, "only count the changes per locale")
	addJSONFlag(cmd, &asJSON)
	return cmd
}

func (a *app) releaseDiff(ctx context.Context, ref, against string, stat, asJSON bool) error {
	p, err := a.loadProject(ctx, config.Need{})
	if err != nil {
		return err
	}
	to, err := findRelease(ctx, p.client, ref)
	if err != nil {
		return err
	}
	againstID := ""
	if against != "" {
		from, err := findRelease(ctx, p.client, against)
		if err != nil {
			return err
		}
		againstID = from.ID
	}
	diff, err := p.client.ReleaseDiff(ctx, to.ID, againstID)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}
	if asJSON {
		return printJSON(a.stdout, diff)
	}
	from := "nothing"
	if diff.From != nil {
		from = fmt.Sprintf("v%d", diff.From.Version)
	}
	fmt.Fprintf(a.stdout, "v%d against %s\n", diff.To.Version, from)
	if len(diff.Locales) == 0 {
		fmt.Fprintln(a.stdout, "no changes")
		return nil
	}
	for _, l := range diff.Locales {
		fmt.Fprintf(a.stdout, "%s: %d added, %d changed, %d removed\n", l.Locale, len(l.Added), len(l.Changed), len(l.Removed))
		if stat {
			continue
		}
		for _, v := range l.Added {
			fmt.Fprintf(a.stdout, "  + %s: %q\n", v.Key, v.Value)
		}
		for _, c := range l.Changed {
			fmt.Fprintf(a.stdout, "  ~ %s: %q → %q\n", c.Key, c.OldValue, c.NewValue)
		}
		for _, v := range l.Removed {
			fmt.Fprintf(a.stdout, "  - %s\n", v.Key)
		}
	}
	return nil
}
