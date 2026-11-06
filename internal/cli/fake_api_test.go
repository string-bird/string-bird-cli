package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/string-bird/string-bird-cli/internal/api"
)

const (
	testKey     = "sb_test_key"
	testOrg     = "01a1b100-0000-7000-8000-00000000000a"
	testProject = "01a1b100-0000-7000-8000-00000000000b"
)

// fakeAPI is a hand-written stand-in for the String Bird API: the routes the
// CLI calls, with in-memory state, recorded requests and injectable errors.
type fakeAPI struct {
	mu sync.Mutex

	project api.Project
	// projects is what the org-level project list shows; nil means just project.
	projects []api.Project
	exports  map[string]fakeExport // by locale
	releases []api.Release         // oldest first
	envs     map[string]string     // environment name → release id
	// publishAfter is how many GETs a pending release stays pending; then it
	// becomes publishAs (published or failed).
	publishAfter int
	publishAs    string

	// fail answers a route ("POST /import") with an error.
	fail map[string]fakeError
	// rateLimited answers the first n requests with 429.
	rateLimited int

	requests []fakeRequest
}

type fakeExport struct {
	body    string
	skipped []string
}

type fakeError struct {
	status   int
	code     string
	problems []api.Problem
}

type fakeRequest struct {
	method string
	path   string // relative to the project: /import, /releases, ""
	query  map[string]string
	body   string
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		project: api.Project{
			ID: testProject, Name: "Web", Slug: "web", BaseLocale: "en", BundleFormat: "icu_flat",
			Locales: []string{"de", "en", "fr"},
		},
		exports: map[string]fakeExport{},
		envs:    map[string]string{},
		fail:    map[string]fakeError{},
	}
}

func (f *fakeAPI) handler() http.Handler {
	mux := http.NewServeMux()
	prefix := "/v1/orgs/{org}/projects/{project}"
	route := func(pattern string, h func(w http.ResponseWriter, r *http.Request)) {
		method, path, _ := strings.Cut(pattern, " ")
		mux.HandleFunc(method+" "+prefix+path, func(w http.ResponseWriter, r *http.Request) {
			f.serve(w, r, pattern, h)
		})
	}
	route("GET ", f.getProject)
	route("POST /import", f.importFile)
	route("GET /export", f.export)
	route("POST /releases", f.createRelease)
	route("GET /releases", f.listReleases)
	route("GET /releases/{id}", f.getRelease)
	route("GET /environments", f.listEnvironments)
	route("PUT /environments/{name}", f.promote)
	mux.HandleFunc("GET /v1/orgs/{org}/projects", func(w http.ResponseWriter, r *http.Request) {
		f.serve(w, r, "GET /projects", f.listProjects)
	})
	return mux
}

// serve checks auth and scope, records the request and applies injected
// errors before calling the route's handler.
func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request, route string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()

	body, _ := io.ReadAll(r.Body)
	query := map[string]string{}
	for k, v := range r.URL.Query() {
		query[k] = v[0]
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/orgs/"+r.PathValue("org")+"/projects/"+r.PathValue("project"))
	if r.PathValue("project") == "" {
		path = "(org)/projects"
	}
	f.requests = append(f.requests, fakeRequest{method: r.Method, path: path, query: query, body: string(body)})
	r.Body = io.NopCloser(strings.NewReader(string(body)))

	switch {
	case r.Header.Get("Authorization") != "Bearer "+testKey:
		writeError(w, fakeError{status: http.StatusUnauthorized, code: "invalid_api_key"})
	case r.PathValue("org") != testOrg:
		writeError(w, fakeError{status: http.StatusNotFound, code: "org_not_found"})
	case r.PathValue("project") != "" && r.PathValue("project") != testProject:
		writeError(w, fakeError{status: http.StatusNotFound, code: "project_not_found"})
	case f.rateLimited > 0:
		f.rateLimited--
		w.Header().Set("Retry-After", "7")
		writeError(w, fakeError{status: http.StatusTooManyRequests, code: "rate_limited"})
	default:
		if e, ok := f.fail[strings.TrimSpace(route)]; ok {
			writeError(w, e)
			return
		}
		h(w, r)
	}
}

