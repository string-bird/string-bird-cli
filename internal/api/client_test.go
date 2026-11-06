package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorder is a hand-written test server: it answers with the queued
// responses in order and records what it was sent.
type recorder struct {
	mu        sync.Mutex
	responses []response
	requests  []*http.Request
	bodies    []string
}

type response struct {
	status  int
	headers map[string]string
	body    string
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	body, _ := io.ReadAll(req.Body)
	r.requests = append(r.requests, req)
	r.bodies = append(r.bodies, string(body))
	if len(r.responses) == 0 {
		http.Error(w, "no response queued", http.StatusTeapot)
		return
	}
	resp := r.responses[0]
	if len(r.responses) > 1 {
		r.responses = r.responses[1:]
	}
	for k, v := range resp.headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(resp.status)
	io.WriteString(w, resp.body)
}

// newTestClient serves rec and records the waits between retries.
func newTestClient(t *testing.T, rec *recorder) (*Client, *[]time.Duration) {
	t.Helper()
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)
	c := New(Options{
		BaseURL: srv.URL + "/", APIKey: "sb_key", OrgID: "org 1", ProjectID: "proj/1", UserAgent: "sb/test",
	})
	var slept []time.Duration
	c.SetSleep(func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	})
	return c, &slept
}

func jsonResponse(status int, body string) response {
	return response{status: status, headers: map[string]string{"Content-Type": "application/json"}, body: body}
}

