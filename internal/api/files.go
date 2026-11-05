package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// ImportOptions are the query options of POST /import.
type ImportOptions struct {
	Locale        string
	Format        string
	DryRun        bool
	SkipExisting  bool
	DeleteMissing bool
}

// ImportResult is the answer of an import or dry run.
type ImportResult struct {
	DryRun          bool          `json:"dry_run"`
	ChangeSetID     *string       `json:"change_set_id"`
	Summary         ImportSummary `json:"summary"`
	AddedKeys       []string      `json:"added_keys"`
	Created         []string      `json:"created"`
	Updated         []UpdatedKey  `json:"updated"`
	SkippedExisting []string      `json:"skipped_existing"`
	DeletedKeys     []string      `json:"deleted_keys"`
}

// ImportSummary counts what an import did, or would do.
type ImportSummary struct {
	AddedKeys       int `json:"added_keys"`
	Created         int `json:"created"`
	Updated         int `json:"updated"`
	Unchanged       int `json:"unchanged"`
	SkippedExisting int `json:"skipped_existing"`
	DeletedKeys     int `json:"deleted_keys"`
}

// UpdatedKey is a translation an import changed.
type UpdatedKey struct {
	Key      string `json:"key"`
	OldValue string `json:"old_value"`
	NewValue string `json:"new_value"`
}

// Import uploads one locale's file. The server parses and validates it; a bad
// entry rejects the whole file with invalid_import and its Problems.
func (c *Client) Import(ctx context.Context, file []byte, opts ImportOptions) (ImportResult, error) {
	q := url.Values{"locale": {opts.Locale}}
	if opts.Format != "" {
		q.Set("format", opts.Format)
	}
	setBool(q, "dry_run", opts.DryRun)
	setBool(q, "skip_existing", opts.SkipExisting)
	setBool(q, "delete_missing", opts.DeleteMissing)

	resp, err := c.do(ctx, http.MethodPost, c.projectPath("/import", q), file, "application/octet-stream")
	if err != nil {
		return ImportResult{}, err
	}
	defer resp.Body.Close()
	var out ImportResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ImportResult{}, fmt.Errorf("decode import response: %w", err)
	}
	return out, nil
}

// ExportOptions are the query options of GET /export.
type ExportOptions struct {
	Locale            string
	Format            string
	Structure         string
	IncludeUnapproved bool
	Fallback          bool
}

// Export is a downloaded locale file.
type Export struct {
	Body []byte
	// SkippedKeys are keys the format can't express (first 100).
	SkippedKeys []string
}

// Export downloads one locale's file.
func (c *Client) Export(ctx context.Context, opts ExportOptions) (Export, error) {
	q := url.Values{"locale": {opts.Locale}}
	if opts.Format != "" {
		q.Set("format", opts.Format)
	}
	if opts.Structure != "" {
		q.Set("structure", opts.Structure)
	}
	setBool(q, "include_unapproved", opts.IncludeUnapproved)
	setBool(q, "fallback", opts.Fallback)

	resp, err := c.do(ctx, http.MethodGet, c.projectPath("/export", q), nil, "")
	if err != nil {
		return Export{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Export{}, fmt.Errorf("read export of %s: %w", opts.Locale, err)
	}
	out := Export{Body: body}
	if skipped := resp.Header.Get("X-Skipped-Keys"); skipped != "" {
		if err := json.Unmarshal([]byte(skipped), &out.SkippedKeys); err != nil {
			return Export{}, fmt.Errorf("decode X-Skipped-Keys: %w", err)
		}
	}
	return out, nil
}

func setBool(q url.Values, name string, v bool) {
	if v {
		q.Set(name, strconv.FormatBool(v))
	}
}
