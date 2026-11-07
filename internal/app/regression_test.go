package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/signature"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/task"
	"github.com/stretchr/testify/require"
)

func TestRegressionEnvironmentTokenSurvivesManagedSave(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	disk := config.Config{Version: 1, PublicURL: "http://example.test", WebhookToken: "disk-token"}
	if e := config.Save(p, disk); e != nil {
		t.Fatal(e)
	}
	t.Setenv("WEBHOOK_TOKEN", "environment-token")
	cfg, e := config.Load(p)
	if e != nil {
		t.Fatal(e)
	}
	a, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	payload, e := json.Marshal(disk.Managed())
	if e != nil {
		t.Fatal(e)
	}
	rr := httptest.NewRecorder()
	a.Handler().ServeHTTP(rr, httptest.NewRequest("PUT", "/api/config/managed", bytes.NewReader(payload)))
	if rr.Code != 200 {
		t.Fatalf("save status=%d: %s", rr.Code, rr.Body.String())
	}
	_, token := a.streamRuntime()
	if token != "environment-token" {
		t.Fatal("managed save replaced active environment token with disk token")
	}
	saved, e := os.ReadFile(p)
	require.NoError(t, e)
	require.NotContains(t, string(saved), "environment-token")
	require.Contains(t, string(saved), "disk-token")
	rr = httptest.NewRecorder()
	a.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/config/webhook-token", nil))
	require.Equal(t, 409, rr.Code, rr.Body.String())
}

type reviewBlockingStorage struct {
	storage.Storage
	entered chan struct{}
	release chan struct{}
}

func (s reviewBlockingStorage) List(ctx context.Context, p string) ([]storage.Entry, error) {
	close(s.entered)
	<-s.release
	return s.Storage.List(ctx, p)
}
func TestRegressionReloadWaitsForOldWriter(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	if e := os.WriteFile(filepath.Join(src, "film.mkv"), []byte("media"), 0644); e != nil {
		t.Fatal(e)
	}
	cfg := config.Config{PublicURL: "http://example.test", WebhookToken: "old-token", Storages: []config.StorageConfig{{ID: "local", Type: "local", Root: src}}, Tasks: []config.TaskConfig{{ID: "films", StorageID: "local", Source: "/", Destination: dst}}}
	a, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	blocked := reviewBlockingStorage{a.storages["local"], entered, release}
	a.manager = task.NewManager(cfg.Tasks, map[string]storage.Storage{"local": blocked}, task.NewGenerator(cfg.PublicURL, cfg.WebhookToken))
	a.manager.SetHistory(a.history)
	oldManager := a.manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if e = a.Start(ctx); e != nil {
		t.Fatal(e)
	}
	if e = oldManager.Enqueue("films", ""); e != nil {
		t.Fatal(e)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("old task not started")
	}
	candidate := cfg.Clone()
	candidate.WebhookToken = "new-token"
	if _, err := a.reloadRuntime(candidate); err == nil {
		close(release)
		t.Fatal("busy reload must be rejected")
	}
	_, token := a.streamRuntime()
	if token != "old-token" {
		close(release)
		t.Fatal("rejected reload changed token")
	}
	close(release)
	require.Eventually(t, func() bool { _, err := a.reloadRuntime(candidate); return err == nil }, 3*time.Second, time.Millisecond)
	if e = a.manager.Enqueue("films", ""); e != nil {
		t.Fatal(e)
	}
	require.Eventually(t, func() bool { return !a.manager.Statuses()[0].EndedAt.IsZero() }, 3*time.Second, time.Millisecond)
	data, e := os.ReadFile(filepath.Join(dst, "film.strm"))
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(data), signature.Create("new-token", "local", "/film.mkv")) {
		t.Fatal("new generation did not replace old signature")
	}

}

