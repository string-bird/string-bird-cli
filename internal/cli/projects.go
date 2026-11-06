package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) projectsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List the organization's projects",
		Args:  cobra.NoArgs,
	}
	var asJSON bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List the projects your API key can see",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.projectsList(cmd.Context(), asJSON)
		},
	}
	addJSONFlag(list, &asJSON)
	cmd.AddCommand(list)
	return cmd
}

func (a *app) projectsList(ctx context.Context, asJSON bool) error {
	cfg, client, err := a.loadOrg()
	if err != nil {
		return err
	}
	projects, err := client.Projects(ctx)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	if asJSON {
		return printJSON(a.stdout, projects)
	}
	if len(projects) == 0 {
		fmt.Fprintln(a.stderr, "This API key can't see any project.")
		return nil
	}
	t := newTable(a.stdout, "", "SLUG", "NAME", "BASE", "LOCALES", "ID")
	for _, p := range projects {
		current := ""
		if p.ID == cfg.ProjectID || strings.EqualFold(p.Slug, cfg.ProjectID) {
			current = "*"
		}
		t.row(current, p.Slug, p.Name, p.BaseLocale, strconv.Itoa(len(p.Locales)), p.ID)
	}
	t.flush()
	return nil
}
