// Package cli holds the sb commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/api"
	"github.com/string-bird/string-bird-cli/internal/config"
)

// defaultAPIURL is the hosted String Bird API, the only one users talk to.
const defaultAPIURL = "https://api.stringbird.io"

// Environment variables the CLI reads.
const (
	envAPIKey = "STRINGBIRD_API_KEY"
	// envAPIURL points sb at a dev or test API instead of production. It's
	// for String Bird's own development, so it isn't documented.
	envAPIURL = "STRINGBIRD_API_URL"
)

// app is what every command shares: global flags, output and the environment.
type app struct {
	configPath string
	apiKey     string

	version string
	stdout  io.Writer
	stderr  io.Writer
	getenv  func(string) string
	// sleep waits between polls and retries; tests make it instant.
	sleep func(ctx context.Context, d time.Duration) error
}

// Execute runs sb with the process's arguments and returns the exit code.
func Execute(version string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	a := &app{version: version, stdout: os.Stdout, stderr: os.Stderr, getenv: os.Getenv}
	return a.run(ctx, os.Args[1:])
}

func (a *app) run(ctx context.Context, args []string) int {
	root := a.rootCmd()
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(a.stderr, "sb: %v\n", err)
		return 1
	}
	return 0
}

func (a *app) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "sb",
		Short: "String Bird client: push and pull translation files, publish releases",
		Long: `sb is the String Bird client. It uploads source and translation files,
downloads translated files, and publishes releases to the CDN.

Project settings live in stringbird.yaml. The API key comes from
STRINGBIRD_API_KEY (or --api-key), never from the config file.`,
		Version:       a.version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	flags := root.PersistentFlags()
	flags.StringVarP(&a.configPath, "config", "c", config.DefaultPath, "project config file")
	flags.StringVar(&a.apiKey, "api-key", "", "API key (default $"+envAPIKey+")")

	root.AddCommand(a.pushCmd(), a.pullCmd(), a.releaseCmd())
	return root
}

// project is a loaded config with a client for its project.
type project struct {
	cfg    config.Config
	client *api.Client
	// dir is the config file's directory; file paths are relative to it.
	dir string
}

// path is a locale's file, relative to the working directory.
func (p project) path(locale string) string {
	return filepath.Join(p.dir, filepath.FromSlash(p.cfg.FilePath(locale)))
}

func (a *app) loadProject() (project, error) {
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return project{}, err
	}
	key := firstSet(a.apiKey, a.getenv(envAPIKey))
	if key == "" {
		return project{}, errors.New("no API key: set " + envAPIKey + " or pass --api-key")
	}
	client := api.New(api.Options{
		BaseURL:   a.baseURL(),
		APIKey:    key,
		OrgID:     cfg.OrgID,
		ProjectID: cfg.ProjectID,
		UserAgent: "sb/" + a.version,
	})
	if a.sleep != nil {
		client.SetSleep(a.sleep)
	}
	return project{cfg: cfg, client: client, dir: filepath.Dir(a.configPath)}, nil
}

// baseURL is the API to call: production unless STRINGBIRD_API_URL points
// elsewhere.
func (a *app) baseURL() string {
	return firstSet(a.getenv(envAPIURL), defaultAPIURL)
}

func (a *app) wait(ctx context.Context, d time.Duration) error {
	if a.sleep != nil {
		return a.sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func firstSet(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