func TestMalformedRunBodiesNeverEnqueue(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cfg := config.Config{PublicURL: "http://example.test", WebhookToken: "token", Storages: []config.StorageConfig{{ID: "local", Type: "local", Root: src}}, Tasks: []config.TaskConfig{{ID: "films", StorageID: "local", Source: "/", Destination: dst}}}
	a, err := New(cfg)
	require.NoError(t, err)
	for _, body := range []string{`null`, `[]`, `{"path":1}`, `{"path":"/"} {}`, `{"unknown":true}`, `{"path":"/"}` + strings.Repeat(" ", 1<<20)} {
		rr := httptest.NewRecorder()
		a.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/api/tasks/films/run", strings.NewReader(body)))
		require.Equal(t, 400, rr.Code, rr.Body.String())
		require.Equal(t, 0, a.manager.Statuses()[0].Queued)
	}
}

func TestConfigBodiesRejectNullTrailingAndOversizeWithoutSaving(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Config{Version: 1, PublicURL: "http://example.test", WebhookToken: "disk-token"}
	require.NoError(t, config.Save(filename, cfg))
	cfg.Path = filename
	a, err := New(cfg)
	require.NoError(t, err)
	before, err := os.ReadFile(filename)
	require.NoError(t, err)
	for _, route := range []string{"/api/config", "/api/config/managed"} {
		for _, body := range []string{"null", `{} {}`, `{}` + strings.Repeat(" ", 2<<20)} {
			rr := httptest.NewRecorder()
			a.Handler().ServeHTTP(rr, httptest.NewRequest("PUT", route, strings.NewReader(body)))
			require.Equal(t, 400, rr.Code, rr.Body.String())
			after, err := os.ReadFile(filename)
			require.NoError(t, err)
			require.Equal(t, before, after)
		}
	}
}

func TestManagedConfigRejectsEncodedOutputOverLimitWithoutSaving(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.json")
	current := config.Config{Version: config.CurrentVersion, PublicURL: "http://example.test", WebhookToken: "current-token"}
	previous := current
	previous.WebhookToken = "previous-token"
	require.NoError(t, config.Save(filename, previous))
	require.NoError(t, config.Save(filename, current))
	current.Path = filename
	a, err := New(current)
	require.NoError(t, err)
	managed := current.Managed()
	managed.Storages = []config.StorageConfig{{ID: "media", Type: "local", Root: t.TempDir()}}
	managed.Tasks = []config.TaskConfig{{
		ID: "movies", Name: strings.Repeat("<", 350000), StorageID: "media", Source: "/", Destination: t.TempDir(),
	}}
	var payload bytes.Buffer
	encoder := json.NewEncoder(&payload)
	encoder.SetEscapeHTML(false)
	require.NoError(t, encoder.Encode(managed))
	require.Less(t, payload.Len(), 2<<20)
	activeBytes, err := os.ReadFile(filename)
	require.NoError(t, err)
	backupBytes, err := os.ReadFile(filename + ".bak")
	require.NoError(t, err)

	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, httptest.NewRequest("PUT", "/api/config/managed", bytes.NewReader(payload.Bytes())))

	require.Equal(t, 400, response.Code, response.Body.String())
	afterActive, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, activeBytes, afterActive)
	afterBackup, err := os.ReadFile(filename + ".bak")
	require.NoError(t, err)
	require.Equal(t, backupBytes, afterBackup)
	_, err = config.LoadDisk(filename)
	require.NoError(t, err)
}