func TestRequests(t *testing.T) {
	tests := []struct {
		name        string
		call        func(c *Client) error
		wantMethod  string
		wantPath    string // escaped
		wantQuery   string
		wantBody    string
		wantContent string
		respond     response
	}{
		{
			name:       "project",
			call:       func(c *Client) error { _, err := c.Project(context.Background()); return err },
			wantMethod: http.MethodGet,
			wantPath:   "/v1/orgs/org%201/projects/proj%2F1",
			respond:    jsonResponse(200, `{"project":{"id":"p"}}`),
		},
		{
			name: "import with every option",
			call: func(c *Client) error {
				_, err := c.Import(context.Background(), []byte(`{"a":"b"}`), ImportOptions{
					Locale: "de-AT", Format: "i18next", DryRun: true, SkipExisting: true, DeleteMissing: true,
				})
				return err
			},
			wantMethod:  http.MethodPost,
			wantPath:    "/v1/orgs/org%201/projects/proj%2F1/import",
			wantQuery:   "delete_missing=true&dry_run=true&format=i18next&locale=de-AT&skip_existing=true",
			wantBody:    `{"a":"b"}`,
			wantContent: "application/octet-stream",
			respond:     jsonResponse(200, `{"summary":{}}`),
		},
		{
			name: "import leaves false options out",
			call: func(c *Client) error {
				_, err := c.Import(context.Background(), []byte("x"), ImportOptions{Locale: "en"})
				return err
			},
			wantMethod:  http.MethodPost,
			wantPath:    "/v1/orgs/org%201/projects/proj%2F1/import",
			wantQuery:   "locale=en",
			wantBody:    "x",
			wantContent: "application/octet-stream",
			respond:     jsonResponse(200, `{"summary":{}}`),
		},
		{
			name: "export",
			call: func(c *Client) error {
				_, err := c.Export(context.Background(), ExportOptions{
					Locale: "de", Format: "json", Structure: "nested", IncludeUnapproved: true, Fallback: true,
				})
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/orgs/org%201/projects/proj%2F1/export",
			wantQuery:  "fallback=true&format=json&include_unapproved=true&locale=de&structure=nested",
			respond:    response{status: 200, body: "{}"},
		},
		{
			name: "create release",
			call: func(c *Client) error {
				_, err := c.CreateRelease(context.Background(), "note", "staging")
				return err
			},
			wantMethod:  http.MethodPost,
			wantPath:    "/v1/orgs/org%201/projects/proj%2F1/releases",
			wantBody:    `{"note":"note","environment":"staging"}`,
			wantContent: "application/json",
			respond:     jsonResponse(201, `{"release":{"id":"r"}}`),
		},
		{
			name: "create release without options sends an empty object",
			call: func(c *Client) error {
				_, err := c.CreateRelease(context.Background(), "", "")
				return err
			},
			wantMethod:  http.MethodPost,
			wantPath:    "/v1/orgs/org%201/projects/proj%2F1/releases",
			wantBody:    `{}`,
			wantContent: "application/json",
			respond:     jsonResponse(201, `{"release":{"id":"r"}}`),
		},
		{
			name:       "release by id",
			call:       func(c *Client) error { _, err := c.Release(context.Background(), "rel/1"); return err },
			wantMethod: http.MethodGet,
			wantPath:   "/v1/orgs/org%201/projects/proj%2F1/releases/rel%2F1",
			respond:    jsonResponse(200, `{"release":{"id":"rel/1"}}`),
		},
		{
			name: "release by version",
			call: func(c *Client) error {
				_, _, err := c.ReleaseByVersion(context.Background(), 12)
				return err
			},
			wantMethod: http.MethodGet,
			wantPath:   "/v1/orgs/org%201/projects/proj%2F1/releases",
			wantQuery:  "before=13&limit=1",
			respond:    jsonResponse(200, `{"releases":[]}`),
		},
		{
			name:       "environments",
			call:       func(c *Client) error { _, err := c.Environments(context.Background()); return err },
			wantMethod: http.MethodGet,
			wantPath:   "/v1/orgs/org%201/projects/proj%2F1/environments",
			respond:    jsonResponse(200, `{"environments":[]}`),
		},
		{
			name: "promote",
			call: func(c *Client) error {
				_, err := c.Promote(context.Background(), "pr-42", "rel-1")
				return err
			},
			wantMethod:  http.MethodPut,
			wantPath:    "/v1/orgs/org%201/projects/proj%2F1/environments/pr-42",
			wantBody:    `{"release_id":"rel-1"}`,
			wantContent: "application/json",
			respond:     jsonResponse(200, `{"environment":{"name":"pr-42"}}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{responses: []response{tt.respond}}
			c, _ := newTestClient(t, rec)

			require.NoError(t, tt.call(c))

			require.Len(t, rec.requests, 1)
			req := rec.requests[0]
			assert.Equal(t, tt.wantMethod, req.Method)
			assert.Equal(t, tt.wantPath, req.URL.EscapedPath())
			assert.Equal(t, tt.wantQuery, req.URL.RawQuery)
			if tt.wantContent == "application/json" {
				assert.JSONEq(t, tt.wantBody, rec.bodies[0])
			} else {
				assert.Equal(t, tt.wantBody, rec.bodies[0])
			}
			assert.Equal(t, tt.wantContent, req.Header.Get("Content-Type"))
			assert.Equal(t, "Bearer sb_key", req.Header.Get("Authorization"))
			assert.Equal(t, "application/json", req.Header.Get("Accept"))
			assert.Equal(t, "sb/test", req.Header.Get("User-Agent"))
		})
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name      string
		responses []response
		want      *Error
		wantSlept []time.Duration
		wantCalls int
	}{
		{
			name:      "api error",
			responses: []response{jsonResponse(404, `{"error":{"code":"project_not_found","message":"project not found"}}`)},
			want:      &Error{Status: 404, Code: "project_not_found", Message: "project not found", RetryAfter: time.Second},
			wantCalls: 1,
		},
		{
			name: "import problems",
			responses: []response{jsonResponse(422,
				`{"error":{"code":"invalid_import","message":"bad file","problems":[{"key":"a","reason":"broken ICU"}]}}`)},
			want: &Error{
				Status: 422, Code: "invalid_import", Message: "bad file",
				Problems: []Problem{{Key: "a", Reason: "broken ICU"}}, RetryAfter: time.Second,
			},
			wantCalls: 1,
		},
		{
			name:      "proxy page",
			responses: []response{{status: 502, body: "  <html>Bad Gateway</html>\n"}},
			want:      &Error{Status: 502, Code: "http_502", Message: "<html>Bad Gateway</html>", RetryAfter: time.Second},
			wantCalls: 1,
		},
		{
			name:      "long proxy page is cut",
			responses: []response{{status: 503, body: strings.Repeat("ü", 300)}},
			want:      &Error{Status: 503, Code: "http_503", Message: strings.Repeat("ü", 200) + "…", RetryAfter: time.Second},
			wantCalls: 1,
		},
		{
			name: "rate limited then fine",
			responses: []response{
				{status: 429, headers: map[string]string{"Retry-After": "3"}, body: `{"error":{"code":"rate_limited"}}`},
				jsonResponse(200, `{"project":{"id":"p"}}`),
			},
			wantSlept: []time.Duration{3 * time.Second},
			wantCalls: 2,
		},
		{
			name: "rate limited past the retries",
			responses: []response{
				{status: 429, headers: map[string]string{"Retry-After": "7200"}, body: `{"error":{"code":"rate_limited","message":"slow down"}}`},
			},
			want:      &Error{Status: 429, Code: "rate_limited", Message: "slow down", RetryAfter: time.Minute},
			wantSlept: []time.Duration{time.Minute, time.Minute, time.Minute},
			wantCalls: 4,
		},
		{
			name: "server errors aren't retried",
			responses: []response{
				jsonResponse(500, `{"error":{"code":"internal_error","message":"boom"}}`),
				jsonResponse(200, `{"project":{"id":"p"}}`),
			},
			want:      &Error{Status: 500, Code: "internal_error", Message: "boom", RetryAfter: time.Second},
			wantCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{responses: tt.responses}
			c, slept := newTestClient(t, rec)

			_, err := c.Project(context.Background())

			if tt.want == nil {
				require.NoError(t, err)
			} else {
				var apiErr *Error
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, tt.want, apiErr)
			}
			assert.Equal(t, tt.wantSlept, *slept)
			assert.Len(t, rec.requests, tt.wantCalls)
		})
	}
}

func TestRetryStopsWhenCancelled(t *testing.T) {
	rec := &recorder{responses: []response{{status: 429, body: `{"error":{"code":"rate_limited"}}`}}}
	c, _ := newTestClient(t, rec)
	c.SetSleep(sleepContext)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Project(ctx)

	require.Error(t, err)
	assert.LessOrEqual(t, len(rec.requests), 1)
}

func TestRetryAfter(t *testing.T) {
	for header, want := range map[string]time.Duration{
		"":      time.Second,
		"0":     time.Second,
		"-5":    time.Second,
		"soon":  time.Second,
		" 12 ":  12 * time.Second,
		"60":    time.Minute,
		"86400": time.Minute,
		// HTTP dates are allowed by the spec; the API sends seconds.
		"Wed, 21 Oct 2026 07:28:00 GMT": time.Second,
	} {
		t.Run(header, func(t *testing.T) {
			assert.Equal(t, want, retryAfter(header))
		})
	}
}

func TestResponses(t *testing.T) {
	t.Run("export skipped keys", func(t *testing.T) {
		rec := &recorder{responses: []response{{
			status: 200, headers: map[string]string{"X-Skipped-Keys": `["a","b.c"]`}, body: `{"x":"y"}`,
		}}}
		c, _ := newTestClient(t, rec)

		exp, err := c.Export(context.Background(), ExportOptions{Locale: "de"})

		require.NoError(t, err)
		assert.Equal(t, `{"x":"y"}`, string(exp.Body))
		assert.Equal(t, []string{"a", "b.c"}, exp.SkippedKeys)
	})
	t.Run("bad skipped keys header", func(t *testing.T) {
		rec := &recorder{responses: []response{{status: 200, headers: map[string]string{"X-Skipped-Keys": "a,b"}, body: "{}"}}}
		c, _ := newTestClient(t, rec)

		_, err := c.Export(context.Background(), ExportOptions{Locale: "de"})

		assert.ErrorContains(t, err, "decode X-Skipped-Keys")
	})
	t.Run("import result", func(t *testing.T) {
		rec := &recorder{responses: []response{jsonResponse(200, `{"dry_run":false,"change_set_id":"cs","summary":{"added_keys":1,"created":2,"updated":3,"unchanged":4,"skipped_existing":5,"deleted_keys":6},"added_keys":["k"],"created":["c"],"updated":[{"key":"u","old_value":"o","new_value":"n"}],"skipped_existing":["s"],"deleted_keys":["d"]}`)}}
		c, _ := newTestClient(t, rec)

		res, err := c.Import(context.Background(), []byte("{}"), ImportOptions{Locale: "en"})

		require.NoError(t, err)
		cs := "cs"
		assert.Equal(t, ImportResult{
			ChangeSetID: &cs,
			Summary:     ImportSummary{AddedKeys: 1, Created: 2, Updated: 3, Unchanged: 4, SkippedExisting: 5, DeletedKeys: 6},
			AddedKeys:   []string{"k"}, Created: []string{"c"},
			Updated:         []UpdatedKey{{Key: "u", OldValue: "o", NewValue: "n"}},
			SkippedExisting: []string{"s"}, DeletedKeys: []string{"d"},
		}, res)
	})
	t.Run("nulls in a release", func(t *testing.T) {
		rec := &recorder{responses: []response{jsonResponse(200, `{"release":{"id":"r","version":3,"status":"pending","note":null,"error":null,"published_at":null,"locales":["de"]}}`)}}
		c, _ := newTestClient(t, rec)

		rel, err := c.Release(context.Background(), "r")

		require.NoError(t, err)
		assert.Equal(t, Release{ID: "r", Version: 3, Status: ReleasePending, Locales: []string{"de"}}, rel)
	})
	t.Run("release by version finds it", func(t *testing.T) {
		rec := &recorder{responses: []response{jsonResponse(200, `{"releases":[{"id":"r12","version":12}],"next_before":12}`)}}
		c, _ := newTestClient(t, rec)

		rel, ok, err := c.ReleaseByVersion(context.Background(), 12)

		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, "r12", rel.ID)
	})
	t.Run("release by version, gap in the versions", func(t *testing.T) {
		rec := &recorder{responses: []response{jsonResponse(200, `{"releases":[{"id":"r11","version":11}]}`)}}
		c, _ := newTestClient(t, rec)

		_, ok, err := c.ReleaseByVersion(context.Background(), 12)

		require.NoError(t, err)
		assert.False(t, ok)
	})
	t.Run("not json", func(t *testing.T) {
		rec := &recorder{responses: []response{{status: 200, body: "<html>"}}}
		c, _ := newTestClient(t, rec)

		_, err := c.Project(context.Background())

		assert.ErrorContains(t, err, "decode /v1/orgs/org 1/projects/proj/1 response")
	})
	t.Run("connection refused", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		c := New(Options{BaseURL: url, OrgID: "o", ProjectID: "p"})

		_, err := c.Project(context.Background())

		require.Error(t, err)
		var apiErr *Error
		assert.False(t, errors.As(err, &apiErr))
		assert.Contains(t, err.Error(), "GET /v1/orgs/o/projects/p")
	})
}

func TestErrorMessage(t *testing.T) {
	assert.Equal(t, "plan_limit_reached: Too many keys (HTTP 403)",
		(&Error{Status: 403, Code: "plan_limit_reached", Message: "Too many keys"}).Error())
	assert.Equal(t, "http_502 (HTTP 502)", (&Error{Status: 502, Code: "http_502"}).Error())
}

func TestKeysPages(t *testing.T) {
	rec := &recorder{responses: []response{
		jsonResponse(200, `{"keys":[{"key":"a"},{"key":"b"}],"next_cursor":"c1"}`),
		jsonResponse(200, `{"keys":[{"key":"c"}],"next_cursor":null}`),
	}}
	c, _ := newTestClient(t, rec)

	keys, err := c.Keys(context.Background(), KeyFilter{Prefix: "x.", Locale: "de", Status: StatusMissing}, 0)

	require.NoError(t, err)
	assert.Len(t, keys, 3)
	require.Len(t, rec.requests, 2)
	assert.Equal(t, "limit=500&locale=de&prefix=x.&status=missing", rec.requests[0].URL.RawQuery)
	assert.Equal(t, "cursor=c1&limit=500&locale=de&prefix=x.&status=missing", rec.requests[1].URL.RawQuery)
}

func TestKeysLimit(t *testing.T) {
	rec := &recorder{responses: []response{jsonResponse(200, `{"keys":[{"key":"a"},{"key":"b"}],"next_cursor":"c1"}`)}}
	c, _ := newTestClient(t, rec)

	keys, err := c.Keys(context.Background(), KeyFilter{}, 2)

	require.NoError(t, err)
	assert.Len(t, keys, 2)
	require.Len(t, rec.requests, 1)
	assert.Equal(t, "limit=2", rec.requests[0].URL.RawQuery)
}

func TestRaw(t *testing.T) {
	rec := &recorder{responses: []response{
		{status: 429, headers: map[string]string{"Retry-After": "2"}, body: `{"error":{"code":"rate_limited"}}`},
		jsonResponse(404, `{"error":{"code":"key_not_found"}}`),
	}}
	c, slept := newTestClient(t, rec)

	resp, err := c.Raw(context.Background(), "DELETE", "/v1/orgs/o/projects/p/keys/k?x=1", []byte("b"), "text/plain")

	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, 404, resp.StatusCode)
	assert.JSONEq(t, `{"error":{"code":"key_not_found"}}`, string(body))
	assert.Equal(t, []time.Duration{2 * time.Second}, *slept)
	require.Len(t, rec.requests, 2)
	assert.Equal(t, "/v1/orgs/o/projects/p/keys/k", rec.requests[1].URL.Path)
	assert.Equal(t, "x=1", rec.requests[1].URL.RawQuery)
	assert.Equal(t, "b", rec.bodies[1])
	assert.Equal(t, "text/plain", rec.requests[1].Header.Get("Content-Type"))
}
