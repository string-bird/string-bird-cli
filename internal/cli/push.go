package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/api"
)

type pushOptions struct {
	translations  bool
	locales       []string
	dryRun        bool
	skipExisting  bool
	deleteMissing bool
}

func (a *app) pushCmd() *cobra.Command {
	var opts pushOptions
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Upload the source file, and optionally translations",
		Long: `Upload files through the import endpoint. The server parses and validates
them; a file with a bad entry is rejected whole and nothing of it is saved.

By default only the base-locale (source) file is pushed: new keys are added and
changed texts saved. --translations also pushes every other project locale whose
file exists, overwriting translations that differ unless --skip-existing.`,
		Example: `  sb push
  sb push --dry-run --delete-missing
  sb push --translations --skip-existing
  sb push --locale de --locale fr`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.push(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&opts.translations, "translations", false, "also push the other locales' files")
	f.StringSliceVarP(&opts.locales, "locale", "l", nil, "push only these locales (repeatable)")
	f.BoolVar(&opts.dryRun, "dry-run", false, "show what would change without saving")
	f.BoolVar(&opts.skipExisting, "skip-existing", false, "don't overwrite existing translations")
	f.BoolVar(&opts.deleteMissing, "delete-missing", false, "delete keys the base-locale file lacks")
	cmd.MarkFlagsMutuallyExclusive("translations", "locale")
	return cmd
}

func (a *app) push(ctx context.Context, opts pushOptions) error {
	p, err := a.loadProject()
	if err != nil {
		return err
	}
	proj, err := p.client.Project(ctx)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	locales, explicit := pushLocales(proj, opts)
	if opts.deleteMissing && !containsFold(locales, proj.BaseLocale) {
		return fmt.Errorf("--delete-missing works on the base locale (%s) only", proj.BaseLocale)
	}

	pushed := 0
	for _, locale := range locales {
		path := p.path(locale)
		file, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) && !explicit && !strings.EqualFold(locale, proj.BaseLocale) {
			fmt.Fprintf(a.stderr, "skip %s: %s not found\n", locale, path)
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s file: %w", locale, err)
		}
		res, err := p.client.Import(ctx, file, api.ImportOptions{
			Locale:        locale,
			Format:        p.cfg.Format,
			DryRun:        opts.dryRun,
			SkipExisting:  opts.skipExisting,
			DeleteMissing: opts.deleteMissing && strings.EqualFold(locale, proj.BaseLocale),
		})
		if err != nil {
			a.printProblems(err)
			return fmt.Errorf("push %s (%s): %w", locale, path, err)
		}
		pushed++
		verb := "pushed"
		if res.DryRun {
			verb = "dry run"
		}
		fmt.Fprintf(a.stdout, "%s %s (%s): %s\n", verb, locale, path, summarize(res.Summary))
		if res.DryRun {
			a.printChanges(res)
		}
	}
	if pushed == 0 {
		return errors.New("no files pushed")
	}
	return nil
}

// pushLocales picks the locales to push, base locale first so new keys exist
// before their translations arrive. explicit is true when --locale named them.
func pushLocales(proj api.Project, opts pushOptions) (locales []string, explicit bool) {
	if len(opts.locales) > 0 {
		var base, rest []string
		for _, l := range opts.locales {
			if strings.EqualFold(l, proj.BaseLocale) {
				base = append(base, l)
			} else {
				rest = append(rest, l)
			}
		}
		return append(base, rest...), true
	}
	locales = []string{proj.BaseLocale}
	if opts.translations {
		for _, l := range proj.Locales {
			if !strings.EqualFold(l, proj.BaseLocale) {
				locales = append(locales, l)
			}
		}
	}
	return locales, false
}

func summarize(s api.ImportSummary) string {
	var parts []string
	for _, c := range []struct {
		n           int
		one, others string
	}{
		{s.AddedKeys, "key added", "keys added"},
		{s.Created, "translation created", "translations created"},
		{s.Updated, "updated", "updated"},
		{s.DeletedKeys, "key deleted", "keys deleted"},
		{s.SkippedExisting, "skipped as existing", "skipped as existing"},
		{s.Unchanged, "unchanged", "unchanged"},
	} {
		switch {
		case c.n == 1:
			parts = append(parts, "1 "+c.one)
		case c.n > 1:
			parts = append(parts, fmt.Sprintf("%d %s", c.n, c.others))
		}
	}
	if len(parts) == 0 {
		return "empty file"
	}
	return strings.Join(parts, ", ")
}

func (a *app) printChanges(res api.ImportResult) {
	for _, k := range res.AddedKeys {
		fmt.Fprintf(a.stdout, "  + %s\n", k)
	}
	for _, u := range res.Updated {
		fmt.Fprintf(a.stdout, "  ~ %s: %q → %q\n", u.Key, u.OldValue, u.NewValue)
	}
	for _, k := range res.DeletedKeys {
		fmt.Fprintf(a.stdout, "  - %s\n", k)
	}
}

// printProblems lists the bad entries of a rejected import.
func (a *app) printProblems(err error) {
	var apiErr *api.Error
	if !errors.As(err, &apiErr) {
		return
	}
	for _, p := range apiErr.Problems {
		fmt.Fprintf(a.stderr, "  %s: %s\n", p.Key, p.Reason)
	}
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