func TestRestoreRejectedByBusyRuntimePreservesConfigAndBackup(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.json")
	root := t.TempDir()
	active := config.Config{
		Version:      config.CurrentVersion,
		PublicURL:    "http://example.test",
		WebhookToken: "active-token",
		Storages:     []config.StorageConfig{{ID: "media", Type: "local", Root: root}},
		Tasks:        []config.TaskConfig{{ID: "movies", StorageID: "media", Source: "/", Destination: t.TempDir()}},
	}
	backup := active.Clone()
	backup.WebhookToken = "backup-token"
	require.NoError(t, config.Save(filename, backup))
	require.NoError(t, config.Save(filename, active))
	active.Path = filename
	a, err := New(active)
	require.NoError(t, err)
	require.NoError(t, a.manager.Enqueue("movies", ""))
	activeBytes, err := os.ReadFile(filename)
	require.NoError(t, err)
	backupBytes, err := os.ReadFile(filename + ".bak")
	require.NoError(t, err)

	response := httptest.NewRecorder()
	a.Handler().ServeHTTP(response, httptest.NewRequest("POST", "/api/config/restore", nil))

	require.Equal(t, 409, response.Code, response.Body.String())
	afterActive, err := os.ReadFile(filename)
	require.NoError(t, err)
	require.Equal(t, activeBytes, afterActive)
	afterBackup, err := os.ReadFile(filename + ".bak")
	require.NoError(t, err)
	require.Equal(t, backupBytes, afterBackup)
	_, token := a.streamRuntime()
	require.Equal(t, "active-token", token)
}

func TestRegressionFailedReloadKeepsHistoryTarget(t *testing.T) {
	oldPath, newPath := filepath.Join(t.TempDir(), "old.jsonl"), filepath.Join(t.TempDir(), "new.jsonl")
	cfg := config.Config{PublicURL: "http://example.test", WebhookToken: "token", History: config.HistoryConfig{Path: oldPath}}
	a, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if e = a.Start(ctx); e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	candidate := cfg.Clone()
	candidate.History.Path = newPath
	candidate.MediaProxy = config.MediaProxyConfig{Enabled: true, Listen: listener.Addr().String(), Upstream: "http://127.0.0.1:1"}
	if _, e = a.reloadRuntime(candidate); e == nil {
		t.Fatal("expected occupied-port rejection")
	}
	if _, e = os.Stat(newPath); e == nil {
		t.Fatal("failed reload already changed history path/persisted state before rejecting proxy")
	}
}

func TestRegressionCloudDriveSiblingEventsCovered(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cfg := config.Config{PublicURL: "http://example.test", WebhookToken: "token", Storages: []config.StorageConfig{{ID: "local", Type: "local", Root: src}}, Tasks: []config.TaskConfig{{ID: "films", StorageID: "local", Source: "/library", Destination: dst}}, Integrations: config.IntegrationConfig{CloudDrive2: map[string]string{"/external": "local/library"}}}
	a, e := New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	body := `{"data":[{"is_dir":false,"source_file":"/external/a/one.mkv"},{"is_dir":false,"source_file":"/external/b/two.mkv"}]}`
	rr := httptest.NewRecorder()
	a.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/webhook/token/file_notify", strings.NewReader(body)))
	if rr.Code != 202 {
		t.Fatalf("%d: %s", rr.Code, rr.Body.String())
	}
	events := a.history.Snapshot("films", 100)
	covered := false
	for _, event := range events {
		if event.Path == "/library" || event.Path == "/library/b" {
			covered = true
		}
	}
	if !covered {
		t.Fatalf("second sibling event lost; queued history=%+v", events)
	}
}

func TestMoviePilotSiblingEventsCoverCommonAncestor(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cfg := config.Config{PublicURL: "http://example.test", WebhookToken: "token", Storages: []config.StorageConfig{{ID: "local", Type: "local", Root: src}}, Tasks: []config.TaskConfig{{ID: "films", StorageID: "local", Source: "/library", Destination: dst}}, Integrations: config.IntegrationConfig{MoviePilot: map[string]string{"/external": "local/library"}}}
	a, err := New(cfg)
	require.NoError(t, err)
	body := `{"type":"transfer.complete","data":{"transferinfo":{"file_list_new":["/external/a/one.mkv","/external/b/two.mkv"]}}}`
	rr := httptest.NewRecorder()
	a.Handler().ServeHTTP(rr, httptest.NewRequest("POST", "/webhook/token/moviepilot", strings.NewReader(body)))
	require.Equal(t, 202, rr.Code, rr.Body.String())
	events := a.history.Snapshot("films", 100)
	require.Len(t, events, 1)
	require.Equal(t, "/library", events[0].Path)
}
