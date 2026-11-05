package cli

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPull(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		exports map[string]fakeExport
		args    []string
		// wantQueries are the export queries in order; wantFiles what's on disk.
		wantCode    int
		wantQueries []map[string]string
		wantFiles   map[string]string
		wantStdout  []string
		wantStderr  []string
	}{
		{
			name: "every project locale",
			exports: map[string]fakeExport{
				"de": {body: `{"hi":"Hallo"}`}, "en": {body: `{"hi":"Hi"}`}, "fr": {body: `{"hi":"Salut"}`},
			},
			args: []string{"pull"},
			wantQueries: []map[string]string{
				{"locale": "de"}, {"locale": "en"}, {"locale": "fr"},
			},
			wantFiles: map[string]string{
				"locales/de.json": `{"hi":"Hallo"}`, "locales/en.json": `{"hi":"Hi"}`, "locales/fr.json": `{"hi":"Salut"}`,
			},
			wantStdout: []string{"pulled de → ", "pulled fr → "},
		},
		{
			name:    "format, structure and options",
			config:  "format: i18next\nstructure: flat\n",
			exports: map[string]fakeExport{"de": {body: "{}", skipped: []string{"cart.count", "gender"}}},
			args:    []string{"pull", "--locale", "de", "--include-unapproved", "--fallback"},
			wantQueries: []map[string]string{{
				"locale": "de", "format": "i18next", "structure": "flat",
				"include_unapproved": "true", "fallback": "true",
			}},
			wantFiles:  map[string]string{"locales/de.json": "{}"},
			wantStderr: []string{"warning: de: 2 keys the format can't express were left out: cart.count, gender"},
		},
		{
			name:        "unknown locale",
			exports:     map[string]fakeExport{},
			args:        []string{"pull", "--locale", "xx"},
			wantCode:    1,
			wantQueries: []map[string]string{{"locale": "xx"}},
			wantStderr:  []string{"pull xx: locale_not_found"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.exports = tt.exports
			p := newTestProject(t, fake, tt.config)

			res := p.run(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stderr: %s", res.stderr)
			var got []map[string]string
			for _, r := range p.requestsTo(http.MethodGet, "/export") {
				got = append(got, r.query)
			}
			assert.Equal(t, tt.wantQueries, got)
			for path, content := range tt.wantFiles {
				assert.Equal(t, content, p.read(path), path)
			}
			for _, s := range tt.wantStdout {
				assert.Contains(t, res.stdout, s)
			}
			for _, s := range tt.wantStderr {
				assert.Contains(t, res.stderr, s)
			}
		})
	}
}

func TestPullReplacesFiles(t *testing.T) {
	fake := newFakeAPI()
	fake.exports = map[string]fakeExport{"de": {body: `{"new":"Neu"}`}}
	p := newTestProject(t, fake, "")
	p.write("locales/de.json", `{"old":"Alt"}`)

	res := p.run("pull", "-l", "de")

	require.Equal(t, 0, res.code, res.stderr)
	assert.Equal(t, `{"new":"Neu"}`, p.read("locales/de.json"))
}
