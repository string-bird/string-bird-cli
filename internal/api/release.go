package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Release statuses.
const (
	ReleasePending   = "pending"
	ReleasePublished = "published"
	ReleaseFailed    = "failed"
)

// Release is an immutable snapshot of approved translations.
type Release struct {
	ID           string     `json:"id"`
	Version      int        `json:"version"`
	Status       string     `json:"status"`
	BundleFormat string     `json:"bundle_format"`
	Note         string     `json:"note"`
	KeyCount     int        `json:"key_count"`
	Locales      []string   `json:"locales"`
	Error        string     `json:"error"`
	CreatedAt    time.Time  `json:"created_at"`
	PublishedAt  *time.Time `json:"published_at"`
}

// Environment is a named pointer to a release.
type Environment struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Release   *Release  `json:"release"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateRelease snapshots the project's approved translations as the next
// version. With environment set, that environment points at it too.
func (c *Client) CreateRelease(ctx context.Context, note, environment string) (Release, error) {
	in := struct {
		Note        string `json:"note,omitempty"`
		Environment string `json:"environment,omitempty"`
	}{note, environment}
	var out struct {
		Release Release `json:"release"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.projectPath("/releases", nil), in, &out); err != nil {
		return Release{}, err
	}
	return out.Release, nil
}

// Release fetches one release by id.
func (c *Client) Release(ctx context.Context, id string) (Release, error) {
	var out struct {
		Release Release `json:"release"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.projectPath("/releases/"+url.PathEscape(id), nil), nil, &out); err != nil {
		return Release{}, err
	}
	return out.Release, nil
}

// ReleaseByVersion finds a release by its version number. The list is newest
// first, so the one release before version+1 is that version, if it exists.
func (c *Client) ReleaseByVersion(ctx context.Context, version int) (Release, bool, error) {
	q := url.Values{"before": {strconv.Itoa(version + 1)}, "limit": {"1"}}
	var out struct {
		Releases []Release `json:"releases"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.projectPath("/releases", q), nil, &out); err != nil {
		return Release{}, false, err
	}
	if len(out.Releases) == 0 || out.Releases[0].Version != version {
		return Release{}, false, nil
	}
	return out.Releases[0], true, nil
}

// Environments lists the project's environments by name.
func (c *Client) Environments(ctx context.Context) ([]Environment, error) {
	var out struct {
		Environments []Environment `json:"environments"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.projectPath("/environments", nil), nil, &out); err != nil {
		return nil, err
	}
	return out.Environments, nil
}

// Promote points an environment at a release: a promotion, or a rollback to
// an older one.
func (c *Client) Promote(ctx context.Context, environment, releaseID string) (Environment, error) {
	in := struct {
		ReleaseID string `json:"release_id"`
	}{releaseID}
	var out struct {
		Environment Environment `json:"environment"`
	}
	path := c.projectPath("/environments/"+url.PathEscape(environment), nil)
	if err := c.doJSON(ctx, http.MethodPut, path, in, &out); err != nil {
		return Environment{}, err
	}
	return out.Environment, nil
}
