package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/config"
)

func (a *app) localesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "locales",
		Short: "List and add the project's locales",
		Args:  cobra.NoArgs,
	}
	var asJSON bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List the project's locales",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.localesList(cmd.Context(), asJSON)
		},
	}
	addJSONFlag(list, &asJSON)
	add := &cobra.Command{
		Use:     "add <locale>...",
		Short:   "Add locales to the project",
		Long:    "Add locales by their BCP 47 code. Codes are stored canonical: de-at becomes de-AT.",
		Example: "  sb locales add de fr pt-BR",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.localesAdd(cmd.Context(), args)
		},
	}
	cmd.AddCommand(list, add)
	return cmd
}

func (a *app) localesList(ctx context.Context, asJSON bool) error {
	p, err := a.loadProject(ctx, config.Need{})
	if err != nil {
		return err
	}
	proj, err := p.client.Project(ctx)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if asJSON {
		type locale struct {
			Code string `json:"code"`
			Base bool   `json:"base"`
		}
		out := []locale{}
		for _, l := range proj.Locales {
			out = append(out, locale{Code: l, Base: strings.EqualFold(l, proj.BaseLocale)})
		}
		return printJSON(a.stdout, out)
	}
	for _, l := range proj.Locales {
		if strings.EqualFold(l, proj.BaseLocale) {
			fmt.Fprintf(a.stdout, "%s (base)\n", l)
		} else {
			fmt.Fprintln(a.stdout, l)
		}
	}
	return nil
}

func (a *app) localesAdd(ctx context.Context, codes []string) error {
	p, err := a.loadProject(ctx, config.Need{})
	if err != nil {
		return err
	}
	for _, code := range codes {
		l, err := p.client.AddLocale(ctx, code)
		if err != nil {
			return fmt.Errorf("add %s: %w", code, err)
		}
		fmt.Fprintf(a.stdout, "added %s\n", l.Code)
	}
	return nil
}
