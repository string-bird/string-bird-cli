package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/config"
)

func (a *app) envCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "List and create environments",
		Long: `An environment (production, staging, pr-42, ...) serves one release on the CDN.
Point it at a release with sb release create --env or sb release promote.`,
		Args: cobra.NoArgs,
	}
	var asJSON bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List environments and the release each serves",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.envList(cmd.Context(), asJSON)
		},
	}
	addJSONFlag(list, &asJSON)
	create := &cobra.Command{
		Use:     "create <name>",
		Short:   "Create an environment",
		Long:    "Create an environment: lowercase letters, digits and dashes, up to 40 characters.",
		Example: "  sb env create staging",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.envCreate(cmd.Context(), args[0])
		},
	}
	cmd.AddCommand(list, create)
	return cmd
}

func (a *app) envList(ctx context.Context, asJSON bool) error {
	p, err := a.loadProject(ctx, config.Need{})
	if err != nil {
		return err
	}
	envs, err := p.client.Environments(ctx)
	if err != nil {
		return fmt.Errorf("list environments: %w", err)
	}
	if asJSON {
		return printJSON(a.stdout, envs)
	}
	t := newTable(a.stdout, "NAME", "RELEASE", "STATUS", "UPDATED")
	for _, e := range envs {
		if e.Release == nil {
			t.row(e.Name, "-", "-", formatTime(e.UpdatedAt))
			continue
		}
		t.row(e.Name, fmt.Sprintf("v%d", e.Release.Version), e.Release.Status, formatTime(e.UpdatedAt))
	}
	t.flush()
	return nil
}

func (a *app) envCreate(ctx context.Context, name string) error {
	p, err := a.loadProject(ctx, config.Need{})
	if err != nil {
		return err
	}
	env, err := p.client.CreateEnvironment(ctx, name)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	fmt.Fprintf(a.stdout, "created %s\n", env.Name)
	return nil
}
