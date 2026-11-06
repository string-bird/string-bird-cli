package cli

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/string-bird/string-bird-cli/internal/api"
)

// TestFlagRules covers flags cobra rejects before any request.
func TestFlagRules(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{"push translations with locales", []string{"push", "--translations", "--locale", "de"}, "[locale translations] were all set"},
		{"promote with both sources", []string{"release", "promote", "production", "--from", "staging", "--release", "v1"}, "[from release] were all set"},
		{"promote without an environment", []string{"release", "promote", "--from", "staging"}, "accepts 1 arg(s), received 0"},
		{"push takes no arguments", []string{"push", "de"}, `unknown command "de" for "sb push"`},
		{"unknown command", []string{"sync"}, `unknown command "sync" for "sb"`},
		{"unknown flag", []string{"pull", "--force"}, "unknown flag: --force"},
		{"bad timeout", []string{"release", "create", "--wait", "--timeout", "soon"}, `invalid argument "soon" for "--timeout"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestProject(t, newFakeAPI(), "")

			res := p.run(tt.args...)

			assert.Equal(t, 1, res.code)
			assert.Contains(t, res.stderr, tt.wantStderr)
			assert.Empty(t, p.requests())
		})
	}
}

func TestReleaseCreateWaitTimesOut(t *testing.T) {
	fake := newFakeAPI()
	fake.publishAfter = 1000
	p := newTestProject(t, fake, "")
	p.sleepErr = context.DeadlineExceeded

	res := p.run("release", "create", "--wait", "--timeout", "30s")

	require.Equal(t, 1, res.code)
	assert.Contains(t, res.stdout, "created v1")
	assert.Contains(t, res.stderr, "v1 still pending after 30s")
	assert.Empty(t, p.requestsTo(http.MethodGet, "/releases/"+relID(1)))
}

func TestReleaseCreateWaitAlreadyPublished(t *testing.T) {
	fake := newFakeAPI()
	p := newTestProject(t, fake, "")
	fake.addRelease(api.ReleasePublished) // v1; the new one is v2
	fake.publishAs = api.ReleasePublished

	res := p.run("release", "create", "--wait")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Contains(t, res.stdout, "created v2")
	assert.Contains(t, res.stdout, "published v2")
}

func TestFilePaths(t *testing.T) {
	tests := []struct {
		name  string
		files string
		want  map[string]string
	}{
		{
			name:  "folder per locale",
			files: "src/i18n/{locale}/messages.json",
			want:  map[string]string{"src/i18n/de/messages.json": "de", "src/i18n/en/messages.json": "en"},
		},
		{
			name:  "locale twice",
			files: `"{locale}/app.{locale}.json"`,
			want:  map[string]string{"de/app.de.json": "de", "en/app.en.json": "en"},
		},
		{
			name:  "parent folder",
			files: "../shared/{locale}.json",
			want:  map[string]string{"../shared/de.json": "de", "../shared/en.json": "en"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.project.Locales = []string{"de", "en"}
			fake.exports = map[string]fakeExport{"de": {body: "de"}, "en": {body: "en"}}
			p := newTestProject(t, fake, "")
			p.write("stringbird.yaml", "org_id: "+testOrg+"\nproject_id: "+testProject+"\nfiles: "+tt.files+"\n")

			pulled := p.run("pull")
			require.Equal(t, 0, pulled.code, pulled.stderr)
			for path, content := range tt.want {
				assert.Equal(t, content, p.read(path))
			}

			pushed := p.run("push", "--translations")
			require.Equal(t, 0, pushed.code, pushed.stderr)
			var bodies []string
			for _, r := range p.requestsTo(http.MethodPost, "/import") {
				bodies = append(bodies, r.query["locale"]+"="+r.body)
			}
			assert.Equal(t, []string{"en=en", "de=de"}, bodies)
		})
	}
}

func TestPushBaseLocaleIgnoresCase(t *testing.T) {
	fake := newFakeAPI()
	fake.project.BaseLocale = "en-US"
	fake.project.Locales = []string{"de", "en-US"}
	p := newTestProject(t, fake, "")
	p.write("locales/en-us.json", "a")
	p.write("locales/de.json", "b")

	res := p.run("push", "--locale", "de", "--locale", "en-us", "--delete-missing")

	require.Equal(t, 0, res.code, res.stderr)
	imports := p.requestsTo(http.MethodPost, "/import")
	require.Len(t, imports, 2)
	assert.Equal(t, map[string]string{"locale": "en-us", "delete_missing": "true"}, imports[0].query)
	assert.Equal(t, map[string]string{"locale": "de"}, imports[1].query)
}

func TestSummarize(t *testing.T) {
	tests := []struct {
		s    api.ImportSummary
		want string
	}{
		{api.ImportSummary{}, "empty file"},
		{api.ImportSummary{AddedKeys: 1, Created: 1, DeletedKeys: 1}, "1 key added, 1 translation created, 1 key deleted"},
		{api.ImportSummary{AddedKeys: 2, Created: 3, DeletedKeys: 4}, "2 keys added, 3 translations created, 4 keys deleted"},
		{api.ImportSummary{Updated: 1, SkippedExisting: 2, Unchanged: 3}, "1 updated, 2 skipped as existing, 3 unchanged"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, summarize(tt.s))
		})
	}
}

func TestPullStopsAtFirstError(t *testing.T) {
	fake := newFakeAPI()
	fake.exports = map[string]fakeExport{"de": {body: "de"}}
	p := newTestProject(t, fake, "")

	res := p.run("pull")

	require.Equal(t, 1, res.code)
	assert.Equal(t, "de", p.read("locales/de.json"))
	assert.Contains(t, res.stderr, "pull en: locale_not_found")
	assert.Len(t, p.requestsTo(http.MethodGet, "/export"), 2)
}

func TestPullWithoutLocales(t *testing.T) {
	fake := newFakeAPI()
	fake.project.Locales = nil
	p := newTestProject(t, fake, "")

	res := p.run("pull")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Empty(t, p.requestsTo(http.MethodGet, "/export"))
}
