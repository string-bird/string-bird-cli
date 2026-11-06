package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/string-bird/string-bird-cli/internal/api"
)

// key builds a fake key; each translation is "value" or "value|unapproved",
// "value|outdated" or both.
func key(name string, translations map[string]string) api.Key {
	k := api.Key{ID: "id-" + name, Key: name, Translations: map[string]api.Translation{}}
	for locale, spec := range translations {
		t := api.Translation{Locale: locale, Value: spec, Approved: true}
		for {
			var flag string
			before, after, found := cutLast(t.Value, "|")
			if !found {
				break
			}
			t.Value, flag = before, after
			switch flag {
			case "unapproved":
				t.Approved, t.Origin = false, "machine"
			case "outdated":
				t.Outdated = true
			}
		}
		k.Translations[locale] = t
	}
	return k
}

func cutLast(s, sep string) (string, string, bool) {
	for i := len(s) - len(sep); i >= 0; i-- {
		if s[i:i+len(sep)] == sep {
			return s[:i], s[i+len(sep):], true
		}
	}
	return s, "", false
}

// testKeys: de has 1 ready, 1 missing, 1 outdated and unapproved, 1 unapproved;
// fr has nothing.
func testKeys() []api.Key {
	return []api.Key{
		key("auth.login", map[string]string{"en": "Log in", "de": "Anmelden"}),
		key("auth.logout", map[string]string{"en": "Log out"}),
		key("cart.empty", map[string]string{"en": "Your cart is empty", "de": "Leer|outdated|unapproved"}),
		key("cart.title", map[string]string{"en": "Cart", "de": "Warenkorb|unapproved"}),
	}
}

func TestProjectsList(t *testing.T) {
	fake := newFakeAPI()
	fake.projects = []api.Project{
		{ID: otherID, Name: "Mobile", Slug: "mobile", BaseLocale: "en"},
		{ID: testProject, Name: "Web", Slug: "web", BaseLocale: "en", Locales: []string{"de", "en"}},
	}
	p := newTestProject(t, fake, "")

	res := p.run("projects", "list")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Regexp(t, `(?m)^\s+mobile\s+Mobile\s+en\s+0\s+`+otherID, res.stdout)
	assert.Regexp(t, `(?m)^\*\s+web\s+Web\s+en\s+2\s+`+testProject, res.stdout)

	res = p.run("projects", "list", "--json")
	require.Equal(t, 0, res.code, res.stderr)
	var got []api.Project
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &got))
	assert.Len(t, got, 2)

	fake.projects = []api.Project{}
	res = p.run("projects", "list")
	require.Equal(t, 0, res.code)
	assert.Contains(t, res.stderr, "can't see any project")
}

func TestProjectsListNeedsOnlyTheOrg(t *testing.T) {
	p := newTestProject(t, newFakeAPI(), "")
	require.NoError(t, os.Remove(filepath.Join(p.dir, "stringbird.yaml")))

	res := p.runHere("projects", "list", "--org", testOrg)

	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, "web")
}

func TestLocales(t *testing.T) {
	fake := newFakeAPI()
	p := newTestProject(t, fake, "")

	res := p.run("locales", "list")
	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, "de\nen (base)\nfr\n", res.stdout)

	res = p.run("locales", "list", "--json")
	require.Equal(t, 0, res.code, res.stderr)
	assert.JSONEq(t, `[{"code":"de","base":false},{"code":"en","base":true},{"code":"fr","base":false}]`, res.stdout)

	res = p.run("locales", "add", "de-at", "it")
	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, "added de-AT\nadded it\n", res.stdout)

	res = p.run("locales", "add", "pl", "de")
	assert.Equal(t, 1, res.code)
	assert.Equal(t, "added pl\n", res.stdout)
	assert.Contains(t, res.stderr, "add de: locale_exists")

	res = p.run("locales", "add")
	assert.Equal(t, 1, res.code)
	assert.Contains(t, res.stderr, "requires at least 1 arg(s)")
}

