package cli

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthAndConnection(t *testing.T) {
	tests := []struct {
		name string
		// env replaces the environment; config is extra config lines.
		env          map[string]string
		config       string
		args         []string
		rateLimited  int
		fail         map[string]fakeError
		wantCode     int
		wantRequests int
		wantSlept    []time.Duration
		wantStderr   string
	}{
		{
			name:         "key from the environment",
			env:          map[string]string{envAPIKey: testKey},
			args:         []string{"pull"},
			wantRequests: 1,
		},
		{
			name:         "key from the flag wins",
			env:          map[string]string{envAPIKey: "sb_wrong"},
			args:         []string{"pull", "--api-key", testKey},
			wantRequests: 1,
		},
		{
			name:       "no key",
			env:        map[string]string{},
			args:       []string{"pull"},
			wantCode:   1,
			wantStderr: "no API key: set STRINGBIRD_API_KEY or pass --api-key",
		},
		{
			name:         "revoked key",
			env:          map[string]string{envAPIKey: "sb_revoked"},
			args:         []string{"pull"},
			wantCode:     1,
			wantRequests: 1,
			wantStderr:   "get project: invalid_api_key: invalid api key (HTTP 401)",
		},
		{
			name:         "key limited to other projects",
			env:          map[string]string{envAPIKey: testKey},
			args:         []string{"pull"},
			fail:         map[string]fakeError{"GET": {status: http.StatusNotFound, code: "project_not_found"}},
			wantCode:     1,
			wantRequests: 1,
			wantStderr:   "project_not_found",
		},
		{
			name:         "rate limited, then retried",
			env:          map[string]string{envAPIKey: testKey},
			args:         []string{"pull"},
			rateLimited:  2,
			wantRequests: 3,
			wantSlept:    []time.Duration{7 * time.Second, 7 * time.Second},
		},
		{
			name:         "rate limited too often",
			env:          map[string]string{envAPIKey: testKey},
			args:         []string{"pull"},
			rateLimited:  10,
			wantCode:     1,
			wantRequests: 4,
			wantSlept:    []time.Duration{7 * time.Second, 7 * time.Second, 7 * time.Second},
			wantStderr:   "rate_limited",
		},
		{
			name:       "api key in the config",
			env:        map[string]string{envAPIKey: testKey},
			config:     "api_key: sb_secret\n",
			args:       []string{"pull"},
			wantCode:   1,
			wantStderr: "api_key doesn't belong in the config file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.project.Locales = nil
			fake.rateLimited = tt.rateLimited
			if tt.fail != nil {
				fake.fail = tt.fail
			}
			p := newTestProject(t, fake, tt.config)
			p.env = tt.env

			res := p.run(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stderr: %s", res.stderr)
			assert.Len(t, p.requests(), tt.wantRequests)
			assert.Equal(t, tt.wantSlept, p.slept)
			assert.Contains(t, res.stderr, tt.wantStderr)
		})
	}
}

func TestAPIURL(t *testing.T) {
	tests := []struct {
		name string
		// url replaces the config's api_url ("" drops it); env and flag
		// override it. "fake" stands for the fake's URL.
		url, env, flag string
		wantCode       int
		wantStderr     string
	}{
		{name: "from the config", url: "fake"},
		{name: "environment over config", url: "http://127.0.0.1:1", env: "fake"},
		{name: "flag over environment", url: "http://127.0.0.1:1", env: "http://127.0.0.1:1", flag: "fake"},
		{name: "environment alone", env: "fake"},
		{name: "none", wantCode: 1, wantStderr: "no API URL"},
		{name: "unreachable", url: "http://127.0.0.1:1", wantCode: 1, wantStderr: "get project: GET /v1/orgs/org-1/projects/proj-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.project.Locales = nil
			p := newTestProject(t, fake, "")
			resolve := func(s string) string {
				if s == "fake" {
					return p.url
				}
				return s
			}
			config := "org_id: org-1\nproject_id: proj-1\nfiles: l/{locale}.json\n"
			if tt.url != "" {
				config += "api_url: " + resolve(tt.url) + "\n"
			}
			p.write("stringbird.yaml", config)
			p.env[envAPIURL] = resolve(tt.env)
			args := []string{"pull"}
			if tt.flag != "" {
				args = append(args, "--api-url", resolve(tt.flag))
			}

			res := p.run(args...)

			require.Equal(t, tt.wantCode, res.code, "stderr: %s", res.stderr)
			assert.Contains(t, res.stderr, tt.wantStderr)
		})
	}
}

func TestVersion(t *testing.T) {
	var stdout strings.Builder
	a := &app{version: "test", stdout: &stdout, stderr: &stdout, getenv: func(string) string { return "" }}

	code := a.run(context.Background(), []string{"--version"})

	require.Equal(t, 0, code)
	assert.Equal(t, "sb version test\n", stdout.String())
}
