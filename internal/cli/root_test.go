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
			p.env[envAPIURL] = p.url

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
		// env and flag set STRINGBIRD_API_URL and --api-url; "fake" stands
		// for the fake's URL.
		env, flag  string
		config     string
		wantCode   int
		wantStderr string
	}{
		{name: "environment", env: "fake"},
		{name: "flag over environment", env: "http://127.0.0.1:1", flag: "fake"},
		{
			name: "unreachable", env: "http://127.0.0.1:1",
			wantCode: 1, wantStderr: "get project: GET /v1/orgs/org-1/projects/proj-1",
		},
		{
			name: "not configurable in the file", env: "fake", config: "api_url: http://127.0.0.1:1\n",
			wantCode: 1, wantStderr: "field api_url not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.project.Locales = nil
			p := newTestProject(t, fake, tt.config)
			resolve := func(s string) string {
				if s == "fake" {
					return p.url
				}
				return s
			}
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

func TestAPIURLDefaultsToProduction(t *testing.T) {
	a := &app{getenv: func(string) string { return "" }}

	assert.Equal(t, "https://api.stringbird.io", a.baseURL())
}

func TestHelpHidesAPIURL(t *testing.T) {
	var out strings.Builder
	a := &app{version: "test", stdout: &out, stderr: &out, getenv: func(string) string { return "" }}

	code := a.run(context.Background(), []string{"pull", "--help"})

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "--api-key")
	assert.NotContains(t, out.String(), "api-url")
}

func TestVersion(t *testing.T) {
	var stdout strings.Builder
	a := &app{version: "test", stdout: &stdout, stderr: &stdout, getenv: func(string) string { return "" }}

	code := a.run(context.Background(), []string{"--version"})

	require.Equal(t, 0, code)
	assert.Equal(t, "sb version test\n", stdout.String())
}