func (f *fakeAPI) listProjects(w http.ResponseWriter, _ *http.Request) {
	projects := f.projects
	if projects == nil {
		projects = []api.Project{f.project}
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (f *fakeAPI) getProject(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"project": f.project})
}

func (f *fakeAPI) importFile(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	body, _ := io.ReadAll(r.Body)
	// Every non-empty line of the fake's files is a new key.
	var added []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		if line != "" {
			added = append(added, q.Get("locale")+":"+line)
		}
	}
	dryRun := q.Get("dry_run") == "true"
	var changeSet *string
	if !dryRun {
		id := "cs-1"
		changeSet = &id
	}
	writeJSON(w, http.StatusOK, api.ImportResult{
		DryRun:      dryRun,
		ChangeSetID: changeSet,
		Summary:     api.ImportSummary{AddedKeys: len(added), Unchanged: 4},
		AddedKeys:   added,
		Updated:     []api.UpdatedKey{{Key: "home.title", OldValue: "Hi", NewValue: "Hello"}},
	})
}

func (f *fakeAPI) export(w http.ResponseWriter, r *http.Request) {
	locale := r.URL.Query().Get("locale")
	exp, ok := f.exports[locale]
	if !ok {
		writeError(w, fakeError{status: http.StatusNotFound, code: "locale_not_found"})
		return
	}
	if len(exp.skipped) > 0 {
		b, _ := json.Marshal(exp.skipped)
		w.Header().Set("X-Skipped-Keys", string(b))
	}
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, exp.body)
}

func (f *fakeAPI) createRelease(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Note        string `json:"note"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, fakeError{status: http.StatusBadRequest, code: "invalid_json"})
		return
	}
	if _, ok := f.envs[in.Environment]; in.Environment != "" && !ok {
		writeError(w, fakeError{status: http.StatusNotFound, code: "environment_not_found"})
		return
	}
	rel := f.addRelease(api.ReleasePending)
	rel.Note = in.Note
	if in.Environment != "" {
		f.envs[in.Environment] = rel.ID
	}
	writeJSON(w, http.StatusCreated, map[string]any{"release": rel})
}

func (f *fakeAPI) listReleases(w http.ResponseWriter, r *http.Request) {
	before, _ := strconv.Atoi(r.URL.Query().Get("before"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	out := []api.Release{}
	for i := len(f.releases) - 1; i >= 0 && len(out) < limit; i-- {
		if before == 0 || f.releases[i].Version < before {
			out = append(out, f.releases[i])
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"releases": out})
}

func (f *fakeAPI) getRelease(w http.ResponseWriter, r *http.Request) {
	rel := f.release(r.PathValue("id"))
	if rel == nil {
		writeError(w, fakeError{status: http.StatusNotFound, code: "release_not_found"})
		return
	}
	if rel.Status == api.ReleasePending {
		if f.publishAfter > 0 {
			f.publishAfter--
		} else {
			rel.Status = f.publishAs
			if rel.Status == api.ReleaseFailed {
				rel.Error = "upload: bucket unreachable"
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"release": rel})
}

func (f *fakeAPI) listEnvironments(w http.ResponseWriter, _ *http.Request) {
	out := []api.Environment{}
	for name, id := range f.envs {
		out = append(out, api.Environment{ID: "env-" + name, Name: name, Release: f.release(id)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"environments": out})
}

func (f *fakeAPI) promote(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var in struct {
		ReleaseID string `json:"release_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, fakeError{status: http.StatusBadRequest, code: "invalid_json"})
		return
	}
	if _, ok := f.envs[name]; !ok {
		writeError(w, fakeError{status: http.StatusNotFound, code: "environment_not_found"})
		return
	}
	rel := f.release(in.ReleaseID)
	if rel == nil {
		writeError(w, fakeError{status: http.StatusNotFound, code: "release_not_found"})
		return
	}
	f.envs[name] = rel.ID
	writeJSON(w, http.StatusOK, map[string]any{"environment": api.Environment{ID: "env-" + name, Name: name, Release: rel}})
}

