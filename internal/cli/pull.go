package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/api"
)

type pullOptions struct {
	locales           []string
	includeUnapproved bool
	fallback          bool
}

func (a *app) pullCmd() *cobra.Command {
	var opts pullOptions
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Download translated files",
		Long: `Download every project locale (or the ones named) through the export
endpoint and write them to the files path in the config, replacing what's there.

Machine translations awaiting approval are left out unless --include-unapproved.`,
		Example: `  sb pull
  sb pull --locale de --fallback`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.pull(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	f.StringSliceVarP(&opts.locales, "locale", "l", nil, "pull only these locales (repeatable)")
	f.BoolVar(&opts.includeUnapproved, "include-unapproved", false, "include machine translations awaiting approval")
	f.BoolVar(&opts.fallback, "fallback", false, "fill missing translations from the fallback chain (de-AT → de → base)")
	return cmd
}

func (a *app) pull(ctx context.Context, opts pullOptions) error {
	p, err := a.loadProject()
	if err != nil {
		return err
	}
	locales := opts.locales
	if len(locales) == 0 {
		proj, err := p.client.Project(ctx)
		if err != nil {
			return fmt.Errorf("get project: %w", err)
		}
		locales = proj.Locales
	}

	for _, locale := range locales {
		exp, err := p.client.Export(ctx, api.ExportOptions{
			Locale:            locale,
			Format:            p.cfg.Format,
			Structure:         p.cfg.Structure,
			IncludeUnapproved: opts.includeUnapproved,
			Fallback:          opts.fallback,
		})
		if err != nil {
			return fmt.Errorf("pull %s: %w", locale, err)
		}
		path := p.path(locale)
		if err := writeFile(path, exp.Body); err != nil {
			return fmt.Errorf("pull %s: %w", locale, err)
		}
		fmt.Fprintf(a.stdout, "pulled %s → %s\n", locale, path)
		if n := len(exp.SkippedKeys); n > 0 {
			fmt.Fprintf(a.stderr, "warning: %s: %d keys the format can't express were left out: %s\n",
				locale, n, strings.Join(exp.SkippedKeys, ", "))
		}
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}
