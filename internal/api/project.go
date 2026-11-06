package api

import (
	"context"
	"net/http"
	"net/url"
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

// Projects lists the projects of the client's organization that the API key
// can see, by name.
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var out struct {
		Projects []Project `json:"projects"`
	}
	path := c.baseURL + "/v1/orgs/" + url.PathEscape(c.orgID) + "/projects"
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Projects, nil
}

// ForProject is a copy of the client that calls another project's routes.
func (c *Client) ForProject(id string) *Client {
	cp := *c
	cp.projectID = id
	return &cp
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
