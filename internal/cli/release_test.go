package cli

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/string-bird/string-bird-cli/internal/api"
)

func TestReleaseCreate(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		publishAfter int
		publishAs    string
		fail         map[string]fakeError
		wantCode     int
		wantBody     string
		wantStdout   []string
		wantStderr   []string
		wantEnvs     map[string]string
		wantPolls    int
	}{
		{
			name:       "plain",
			args:       []string{"release", "create"},
			wantBody:   `{}`,
			wantStdout: []string{"created v1 (120 keys, de, en)\n"},
			wantEnvs:   map[string]string{"staging": ""},
		},
		{
			name:       "into an environment with a note",
			args:       []string{"release", "create", "--env", "staging", "--note", "Spring sale"},
			wantBody:   `{"note":"Spring sale","environment":"staging"}`,
			wantStdout: []string{"created v1", "staging → v1\n"},
			wantEnvs:   map[string]string{"staging": relID(1)},
		},
		{
			name:         "wait until published",
			args:         []string{"release", "create", "--wait"},
			publishAfter: 2,
			publishAs:    api.ReleasePublished,
			wantBody:     `{}`,
			wantStdout:   []string{"published v1\n"},
			wantEnvs:     map[string]string{"staging": ""},
			wantPolls:    3,
		},
		{
			name:       "wait until failed",
			args:       []string{"release", "create", "--wait"},
			publishAs:  api.ReleaseFailed,
			wantCode:   1,
			wantBody:   `{}`,
			wantStderr: []string{"v1 failed: upload: bucket unreachable"},
			wantEnvs:   map[string]string{"staging": ""},
			wantPolls:  1,
		},
		{
			name:       "unknown environment",
			args:       []string{"release", "create", "--env", "qa"},
			wantCode:   1,
			wantBody:   `{"environment":"qa"}`,
			wantStderr: []string{"create release: environment_not_found"},
			wantEnvs:   map[string]string{"staging": ""},
		},
		{
			name: "nothing to release",
			args: []string{"release", "create"},
			fail: map[string]fakeError{"POST /releases": {
				status: http.StatusUnprocessableEntity, code: "nothing_to_release",
			}},
			wantCode:   1,
			wantBody:   `{}`,
			wantStderr: []string{"nothing_to_release"},
			wantEnvs:   map[string]string{"staging": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			fake.envs = map[string]string{"staging": ""}
			fake.publishAfter, fake.publishAs = tt.publishAfter, tt.publishAs
			if tt.fail != nil {
				fake.fail = tt.fail
			}
			p := newTestProject(t, fake, "")

			res := p.run(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stderr: %s", res.stderr)
			creates := p.requestsTo(http.MethodPost, "/releases")
			require.Len(t, creates, 1)
			assert.JSONEq(t, tt.wantBody, creates[0].body)
			assert.Len(t, p.requestsTo(http.MethodGet, "/releases/"+relID(1)), tt.wantPolls)
			assert.Len(t, p.slept, tt.wantPolls)
			assert.Equal(t, tt.wantEnvs, fake.envs)
			for _, s := range tt.wantStdout {
				assert.Contains(t, res.stdout, s)
			}
			for _, s := range tt.wantStderr {
				assert.Contains(t, res.stderr, s)
			}
		})
	}
}

func TestReleasePromote(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantCode     int
		wantRequests []string
		wantEnvs     map[string]string
		wantStdout   string
		wantStderr   string
	}{
		{
			name:         "from another environment",
			args:         []string{"release", "promote", "production", "--from", "staging"},
			wantRequests: []string{"GET /environments", "PUT /environments/production"},
			wantEnvs:     map[string]string{"production": relID(3), "staging": relID(3), "preview": ""},
			wantStdout:   "production → v3\n",
		},
		{
			name:         "roll back by version",
			args:         []string{"release", "promote", "production", "--release", "v1"},
			wantRequests: []string{"GET /releases", "PUT /environments/production"},
			wantEnvs:     map[string]string{"production": relID(1), "staging": relID(3), "preview": ""},
			wantStdout:   "production → v1\n",
		},
		{
			name:         "by bare version",
			args:         []string{"release", "promote", "staging", "--release", "2"},
			wantRequests: []string{"GET /releases", "PUT /environments/staging"},
			wantEnvs:     map[string]string{"production": relID(2), "staging": relID(2), "preview": ""},
			wantStdout:   "staging → v2\n",
		},
		{
			name:         "by id",
			args:         []string{"release", "promote", "production", "--release", relID(3)},
			wantRequests: []string{"GET /releases/" + relID(3), "PUT /environments/production"},
			wantEnvs:     map[string]string{"production": relID(3), "staging": relID(3), "preview": ""},
			wantStdout:   "production → v3\n",
		},
		{
			name:         "unknown version",
			args:         []string{"release", "promote", "production", "--release", "v9"},
			wantCode:     1,
			wantRequests: []string{"GET /releases"},
			wantEnvs:     map[string]string{"production": relID(2), "staging": relID(3), "preview": ""},
			wantStderr:   "release v9 not found",
		},
		{
			name:         "source environment without a release",
			args:         []string{"release", "promote", "production", "--from", "preview"},
			wantCode:     1,
			wantRequests: []string{"GET /environments"},
			wantEnvs:     map[string]string{"production": relID(2), "staging": relID(3), "preview": ""},
			wantStderr:   "environment preview has no release yet",
		},
		{
			name:         "unknown target environment",
			args:         []string{"release", "promote", "qa", "--from", "staging"},
			wantCode:     1,
			wantRequests: []string{"GET /environments", "PUT /environments/qa"},
			wantEnvs:     map[string]string{"production": relID(2), "staging": relID(3), "preview": ""},
			wantStderr:   "promote qa: environment_not_found",
		},
		{
			name:       "needs a release or a source",
			args:       []string{"release", "promote", "production"},
			wantCode:   1,
			wantEnvs:   map[string]string{"production": relID(2), "staging": relID(3), "preview": ""},
			wantStderr: "at least one of the flags in the group [release from] is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFakeAPI()
			for range 3 {
				fake.addRelease(api.ReleasePublished)
			}
			fake.envs = map[string]string{"production": relID(2), "staging": relID(3), "preview": ""}
			p := newTestProject(t, fake, "")

			res := p.run(tt.args...)

			require.Equal(t, tt.wantCode, res.code, "stderr: %s", res.stderr)
			assert.Equal(t, tt.wantRequests, nilIfEmpty(p.requests()))
			assert.Equal(t, tt.wantEnvs, fake.envs)
			assert.Equal(t, tt.wantStdout, res.stdout)
			assert.Contains(t, res.stderr, tt.wantStderr)
		})
	}
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}
