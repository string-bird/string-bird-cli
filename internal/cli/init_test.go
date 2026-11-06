package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/string-bird/string-bird-cli/internal/api"
	"github.com/string-bird/string-bird-cli/internal/config"
)

func TestInit(t *testing.T) {
	mobile := api.Project{ID: otherID, Name: "Mobile", Slug: "mobile", BaseLocale: "en-US"}
	tests := []struct {
		name string
		// files are written to the project folder first.
		files       map[string]string
		env         map[string]string
		noKey       bool
		interactive bool
		stdin       string
		projects    []api.Project
		args        []string
		wantCode    int
		want        config.Config
		wantNote    string
		wantStdout  string
		wantStderr  string
	}{
		{
			name:       "all flags, slug resolved to the id",
			args:       []string{"init", "--org", testOrg, "--project", "web", "--files", "src/locales/{locale}.json"},
			want:       config.Config{OrgID: testOrg, ProjectID: testProject, Files: "src/locales/{locale}.json", Format: "json"},
			wantNote:   "# Web (web)",
			wantStdout: "Wrote ",
		},
		{
			name:  "org and project from the environment, files and format detected",
			files: map[string]string{"public/locales/en/translation.json": `{"hi": "Hello {{name}}"}`, "public/locales/de/translation.json": "{}"},
			env:   map[string]string{envOrgID: testOrg, envProjectID: testProject},
			args:  []string{"init"},
			want:  config.Config{OrgID: testOrg, ProjectID: testProject, Files: "public/locales/{locale}/translation.json", Format: "i18next"},
		},
		{
			name:     "the key's only project is picked; files found by its base locale",
			files:    map[string]string{"locales/en.json": `{"a": "b"}`, "lang/en-US.json": `{"a": "b"}`},
			projects: []api.Project{mobile},
			args:     []string{"init", "--org", testOrg},
			want:     config.Config{OrgID: testOrg, ProjectID: otherID, Files: "lang/{locale}.json", Format: "json"},
			wantNote: "# Mobile (mobile)",
		},
		{
			name:        "interactive: pick a project, accept the detected files",
			files:       map[string]string{"src/i18n/en.json": `{"a": "b"}`, "src/i18n/de.json": "{}", "docs/en.json": "{}"},
			interactive: true,
			stdin:       testOrg + "\n9\n1\n\n",
			args:        []string{"init"},
			want:        config.Config{OrgID: testOrg, ProjectID: testProject, Files: "src/i18n/{locale}.json", Format: "json"},
			wantStderr:  "Pick a number from 1 to 2.",
		},
		{
			name:        "interactive: type a files path",
			interactive: true,
			stdin:       "translations/{locale}/app.json\n",
			args:        []string{"init", "--org", testOrg, "--project", testProject},
			want:        config.Config{OrgID: testOrg, ProjectID: testProject, Files: "translations/{locale}/app.json", Format: "json"},
		},
		{
			name:       "several projects without a terminal",
			args:       []string{"init", "--org", testOrg},
			wantCode:   1,
			wantStderr: "pass --project; this API key can see: web, mobile",
		},
		{
			name:       "no org without a terminal",
			args:       []string{"init"},
			wantCode:   1,
			wantStderr: "pass --org (sb init asks only in a terminal)",
		},
		{
			name:       "no files found without a terminal",
			args:       []string{"init", "--org", testOrg, "--project", "web"},
			wantCode:   1,
			wantStderr: "pass --files",
		},
		{
			name:       "unknown project",
			args:       []string{"init", "--org", testOrg, "--project", "shop", "--files", "{locale}.json"},
			wantCode:   1,
			wantStderr: `project "shop" not found; this API key can see: web, mobile`,
		},
		{
			name:       "no key: project id taken as given",
			noKey:      true,
			args:       []string{"init", "--org", testOrg, "--project", otherID, "--files", "{locale}.json", "--format", "i18next"},
			want:       config.Config{OrgID: testOrg, ProjectID: otherID, Files: "{locale}.json", Format: "i18next"},
			wantStdout: "Next: set STRINGBIRD_API_KEY",
			wantStderr: "No API key set, so the project isn't checked",
		},
		{
			name:       "no key: a slug can't be looked up",
			noKey:      true,
			args:       []string{"init", "--org", testOrg, "--project", "web", "--files", "{locale}.json"},
			wantCode:   1,
			wantStderr: `project "web" isn't an id; a slug needs an API key to look up`,
		},
		{
			name:       "files without a placeholder",
			args:       []string{"init", "--org", testOrg, "--project", "web", "--files", "en.json"},
			wantCode:   1,
			wantStderr: `files "en.json" needs a {locale} placeholder`,
		},
		{
			name:       "unknown format",
			args:       []string{"init", "--format", "xliff"},
			wantCode:   1,
			wantStderr: `unknown format "xliff"; use json or i18next`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.projects = []api.Project{fake.project, mobile}
			if tt.projects != nil {
				fake.projects = tt.projects
			}
			p := newTestProject(t, fake, "")
			require.NoError(t, os.Remove(filepath.Join(p.dir, "stringbird.yaml")))
			for path, content := range tt.files {
				p.write(path, content)
			}
			for k, v := range tt.env {
				p.env[k] = v
			}
			if tt.noKey {
				delete(p.env, envAPIKey)
			}
			p.interactive, p.stdin = tt.interactive, tt.stdin

			res := p.run(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stdout: %s\nstderr: %s", res.stdout, res.stderr)
			assert.Contains(t, res.stdout, tt.wantStdout)
			assert.Contains(t, res.stderr, tt.wantStderr)
			if tt.wantCode != 0 {
				assert.NoFileExists(t, filepath.Join(p.dir, "stringbird.yaml"))
				return
			}
			written := p.read("stringbird.yaml")
			got, err := config.Parse([]byte(written))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Contains(t, written, tt.wantNote)
		})
	}
}