func TestKeysList(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		pageSize   int
		wantCode   int
		wantQuery  map[string]string
		wantPages  int
		wantRows   []string
		wantAbsent []string
		wantStderr string
	}{
		{
			name:      "every key with the base text",
			args:      []string{"keys", "list"},
			wantQuery: map[string]string{"limit": "500"},
			wantPages: 1,
			wantRows:  []string{`(?m)^KEY\s+EN$`, `(?m)^auth\.login\s+Log in$`, `(?m)^cart\.title\s+Cart$`},
		},
		{
			name:       "missing in a locale",
			args:       []string{"keys", "list", "--missing", "de"},
			wantQuery:  map[string]string{"limit": "500", "locale": "de", "status": "missing"},
			wantPages:  1,
			wantRows:   []string{`(?m)^KEY\s+EN\s+DE$`, `(?m)^auth\.logout\s+Log out\s+-$`},
			wantAbsent: []string{"auth.login "},
			wantStderr: "1 keys",
		},
		{
			name:      "unapproved, marked",
			args:      []string{"keys", "list", "--unapproved", "de"},
			wantQuery: map[string]string{"limit": "500", "locale": "de", "status": "unapproved"},
			wantPages: 1,
			wantRows:  []string{`(?m)^cart\.empty\s+Your cart is empty\s+Leer \(outdated\)$`, `(?m)^cart\.title\s+Cart\s+Warenkorb \(unapproved\)$`},
		},
		{
			name:      "prefix and a locale column",
			args:      []string{"keys", "list", "--prefix", "auth.", "-l", "de"},
			wantQuery: map[string]string{"limit": "500", "locale": "de", "prefix": "auth."},
			wantPages: 1,
			wantRows:  []string{`(?m)^auth\.login\s+Log in\s+Anmelden$`, `(?m)^auth\.logout\s+Log out\s+-$`},
		},
		{
			name:       "follows every page",
			args:       []string{"keys", "list"},
			pageSize:   1,
			wantQuery:  map[string]string{"limit": "500", "cursor": "3"},
			wantPages:  4,
			wantStderr: "4 keys",
		},
		{
			name:       "limit stops early",
			args:       []string{"keys", "list", "--limit", "2"},
			pageSize:   1,
			wantQuery:  map[string]string{"limit": "1", "cursor": "1"},
			wantPages:  2,
			wantStderr: "2 keys",
		},
		{
			name:       "nothing matches",
			args:       []string{"keys", "list", "--prefix", "zzz"},
			wantQuery:  map[string]string{"limit": "500", "prefix": "zzz"},
			wantPages:  1,
			wantStderr: "No keys match.",
		},
		{
			name:       "one filter at a time",
			args:       []string{"keys", "list", "--missing", "de", "--outdated", "fr"},
			wantCode:   1,
			wantStderr: "[missing outdated] were all set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.keys, fake.pageSize = testKeys(), tt.pageSize
			p := newTestProject(t, fake, "")

			res := p.run(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stderr: %s", res.stderr)
			pages := p.requestsTo(http.MethodGet, "/keys")
			require.Len(t, pages, tt.wantPages)
			if tt.wantPages > 0 {
				assert.Equal(t, tt.wantQuery, pages[len(pages)-1].query)
			}
			for _, row := range tt.wantRows {
				assert.Regexp(t, row, res.stdout)
			}
			for _, s := range tt.wantAbsent {
				assert.NotContains(t, res.stdout, s)
			}
			assert.Contains(t, res.stderr, tt.wantStderr)
		})
	}
}

func TestKeysListJSON(t *testing.T) {
	fake := newFakeAPI()
	fake.keys = testKeys()
	p := newTestProject(t, fake, "")

	res := p.run("keys", "list", "--missing", "fr", "--json")

	require.Equal(t, 0, res.code, res.stderr)
	var got []api.Key
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &got))
	assert.Len(t, got, 4)
	assert.Equal(t, "Anmelden", got[0].Translations["de"].Value)

	res = p.run("keys", "list", "--prefix", "none", "--json")
	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, "[]\n", res.stdout)
}

func TestStatus(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantRows   []string
		wantStderr string
	}{
		{
			name: "every locale but the base",
			args: []string{"status"},
			wantRows: []string{
				`(?m)^LOCALE\s+READY\s+MISSING\s+OUTDATED\s+UNAPPROVED$`,
				`(?m)^de\s+25%\s+1\s+1\s+2$`,
				`(?m)^fr\s+0%\s+4\s+0\s+0$`,
			},
			wantStderr: "4 keys, base locale en",
		},
		{
			name:       "below the minimum",
			args:       []string{"status", "--min", "20"},
			wantCode:   1,
			wantStderr: "below 20% ready: fr (0%)",
		},
		{
			name:     "one locale above the minimum",
			args:     []string{"status", "-l", "de", "--min", "25"},
			wantRows: []string{`(?m)^de\s+25%`},
		},
		{
			name:       "minimum out of range",
			args:       []string{"status", "--min", "101"},
			wantCode:   1,
			wantStderr: "--min is a percentage from 0 to 100",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.keys = testKeys()
			p := newTestProject(t, fake, "")

			res := p.run(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stderr: %s", res.stderr)
			for _, row := range tt.wantRows {
				assert.Regexp(t, row, res.stdout)
			}
			assert.Contains(t, res.stderr, tt.wantStderr)
		})
	}
}

