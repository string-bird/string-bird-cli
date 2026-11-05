// Package config reads the project config file, stringbird.yaml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// DefaultPath is where commands look for the config when --config isn't given.
const DefaultPath = "stringbird.yaml"

// LocalePlaceholder marks where the locale code goes in Files.
const LocalePlaceholder = "{locale}"

// Config is the project config. It never holds the API key: that comes from
// STRINGBIRD_API_KEY or --api-key, so the file can be committed.
type Config struct {
	APIURL    string `yaml:"api_url"`
	OrgID     string `yaml:"org_id"`
	ProjectID string `yaml:"project_id"`
	// Files is the path of the translation files relative to the config file,
	// with {locale} where the locale code goes: locales/{locale}.json.
	Files string `yaml:"files"`
	// Format is the file format the server converts from and to: json (ICU
	// MessageFormat values) or i18next. Empty means the server default, json.
	Format string `yaml:"format"`
	// Structure is the key layout of pulled files: flat or nested. Empty means
	// the server default for the format.
	Structure string `yaml:"structure"`

	// APIKey is only here to reject it with a helpful message.
	APIKey string `yaml:"api_key"`
}

// Load reads and checks the config at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	cfg, err := Parse(data)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Parse decodes and checks a config. Unknown fields are errors, so typos
// don't go unnoticed.
func Parse(data []byte) (Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.check(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) check() error {
	if c.APIKey != "" {
		return errors.New("api_key doesn't belong in the config file; set STRINGBIRD_API_KEY or pass --api-key")
	}
	var missing []string
	for _, f := range []struct{ name, value string }{
		{"org_id", c.OrgID},
		{"project_id", c.ProjectID},
		{"files", c.Files},
	} {
		if strings.TrimSpace(f.value) == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}
	if !strings.Contains(c.Files, LocalePlaceholder) {
		return fmt.Errorf("files %q needs a %s placeholder", c.Files, LocalePlaceholder)
	}
	return nil
}

// FilePath is the file of one locale.
func (c Config) FilePath(locale string) string {
	return strings.ReplaceAll(c.Files, LocalePlaceholder, locale)
}
