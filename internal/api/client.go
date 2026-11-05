// Package api is a thin client for the String Bird REST API, covering what
// the CLI needs: projects, import, export, releases and environments.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxRetries is how often a rate-limited request is sent again.
const maxRetries = 3

// maxRetryWait caps the wait a Retry-After header can ask for.
const maxRetryWait = time.Minute

// Client calls one project's routes with an API key.
type Client struct {
	baseURL   string
	apiKey    string
	orgID     string
	projectID string
	userAgent string
	http      *http.Client
	// sleep waits between retries; tests replace it.
	sleep func(ctx context.Context, d time.Duration) error
}

// Options configures a Client.
type Options struct {
	BaseURL   string
	APIKey    string
	OrgID     string
	ProjectID string
	UserAgent string
	HTTP      *http.Client
}

// New returns a client for the project in opts.
func New(opts Options) *Client {
	hc := opts.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Minute}
	}
	return &Client{
		baseURL:   strings.TrimRight(opts.BaseURL, "/"),
		apiKey:    opts.APIKey,
		orgID:     opts.OrgID,
		projectID: opts.ProjectID,
		userAgent: opts.UserAgent,
		http:      hc,
		sleep:     sleepContext,
	}
}

// SetSleep replaces how the client waits before retrying. For tests.
func (c *Client) SetSleep(sleep func(ctx context.Context, d time.Duration) error) {
	c.sleep = sleep
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// projectPath is the URL of a route under the client's project.
func (c *Client) projectPath(path string, query url.Values) string {
	u := c.baseURL + "/v1/orgs/" + url.PathEscape(c.orgID) + "/projects/" + url.PathEscape(c.projectID) + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

// do sends a request, retrying when rate limited, and returns the response
// when its status is 2xx or 304. Any other status is an *Error. The caller
// closes the body.
func (c *Client) do(ctx context.Context, method, url string, body []byte, contentType string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		if c.userAgent != "" {
			req.Header.Set("User-Agent", c.userAgent)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", method, req.URL.Path, err)
		}
		if resp.StatusCode < 300 || resp.StatusCode == http.StatusNotModified {
			return resp, nil
		}
		apiErr := readError(resp)
		resp.Body.Close()
		if apiErr.Status != http.StatusTooManyRequests || attempt >= maxRetries {
			return nil, apiErr
		}
		if err := c.sleep(ctx, apiErr.RetryAfter); err != nil {
			return nil, apiErr
		}
	}
}

// doJSON sends in as JSON (when not nil) and decodes the response into out
// (when not nil).
func (c *Client) doJSON(ctx context.Context, method, url string, in, out any) error {
	var body []byte
	contentType := ""
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body, contentType = b, "application/json"
	}
	resp, err := c.do(ctx, method, url, body, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s response: %w", resp.Request.URL.Path, err)
	}
	return nil
}
