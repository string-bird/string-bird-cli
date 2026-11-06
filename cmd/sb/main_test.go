package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildSB builds the binary the way a release does, with a version stamped in.
func buildSB(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sb")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-X main.version=1.2.3", "-o", bin, ".")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return bin
}

type result struct {
	code           int
	stdout, stderr string
}

func runSB(t *testing.T, bin, dir string, env []string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else {
		require.NoError(t, err)
	}
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// TestBinary runs the built sb as a user would: exit codes, output streams,
// the default config path and the API URL override.
func TestBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := buildSB(t)

	api := http.NewServeMux()
	api.HandleFunc("GET /v1/orgs/o/projects/p", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sb_k" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"code":"invalid_api_key","message":"invalid API key"}}`))
			return
		}
		assert.Equal(t, "sb/1.2.3", r.Header.Get("User-Agent"))
		w.Write([]byte(`{"project":{"id":"p","base_locale":"en","locales":["en"]}}`))
	})
	api.HandleFunc("GET /v1/orgs/o/projects/p/export", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hello":"Hello"}`))
	})
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "stringbird.yaml"),
		[]byte("org_id: o\nproject_id: p\nfiles: locales/{locale}.json\n"), 0o644))
	env := []string{"STRINGBIRD_API_URL=" + srv.URL, "STRINGBIRD_API_KEY=sb_k"}

	t.Run("version", func(t *testing.T) {
		res := runSB(t, bin, project, nil, "--version")
		assert.Equal(t, result{stdout: "sb version 1.2.3\n"}, res)
	})
	t.Run("help lists the commands", func(t *testing.T) {
		res := runSB(t, bin, project, nil, "--help")
		assert.Equal(t, 0, res.code)
		for _, cmd := range []string{"push", "pull", "release"} {
			assert.Contains(t, res.stdout, cmd)
		}
		assert.NotContains(t, res.stdout, "api-url")
	})
	t.Run("pull with the default config", func(t *testing.T) {
		res := runSB(t, bin, project, env, "pull")
		require.Equal(t, 0, res.code, res.stderr)
		got, err := os.ReadFile(filepath.Join(project, "locales", "en.json"))
		require.NoError(t, err)
		assert.Equal(t, `{"hello":"Hello"}`, string(got))
		assert.Empty(t, res.stderr)
	})
	t.Run("errors go to stderr with exit code 1", func(t *testing.T) {
		res := runSB(t, bin, project, []string{"STRINGBIRD_API_URL=" + srv.URL, "STRINGBIRD_API_KEY=sb_bad"}, "pull")
		assert.Equal(t, 1, res.code)
		assert.Empty(t, res.stdout)
		assert.Equal(t, "sb: get project: invalid_api_key: invalid API key (HTTP 401)\n", res.stderr)
	})
	t.Run("no config here", func(t *testing.T) {
		res := runSB(t, bin, t.TempDir(), env, "pull")
		assert.Equal(t, 1, res.code)
		assert.Contains(t, res.stderr, "sb: read config: open stringbird.yaml")
	})
}