func TestStatusJSON(t *testing.T) {
	fake := newFakeAPI()
	fake.keys = testKeys()
	p := newTestProject(t, fake, "")

	res := p.run("status", "-l", "de", "--json")

	require.Equal(t, 0, res.code, res.stderr)
	assert.JSONEq(t, `[{"locale":"de","total":4,"missing":1,"outdated":1,"unapproved":2,"ready":1,"ready_percent":25}]`, res.stdout)
}

func TestFormatPercent(t *testing.T) {
	for in, want := range map[float64]string{0: "0%", 25: "25%", 33.333: "33.3%", 99.96: "99.9%", 100: "100%"} {
		assert.Equal(t, want, formatPercent(in), in)
	}
}

func TestTranslate(t *testing.T) {
	cs := "cs-1"
	running := api.Job{ID: "job-1", Status: api.JobRunning, Total: 3, Done: 1, Locales: []string{"de", "fr"}}
	tests := []struct {
		name       string
		args       []string
		steps      []api.Job
		fail       map[string]fakeError
		wantCode   int
		wantBody   string
		wantStdout []string
		wantStderr string
		wantPolls  int
	}{
		{
			name:       "start and return",
			args:       []string{"translate"},
			wantBody:   `{}`,
			wantStdout: []string{"started run job-1: 3 strings (de, fr)"},
		},
		{
			name:       "scope and locales",
			args:       []string{"translate", "--scope", "needed", "-l", "de", "-l", "fr"},
			wantBody:   `{"scope":"needed","locales":["de","fr"]}`,
			wantStdout: []string{"started run job-1"},
		},
		{
			name: "wait until done",
			args: []string{"translate", "--wait"},
			steps: []api.Job{
				running,
				{ID: "job-1", Status: api.JobCompleted, Total: 3, Done: 3, ChangeSetID: &cs},
			},
			wantBody:   `{}`,
			wantStdout: []string{"completed: 3 translated, 0 failed", "Review and approve: sb approve --run job-1"},
			wantStderr: "  1/3",
			wantPolls:  2,
		},
		{
			name: "some strings failed",
			args: []string{"translate", "--wait"},
			steps: []api.Job{{ID: "job-1", Status: api.JobCompleted, Total: 3, Done: 2, Failed: 1, ChangeSetID: &cs,
				Failures: []api.JobFailure{{Key: "cart.items", Locale: "fr", Reason: "plural forms missing"}}}},
			wantCode:   1,
			wantBody:   `{}`,
			wantStdout: []string{"completed: 2 translated, 1 failed", "  fr cart.items: plural forms missing"},
			wantStderr: "run job-1: 1 strings couldn't be translated",
			wantPolls:  1,
		},
		{
			name:       "run failed",
			args:       []string{"translate", "--wait"},
			steps:      []api.Job{{ID: "job-1", Status: api.JobFailed, Total: 3, Error: "mt_budget_exhausted"}},
			wantCode:   1,
			wantBody:   `{}`,
			wantStderr: "run job-1 failed: mt_budget_exhausted",
			wantPolls:  1,
		},
		{
			name: "nothing to translate is fine",
			args: []string{"translate", "--wait"},
			fail: map[string]fakeError{"POST /machine-translations": {
				status: http.StatusUnprocessableEntity, code: "nothing_to_translate",
			}},
			wantBody:   `{}`,
			wantStderr: "Nothing to translate.",
		},
		{
			name: "machine translation off",
			args: []string{"translate"},
			fail: map[string]fakeError{"POST /machine-translations": {
				status: http.StatusForbidden, code: "mt_disabled",
			}},
			wantCode:   1,
			wantBody:   `{}`,
			wantStderr: "start machine translation: mt_disabled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.job = api.Job{ID: "job-1", Status: api.JobQueued, Total: 3, Locales: []string{"de", "fr"}}
			fake.jobSteps = tt.steps
			if tt.fail != nil {
				fake.fail = tt.fail
			}
			p := newTestProject(t, fake, "")

			res := p.run(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stdout: %s\nstderr: %s", res.stdout, res.stderr)
			starts := p.requestsTo(http.MethodPost, "/machine-translations")
			require.Len(t, starts, 1)
			assert.JSONEq(t, tt.wantBody, starts[0].body)
			assert.Len(t, p.requestsTo(http.MethodGet, "/machine-translations/job-1"), tt.wantPolls)
			for _, s := range tt.wantStdout {
				assert.Contains(t, res.stdout, s)
			}
			assert.Contains(t, res.stderr, tt.wantStderr)
		})
	}
}

