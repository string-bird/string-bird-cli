package api

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Machine translation run statuses. Queued and running are in progress.
const (
	JobQueued    = "queued"
	JobRunning   = "running"
	JobCompleted = "completed"
	JobFailed    = "failed"
	JobCancelled = "cancelled"
)

// Job is a machine translation run.
type Job struct {
	ID          string                 `json:"id"`
	Status      string                 `json:"status"`
	Scope       string                 `json:"scope"`
	Locales     []string               `json:"locales"`
	Total       int                    `json:"total"`
	Done        int                    `json:"done"`
	Failed      int                    `json:"failed"`
	Progress    map[string]JobProgress `json:"progress"`
	Failures    []JobFailure           `json:"failures"`
	ChangeSetID *string                `json:"change_set_id"`
	Error       string                 `json:"error"`
	CreatedAt   time.Time              `json:"created_at"`
	FinishedAt  *time.Time             `json:"finished_at"`
}

// JobProgress counts one locale of a run.
type JobProgress struct {
	Total  int `json:"total"`
	Done   int `json:"done"`
	Failed int `json:"failed"`
}

// JobFailure is a string a run couldn't translate.
type JobFailure struct {
	KeyID  string `json:"key_id"`
	Key    string `json:"key"`
	Locale string `json:"locale"`
	Reason string `json:"reason"`
}

// Finished reports whether the run is over.
func (j Job) Finished() bool {
	return j.Status != JobQueued && j.Status != JobRunning
}

// TranslateRequest starts a run. Scope is missing (default), outdated or
// needed; no locales means every locale but the base.
type TranslateRequest struct {
	Scope   string   `json:"scope,omitempty"`
	Locales []string `json:"locales,omitempty"`
}

// StartMachineTranslation starts a machine translation run in the background.
func (c *Client) StartMachineTranslation(ctx context.Context, r TranslateRequest) (Job, error) {
	var out struct {
		Job Job `json:"job"`
	}
	if err := c.doJSON(ctx, http.MethodPost, c.projectPath("/machine-translations", nil), r, &out); err != nil {
		return Job{}, err
	}
	return out.Job, nil
}

// MachineTranslation fetches a run.
func (c *Client) MachineTranslation(ctx context.Context, id string) (Job, error) {
	var out struct {
		Job Job `json:"job"`
	}
	if err := c.doJSON(ctx, http.MethodGet, c.projectPath("/machine-translations/"+url.PathEscape(id), nil), nil, &out); err != nil {
		return Job{}, err
	}
	return out.Job, nil
}

// ApproveRequest picks the machine translations to approve; the filters
// combine, and at least one of Locale and ChangeSetID is needed.
type ApproveRequest struct {
	Locale      string `json:"locale,omitempty"`
	ChangeSetID string `json:"change_set_id,omitempty"`
	Prefix      string `json:"prefix,omitempty"`
}

// ApproveResult counts an approval.
type ApproveResult struct {
	Approved        int     `json:"approved"`
	SkippedOutdated int     `json:"skipped_outdated"`
	ChangeSetID     *string `json:"change_set_id"`
}

// Approve approves machine translations awaiting approval.
func (c *Client) Approve(ctx context.Context, r ApproveRequest) (ApproveResult, error) {
	var out ApproveResult
	if err := c.doJSON(ctx, http.MethodPost, c.projectPath("/translations/approve", nil), r, &out); err != nil {
		return ApproveResult{}, err
	}
	return out, nil
}