func TestInitKeepsAnExistingConfig(t *testing.T) {
	p := newTestProject(t, newFakeAPI(), "")
	before := p.read("stringbird.yaml")
	args := []string{"init", "--org", testOrg, "--project", "web", "--files", "new/{locale}.json"}

	res := p.run(args...)

	assert.Equal(t, 1, res.code)
	assert.Contains(t, res.stderr, "already exists; pass --force to replace it")
	assert.Equal(t, before, p.read("stringbird.yaml"))

	res = p.run(append(args, "--force")...)

	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, p.read("stringbird.yaml"), `files: "new/{locale}.json"`)
}

func TestInitThenPull(t *testing.T) {
	fake := newFakeAPI()
	fake.exports = map[string]fakeExport{"de": {body: `{"hi":"Hallo"}`}}
	p := newTestProject(t, fake, "")
	require.NoError(t, os.Remove(filepath.Join(p.dir, "stringbird.yaml")))
	p.write("i18n/en.json", "{}")

	res := p.runHere("init", "--org", testOrg, "--project", "web")
	require.Equal(t, 0, res.code, res.stderr)

	res = p.runHere("pull", "-l", "de")
	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, `{"hi":"Hallo"}`, p.read("i18n/de.json"))
}

func TestDetectFiles(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		base  string
		want  []string
	}{
		{
			name:  "file per locale",
			files: []string{"src/locales/en.json", "src/locales/de.json", "src/locales/fr.json"},
			base:  "en",
			want:  []string{"src/locales/{locale}.json"},
		},
		{
			name:  "folder per locale",
			files: []string{"public/locales/en/translation.json", "public/locales/de/translation.json", "public/locales/en/common.json"},
			base:  "en",
			want:  []string{"public/locales/{locale}/translation.json", "public/locales/{locale}/common.json"},
		},
		{
			name:  "more locales first, then shallower",
			files: []string{"a/b/en.json", "a/b/de.json", "a/b/fr.json", "en.json", "x/en.json", "x/de.json"},
			base:  "en",
			want:  []string{"a/b/{locale}.json", "x/{locale}.json", "{locale}.json"},
		},
		{
			name:  "skips dependencies, hidden and build folders",
			files: []string{"node_modules/pkg/en.json", ".cache/en.json", "dist/en.json", "vendor/x/en.json", "lang/en.json"},
			base:  "en",
			want:  []string{"lang/{locale}.json"},
		},
		{
			name:  "region locale, any case",
			files: []string{"i18n/en-US.json", "i18n/de-DE.json"},
			base:  "en-us",
			want:  []string{"i18n/{locale}.json"},
		},
		{
			name:  "ignores other files",
			files: []string{"package.json", "en.yaml", "src/en.ts", "tsconfig.json"},
			base:  "en",
			want:  []string{},
		},
		{
			name:  "too deep",
			files: []string{"a/b/c/d/e/f/en.json"},
			base:  "en",
			want:  []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, f := range tt.files {
				path := filepath.Join(root, filepath.FromSlash(f))
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte("{}"), 0o644))
			}

			assert.Equal(t, tt.want, detectFiles(root, tt.base))
		})
	}
}

func TestGuessFormat(t *testing.T) {
	dir := t.TempDir()
	for content, want := range map[string]string{
		`{"hi": "Hello, {name}!"}`:                                   "json",
		`{"items": "{count, plural, one {# item} other {# items}}"}`: "json",
		`{"hi": "Hello, {{name}}!"}`:                                 "i18next",
		`{"item_one": "an item", "item_other": "items"}`:             "i18next",
		`{"cart": {"item_other": "{count} items"}}`:                  "i18next",
	} {
		path := filepath.Join(dir, "en.json")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		assert.Equal(t, want, guessFormat(path), content)
	}
	assert.Equal(t, "json", guessFormat(filepath.Join(dir, "missing.json")))
}
