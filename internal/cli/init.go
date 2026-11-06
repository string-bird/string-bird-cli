package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/api"
	"github.com/string-bird/string-bird-cli/internal/config"
)

// Formats the server converts from and to.
var formats = []string{"json", "i18next"}

type initOptions struct {
	format string
	force  bool
}

func (a *app) initCmd() *cobra.Command {
	var opts initOptions
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create stringbird.yaml for this project",
		Long: `Create stringbird.yaml, the project config the other commands read.

Values come from --org, --project and --files (or STRINGBIRD_ORG_ID and
STRINGBIRD_PROJECT_ID). Whatever is missing is asked for in a terminal: with an
API key set, the projects the key can see are listed to pick from, and the
files path is suggested from the locale files found in this folder.

--project takes the project's id or slug; the config gets its id.`,
		Example: `  sb init
  sb init --org 0193a1b2-... --project web --files "src/locales/{locale}.json"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.init(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&opts.format, "format", "", "file format: json (ICU) or i18next (default: guessed from the files)")
	f.BoolVar(&opts.force, "force", false, "replace an existing config")
	return cmd
}

func (a *app) init(ctx context.Context, opts initOptions) error {
	path := a.configPath
	if _, err := os.Stat(path); err == nil && !opts.force {
		return fmt.Errorf("%s already exists; pass --force to replace it", path)
	}
	if opts.format != "" && !contains(formats, opts.format) {
		return fmt.Errorf("unknown format %q; use %s", opts.format, strings.Join(formats, " or "))
	}
	p := prompter{in: bufio.NewReader(a.stdin), out: a.stderr, interactive: a.interactive}

	orgID := firstSet(a.org, a.getenv(envOrgID))
	if orgID == "" {
		var err error
		if orgID, err = p.ask("Organization id (dashboard → organization settings)", "", "--org"); err != nil {
			return err
		}
	}

	proj, note, err := a.initProject(ctx, p, orgID)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	base := firstSet(proj.BaseLocale, "en")
	files := a.files
	if files == "" {
		found := detectFiles(dir, base)
		if len(found) > 1 && p.interactive {
			fmt.Fprintf(p.out, "Locale files found: %s\n", strings.Join(found, ", "))
		}
		if files, err = p.ask("Translation files, {locale} where the locale goes", first(found), "--files"); err != nil {
			return err
		}
	}
	if !strings.Contains(files, config.LocalePlaceholder) {
		return fmt.Errorf("files %q needs a %s placeholder, e.g. src/locales/{locale}.json", files, config.LocalePlaceholder)
	}

	format := opts.format
	if format == "" {
		format = guessFormat(filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(files, config.LocalePlaceholder, base))))
	}

	cfg := config.Config{OrgID: orgID, ProjectID: proj.ID, Files: files, Format: format}
	if err := os.WriteFile(path, cfg.Encode(note), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Fprintf(a.stdout, "Wrote %s for project %s: %s files at %s\n", path, firstSet(note, proj.ID), format, files)
	if a.key() == "" {
		fmt.Fprintf(a.stdout, "Next: set %s, then run sb push --dry-run\n", envAPIKey)
	} else {
		fmt.Fprintln(a.stdout, "Next: sb push --dry-run")
	}
	return nil
}

// initProject picks the project: checked against the API when there's a key,
// taken as given when there isn't. note describes it for the config comment.
func (a *app) initProject(ctx context.Context, p prompter, orgID string) (proj api.Project, note string, err error) {
	ref := firstSet(a.project, a.getenv(envProjectID))
	if a.key() == "" {
		fmt.Fprintf(a.stderr, "No API key set, so the project isn't checked. Set %s to pick it from a list.\n", envAPIKey)
		if ref == "" {
			if ref, err = p.ask("Project id", "", "--project"); err != nil {
				return api.Project{}, "", err
			}
		}
		if !uuidPattern.MatchString(ref) {
			return api.Project{}, "", fmt.Errorf("project %q isn't an id; a slug needs an API key to look up", ref)
		}
		return api.Project{ID: ref}, "", nil
	}

	client, err := a.newClient(orgID)
	if err != nil {
		return api.Project{}, "", err
	}
	if ref != "" {
		proj, err = findProject(ctx, client, ref)
	} else {
		proj, err = pickProject(ctx, client, p)
	}
	if err != nil {
		return api.Project{}, "", err
	}
	return proj, fmt.Sprintf("%s (%s)", proj.Name, proj.Slug), nil
}

func pickProject(ctx context.Context, c *api.Client, p prompter) (api.Project, error) {
	projects, err := c.Projects(ctx)
	if err != nil {
		return api.Project{}, fmt.Errorf("list projects: %w", err)
	}
	switch {
	case len(projects) == 0:
		return api.Project{}, errors.New("this API key can't see any project; create one in the dashboard or use another key")
	case len(projects) == 1:
		return projects[0], nil
	case !p.interactive:
		var slugs []string
		for _, proj := range projects {
			slugs = append(slugs, proj.Slug)
		}
		return api.Project{}, fmt.Errorf("pass --project; this API key can see: %s", strings.Join(slugs, ", "))
	}
	fmt.Fprintln(p.out, "Projects:")
	for i, proj := range projects {
		fmt.Fprintf(p.out, "  %d) %s (%s)\n", i+1, proj.Name, proj.Slug)
	}
	for {
		answer, err := p.ask("Project number", "1", "--project")
		if err != nil {
			return api.Project{}, err
		}
		if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(projects) {
			return projects[n-1], nil
		}
		fmt.Fprintf(p.out, "Pick a number from 1 to %d.\n", len(projects))
	}
}

// prompter asks for values in a terminal.
type prompter struct {
	in          *bufio.Reader
	out         io.Writer
	interactive bool
}

// ask returns the answer, or def for an empty one. Without a terminal it
// returns def, or an error naming flag when there's no default.
func (p prompter) ask(question, def, flag string) (string, error) {
	if !p.interactive {
		if def == "" {
			return "", fmt.Errorf("pass %s (sb init asks only in a terminal)", flag)
		}
		return def, nil
	}
	for {
		if def != "" {
			fmt.Fprintf(p.out, "%s [%s]: ", question, def)
		} else {
			fmt.Fprintf(p.out, "%s: ", question)
		}
		line, err := p.in.ReadString('\n')
		answer := strings.TrimSpace(line)
		if answer == "" {
			answer = def
		}
		if answer != "" {
			return answer, nil
		}
		if err != nil {
			return "", fmt.Errorf("no answer for %q", question)
		}
	}
}

// skipDirs are never searched for locale files.
var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true, "out": true,
	"target": true, "coverage": true, "bin": true, "obj": true,
}

// maxDepth is how deep detectFiles looks below the project folder.
const maxDepth = 5

// detectFiles suggests files patterns from the base locale's files under root:
// src/locales/en.json gives src/locales/{locale}.json, public/locales/en/
// translation.json gives public/locales/{locale}/translation.json. Patterns
// matching more locales come first, then shallower ones.
func detectFiles(root, base string) []string {
	counts := map[string]int{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if path != root && (strings.HasPrefix(d.Name(), ".") || skipDirs[d.Name()] || strings.Count(rel, "/") >= maxDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		if pattern := localePattern(rel, base); pattern != "" {
			counts[pattern] = countLocales(root, pattern)
		}
		return nil
	})
	patterns := make([]string, 0, len(counts))
	for p := range counts {
		patterns = append(patterns, p)
	}
	sort.Slice(patterns, func(i, j int) bool {
		if counts[patterns[i]] != counts[patterns[j]] {
			return counts[patterns[i]] > counts[patterns[j]]
		}
		if di, dj := strings.Count(patterns[i], "/"), strings.Count(patterns[j], "/"); di != dj {
			return di < dj
		}
		return patterns[i] < patterns[j]
	})
	return patterns
}

// localePattern is the files pattern rel would match for the base locale, or
// "" when rel isn't a base-locale file.
func localePattern(rel, base string) string {
	if !strings.EqualFold(filepath.Ext(rel), ".json") {
		return ""
	}
	parts := strings.Split(rel, "/")
	name := parts[len(parts)-1]
	switch {
	case strings.EqualFold(name, base+".json"):
		parts[len(parts)-1] = config.LocalePlaceholder + ".json"
	case len(parts) >= 2 && strings.EqualFold(parts[len(parts)-2], base):
		parts[len(parts)-2] = config.LocalePlaceholder
	default:
		return ""
	}
	return strings.Join(parts, "/")
}

// localeCode looks like a BCP 47 code: en, de-AT, zh-Hant, pt_BR.
var localeCode = regexp.MustCompile(`^[a-zA-Z]{2,3}([-_][a-zA-Z0-9]{2,8})*$`)

// countLocales counts the files that match pattern with a locale-like code.
func countLocales(root, pattern string) int {
	glob := filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(pattern, config.LocalePlaceholder, "*")))
	matches, _ := filepath.Glob(glob)
	prefix, suffix, _ := strings.Cut(filepath.FromSlash(pattern), config.LocalePlaceholder)
	n := 0
	for _, m := range matches {
		rel, err := filepath.Rel(root, m)
		if err != nil {
			continue
		}
		code := strings.TrimSuffix(strings.TrimPrefix(rel, prefix), suffix)
		if localeCode.MatchString(code) {
			n++
		}
	}
	return n
}

// i18nextSigns are what classic i18next JSON has and ICU JSON doesn't:
// {{name}} interpolation and key_one / key_other plural keys.
var i18nextSigns = regexp.MustCompile(`\{\{|"[^"]+_(zero|one|two|few|many|other)"\s*:`)

// guessFormat reads the base-locale file: i18next when it looks like classic
// i18next JSON, json (ICU) otherwise or when there's no file yet.
func guessFormat(path string) string {
	data, err := os.ReadFile(path)
	if err == nil && i18nextSigns.Match(data) {
		return "i18next"
	}
	return "json"
}

func first(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[0]
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
