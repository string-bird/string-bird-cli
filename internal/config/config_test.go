package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    Config
		wantErr string
	}{
		{
			name: "full",
			yaml: `org_id: org-1
project_id: proj-1
files: src/locales/{locale}.json
format: i18next
structure: nested
`,
			want: Config{
				OrgID: "org-1", ProjectID: "proj-1",
				Files: "src/locales/{locale}.json", Format: "i18next", Structure: "nested",
			},
		},
		{
			name: "minimal",
			yaml: "org_id: o\nproject_id: p\nfiles: '{locale}.json'\n",
			want: Config{OrgID: "o", ProjectID: "p", Files: "{locale}.json"},
		},
		{
			name:    "no api url: sb always talks to production",
			yaml:    "api_url: https://api.example.com\norg_id: o\nproject_id: p\nfiles: '{locale}.json'\n",
			wantErr: "field api_url not found",
		},
		{
			name: "empty: flags may fill it in",
			yaml: "",
			want: Config{},
		},
		{
			name:    "api key",
			yaml:    "org_id: o\nproject_id: p\nfiles: '{locale}.json'\napi_key: sb_x\n",
			wantErr: "api_key doesn't belong in the config file",
		},
		{
			name:    "unknown field",
			yaml:    "org_id: o\nproject_id: p\nfiles: '{locale}.json'\nformt: json\n",
			wantErr: "field formt not found",
		},
		{
			name:    "unquoted placeholder first",
			yaml:    "org_id: o\nproject_id: p\nfiles: {locale}/app.json\n",
			wantErr: `a value starting with { needs quotes: files: "{locale}/app.json"`,
		},
		{
			name: "quoted placeholder first",
			yaml: "org_id: o\nproject_id: p\nfiles: \"{locale}/app.json\"\n",
			want: Config{OrgID: "o", ProjectID: "p", Files: "{locale}/app.json"},
		},
		{
			name:    "not yaml",
			yaml:    "org_id: [",
			wantErr: "parse config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.yaml))

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		wantErr     string
		wantMissing []string
	}{
		{name: "complete", cfg: Config{OrgID: "o", ProjectID: "p", Files: "{locale}.json"}},
		{name: "empty", cfg: Config{}, wantErr: "missing org_id, project_id, files", wantMissing: []string{"org_id", "project_id", "files"}},
		{name: "blank project", cfg: Config{OrgID: "o", ProjectID: "  ", Files: "{locale}.json"}, wantErr: "missing project_id", wantMissing: []string{"project_id"}},
		{name: "no placeholder", cfg: Config{OrgID: "o", ProjectID: "p", Files: "en.json"}, wantErr: `files "en.json" needs a {locale} placeholder`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr)
			var missing *MissingError
			if tt.wantMissing != nil {
				require.ErrorAs(t, err, &missing)
				assert.Equal(t, tt.wantMissing, missing.Fields)
			} else {
				assert.False(t, errors.As(err, &missing))
			}
		})
	}
}

func TestEncode(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		note string
		want string
	}{
		{
			name: "everything",
			cfg: Config{
				OrgID: "01a1b100-0000-7000-8000-00000000000a", ProjectID: "01a1b100-0000-7000-8000-00000000000b",
				Files: "{locale}/app.json", Format: "i18next", Structure: "nested",
			},
			note: "Web app (web)",
			want: `org_id: 01a1b100-0000-7000-8000-00000000000a
project_id: 01a1b100-0000-7000-8000-00000000000b # Web app (web)
files: "{locale}/app.json"
format: i18next
structure: nested
`,
		},
		{
			name: "plain path, no note or optional fields",
			cfg:  Config{OrgID: "o", ProjectID: "p", Files: "src/locales/{locale}.json"},
			want: "org_id: o\nproject_id: p\nfiles: \"src/locales/{locale}.json\"\n",
		},
		{
			name: "values YAML would misread",
			cfg:  Config{OrgID: "true", ProjectID: "a: b", Files: "l/{locale}.json # x"},
			want: "org_id: \"true\"\nproject_id: \"a: b\"\nfiles: \"l/{locale}.json # x\"\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := tt.cfg.Encode(tt.note)

			assert.True(t, strings.HasPrefix(string(out), "# String Bird project config"))
			assert.True(t, strings.HasSuffix(string(out), tt.want), string(out))
			back, err := Parse(out)
			require.NoError(t, err)
			assert.Equal(t, tt.cfg, back, "round trip")
		})
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultPath)
	require.NoError(t, os.WriteFile(path, []byte("org_id: o\nproject_id: p\nfiles: x/{locale}/{locale}.json\n"), 0o644))

	cfg, err := Load(path)

	require.NoError(t, err)
	assert.Equal(t, "x/de-AT/de-AT.json", cfg.FilePath("de-AT"))

	_, err = Load(filepath.Join(dir, "missing.yaml"))
	assert.ErrorContains(t, err, "read config")
}
