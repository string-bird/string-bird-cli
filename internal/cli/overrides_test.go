package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/string-bird/string-bird-cli/internal/api"
)

const otherID = "01a1b100-0000-7000-8000-0000000000ff"

// TestOverrides covers where org, project and files come from: flags first,
// then STRINGBIRD_ORG_ID / STRINGBIRD_PROJECT_ID, then stringbird.yaml.
func TestOverrides(t *testing.T) {
	tests := []struct {
		name string
		// config replaces stringbird.yaml; "-" removes it.
		config string
		env    map[string]string
		args   []string
		// wantCode, wantStderr; wantRequests are the requests in order.
		wantCode     int
		wantStderr   string
		wantRequests []string
	}{
		{
			name:         "everything from the file",
			args:         []string{"pull", "-l", "de"},
			wantRequests: []string{"GET /export"},
		},
		{
			name:         "environment over the file",
			config:       "org_id: " + otherID + "\nproject_id: " + otherID + "\nfiles: l/{locale}.json\n",
			env:          map[string]string{envOrgID: testOrg, envProjectID: testProject},
			args:         []string{"pull", "-l", "de"},
			wantRequests: []string{"GET /export"},
		},
		{
			name:         "flags over the environment",
			env:          map[string]string{envOrgID: otherID, envProjectID: otherID},
			args:         []string{"pull", "-l", "de", "--org", testOrg, "--project", testProject},
			wantRequests: []string{"GET /export"},
		},
		{
			name:         "no file: flags alone",
			config:       "-",
			args:         []string{"pull", "-l", "de", "--org", testOrg, "--project", testProject, "--files", "out/{locale}.json"},
			wantRequests: []string{"GET /export"},
		},
		{
			name:         "no file: environment and --files",
			config:       "-",
			env:          map[string]string{envOrgID: testOrg, envProjectID: testProject},
			args:         []string{"pull", "-l", "de", "--files", "out/{locale}.json"},
			wantRequests: []string{"GET /export"},
		},
		{
			name:       "no file, nothing set",
			config:     "-",
			args:       []string{"pull"},
			wantCode:   1,
			wantStderr: "no stringbird.yaml here: run `sb init` to create one, or pass --org (or STRINGBIRD_ORG_ID), --project (or STRINGBIRD_PROJECT_ID), --files",
		},
		{
			name:       "no file, files missing",
			config:     "-",
			args:       []string{"pull", "--org", testOrg, "--project", testProject},
			wantCode:   1,
			wantStderr: "no stringbird.yaml here: run `sb init` to create one, or pass --files",
		},
		{
			name:       "file lacks the project",
			config:     "org_id: " + testOrg + "\nfiles: l/{locale}.json\n",
			args:       []string{"pull"},
			wantCode:   1,
			wantStderr: "stringbird.yaml: missing project_id; set it there or pass --project (or STRINGBIRD_PROJECT_ID)",
		},
		{
			name:         "project by slug",
			config:       "org_id: " + testOrg + "\nproject_id: web\nfiles: l/{locale}.json\n",
			args:         []string{"pull", "-l", "de"},
			wantRequests: []string{"GET (org)/projects", "GET /export"},
		},
		{
			name:         "slug flag, any case",
			args:         []string{"pull", "-l", "de", "--project", "WEB"},
			wantRequests: []string{"GET (org)/projects", "GET /export"},
		},
		{
			name:         "unknown slug",
			args:         []string{"pull", "--project", "shop"},
			wantCode:     1,
			wantStderr:   `project "shop" not found; this API key can see: web, mobile`,
			wantRequests: []string{"GET (org)/projects"},
		},
		{
			name:       "--files without a placeholder",
			args:       []string{"pull", "--files", "en.json"},
			wantCode:   1,
			wantStderr: `files "en.json" needs a {locale} placeholder`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.exports = map[string]fakeExport{"de": {body: "de"}}
			fake.projects = []api.Project{fake.project, {ID: otherID, Slug: "mobile", Name: "Mobile"}}
			p := newTestProject(t, fake, "")
			switch tt.config {
			case "":
			case "-":
				require.NoError(t, os.Remove(filepath.Join(p.dir, "stringbird.yaml")))
			default:
				p.write("stringbird.yaml", tt.config)
			}
			for k, v := range tt.env {
				p.env[k] = v
			}

			res := p.runHere(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stderr: %s", res.stderr)
			assert.Contains(t, res.stderr, tt.wantStderr)
			assert.Equal(t, tt.wantRequests, nilIfEmpty(p.requests()))
		})
	}
}

func TestFilesFlagIsRelativeToTheWorkingDirectory(t *testing.T) {
	fake := newFakeAPI()
	fake.exports = map[string]fakeExport{"de": {body: "de"}}
	p := newTestProject(t, fake, "")
	p.write("sub/stringbird.yaml", "org_id: "+testOrg+"\nproject_id: "+testProject+"\nfiles: from-config/{locale}.json\n")

	res := p.runHere("pull", "-l", "de", "--config", filepath.Join("sub", "stringbird.yaml"), "--files", "flag/{locale}.json")
	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, "de", p.read("flag/de.json"))

	res = p.runHere("pull", "-l", "de", "--config", filepath.Join("sub", "stringbird.yaml"))
	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, "de", p.read("sub/from-config/de.json"))
}

func TestExplicitConfigMustExist(t *testing.T) {
	p := newTestProject(t, newFakeAPI(), "")

	res := p.runHere("pull", "--config", "elsewhere.yaml", "--org", testOrg, "--project", testProject, "--files", "{locale}.json")

	assert.Equal(t, 1, res.code)
	assert.Contains(t, res.stderr, "read config: open elsewhere.yaml")
	assert.Empty(t, p.requests())
}

func TestProjectIDNeedsNoLookup(t *testing.T) {
	fake := newFakeAPI()
	fake.exports = map[string]fakeExport{"de": {body: "de"}}
	p := newTestProject(t, fake, "")

	res := p.run("pull", "-l", "de")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Empty(t, p.requestsTo(http.MethodGet, "(org)/projects"))
}
