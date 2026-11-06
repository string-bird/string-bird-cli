package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/api"
	"github.com/string-bird/string-bird-cli/internal/config"
)

type keysOptions struct {
	prefix     string
	locale     string
	missing    string
	outdated   string
	unapproved string
	limit      int
	json       bool
}

func (a *app) keysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "List translation keys",
		Args:  cobra.NoArgs,
	}
	var opts keysOptions
	list := &cobra.Command{
		Use:   "list",
		Short: "List keys with their texts",
		Long: `List the project's keys by name, with the base-locale text and, with
--locale or a status filter, that locale's text.

The status filters list the keys that still need work in one locale:
--missing de (no translation), --outdated de (base text changed since), and
--unapproved de (machine translations awaiting approval).`,
		Example: `  sb keys list --prefix auth.
  sb keys list --missing de
  sb keys list --unapproved fr --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.keysList(cmd.Context(), opts)
		},
	}
	f := list.Flags()
	f.StringVar(&opts.prefix, "prefix", "", "only keys starting with this, like auth.")
	f.StringVarP(&opts.locale, "locale", "l", "", "also show this locale's text")
	f.StringVar(&opts.missing, "missing", "", "only keys without a translation in this locale")
	f.StringVar(&opts.outdated, "outdated", "", "only keys whose translation in this locale is outdated")
	f.StringVar(&opts.unapproved, "unapproved", "", "only keys with a machine translation awaiting approval in this locale")
	f.IntVar(&opts.limit, "limit", 0, "list at most this many keys (default all)")
	list.MarkFlagsMutuallyExclusive("locale", "missing", "outdated", "unapproved")
	addJSONFlag(list, &opts.json)
	cmd.AddCommand(list)
	return cmd
}

func (a *app) keysList(ctx context.Context, opts keysOptions) error {
	if opts.limit < 0 {
		return errors.New("--limit can't be negative")
	}
	p, err := a.loadProject(ctx, config.Need{})
	if err != nil {
		return err
	}
	filter := api.KeyFilter{Prefix: opts.prefix, Locale: opts.locale}
	for status, locale := range map[string]string{
		api.StatusMissing: opts.missing, api.StatusOutdated: opts.outdated, api.StatusUnapproved: opts.unapproved,
	} {
		if locale != "" {
			filter.Status, filter.Locale = status, locale
		}
	}
	keys, err := p.client.Keys(ctx, filter, opts.limit)
	if err != nil {
		return fmt.Errorf("list keys: %w", err)
	}
	if opts.json {
		if keys == nil {
			keys = []api.Key{}
		}
		return printJSON(a.stdout, keys)
	}
	if len(keys) == 0 {
		fmt.Fprintln(a.stderr, "No keys match.")
		return nil
	}
	proj, err := p.client.Project(ctx)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	header := []string{"KEY", strings.ToUpper(proj.BaseLocale)}
	other := filter.Locale
	if strings.EqualFold(other, proj.BaseLocale) {
		other = ""
	}
	if other != "" {
		header = append(header, strings.ToUpper(other))
	}
	t := newTable(a.stdout, header...)
	for _, k := range keys {
		row := []string{k.Key, orDash(shorten(translation(k, proj.BaseLocale).Value, 50))}
		if other != "" {
			tr := translation(k, other)
			value := orDash(shorten(tr.Value, 50))
			switch {
			case tr.Value == "":
			case tr.Outdated:
				value += " (outdated)"
			case !tr.Approved:
				value += " (unapproved)"
			}
			row = append(row, value)
		}
		t.row(row...)
	}
	t.flush()
	fmt.Fprintf(a.stderr, "%d keys\n", len(keys))
	return nil
}

// translation is a key's translation in locale, matched ignoring case.
func translation(k api.Key, locale string) api.Translation {
	if t, ok := k.Translations[locale]; ok {
		return t
	}
	for l, t := range k.Translations {
		if strings.EqualFold(l, locale) {
			return t
		}
	}
	return api.Translation{}
}