// addRelease appends the next version. The caller holds f.mu.
func (f *fakeAPI) addRelease(status string) *api.Release {
	v := len(f.releases) + 1
	f.releases = append(f.releases, api.Release{
		ID: fmt.Sprintf("rel-%d", v), Version: v, Status: status, BundleFormat: "icu_flat",
		KeyCount: 120, Locales: []string{"de", "en"}, CreatedAt: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC),
	})
	return &f.releases[v-1]
}

func (f *fakeAPI) release(id string) *api.Release {
	for i := range f.releases {
		if f.releases[i].ID == id {
			return &f.releases[i]
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, e fakeError) {
	body := map[string]any{"code": e.code, "message": strings.ReplaceAll(e.code, "_", " ")}
	if e.problems != nil {
		body["problems"] = e.problems
	}
	writeJSON(w, e.status, map[string]any{"error": body})
}

// testProjectDir is a project folder with stringbird.yaml pointing at a fake
// API, and sb run inside it.
type testProjectDir struct {
	t   *testing.T
	dir string
	api *fakeAPI
	url string
	env map[string]string
	// slept collects the waits sb asked for.
	slept []time.Duration
	// sleepErr is what every wait returns, e.g. a deadline.
	sleepErr error
	// stdin is what sb reads from; interactive makes it a terminal.
	stdin       string
	interactive bool
}

func newTestProject(t *testing.T, fake *fakeAPI, extraConfig string) *testProjectDir {
	t.Helper()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	p := &testProjectDir{
		t: t, dir: t.TempDir(), api: fake, url: srv.URL,
		env: map[string]string{envAPIKey: testKey, envAPIURL: srv.URL},
	}
	p.write("stringbird.yaml", fmt.Sprintf("org_id: %s\nproject_id: %s\nfiles: locales/{locale}.json\n%s",
		testOrg, testProject, extraConfig))
	return p
}

func (p *testProjectDir) write(rel, content string) {
	p.t.Helper()
	path := filepath.Join(p.dir, filepath.FromSlash(rel))
	require.NoError(p.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(p.t, os.WriteFile(path, []byte(content), 0o644))
}

func (p *testProjectDir) read(rel string) string {
	p.t.Helper()
	b, err := os.ReadFile(filepath.Join(p.dir, filepath.FromSlash(rel)))
	require.NoError(p.t, err)
	return string(b)
}

type runResult struct {
	code   int
	stdout string
	stderr string
}

// run runs sb with the project's config.
func (p *testProjectDir) run(args ...string) runResult {
	p.t.Helper()
	return p.exec(append(args, "--config", filepath.Join(p.dir, "stringbird.yaml")))
}

// runHere runs sb inside the project folder without --config, so it looks for
// stringbird.yaml in the working directory like a user's shell would.
func (p *testProjectDir) runHere(args ...string) runResult {
	p.t.Helper()
	p.t.Chdir(p.dir)
	return p.exec(args)
}

func (p *testProjectDir) exec(args []string) runResult {
	var stdout, stderr strings.Builder
	a := &app{
		version:     "test",
		stdin:       strings.NewReader(p.stdin),
		stdout:      &stdout,
		stderr:      &stderr,
		interactive: p.interactive,
		getenv:      func(k string) string { return p.env[k] },
		sleep: func(_ context.Context, d time.Duration) error {
			p.slept = append(p.slept, d)
			return p.sleepErr
		},
	}
	code := a.run(context.Background(), args)
	return runResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// requests are the requests the fake got, as "METHOD path".
func (p *testProjectDir) requests() []string {
	p.api.mu.Lock()
	defer p.api.mu.Unlock()
	out := []string{}
	for _, r := range p.api.requests {
		out = append(out, strings.TrimSpace(r.method+" "+r.path))
	}
	return out
}

// requestsTo are the recorded requests to one route.
func (p *testProjectDir) requestsTo(method, path string) []fakeRequest {
	p.api.mu.Lock()
	defer p.api.mu.Unlock()
	var out []fakeRequest
	for _, r := range p.api.requests {
		if r.method == method && r.path == path {
			out = append(out, r)
		}
	}
	return out
}
