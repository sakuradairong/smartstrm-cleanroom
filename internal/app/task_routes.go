package app

import (
	"fmt"
	"net/http"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/task"
)

func (a *App) registerTaskRoutes(register func(string, http.Handler)) {
	register("GET /api/tasks", a.basicAuth(http.HandlerFunc(a.listTasks)))
	register("POST /api/tasks/run-all", a.basicAuth(http.HandlerFunc(a.runAllTasks)))
	register("POST /api/tasks/{id}/run", a.basicAuth(http.HandlerFunc(a.runTask)))
	register("POST /api/tasks/{id}/stop", a.basicAuth(http.HandlerFunc(a.stopTask)))
	register("POST /api/tasks/{id}/full-overwrite", a.basicAuth(http.HandlerFunc(a.fullOverwriteTask)))
	register("POST /api/tasks/{id}/replace-content", a.basicAuth(http.HandlerFunc(a.replaceTaskContent)))
	register("GET /api/tasks/{id}/generated", a.basicAuth(http.HandlerFunc(a.previewTaskGenerated)))
	register("DELETE /api/tasks/{id}/generated", a.basicAuth(http.HandlerFunc(a.clearTaskGenerated)))
	register("POST /api/tasks/{id}/covers", a.basicAuth(http.HandlerFunc(a.extractTaskCovers)))
	register("GET /api/tasks/{id}/preview", a.basicAuth(http.HandlerFunc(a.previewTask)))
}

func (a *App) runAllTasks(w http.ResponseWriter, _ *http.Request) {
	if len(a.config.Tasks) == 0 {
		writeError(w, http.StatusConflict, fmt.Errorf("no tasks configured"))
		return
	}
	requests := make([]task.BatchRequest, len(a.config.Tasks))
	ids := make([]string, len(a.config.Tasks))
	for index, configured := range a.config.Tasks {
		requests[index] = task.BatchRequest{TaskID: configured.ID}
		ids[index] = configured.ID
	}
	if err := a.manager.EnqueueBatch(requests, 0); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, struct {
		Status string   `json:"status"`
		Tasks  []string `json:"tasks"`
	}{Status: "queued", Tasks: ids})
}
