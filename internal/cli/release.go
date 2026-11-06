package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/api"
)

// pollInterval is how often release create --wait checks the release.
const pollInterval = 2 * time.Second

func (a *app) releaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Create releases and promote them to environments",
		Long: `A release is an immutable snapshot of the approved translations, published to
the CDN. An environment (production, staging, ...) points at one release;
promoting points it at another, and pointing it at an older one rolls back.`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(a.releaseCreateCmd(), a.releasePromoteCmd())
	return cmd
}

type releaseCreateOptions struct {
	environment string
	note        string
	wait        bool
	timeout     time.Duration
}

func (a *app) releaseCreateCmd() *cobra.Command {
	var opts releaseCreateOptions
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Snapshot the approved translations as the next release",
		Example: `  sb release create --note "Spring sale copy"
  sb release create --env staging --wait`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.releaseCreate(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&opts.environment, "env", "e", "", "also point this environment at the new release")
	f.StringVar(&opts.note, "note", "", "note shown with the release")
	f.BoolVar(&opts.wait, "wait", false, "wait until the release is published (or fails)")
	f.DurationVar(&opts.timeout, "timeout", 5*time.Minute, "how long --wait waits")
	return cmd
}

func (a *app) releaseCreate(ctx context.Context, opts releaseCreateOptions) error {
	p, err := a.loadProject(ctx)
	if err != nil {
		return err
	}
	rel, err := p.client.CreateRelease(ctx, opts.note, opts.environment)
	if err != nil {
		return fmt.Errorf("create release: %w", err)
	}
	fmt.Fprintf(a.stdout, "created v%d (%d keys, %s)\n", rel.Version, rel.KeyCount, strings.Join(rel.Locales, ", "))
	if opts.environment != "" {
		fmt.Fprintf(a.stdout, "%s → v%d\n", opts.environment, rel.Version)
	}
	if !opts.wait {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	for rel.Status == api.ReleasePending {
		if err := a.wait(ctx, pollInterval); err != nil {
			return fmt.Errorf("v%d still pending after %s: %w", rel.Version, opts.timeout, err)
		}
		if rel, err = p.client.Release(ctx, rel.ID); err != nil {
			return fmt.Errorf("check release: %w", err)
		}
	}
	if rel.Status != api.ReleasePublished {
		return fmt.Errorf("v%d %s: %s", rel.Version, rel.Status, rel.Error)
	}
	fmt.Fprintf(a.stdout, "published v%d\n", rel.Version)
	return nil
}

type releasePromoteOptions struct {
	release string
	from    string
}

func (a *app) releasePromoteCmd() *cobra.Command {
	var opts releasePromoteOptions
	cmd := &cobra.Command{
		Use:   "promote <environment>",
		Short: "Point an environment at a release (promote or roll back)",
		Long: `Point an environment at a release: the one another environment serves
(--from), or one by version or id (--release). Nothing is rebuilt: the
environment serves byte for byte what the release holds.`,
		Example: `  sb release promote production --from staging
  sb release promote production --release v12`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.releasePromote(cmd.Context(), args[0], opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.release, "release", "", "release version (12, v12) or id")
	f.StringVar(&opts.from, "from", "", "promote the release this environment serves")
	cmd.MarkFlagsOneRequired("release", "from")
	cmd.MarkFlagsMutuallyExclusive("release", "from")
	return cmd
}

func (a *app) releasePromote(ctx context.Context, environment string, opts releasePromoteOptions) error {
	p, err := a.loadProject(ctx)
	if err != nil {
		return err
	}
	var rel api.Release
	if opts.from != "" {
		rel, err = environmentRelease(ctx, p.client, opts.from)
	} else {
		rel, err = findRelease(ctx, p.client, opts.release)
	}
	if err != nil {
		return err
	}
	env, err := p.client.Promote(ctx, environment, rel.ID)
	if err != nil {
		return fmt.Errorf("promote %s: %w", environment, err)
	}
	fmt.Fprintf(a.stdout, "%s → v%d\n", env.Name, rel.Version)
	return nil
}

func environmentRelease(ctx context.Context, c *api.Client, name string) (api.Release, error) {
	envs, err := c.Environments(ctx)
	if err != nil {
		return api.Release{}, fmt.Errorf("list environments: %w", err)
	}
	for _, env := range envs {
		if env.Name != name {
			continue
		}
		if env.Release == nil {
			return api.Release{}, fmt.Errorf("environment %s has no release yet", name)
		}
		return *env.Release, nil
	}
	return api.Release{}, fmt.Errorf("environment %s not found", name)
}

// findRelease takes a version (12 or v12) or a release id.
func findRelease(ctx context.Context, c *api.Client, ref string) (api.Release, error) {
	version, err := strconv.Atoi(strings.TrimPrefix(strings.ToLower(ref), "v"))
	if err != nil {
		rel, err := c.Release(ctx, ref)
		if err != nil {
			return api.Release{}, fmt.Errorf("get release %s: %w", ref, err)
		}
		return rel, nil
	}
	if version < 1 {
		return api.Release{}, errors.New("release versions start at 1")
	}
	rel, ok, err := c.ReleaseByVersion(ctx, version)
	if err != nil {
		return api.Release{}, fmt.Errorf("find release v%d: %w", version, err)
	}
	if !ok {
		return api.Release{}, fmt.Errorf("release v%d not found", version)
	}
	return rel, nil
}
