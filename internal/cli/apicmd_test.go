package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPICommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		// stdin and files are the input; noConfig removes stringbird.yaml.
		stdin    string
		files    map[string]string
		noConfig bool
		fail     map[string]fakeError
		wantCode int
		// wantRequest is "METHOD path" of the one request; wantQuery and
		// wantBody what it carried.
		wantRequest string
		wantQuery   map[string]string
		wantBody    string
		wantJSON    bool
		wantStdout  []string
		wantStderr  string
	}{
		{
			name:        "relative to the project, fields as query",
			args:        []string{"api", "-X", "get", "/keys", "-f", "status=missing", "-f", "locale=de"},
			wantRequest: "GET /keys",
			wantQuery:   map[string]string{"status": "missing", "locale": "de"},
			wantStdout:  []string{"{\n  \"keys\": ["},
		},
		{
			name:        "no leading slash, query in the path",
			args:        []string{"api", "-X", "GET", "keys?limit=1", "-f", "prefix=auth."},
			wantRequest: "GET /keys",
			wantQuery:   map[string]string{"limit": "1", "prefix": "auth."},
		},
		{
			name:        "absolute path with {org}, no project needed",
			args:        []string{"api", "/v1/orgs/{org}/projects", "--org", testOrg},
			noConfig:    true,
			wantRequest: "GET (org)/projects",
			wantQuery:   map[string]string{},
			wantStdout:  []string{`"slug": "web"`},
		},
		{
			name:        "fields as a JSON body, typed",
			args:        []string{"api", "/locales", "-f", "locale=it", "-F", "extra=true"},
			wantRequest: "POST /locales",
			wantQuery:   map[string]string{},
			wantBody:    `{"locale":"it","extra":true}`,
			wantJSON:    true,
			wantStdout:  []string{`"code": "it"`},
		},
		{
			name:        "typed field that isn't JSON stays a string",
			args:        []string{"api", "-X", "PUT", "/environments/staging", "-F", "release_id=abc"},
			wantRequest: "PUT /environments/staging",
			wantQuery:   map[string]string{},
			wantBody:    `{"release_id":"abc"}`,
			wantJSON:    true,
			wantCode:    1,
			wantStderr:  "PUT /v1/orgs/" + testOrg + "/projects/" + testProject + "/environments/staging: 404 Not Found",
			wantStdout:  []string{`"code": "environment_not_found"`},
		},
		{
			name:        "file as the body",
			args:        []string{"api", "-X", "POST", "/import?locale=de&dry_run=true", "--input", "de.json"},
			files:       map[string]string{"de.json": "a\nb"},
			wantRequest: "POST /import",
			wantQuery:   map[string]string{"locale": "de", "dry_run": "true"},
			wantBody:    "a\nb",
			wantStdout:  []string{`"dry_run": true`},
		},
		{
			name:        "stdin as the body",
			args:        []string{"api", "/import?locale=en", "--input", "-"},
			stdin:       "x",
			wantRequest: "POST /import",
			wantQuery:   map[string]string{"locale": "en"},
			wantBody:    "x",
		},
		{
			name:        "status line and headers",
			args:        []string{"api", "-i", "/environments"},
			wantRequest: "GET /environments",
			wantQuery:   map[string]string{},
			wantStdout:  []string{"HTTP/1.1 200 OK\nContent-Length: ", "Content-Type: application/json\n", "\n\n{"},
		},
		{
			name:       "fields and input don't mix",
			args:       []string{"api", "/import", "--input", "de.json", "-f", "locale=de"},
			wantCode:   1,
			wantStderr: "--input and fields don't mix",
		},
		{
			name:       "bad field",
			args:       []string{"api", "/keys", "-f", "status"},
			wantCode:   1,
			wantStderr: `field "status" isn't key=value`,
		},
		{
			name:       "relative path needs a project",
			args:       []string{"api", "/keys", "--org", testOrg},
			noConfig:   true,
			wantCode:   1,
			wantStderr: "pass --project (or STRINGBIRD_PROJECT_ID)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.keys = testKeys()
			if tt.fail != nil {
				fake.fail = tt.fail
			}
			p := newTestProject(t, fake, "")
			if tt.noConfig {
				require.NoError(t, os.Remove(filepath.Join(p.dir, "stringbird.yaml")))
			}
			for path, content := range tt.files {
				p.write(path, content)
			}
			p.stdin = tt.stdin

			res := p.runHere(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stdout: %s\nstderr: %s", res.stdout, res.stderr)
			for _, s := range tt.wantStdout {
				assert.Contains(t, res.stdout, s)
			}
			assert.Contains(t, res.stderr, tt.wantStderr)
			if tt.wantRequest == "" {
				assert.Empty(t, p.requests())
				return
			}
			require.Equal(t, []string{tt.wantRequest}, p.requests())
			got := p.api.requests[0]
			assert.Equal(t, tt.wantQuery, got.query)
			if tt.wantJSON {
				assert.JSONEq(t, tt.wantBody, got.body)
			} else {
				assert.Equal(t, tt.wantBody, got.body)
			}
		})
	}
}

func TestParseFields(t *testing.T) {
	got, err := parseFields([]string{"a=1", "b=x=y", "c="}, []string{"n=42", "t=true", "l=[\"a\"]", "s=plain", "z=null"})

	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"a": "1", "b": "x=y", "c": "",
		"n": float64(42), "t": true, "l": []any{"a"}, "s": "plain", "z": nil,
	}, got)
	assert.Equal(t, "42", fieldString(float64(42)))
	assert.Equal(t, `["a"]`, fieldString([]any{"a"}))
}

func TestAPICommandGitBashPath(t *testing.T) {
	p := newTestProject(t, newFakeAPI(), "")

	res := p.run("api", "C:/Program Files/Git/locales")

	assert.Equal(t, 1, res.code)
	assert.Contains(t, res.stderr, `"C:/Program Files/Git/locales" is a Windows path: Git Bash rewrites arguments starting with /`)
	assert.Empty(t, p.requests())
}
