// Package cli holds the sb commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/string-bird/string-bird-cli/internal/api"
	"github.com/string-bird/string-bird-cli/internal/config"
)

// defaultAPIURL is the hosted StringBird API, the only one users talk to.
const defaultAPIURL = "https://api.stringbird.io"

// Environment variables the CLI reads.
const (
	envAPIKey = "STRINGBIRD_API_KEY"
	// envAPIURL points sb at a dev or test API instead of production. It's
	// for StringBird's own development, so it isn't documented.
	envAPIURL = "STRINGBIRD_API_URL"
	// envOrgID and envProjectID override org_id and project_id in the config.
	envOrgID     = "STRINGBIRD_ORG_ID"
	envProjectID = "STRINGBIRD_PROJECT_ID"
)

// app is what every command shares: global flags, output and the environment.
type app struct {
	configPath string
	apiKey     string
	org        string
	project    string
	files      string

	version string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	getenv  func(string) string
	// interactive is true when stdin is a terminal sb may prompt on.
	interactive bool
	// sleep waits between polls and retries; tests make it instant.
	sleep func(ctx context.Context, d time.Duration) error
}

// Execute runs sb with the process's arguments and returns the exit code.
func Execute(version string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	a := &app{
		version: version, stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, getenv: os.Getenv,
		interactive: isTerminal(os.Stdin),
	}
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
		Short: "StringBird client: push and pull translation files, publish releases",
		Long: `sb is the StringBird client. It uploads source and translation files,
downloads translated files, and publishes releases to the CDN.

Project settings live in stringbird.yaml (sb init creates it); --org,
--project and --files override it. The API key comes from STRINGBIRD_API_KEY
(or --api-key), never from the config file.`,
		Version:       a.version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	flags := root.PersistentFlags()
	flags.StringVarP(&a.configPath, "config", "c", config.DefaultPath, "project config file")
	flags.StringVar(&a.apiKey, "api-key", "", "API key (default $"+envAPIKey+")")
	flags.StringVar(&a.org, "org", "", "organization id (default $"+envOrgID+", then org_id in the config)")
	flags.StringVar(&a.project, "project", "", "project id or slug (default $"+envProjectID+", then project_id in the config)")
	flags.StringVar(&a.files, "files", "", "translation files path with {locale}, relative to the working directory")

	root.AddCommand(a.initCmd(), a.pushCmd(), a.pullCmd(), a.releaseCmd(), a.statusCmd(), a.translateCmd(),
		a.approveCmd(), a.keysCmd(), a.localesCmd(), a.envCmd(), a.projectsCmd(), a.apiCmd())
	return root
}

// project is a loaded config with a client for its project.
type project struct {
	cfg    config.Config
	client *api.Client
	// dir is what the files path is relative to: the config file's directory,
	// or the working directory when --files set it.
	dir string
}

// path is a locale's file, relative to the working directory.
func (p project) path(locale string) string {
	return filepath.Join(p.dir, filepath.FromSlash(p.cfg.FilePath(locale)))
}

// loadProject reads stringbird.yaml, applies the flag and environment
// overrides, and returns a client for the project, its slug resolved to an id.
// need says whether the command needs the files path too.
func (a *app) loadProject(ctx context.Context, need config.Need) (project, error) {
	need.Project = true
	cfg, dir, err := a.loadConfig(need)
	if err != nil {
		return project{}, err
	}
	client, err := a.newClient(cfg.OrgID)
	if err != nil {
		return project{}, err
	}
	id, err := resolveProject(ctx, client, cfg.ProjectID)
	if err != nil {
		return project{}, err
	}
	cfg.ProjectID = id
	return project{cfg: cfg, client: client.ForProject(id), dir: dir}, nil
}

// loadOrg is loadProject for commands that work on the organization: only
// org_id is needed.
func (a *app) loadOrg() (config.Config, *api.Client, error) {
	cfg, _, err := a.loadConfig(config.Need{})
	if err != nil {
		return config.Config{}, nil, err
	}
	client, err := a.newClient(cfg.OrgID)
	return cfg, client, err
}

// loadConfig merges stringbird.yaml with --org, --project and --files and
// their environment variables (flags first, then the environment, then the
// file). The file is optional when the overrides cover what need asks for.
func (a *app) loadConfig(need config.Need) (config.Config, string, error) {
	cfg, err := config.Load(a.configPath)
	found := err == nil
	if err != nil && !(errors.Is(err, fs.ErrNotExist) && a.configPath == config.DefaultPath) {
		return config.Config{}, "", err
	}
	dir := filepath.Dir(a.configPath)
	cfg.OrgID = firstSet(a.org, a.getenv(envOrgID), cfg.OrgID)
	cfg.ProjectID = firstSet(a.project, a.getenv(envProjectID), cfg.ProjectID)
	if a.files != "" {
		cfg.Files, dir = a.files, "."
	}

	err = cfg.Validate(need)
	var missing *config.MissingError
	if errors.As(err, &missing) {
		flags := map[string]string{
			"org_id":     "--org (or " + envOrgID + ")",
			"project_id": "--project (or " + envProjectID + ")",
			"files":      "--files",
		}
		var how []string
		for _, f := range missing.Fields {
			how = append(how, flags[f])
		}
		if !found {
			return config.Config{}, "", fmt.Errorf("no %s here: run `sb init` to create one, or pass %s",
				a.configPath, strings.Join(how, ", "))
		}
		return config.Config{}, "", fmt.Errorf("%s: %w; set it there or pass %s", a.configPath, err, strings.Join(how, ", "))
	}
	if err != nil {
		return config.Config{}, "", err
	}
	return cfg, dir, nil
}

// key is the API key from --api-key or STRINGBIRD_API_KEY, if any.
func (a *app) key() string {
	return firstSet(a.apiKey, a.getenv(envAPIKey))
}

// newClient is an API client for an organization; ForProject picks the project.
func (a *app) newClient(orgID string) (*api.Client, error) {
	key := a.key()
	if key == "" {
		return nil, errors.New("no API key: set " + envAPIKey + " or pass --api-key")
	}
	client := api.New(api.Options{
		BaseURL:   a.baseURL(),
		APIKey:    key,
		OrgID:     orgID,
		UserAgent: "sb/" + a.version,
	})
	if a.sleep != nil {
		client.SetSleep(a.sleep)
	}
	return client, nil
}

// uuidPattern matches the ids the API uses (UUIDs).
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// resolveProject turns a project slug into its id. An id is returned as is,
// without a request.
func resolveProject(ctx context.Context, c *api.Client, ref string) (string, error) {
	if uuidPattern.MatchString(ref) {
		return ref, nil
	}
	p, err := findProject(ctx, c, ref)
	if err != nil {
		return "", err
	}
	return p.ID, nil
}

// findProject looks a project up by id or slug among those the key can see.
func findProject(ctx context.Context, c *api.Client, ref string) (api.Project, error) {
	projects, err := c.Projects(ctx)
	if err != nil {
		return api.Project{}, fmt.Errorf("list projects: %w", err)
	}
	var slugs []string
	for _, p := range projects {
		if p.ID == ref || strings.EqualFold(p.Slug, ref) {
			return p, nil
		}
		slugs = append(slugs, p.Slug)
	}
	if len(slugs) == 0 {
		return api.Project{}, fmt.Errorf("project %q not found: this API key can't see any project", ref)
	}
	return api.Project{}, fmt.Errorf("project %q not found; this API key can see: %s", ref, strings.Join(slugs, ", "))
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

// isTerminal reports whether f is an interactive console. /dev/null and NUL
// are character devices too, so the mode bits alone would say yes in CI.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func firstSet(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
