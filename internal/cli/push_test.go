package cli

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/string-bird/string-bird-cli/internal/api"
)

func TestPush(t *testing.T) {
	type importCall struct {
		locale string
		query  map[string]string
		body   string
	}
	tests := []struct {
		name   string
		config string
		files  map[string]string
		args   []string
		fail   map[string]fakeError
		// wantCode is the exit code; wantImports the import requests in order.
		wantCode    int
		wantImports []importCall
		wantStdout  []string
		wantStderr  []string
	}{
		{
			name:  "source only by default",
			files: map[string]string{"locales/en.json": "a\nb", "locales/de.json": "c"},
			args:  []string{"push"},
			wantImports: []importCall{
				{locale: "en", query: map[string]string{"locale": "en"}, body: "a\nb"},
			},
			wantStdout: []string{"pushed en (", "2 keys added, 4 unchanged"},
		},
		{
			name:   "format from the config",
			config: "format: i18next\n",
			files:  map[string]string{"locales/en.json": "a"},
			args:   []string{"push"},
			wantImports: []importCall{
				{locale: "en", query: map[string]string{"locale": "en", "format": "i18next"}, body: "a"},
			},
		},
		{
			name:  "translations after the source, missing files skipped",
			files: map[string]string{"locales/en.json": "a", "locales/de.json": "b"},
			args:  []string{"push", "--translations", "--skip-existing"},
			wantImports: []importCall{
				{locale: "en", query: map[string]string{"locale": "en", "skip_existing": "true"}, body: "a"},
				{locale: "de", query: map[string]string{"locale": "de", "skip_existing": "true"}, body: "b"},
			},
			wantStderr: []string{"skip fr:", "fr.json not found"},
		},
		{
			name:  "named locales, base first",
			files: map[string]string{"locales/en.json": "a", "locales/de.json": "b"},
			args:  []string{"push", "--locale", "de", "--locale", "en"},
			wantImports: []importCall{
				{locale: "en", query: map[string]string{"locale": "en"}, body: "a"},
				{locale: "de", query: map[string]string{"locale": "de"}, body: "b"},
			},
		},
		{
			name:       "named locale without a file",
			files:      map[string]string{"locales/en.json": "a"},
			args:       []string{"push", "--locale", "fr"},
			wantCode:   1,
			wantStderr: []string{"read fr file"},
		},
		{
			name:       "missing source file",
			args:       []string{"push"},
			wantCode:   1,
			wantStderr: []string{"read en file"},
		},
		{
			name:  "dry run lists the changes",
			files: map[string]string{"locales/en.json": "a"},
			args:  []string{"push", "--dry-run", "--delete-missing"},
			wantImports: []importCall{
				{locale: "en", query: map[string]string{"locale": "en", "dry_run": "true", "delete_missing": "true"}, body: "a"},
			},
			wantStdout: []string{"dry run en (", "  + en:a", `  ~ home.title: "Hi" → "Hello"`},
		},
		{
			name:  "delete missing only on the base locale",
			files: map[string]string{"locales/en.json": "a", "locales/de.json": "b", "locales/fr.json": "c"},
			args:  []string{"push", "--translations", "--delete-missing"},
			wantImports: []importCall{
				{locale: "en", query: map[string]string{"locale": "en", "delete_missing": "true"}, body: "a"},
				{locale: "de", query: map[string]string{"locale": "de"}, body: "b"},
				{locale: "fr", query: map[string]string{"locale": "fr"}, body: "c"},
			},
		},
		{
			name:       "delete missing without the base locale",
			files:      map[string]string{"locales/de.json": "b"},
			args:       []string{"push", "--locale", "de", "--delete-missing"},
			wantCode:   1,
			wantStderr: []string{"--delete-missing works on the base locale (en) only"},
		},
		{
			name:  "rejected file lists its problems",
			files: map[string]string{"locales/en.json": "a"},
			args:  []string{"push"},
			fail: map[string]fakeError{"POST /import": {
				status: http.StatusUnprocessableEntity, code: "invalid_import",
				problems: []api.Problem{{Key: "a.b", Reason: "next to a.b.c"}, {Key: "cart", Reason: "broken ICU at 4"}},
			}},
			wantCode:    1,
			wantImports: []importCall{{locale: "en", query: map[string]string{"locale": "en"}, body: "a"}},
			wantStderr:  []string{"  a.b: next to a.b.c\n  cart: broken ICU at 4\n", "push en (", "invalid_import: invalid import (HTTP 422)"},
		},
		{
			name:  "plan limit",
			files: map[string]string{"locales/en.json": "a"},
			args:  []string{"push"},
			fail: map[string]fakeError{"POST /import": {
				status: http.StatusForbidden, code: "plan_limit_reached",
			}},
			wantCode:    1,
			wantImports: []importCall{{locale: "en", query: map[string]string{"locale": "en"}, body: "a"}},
			wantStderr:  []string{"plan_limit_reached"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			if tt.fail != nil {
				fake.fail = tt.fail
			}
			p := newTestProject(t, fake, tt.config)
			for path, content := range tt.files {
				p.write(path, content)
			}

			res := p.run(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stderr: %s", res.stderr)
			var got []importCall
			for _, r := range p.requestsTo(http.MethodPost, "/import") {
				got = append(got, importCall{locale: r.query["locale"], query: r.query, body: r.body})
			}
			assert.Equal(t, tt.wantImports, got)
			for _, s := range tt.wantStdout {
				assert.Contains(t, res.stdout, s)
			}
			for _, s := range tt.wantStderr {
				assert.Contains(t, res.stderr, s)
			}
		})
	}
}
