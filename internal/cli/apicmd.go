package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/string-bird/string-bird-cli/internal/api"
	"github.com/string-bird/string-bird-cli/internal/config"
)

type apiOptions struct {
	method  string
	fields  []string
	typed   []string
	input   string
	include bool
}

func (a *app) apiCmd() *cobra.Command {
	var opts apiOptions
	cmd := &cobra.Command{
		Use:   "api <path>",
		Short: "Call any API route with your key",
		Long: `Send a request to the StringBird API and print the response.

A path starting with /v1/ is used as is; {org} and {project} in it are
replaced by your org and project ids. Any other path is relative to your
project: /keys means /v1/orgs/{org}/projects/{project}/keys.

The method is GET, or POST when fields or --input are given (like gh api);
-X sets another. Fields become query parameters for GET and DELETE and a JSON
body otherwise: -f sets a string, -F a JSON value (true, 42, null, ["a"]) or a
string when it isn't JSON. --input sends a file (or - for stdin) as the body.

The response is printed as is, JSON indented. An error status exits with 1.
Routes are in the API documentation; an API key reaches its organization and
its projects.

In Git Bash, leave out the leading slash (sb api keys): Git Bash turns
arguments starting with / into Windows paths.`,
		Example: `  sb api "/keys?status=missing&locale=de"
  sb api -X GET /keys -f prefix=auth.
  sb api /v1/orgs/{org}/projects
  sb api /locales -f locale=it
  sb api "/import?locale=de&dry_run=true" --input locales/de.json
  sb api -X PUT /environments/production -f release_id=0193...`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.api(cmd.Context(), args[0], opts)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&opts.method, "method", "X", "", "HTTP method (default GET, or POST with fields or --input)")
	f.StringArrayVarP(&opts.fields, "field", "f", nil, "string field key=value (repeatable)")
	f.StringArrayVarP(&opts.typed, "typed-field", "F", nil, "JSON field key=value (repeatable)")
	f.StringVar(&opts.input, "input", "", "file to send as the body, - for stdin")
	f.BoolVarP(&opts.include, "include", "i", false, "print the status line and headers first")
	return cmd
}

func (a *app) api(ctx context.Context, path string, opts apiOptions) error {
	if windowsPath.MatchString(path) {
		return fmt.Errorf("%q is a Windows path: Git Bash rewrites arguments starting with /. "+
			"Leave out the leading slash (sb api locales) or set MSYS_NO_PATHCONV=1", path)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	fields, err := parseFields(opts.fields, opts.typed)
	if err != nil {
		return err
	}
	if opts.input != "" && len(fields) > 0 {
		return errors.New("--input and fields don't mix; put the fields in the query string")
	}
	method := strings.ToUpper(opts.method)
	if method == "" {
		method = http.MethodGet
		if len(fields) > 0 || opts.input != "" {
			method = http.MethodPost
		}
	}

	// Only a path that mentions the project needs one.
	relative := !strings.HasPrefix(path, "/v1/")
	needProject := relative || strings.Contains(path, "{project}")
	var client *api.Client
	var cfg config.Config
	if needProject {
		p, err := a.loadProject(ctx, config.Need{})
		if err != nil {
			return err
		}
		client, cfg = p.client, p.cfg
	} else if cfg, client, err = a.loadOrg(); err != nil {
		return err
	}
	if relative {
		path = "/v1/orgs/{org}/projects/{project}" + path
	}
	path = strings.NewReplacer("{org}", url.PathEscape(cfg.OrgID), "{project}", url.PathEscape(cfg.ProjectID)).Replace(path)

	var body []byte
	contentType := ""
	switch {
	case opts.input == "-":
		if body, err = io.ReadAll(a.stdin); err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		contentType = "application/octet-stream"
	case opts.input != "":
		if body, err = os.ReadFile(opts.input); err != nil {
			return fmt.Errorf("read --input: %w", err)
		}
		contentType = "application/octet-stream"
	case len(fields) > 0 && (method == http.MethodGet || method == http.MethodDelete):
		q := url.Values{}
		for k, v := range fields {
			q.Set(k, fieldString(v))
		}
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		path += sep + q.Encode()
	case len(fields) > 0:
		if body, err = json.Marshal(fields); err != nil {
			return fmt.Errorf("encode fields: %w", err)
		}
		contentType = "application/json"
	}

	resp, err := client.Raw(ctx, method, path, body, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if opts.include {
		fmt.Fprintf(a.stdout, "%s %s\n", resp.Proto, resp.Status)
		names := make([]string, 0, len(resp.Header))
		for name := range resp.Header {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(a.stdout, "%s: %s\n", name, strings.Join(resp.Header[name], ", "))
		}
		fmt.Fprintln(a.stdout)
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, data, "", "  ") == nil {
		pretty.WriteByte('\n')
		data = pretty.Bytes()
	}
	a.stdout.Write(data)
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotModified {
		return fmt.Errorf("%s %s: %s", method, resp.Request.URL.Path, resp.Status)
	}
	return nil
}

// windowsPath is what Git Bash turns /locales into: C:/Program Files/Git/locales.
var windowsPath = regexp.MustCompile(`^[A-Za-z]:[/\\]`)

// parseFields reads -f (strings) and -F (JSON values) key=value pairs.
func parseFields(strs, typed []string) (map[string]any, error) {
	fields := map[string]any{}
	for _, f := range strs {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("field %q isn't key=value", f)
		}
		fields[k] = v
	}
	for _, f := range typed {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("field %q isn't key=value", f)
		}
		var val any
		if err := json.Unmarshal([]byte(v), &val); err != nil {
			val = v
		}
		fields[k] = val
	}
	return fields, nil
}

// fieldString is a field as a query parameter.
func fieldString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
