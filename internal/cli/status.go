package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/api"
	"github.com/string-bird/string-bird-cli/internal/config"
)

type statusOptions struct {
	locales []string
	min     float64
	json    bool
}

func (a *app) statusCmd() *cobra.Command {
	var opts statusOptions
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show how complete each locale is",
		Long: `Count, per locale, the keys that still need work:

  missing     no translation yet
  outdated    the base text changed since the translation was written
  unapproved  machine translation awaiting approval (left out of releases)

READY is the share of keys translated, approved and up to date. With --min,
sb exits with 1 when a locale's READY is below it, to gate a CI job.`,
		Example: `  sb status
  sb status --locale de --min 100`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.status(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	f.StringSliceVarP(&opts.locales, "locale", "l", nil, "only these locales (repeatable; default every locale but the base)")
	f.Float64Var(&opts.min, "min", 0, "fail when a locale is less than this percent ready")
	addJSONFlag(cmd, &opts.json)
	return cmd
}

// localeStatus is one locale's counts.
type localeStatus struct {
	Locale     string  `json:"locale"`
	Total      int     `json:"total"`
	Missing    int     `json:"missing"`
	Outdated   int     `json:"outdated"`
	Unapproved int     `json:"unapproved"`
	Ready      int     `json:"ready"`
	Percent    float64 `json:"ready_percent"`
}

func (a *app) status(ctx context.Context, opts statusOptions) error {
	if opts.min < 0 || opts.min > 100 {
		return errors.New("--min is a percentage from 0 to 100")
	}
	p, err := a.loadProject(ctx, config.Need{})
	if err != nil {
		return err
	}
	proj, err := p.client.Project(ctx)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	locales := opts.locales
	if len(locales) == 0 {
		for _, l := range proj.Locales {
			if !strings.EqualFold(l, proj.BaseLocale) {
				locales = append(locales, l)
			}
		}
	}
	all, err := p.client.Keys(ctx, api.KeyFilter{}, 0)
	if err != nil {
		return fmt.Errorf("list keys: %w", err)
	}

	var rows []localeStatus
	for _, locale := range locales {
		s := localeStatus{Locale: locale, Total: len(all)}
		// A key can be outdated and unapproved at once: count it once for READY.
		needsWork := map[string]bool{}
		for _, status := range []string{api.StatusMissing, api.StatusOutdated, api.StatusUnapproved} {
			keys, err := p.client.Keys(ctx, api.KeyFilter{Locale: locale, Status: status}, 0)
			if err != nil {
				return fmt.Errorf("count %s keys in %s: %w", status, locale, err)
			}
			for _, k := range keys {
				needsWork[k.ID] = true
			}
			switch status {
			case api.StatusMissing:
				s.Missing = len(keys)
			case api.StatusOutdated:
				s.Outdated = len(keys)
			case api.StatusUnapproved:
				s.Unapproved = len(keys)
			}
		}
		s.Ready = s.Total - len(needsWork)
		s.Percent = 100
		if s.Total > 0 {
			s.Percent = float64(s.Ready) * 100 / float64(s.Total)
		}
		rows = append(rows, s)
	}

	if opts.json {
		if rows == nil {
			rows = []localeStatus{}
		}
		if err := printJSON(a.stdout, rows); err != nil {
			return err
		}
	} else {
		t := newTable(a.stdout, "LOCALE", "READY", "MISSING", "OUTDATED", "UNAPPROVED")
		for _, s := range rows {
			t.row(s.Locale, formatPercent(s.Percent), strconv.Itoa(s.Missing), strconv.Itoa(s.Outdated), strconv.Itoa(s.Unapproved))
		}
		t.flush()
		fmt.Fprintf(a.stderr, "%d keys, base locale %s\n", len(all), proj.BaseLocale)
	}

	var below []string
	for _, s := range rows {
		if opts.min > 0 && s.Percent < opts.min {
			below = append(below, fmt.Sprintf("%s (%s)", s.Locale, formatPercent(s.Percent)))
		}
	}
	if len(below) > 0 {
		return fmt.Errorf("below %s ready: %s", formatPercent(opts.min), strings.Join(below, ", "))
	}
	return nil
}

// formatPercent shows whole percents, and one decimal below 100 so that
// 99.96% doesn't read as 100%.
func formatPercent(p float64) string {
	if p >= 100 {
		return "100%"
	}
	s := strconv.FormatFloat(p, 'f', 1, 64)
	if strings.HasSuffix(s, ".0") {
		s = strings.TrimSuffix(s, ".0")
	}
	if s == "100" {
		s = "99.9"
	}
	return s + "%"
}