func TestTranslateJSON(t *testing.T) {
	fake := newFakeAPI()
	fake.job = api.Job{ID: "job-1", Status: api.JobQueued, Total: 3}
	fake.jobSteps = []api.Job{{ID: "job-1", Status: api.JobCompleted, Total: 3, Done: 3}}
	p := newTestProject(t, fake, "")

	res := p.run("translate", "--wait", "--json")

	require.Equal(t, 0, res.code, res.stderr)
	var job api.Job
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &job))
	assert.Equal(t, api.JobCompleted, job.Status)
}

func TestApprove(t *testing.T) {
	cs := "cs-9"
	tests := []struct {
		name       string
		args       []string
		job        api.Job
		wantCode   int
		wantBody   string
		wantStdout string
		wantStderr string
	}{
		{
			name:       "by locale and prefix",
			args:       []string{"approve", "-l", "de", "--prefix", "cart."},
			wantBody:   `{"locale":"de","prefix":"cart."}`,
			wantStdout: "approved 5, skipped 1 outdated\n",
		},
		{
			name:       "by run",
			args:       []string{"approve", "--run", "job-1"},
			job:        api.Job{ID: "job-1", Status: api.JobCompleted, ChangeSetID: &cs},
			wantBody:   `{"change_set_id":"cs-9"}`,
			wantStdout: "approved 5, skipped 1 outdated\n",
		},
		{
			name:       "a run that saved nothing",
			args:       []string{"approve", "--run", "job-1"},
			job:        api.Job{ID: "job-1", Status: api.JobCompleted},
			wantStdout: "approved 0: the run saved nothing\n",
		},
		{
			name:       "unknown run",
			args:       []string{"approve", "--run", "job-404"},
			job:        api.Job{ID: "job-1"},
			wantCode:   1,
			wantStderr: "get run: mt_job_not_found",
		},
		{
			name:       "needs a locale or a run",
			args:       []string{"approve", "--prefix", "cart."},
			wantCode:   1,
			wantStderr: "at least one of the flags in the group [locale run] is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.job = tt.job
			fake.approved = api.ApproveResult{Approved: 5, SkippedOutdated: 1}
			p := newTestProject(t, fake, "")

			res := p.run(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stderr: %s", res.stderr)
			approvals := p.requestsTo(http.MethodPost, "/translations/approve")
			if tt.wantBody == "" {
				assert.Empty(t, approvals)
			} else {
				require.Len(t, approvals, 1)
				assert.JSONEq(t, tt.wantBody, approvals[0].body)
			}
			assert.Equal(t, tt.wantStdout, res.stdout)
			assert.Contains(t, res.stderr, tt.wantStderr)
		})
	}
}

func TestReleaseList(t *testing.T) {
	fake := newFakeAPI()
	for range 3 {
		fake.addRelease(api.ReleasePublished)
	}
	fake.releases[2].Note = "Spring sale"
	fake.envs = map[string]string{"production": relID(2), "staging": relID(3), "preview": relID(3), "qa": ""}
	p := newTestProject(t, fake, "")

	res := p.run("release", "list")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Regexp(t, `(?m)^VERSION\s+STATUS\s+KEYS\s+LOCALES\s+CREATED\s+LIVE IN\s+NOTE$`, res.stdout)
	assert.Regexp(t, `(?m)^v3\s+published\s+120\s+2\s+\S+ \S+\s+preview, staging\s+Spring sale$`, res.stdout)
	assert.Regexp(t, `(?m)^v2\s+.*\s+production\s+-$`, res.stdout)
	assert.Regexp(t, `(?m)^v1\s+.*\s+-\s+-$`, res.stdout)
	assert.Equal(t, map[string]string{"limit": "20"}, p.requestsTo(http.MethodGet, "/releases")[0].query)

	res = p.run("release", "list", "--json", "--limit", "2")
	require.Equal(t, 0, res.code, res.stderr)
	var got []api.Release
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &got))
	assert.Len(t, got, 2)

	res = p.run("release", "list", "--limit", "500")
	assert.Equal(t, 1, res.code)
	assert.Contains(t, res.stderr, "--limit is from 1 to 200")
}

