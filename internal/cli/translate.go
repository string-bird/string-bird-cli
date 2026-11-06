package cli

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/api"
	"github.com/string-bird/string-bird-cli/internal/config"
)

type translateOptions struct {
	locales []string
	scope   string
	wait    bool
	timeout time.Duration
	json    bool
}

func (a *app) translateCmd() *cobra.Command {
	var opts translateOptions
	cmd := &cobra.Command{
		Use:   "translate",
		Short: "Machine-translate what's missing",
		Long: `Start a machine translation run. Results are saved awaiting approval
(sb approve), and never replace a person's translation.

Scopes: missing (default) translates keys with no translation yet; outdated
retranslates machine translations whose base text changed; needed does both.

With --wait, sb follows the run and exits with 1 if it fails or any string
can't be translated.`,
		Example: `  sb translate --wait
  sb translate --locale de --locale fr --scope needed --wait`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.translate(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	f.StringSliceVarP(&opts.locales, "locale", "l", nil, "only these locales (repeatable; default every locale but the base)")
	f.StringVar(&opts.scope, "scope", "", "missing, outdated or needed (default missing)")
	f.BoolVar(&opts.wait, "wait", false, "wait until the run is over")
	f.DurationVar(&opts.timeout, "timeout", 30*time.Minute, "how long --wait waits")
	addJSONFlag(cmd, &opts.json)
	return cmd
}

func (a *app) translate(ctx context.Context, opts translateOptions) error {
	p, err := a.loadProject(ctx, config.Need{})
	if err != nil {
		return err
	}
	job, err := p.client.StartMachineTranslation(ctx, api.TranslateRequest{Scope: opts.scope, Locales: opts.locales})
	var apiErr *api.Error
	if errors.As(err, &apiErr) && apiErr.Code == "nothing_to_translate" {
		fmt.Fprintln(a.stderr, "Nothing to translate.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("start machine translation: %w", err)
	}
	if !opts.json {
		fmt.Fprintf(a.stdout, "started run %s: %d strings (%s)\n", job.ID, job.Total, strings.Join(job.Locales, ", "))
	}
	if !opts.wait {
		if opts.json {
			return printJSON(a.stdout, job)
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	lastDone := -1
	for !job.Finished() {
		if err := a.wait(ctx, pollInterval); err != nil {
			return fmt.Errorf("run %s not finished after %s: %w", job.ID, opts.timeout, err)
		}
		if job, err = p.client.MachineTranslation(ctx, job.ID); err != nil {
			return fmt.Errorf("check run: %w", err)
		}
		if !opts.json && job.Done+job.Failed != lastDone && !job.Finished() {
			lastDone = job.Done + job.Failed
			fmt.Fprintf(a.stderr, "  %d/%d\n", lastDone, job.Total)
		}
	}
	if opts.json {
		if err := printJSON(a.stdout, job); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(a.stdout, "%s: %d translated, %d failed\n", job.Status, job.Done, job.Failed)
		failures := append([]api.JobFailure(nil), job.Failures...)
		sort.Slice(failures, func(i, j int) bool {
			if failures[i].Locale != failures[j].Locale {
				return failures[i].Locale < failures[j].Locale
			}
			return failures[i].Key < failures[j].Key
		})
		for _, f := range failures {
			fmt.Fprintf(a.stdout, "  %s %s: %s\n", f.Locale, f.Key, f.Reason)
		}
		if job.Done > 0 {
			fmt.Fprintf(a.stdout, "Review and approve: sb approve --run %s\n", job.ID)
		}
	}
	switch {
	case job.Status != api.JobCompleted:
		return fmt.Errorf("run %s %s: %s", job.ID, job.Status, orDash(job.Error))
	case job.Failed > 0:
		return fmt.Errorf("run %s: %d strings couldn't be translated", job.ID, job.Failed)
	}
	return nil
}

type approveOptions struct {
	locale string
	run    string
	prefix string
}

func (a *app) approveCmd() *cobra.Command {
	var opts approveOptions
	cmd := &cobra.Command{
		Use:   "approve",
		Short: "Approve machine translations",
		Long: `Approve machine translations awaiting approval, so releases include them.
Pick them by locale, by run (what sb translate wrote), or both; --prefix
narrows either. Outdated ones are skipped: translate them again first.`,
		Example: `  sb approve --run 0193...
  sb approve --locale de --prefix checkout.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.approve(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&opts.locale, "locale", "l", "", "approve this locale's machine translations")
	f.StringVar(&opts.run, "run", "", "approve what this machine translation run wrote")
	f.StringVar(&opts.prefix, "prefix", "", "only keys starting with this")
	cmd.MarkFlagsOneRequired("locale", "run")
	return cmd
}

func (a *app) approve(ctx context.Context, opts approveOptions) error {
	p, err := a.loadProject(ctx, config.Need{})
	if err != nil {
		return err
	}
	req := api.ApproveRequest{Locale: opts.locale, Prefix: opts.prefix}
	if opts.run != "" {
		job, err := p.client.MachineTranslation(ctx, opts.run)
		if err != nil {
			return fmt.Errorf("get run: %w", err)
		}
		if job.ChangeSetID == nil {
			fmt.Fprintln(a.stdout, "approved 0: the run saved nothing")
			return nil
		}
		req.ChangeSetID = *job.ChangeSetID
	}
	res, err := p.client.Approve(ctx, req)
	if err != nil {
		return fmt.Errorf("approve: %w", err)
	}
	fmt.Fprintf(a.stdout, "approved %d", res.Approved)
	if res.SkippedOutdated > 0 {
		fmt.Fprintf(a.stdout, ", skipped %d outdated", res.SkippedOutdated)
	}
	fmt.Fprintln(a.stdout)
	return nil
}
