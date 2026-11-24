package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/task"
	"github.com/stretchr/testify/require"
)

func TestRunAllTasksQueuesConfiguredTasksWithAdminAuth(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		password   string
		wantStatus int
		wantQueued int
	}{
		{name: "missing credentials", wantStatus: http.StatusUnauthorized},
		{name: "wrong credentials", password: "wrong", wantStatus: http.StatusUnauthorized},
		{name: "authenticated", password: "password", wantStatus: http.StatusAccepted, wantQueued: 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			application, err := New(config.Config{
				PublicURL: "http://localhost", WebhookToken: "secret",
				Admin:    config.AdminConfig{Username: "admin", Password: "password"},
				Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: t.TempDir()}},
				Tasks: []config.TaskConfig{
					{ID: "zeta", StorageID: "media", Source: "/", Destination: t.TempDir()},
					{ID: "alpha", StorageID: "media", Source: "/", Destination: t.TempDir()},
				},
			})
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, "/api/tasks/run-all", nil)
			if scenario.password != "" {
				request.SetBasicAuth("admin", scenario.password)
			}
			response := httptest.NewRecorder()

			application.Handler().ServeHTTP(response, request)

			require.Equal(t, scenario.wantStatus, response.Code, response.Body.String())
			if scenario.wantStatus == http.StatusAccepted {
				require.Equal(t, "application/json", response.Header().Get("Content-Type"))
				require.JSONEq(t, `{"status":"queued","tasks":["zeta","alpha"]}`, response.Body.String())
			}
			statusRequest := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
			statusRequest.SetBasicAuth("admin", "password")
			statusResponse := httptest.NewRecorder()
			application.Handler().ServeHTTP(statusResponse, statusRequest)
			require.Equal(t, http.StatusOK, statusResponse.Code)
			var body struct {
				Statuses []task.Status `json:"statuses"`
			}
			require.NoError(t, json.Unmarshal(statusResponse.Body.Bytes(), &body))
			require.Len(t, body.Statuses, 2)
			require.ElementsMatch(t, []string{"alpha", "zeta"}, []string{body.Statuses[0].TaskID, body.Statuses[1].TaskID})
			for _, status := range body.Statuses {
				require.Equal(t, scenario.wantQueued, status.Queued, status.TaskID)
			}
		})
	}
}

func TestRunAllTasksConflictsWhenNoTasksConfigured(t *testing.T) {
	application, err := New(config.Config{
		PublicURL: "http://localhost", WebhookToken: "secret",
		Admin: config.AdminConfig{Username: "admin", Password: "password"},
	})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/tasks/run-all", nil)
	request.SetBasicAuth("admin", "password")
	response := httptest.NewRecorder()

	application.Handler().ServeHTTP(response, request)

	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	require.Equal(t, "application/json", response.Header().Get("Content-Type"))
	var body map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body, 1)
	require.NotEmpty(t, body["error"])
}