func TestReleaseListEmpty(t *testing.T) {
	p := newTestProject(t, newFakeAPI(), "")

	res := p.run("release", "list")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stderr, "No releases yet")
}

func TestReleaseDiff(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		diff        api.Diff
		wantCode    int
		wantDiffOf  string
		wantAgainst map[string]string
		wantStdout  string
		wantStderr  string
	}{
		{
			name: "against the previous release",
			args: []string{"release", "diff", "v3"},
			diff: api.Diff{From: &api.Release{Version: 2}, To: api.Release{Version: 3}, Locales: []api.LocaleDiff{{
				Locale:  "de",
				Added:   []api.DiffValue{{Key: "cart.title", Value: "Warenkorb"}},
				Changed: []api.DiffChanged{{Key: "auth.login", OldValue: "Anmelden", NewValue: "Einloggen"}},
				Removed: []api.DiffValue{{Key: "home.old", Value: "Alt"}},
			}}},
			wantDiffOf:  relID(3),
			wantAgainst: map[string]string{},
			wantStdout: "v3 against v2\n" +
				"de: 1 added, 1 changed, 1 removed\n" +
				"  + cart.title: \"Warenkorb\"\n" +
				"  ~ auth.login: \"Anmelden\" → \"Einloggen\"\n" +
				"  - home.old\n",
		},
		{
			name:        "environments, counts only",
			args:        []string{"release", "diff", "staging", "--against", "production", "--stat"},
			diff:        api.Diff{From: &api.Release{Version: 2}, To: api.Release{Version: 3}, Locales: []api.LocaleDiff{{Locale: "fr", Added: []api.DiffValue{{Key: "a"}, {Key: "b"}}}}},
			wantDiffOf:  relID(3),
			wantAgainst: map[string]string{"against": relID(2)},
			wantStdout:  "v3 against v2\nfr: 2 added, 0 changed, 0 removed\n",
		},
		{
			name:        "first release, no changes",
			args:        []string{"release", "diff", "1"},
			diff:        api.Diff{To: api.Release{Version: 1}},
			wantDiffOf:  relID(1),
			wantAgainst: map[string]string{},
			wantStdout:  "v1 against nothing\nno changes\n",
		},
		{
			name:       "unknown environment",
			args:       []string{"release", "diff", "qa"},
			wantCode:   1,
			wantStderr: "environment qa not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			for range 3 {
				fake.addRelease(api.ReleasePublished)
			}
			fake.envs = map[string]string{"production": relID(2), "staging": relID(3)}
			fake.diff = tt.diff
			p := newTestProject(t, fake, "")

			res := p.run(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stderr: %s", res.stderr)
			assert.Equal(t, tt.wantStdout, res.stdout)
			assert.Contains(t, res.stderr, tt.wantStderr)
			if tt.wantDiffOf != "" {
				diffs := p.requestsTo(http.MethodGet, "/releases/"+tt.wantDiffOf+"/diff")
				require.Len(t, diffs, 1)
				assert.Equal(t, tt.wantAgainst, diffs[0].query)
			}
		})
	}
}

func TestEnv(t *testing.T) {
	fake := newFakeAPI()
	fake.addRelease(api.ReleasePublished)
	fake.envs = map[string]string{"production": relID(1)}
	p := newTestProject(t, fake, "")

	res := p.run("env", "create", "staging")
	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, "created staging\n", res.stdout)

	res = p.run("env", "list")
	require.Equal(t, 0, res.code, res.stderr)
	assert.Regexp(t, `(?m)^NAME\s+RELEASE\s+STATUS\s+UPDATED$`, res.stdout)
	assert.Regexp(t, `(?m)^production\s+v1\s+published\s+`, res.stdout)
	assert.Regexp(t, `(?m)^staging\s+-\s+-\s+`, res.stdout)

	res = p.run("env", "create", "staging")
	assert.Equal(t, 1, res.code)
	assert.Contains(t, res.stderr, "create staging: environment_exists")

	res = p.run("env", "list", "--json")
	require.Equal(t, 0, res.code, res.stderr)
	var envs []api.Environment
	require.NoError(t, json.Unmarshal([]byte(res.stdout), &envs))
	assert.Len(t, envs, 2)
}
