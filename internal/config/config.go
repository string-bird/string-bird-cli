// Package config reads and writes the project config file, stringbird.yaml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
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
	OrgID string `yaml:"org_id"`
	// ProjectID is the project's id or its slug.
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

// Load reads the config at path. It doesn't check that every field is set:
// flags and environment variables can fill in the rest; call Validate after.
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

// unquotedBrace finds a value starting with an unquoted {, like {locale}.
var unquotedBrace = regexp.MustCompile(`(?m)^\s*\w+:\s*\{`)

// Parse decodes a config. Unknown fields are errors, so typos don't go
// unnoticed, and so is an API key.
func Parse(data []byte) (Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		if unquotedBrace.Match(data) {
			// files: {locale}/app.json reads as a YAML map.
			return Config{}, fmt.Errorf("parse config: %w (a value starting with { needs quotes: files: \"{locale}/app.json\")", err)
		}
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.APIKey != "" {
		return Config{}, errors.New("api_key doesn't belong in the config file; set STRINGBIRD_API_KEY or pass --api-key")
	}
	return cfg, nil
}

// Validate checks that the config can be used for what need says: those
// fields set, and a {locale} placeholder in Files. Missing fields are named by
// their YAML key.
func (c Config) Validate(need Need) error {
	var missing []string
	for _, f := range []struct {
		name, value string
		needed      bool
	}{
		{"org_id", c.OrgID, true},
		{"project_id", c.ProjectID, need.Project},
		{"files", c.Files, need.Files},
	} {
		if f.needed && strings.TrimSpace(f.value) == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return &MissingError{Fields: missing}
	}
	if need.Files && !strings.Contains(c.Files, LocalePlaceholder) {
		return fmt.Errorf("files %q needs a %s placeholder", c.Files, LocalePlaceholder)
	}
	return nil
}

// Need is what a command needs from the config besides org_id, which every
// command needs.
type Need struct {
	Project bool
	Files   bool
}

// All is what push and pull need: every field.
var All = Need{Project: true, Files: true}

// MissingError names the required fields a config lacks.
type MissingError struct {
	Fields []string
}

func (e *MissingError) Error() string {
	return "missing " + strings.Join(e.Fields, ", ")
}

// FilePath is the file of one locale.
func (c Config) FilePath(locale string) string {
	return strings.ReplaceAll(c.Files, LocalePlaceholder, locale)
}

// Encode renders the config as a commented stringbird.yaml. projectNote, when
// set, is written as a comment after the project id (its name, say).
func (c Config) Encode(projectNote string) []byte {
	var b strings.Builder
	b.WriteString("# String Bird project config: https://github.com/string-bird/string-bird-cli#configuration\n")
	b.WriteString("# The API key never goes here: set STRINGBIRD_API_KEY.\n")
	fmt.Fprintf(&b, "org_id: %s\n", quote(c.OrgID))
	fmt.Fprintf(&b, "project_id: %s", quote(c.ProjectID))
	if projectNote != "" {
		fmt.Fprintf(&b, " # %s", projectNote)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "files: %s\n", quote(c.Files))
	if c.Format != "" {
		fmt.Fprintf(&b, "format: %s\n", quote(c.Format))
	}
	if c.Structure != "" {
		fmt.Fprintf(&b, "structure: %s\n", quote(c.Structure))
	}
	return []byte(b.String())
}

// plainScalar is a value YAML reads back as the same string without quotes.
var plainScalar = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

func quote(s string) string {
	if plainScalar.MatchString(s) && s != "true" && s != "false" && s != "null" {
		return s
	}
	return strconv.Quote(s)
}
