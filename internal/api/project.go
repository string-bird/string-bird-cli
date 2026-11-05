package api

import (
	"context"
	"net/http"
)

// Project is the part of the project JSON the CLI uses.
type Project struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	PublicID     string   `json:"public_id"`
	BaseLocale   string   `json:"base_locale"`
	BundleFormat string   `json:"bundle_format"`
	Locales      []string `json:"locales"`
}

// Project fetches the client's project.
func (c *Client) Project(ctx context.Context) (Project, error) {
	var out struct {
		Project Project `json:"project"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.projectPath("", nil), nil, &out); err != nil {
		return Project{}, err
	}
	return out.Project, nil
}
