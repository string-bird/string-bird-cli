package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Key statuses a key list can filter by, for one locale.
const (
	StatusMissing    = "missing"
	StatusOutdated   = "outdated"
	StatusUnapproved = "unapproved"
)

// maxPageSize is the most keys the API returns per page.
const maxPageSize = 500

// Key is a translation key with its translations by locale. Missing
// translations are absent from Translations.
type Key struct {
	ID           string                 `json:"id"`
	Key          string                 `json:"key"`
	Description  string                 `json:"description"`
	MaxLength    *int                   `json:"max_length"`
	Translations map[string]Translation `json:"translations"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// Translation is a key's text in one locale.
type Translation struct {
	ID       string  `json:"id"`
	Locale   string  `json:"locale"`
	Value    string  `json:"value"`
	Origin   string  `json:"origin"`
	Approved bool    `json:"approved"`
	Outdated bool    `json:"outdated"`
	Issues   []Issue `json:"issues"`
}

// Issue is a warning on a translation: placeholder_missing, too_long, ...
type Issue struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Argument string `json:"argument,omitempty"`
}

// KeyFilter narrows a key list. Status needs Locale.
type KeyFilter struct {
	Prefix string
	Locale string
	Status string
}

// Keys lists every key matching the filter, by name, following the pages.
// limit stops early (0: all).
func (c *Client) Keys(ctx context.Context, f KeyFilter, limit int) ([]Key, error) {
	var all []Key
	cursor := ""
	for {
		q := url.Values{"limit": {strconv.Itoa(maxPageSize)}}
		if limit > 0 && limit-len(all) < maxPageSize {
			q.Set("limit", strconv.Itoa(limit-len(all)))
		}
		for name, v := range map[string]string{"prefix": f.Prefix, "locale": f.Locale, "status": f.Status, "cursor": cursor} {
			if v != "" {
				q.Set(name, v)
			}
		}
		var page struct {
			Keys       []Key   `json:"keys"`
			NextCursor *string `json:"next_cursor"`
		}
		if err := c.doJSON(ctx, http.MethodGet, c.projectPath("/keys", q), nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Keys...)
		if page.NextCursor == nil || *page.NextCursor == "" || (limit > 0 && len(all) >= limit) {
			return all, nil
		}
		cursor = *page.NextCursor
	}
}
