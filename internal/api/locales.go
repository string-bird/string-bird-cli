package api

import (
	"context"
	"net/http"
	"time"
)

// Locale is a locale of a project.
type Locale struct {
	Code      string    `json:"code"`
	StyleNote string    `json:"style_note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AddLocale adds a locale to the project. The API answers with its canonical
// code: de-at becomes de-AT.
func (c *Client) AddLocale(ctx context.Context, code string) (Locale, error) {
	in := struct {
		Locale string `json:"locale"`
	}{code}
	var out struct {
		Locale Locale `json:"locale"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.projectPath("/locales", nil), in, &out); err != nil {
		return Locale{}, err
	}
	return out.Locale, nil
}
