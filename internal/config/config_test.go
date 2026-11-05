package config

import (
	"os"
	"path/filepath"
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
			yaml: `api_url: https://api.example.com
org_id: org-1
project_id: proj-1
files: src/locales/{locale}.json
format: i18next
structure: nested
`,
			want: Config{
				APIURL: "https://api.example.com", OrgID: "org-1", ProjectID: "proj-1",
				Files: "src/locales/{locale}.json", Format: "i18next", Structure: "nested",
			},
		},
		{
			name: "api url may come from elsewhere",
			yaml: "org_id: o\nproject_id: p\nfiles: '{locale}.json'\n",
			want: Config{OrgID: "o", ProjectID: "p", Files: "{locale}.json"},
		},
		{
			name:    "empty",
			yaml:    "",
			wantErr: "missing org_id, project_id, files",
		},
		{
			name:    "no placeholder",
			yaml:    "org_id: o\nproject_id: p\nfiles: en.json\n",
			wantErr: `files "en.json" needs a {locale} placeholder`,
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
