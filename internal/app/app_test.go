package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/history"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/signature"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderIndexHTMLTMDBConfigurationWarning(t *testing.T) {
	unconfigured, err := renderIndexHTML(false)
	require.NoError(t, err)
	assert.Contains(t, unconfigured, `id="tmdbStatus"`)
	assert.Contains(t, unconfigured, "TMDB API Key 未配置，请在配置页设置后重启服务")
	assert.Contains(t, unconfigured, "const tmdbConfigured=false")
	assert.Contains(t, unconfigured, "if(!tmdbConfigured){alert(")
	assert.Contains(t, unconfigured, `id="storageCount"`)
	assert.Contains(t, unconfigured, `aria-live="polite"`)
	assert.Contains(t, unconfigured, "storageCount.textContent='显示 '+items.length+' / 共 '+currentEntries.length+' 项'")
	assert.Contains(t, unconfigured, `id="storageContextMenu"`)
	assert.Contains(t, unconfigured, `role="menuitem">复制STRM地址`)
	assert.Contains(t, unconfigured, `storageIsMedia(x)?' tabindex="0" aria-haspopup="menu" data-entry-index="'+i+'"':'')`)
	assert.Contains(t, unconfigured, `function storageIsMedia(item){return storageFileIcon(item)==='🎬'}`)
	assert.Contains(t, unconfigured, `event.key==='ContextMenu'||(event.shiftKey&&event.key==='F10')`)
	assert.Contains(t, unconfigured, `id="runCurrentButton" class="hidden"`)
	assert.Contains(t, unconfigured, `/matching-tasks?path=`)
	assert.Contains(t, unconfigured, `const task=currentMatchingTasks[0]`)
	assert.NotContains(t, unconfigured, `setInterval(load,3000)`)
	assert.Equal(t, 1, strings.Count(unconfigured, `setTimeout(taskPollCycle,3000)`))
	assert.Contains(t, unconfigured, `if(taskPollRunning||document.hidden)return`)
	assert.Contains(t, unconfigured, `document.addEventListener('visibilitychange'`)
	assert.NotContains(t, unconfigured, `historyEvents.unshift(item);historyEvents=historyEvents.slice(0,200);renderHistory()`)
	assert.Contains(t, unconfigured, `queueHistoryEvent(item)`)
	assert.Contains(t, unconfigured, `requestAnimationFrame(flushHistoryEvents)`)
	assert.Contains(t, unconfigured, `if(historyPending.length===200)`)
	assert.Contains(t, unconfigured, `async function loadHistory(){clearPendingHistoryEvents();`)
	assert.Contains(t, unconfigured, `+' / 失败 '+(x.result?.failed||0)`)
	assert.Contains(t, unconfigured, `result=x.error?x.error+' / 失败 '+(x.result?.failed||0):(`)
	assert.Contains(t, unconfigured, `esc(e.error?e.error+' / 失败 '+(e.failed||0):(`)
	assert.Contains(t, unconfigured, `+' / 跳过 '+(e.skipped||0)+' / 失败 '+(e.failed||0)`)
	assert.Contains(t, unconfigured, `href="https://github.com/sakuradairong/smartstrm-cleanroom"`)
	assert.Contains(t, unconfigured, `AGPL-3.0-only`)
	assert.Contains(t, unconfigured, `本程序不提供任何担保`)
	assert.Contains(t, unconfigured, `id="managedStorageRows"`)
	assert.Contains(t, unconfigured, `id="managedTaskRows"`)
	assert.Contains(t, unconfigured, `onclick="editStorage(-1)"`)
	assert.Contains(t, unconfigured, `onclick="editTask(-1)"`)
	assert.Contains(t, unconfigured, `/api/config/managed`)
	assert.Contains(t, unconfigured, `配置文件只需保留监听地址、公开地址、管理员和 Webhook Token`)
	assert.NotContains(t, unconfigured, `id="configEditor"`)
	assert.Contains(t, unconfigured, `id="managedStorageRows"`)
	assert.Contains(t, unconfigured, `id="managedTaskRows"`)
	assert.Contains(t, unconfigured, `onclick="editStorage(-1)"`)
	assert.Contains(t, unconfigured, `onclick="editTask(-1)"`)
	assert.Contains(t, unconfigured, `/api/config/managed`)
	assert.Contains(t, unconfigured, `配置文件只需保留监听地址、公开地址、管理员和 Webhook Token`)
	assert.NotContains(t, unconfigured, `id="configEditor"`)

	configured, err := renderIndexHTML(true)
	require.NoError(t, err)
	assert.Contains(t, configured, `id="tmdbStatus" class="muted"></span>`)
	assert.Contains(t, configured, "const tmdbConfigured=true")
	assert.Equal(t, 1, strings.Count(configured, "async function tmdbRecognize()"))
}

func TestMatchingTasksAPIUsesPathBoundariesAndSpecificity(t *testing.T) {
	application, err := New(config.Config{
		PublicURL: "http://localhost", WebhookToken: "secret",
		Admin:    config.AdminConfig{Username: "admin", Password: "password"},
		Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: t.TempDir()}, {ID: "other", Type: "local", Root: t.TempDir()}},
		Tasks: []config.TaskConfig{
			{ID: "root", Name: "Root Task", StorageID: "media", Source: "/", Destination: t.TempDir()},
			{ID: "shows", Name: "Shows Task", StorageID: "media", Source: "/shows", Destination: t.TempDir()},
			{ID: "show", Name: "One Show", StorageID: "media", Source: "/shows/one", Destination: t.TempDir()},
		},
	})
	require.NoError(t, err)
	call := func(target string, authenticated bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if authenticated {
			request.SetBasicAuth("admin", "password")
		}
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, request)
		return response
	}

	response := call("/api/storages/media/matching-tasks?path=/shows/one/season", false)
	assert.Equal(t, http.StatusUnauthorized, response.Code)
	response = call("/api/storages/media/matching-tasks?path=/shows/one/season", true)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var matches []matchingTaskSummary
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &matches))
	require.Len(t, matches, 3)
	assert.Equal(t, []string{"show", "shows", "root"}, []string{matches[0].ID, matches[1].ID, matches[2].ID})
	assert.Equal(t, "One Show", matches[0].Name)
	assert.Equal(t, "/shows/one", matches[0].Source)

	response = call("/api/storages/media/matching-tasks?path=/shows-other", true)
	require.Equal(t, http.StatusOK, response.Code)
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &matches))
	require.Len(t, matches, 1)
	assert.Equal(t, "root", matches[0].ID)
	response = call("/api/storages/other/matching-tasks?path=/shows", true)
	assert.Equal(t, "[]\n", response.Body.String())
	response = call("/api/storages/missing/matching-tasks?path=/", true)
	assert.Equal(t, http.StatusNotFound, response.Code)
	nulRequest := httptest.NewRequest(http.MethodGet, "/api/storages/media/matching-tasks", nil)
	nulRequest.SetPathValue("id", "media")
	nulRequest.URL.RawQuery = "path=%00"
	response = httptest.NewRecorder()
	application.matchingTasks(response, nulRequest)
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestHomeTMDBStatusDoesNotExposeAPIKey(t *testing.T) {
	const apiKey = "tmdb-private-api-value"
	application, err := New(config.Config{
		PublicURL: "http://localhost", WebhookToken: "secret",
		Admin: config.AdminConfig{Username: "admin", Password: "password"},
		TMDB:  config.TMDBConfig{APIKey: apiKey},
	})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.NotContains(t, response.Body.String(), "tmdbConfigured")

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetBasicAuth("admin", "password")
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "const tmdbConfigured=true")
	assert.NotContains(t, response.Body.String(), apiKey)
}

func TestHTTPAuthenticationWebhookStreamAndEmbyDelete(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	videoPath := filepath.Join(source, "movie.mkv")
	require.NoError(t, os.WriteFile(videoPath, []byte("video-data"), 0o644))
	cfg := config.Config{
		PublicURL: "http://localhost:8024", WebhookToken: "webhook-secret",
		Admin:    config.AdminConfig{Username: "admin", Password: "password"},
		Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: source}},
		Tasks:    []config.TaskConfig{{ID: "movies", StorageID: "media", Source: "/", Destination: destination}},
	}
	application, err := New(cfg)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, application.Start(ctx))

	request := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusUnauthorized, response.Code)

	request = httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	request.SetBasicAuth("admin", "password")
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"movies"`)

	sig := signature.Create("webhook-secret", "media", "/movie.mkv")
	request = httptest.NewRequest(http.MethodGet, "/stream/media?path=%2Fmovie.mkv&sig="+sig, nil)
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "video-data", response.Body.String())

	request = httptest.NewRequest(http.MethodPost, "/webhook/run?token=webhook-secret", bytes.NewBufferString(`{"task_id":"movies"}`))
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusAccepted, response.Code)

	strmPath := filepath.Join(destination, "movie.strm")
	require.Eventually(t, func() bool {
		_, statErr := os.Stat(strmPath)
		return statErr == nil
	}, time.Second, 10*time.Millisecond)
	body := `{"Event":"item.deleted","Item":{"Path":` + quoteJSON(strmPath) + `}}`
	request = httptest.NewRequest(http.MethodPost, "/webhook/emby?token=webhook-secret", bytes.NewBufferString(body))
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	_, err = os.Stat(videoPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(strmPath)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRobotsDisallowsIndexingWithoutAuthentication(t *testing.T) {
	application, err := New(config.Config{
		PublicURL:    "https://media.example",
		Admin:        config.AdminConfig{Username: "admin", Password: "password"},
		WebhookToken: "robots-test-token",
	})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "text/plain; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Equal(t, "public, max-age=86400", response.Header().Get("Cache-Control"))
	assert.Empty(t, response.Header().Get("WWW-Authenticate"))
	assert.Equal(t, "User-agent: *\nDisallow: /\n", response.Body.String())

	request = httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, "public, max-age=86400", response.Header().Get("Cache-Control"))
	assert.Empty(t, response.Header().Get("WWW-Authenticate"))
	assert.Empty(t, response.Body.String())
}

func TestFileIDStreamAndUnsupportedStorageValidation(t *testing.T) {
	instance := &stableIDAppStorage{fileID: "provider-file-123", path: "/Episode.mp4", directURL: "https://media.example/Episode.mp4"}
	application := &App{config: config.Config{PublicURL: "http://media.example", WebhookToken: "id-secret"}, storages: map[string]storage.Storage{"stable": instance}}
	application.handler = application.routes()
	request := httptest.NewRequest(http.MethodGet, "/api/storages/stable/stream-url?path=%2FEpisode.mp4&file_id=true", nil)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	var payload map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	signedURL, err := url.Parse(payload["url"])
	require.NoError(t, err)
	assert.Empty(t, signedURL.Query().Get("path"))
	assert.Equal(t, "provider-file-123", signedURL.Query().Get("id"))
	request = httptest.NewRequest(http.MethodGet, signedURL.RequestURI(), nil)
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusFound, response.Code)
	assert.Equal(t, "https://media.example/Episode.mp4", response.Header().Get("Location"))
	assert.Equal(t, int32(1), instance.resolveCalls.Load())
	instance.directURL = "https://user:direct-secret@media.example/Episode.mp4#fragment-secret"
	request = httptest.NewRequest(http.MethodGet, signedURL.RequestURI(), nil)
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadGateway, response.Code)
	assert.Empty(t, response.Header().Get("Location"))
	assert.NotContains(t, response.Body.String(), "direct-secret")
	assert.NotContains(t, response.Body.String(), "fragment-secret")
	resolveCallsBeforeInvalidSignature := instance.resolveCalls.Load()
	request = httptest.NewRequest(http.MethodGet, "/stream/stable?id=attacker&sig=invalid", nil)
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Equal(t, resolveCallsBeforeInvalidSignature, instance.resolveCalls.Load(), "invalid signatures must not trigger provider ID resolution")

	_, err = New(config.Config{PublicURL: "http://media.example", WebhookToken: "id-secret", Storages: []config.StorageConfig{{ID: "local", Type: "local", Root: t.TempDir()}}, Tasks: []config.TaskConfig{{ID: "bad", StorageID: "local", Destination: t.TempDir(), FileIDMode: true}}})
	require.ErrorContains(t, err, "does not support stable file IDs")
}

type stableIDAppStorage struct {
	fileID       string
	path         string
	directURL    string
	resolveCalls atomic.Int32
}

func (s *stableIDAppStorage) List(context.Context, string) ([]storage.Entry, error) { return nil, nil }
func (s *stableIDAppStorage) Mkdir(context.Context, string) error                   { return nil }
func (s *stableIDAppStorage) Rename(context.Context, string, string) error          { return nil }
func (s *stableIDAppStorage) Move(context.Context, string, string) error            { return nil }
func (s *stableIDAppStorage) Delete(context.Context, string) error                  { return nil }
func (s *stableIDAppStorage) DirectURL(context.Context, string) (string, error) {
	return s.directURL, nil
}
func (s *stableIDAppStorage) FileID(context.Context, string) (string, error) { return s.fileID, nil }
func (s *stableIDAppStorage) ResolveFileID(context.Context, string) (string, error) {
	s.resolveCalls.Add(1)
	return s.path, nil
}

func TestStorageBrowserAPI(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "Episode 1.mkv"), []byte("one"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "Episode 2.mkv"), []byte("two"), 0o644))
	application, err := New(config.Config{
		PublicURL: "https://media.example", WebhookToken: "browser-secret",
		Admin:    config.AdminConfig{Username: "admin", Password: "password"},
		Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: root}},
	})
	require.NoError(t, err)

	request := func(method, target, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
		req.SetBasicAuth("admin", "password")
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, req)
		return response
	}

	response := request(http.MethodGet, "/api/storages", "")
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"media"`)
	response = request(http.MethodPost, "/api/storages/media/mkdir", `{"path":"/archive"}`)
	assert.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	response = request(http.MethodPost, "/api/storages/media/rename", `{"path":"/Episode 1.mkv","new_name":"Show S01E01.mkv"}`)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = request(http.MethodPost, "/api/storages/media/move", `{"path":"/Show S01E01.mkv","destination":"/archive"}`)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = request(http.MethodGet, "/api/storages/media/entries?path=/archive", "")
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "Show S01E01.mkv")
	response = request(http.MethodGet, "/api/storages/media/stream-url?path=/archive/Show%20S01E01.mkv", "")
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"https://media.example/stream/media?`)
	assert.Contains(t, response.Body.String(), `sig=`)

	response = request(http.MethodPost, "/api/storages/media/batch-rename", `{"path":"/","mode":"sequence","prefix":"Episode ","start":3,"width":2,"preserve_extension":true,"preview":true}`)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "Episode 03.mkv")
	response = request(http.MethodPost, "/api/storages/media/batch-rename", `{"path":"/","mode":"sequence","prefix":"Episode ","start":3,"width":2,"preserve_extension":true,"preview":false}`)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	_, err = os.Stat(filepath.Join(root, "Episode 03.mkv"))
	require.NoError(t, err)

	response = request(http.MethodDelete, "/api/storages/media/entry?path=/Episode%2003.mkv", "")
	assert.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	_, err = os.Stat(filepath.Join(root, "Episode 03.mkv"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	response = request(http.MethodPost, "/api/storages/media/rename", `{"path":"/Episode 2.mkv","new_name":"../escape.mkv"}`)
	assert.Equal(t, http.StatusBadGateway, response.Code)
}

func TestStreamURLPreservesLiteralPercentSequences(t *testing.T) {
	root := t.TempDir()
	const name = "100% ready %23 %2F.mkv"
	require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("percent-video"), 0o644))
	application, err := New(config.Config{
		PublicURL: "http://media.example", WebhookToken: "percent-secret",
		Admin:    config.AdminConfig{Username: "admin", Password: "password"},
		Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: root}},
	})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodGet, "/api/storages/media/stream-url?path="+url.QueryEscape("/"+name), nil)
	request.SetBasicAuth("admin", "password")
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	parsed, err := url.Parse(result["url"])
	require.NoError(t, err)
	assert.Contains(t, parsed.RawQuery, "100%25+ready+%2523+%252F.mkv")
	assert.Equal(t, "/"+name, parsed.Query().Get("path"))

	request = httptest.NewRequest(http.MethodGet, parsed.RequestURI(), nil)
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "percent-video", response.Body.String())

	tampered := strings.Replace(parsed.RequestURI(), "%2523", "%23", 1)
	require.NotEqual(t, parsed.RequestURI(), tampered)
	request = httptest.NewRequest(http.MethodGet, tampered, nil)
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestStreamURLPreservesProviderTrailingSpaces(t *testing.T) {
	root := t.TempDir()
	const name = "movie.mkv "
	require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("space-video"), 0o644))
	application, err := New(config.Config{
		PublicURL: "http://media.example", WebhookToken: "space-secret",
		Admin:    config.AdminConfig{Username: "admin", Password: "password"},
		Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: root}},
	})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "/api/storages/media/stream-url?path="+url.QueryEscape("/"+name), nil)
	request.SetBasicAuth("admin", "password")
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	parsed, err := url.Parse(result["url"])
	require.NoError(t, err)
	assert.Equal(t, "/"+name, parsed.Query().Get("path"))
	request = httptest.NewRequest(http.MethodGet, parsed.RequestURI(), nil)
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "space-video", response.Body.String())
}

func TestTaskSTRMPreviewAPI(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "shows"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "shows", "Demo Episode.mkv"), []byte("video"), 0o644))
	application, err := New(config.Config{
		PublicURL: "https://media.example", WebhookToken: "preview-secret",
		Admin:    config.AdminConfig{Username: "admin", Password: "password"},
		Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: root}},
		Tasks: []config.TaskConfig{{
			ID: "shows", StorageID: "media", Source: "/shows", Destination: destination, MediaExt: []string{".mkv"},
			Plugins: []config.PluginConfig{{Type: "custom_strm_filename", Template: "preview-{name}.strm"}},
		}},
	})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodGet, "/api/tasks/shows/preview?path=/shows/Demo%20Episode.mkv", nil)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusUnauthorized, response.Code)

	request = httptest.NewRequest(http.MethodGet, "/api/tasks/shows/preview?path=/shows/Demo%20Episode.mkv", nil)
	request.SetBasicAuth("admin", "password")
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"target":"preview-Demo Episode.strm"`)
	assert.Contains(t, response.Body.String(), `https://media.example/stream/media?`)
	assert.NotContains(t, response.Body.String(), destination)
	entries, err := os.ReadDir(destination)
	require.NoError(t, err)
	assert.Empty(t, entries)

	request = httptest.NewRequest(http.MethodGet, "/api/tasks/shows/preview?path=/outside.mkv", nil)
	request.SetBasicAuth("admin", "password")
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestPublicWebhookProtocolOverridesTask(t *testing.T) {
	root := t.TempDir()
	destination := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "quark", "saved"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "quark", "saved", "Demo WEB.xyz"), []byte("video"), 0o644))
	application, err := New(config.Config{
		PublicURL: "https://media.example", WebhookToken: "public-secret",
		Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: root}},
		Tasks:    []config.TaskConfig{{ID: "movie", Name: "Movie Task", StorageID: "media", Source: "/quark", Destination: destination, MediaExt: []string{".mkv"}}},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, application.Start(ctx))

	payload := `{
		"event":"a_task","extra_external_field":"accepted",
		"task":{"name":"Movie Task","storage_path":"/quark/saved","incremental":true,"plugins":{"replace_regex":{"pattern":" WEB","replacement":""}}},
		"strm":{"media_ext":["xyz"],"copy_ext":["nfo"],"media_size":0}
	}`
	request := httptest.NewRequest(http.MethodPost, "/webhook/public-secret", bytes.NewBufferString(payload))
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	require.Eventually(t, func() bool {
		_, statErr := os.Stat(filepath.Join(destination, "saved", "Demo.strm"))
		return statErr == nil
	}, time.Second, 10*time.Millisecond)

	qPayload := `{"event":"cs_strm","strmtask":"movie","savepath":"/saved","xlist_path_fix":"/quark:/"}`
	request = httptest.NewRequest(http.MethodPost, "/webhook/public-secret", bytes.NewBufferString(qPayload))
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"movie"`)

	request = httptest.NewRequest(http.MethodPost, "/webhook/wrong-secret", bytes.NewBufferString(payload))
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestWebhookHelpers(t *testing.T) {
	assert.Equal(t, "/quark/saved/movie", applyPathFix("/saved/movie", "/quark:/"))
	assert.Equal(t, "/other", applyPathFix("/other", "/quark:/saved"))
	assert.Equal(t, []string{".mkv", ".mp4"}, normalizeExtensions([]string{"mkv", ".MP4", ""}))
	plugins, err := webhookPlugins(map[string]map[string]any{"skip_regex": {"pattern": "sample"}})
	require.NoError(t, err)
	require.Len(t, plugins, 1)
	_, err = webhookPlugins(map[string]map[string]any{"unknown": {"pattern": "x"}})
	require.Error(t, err)
	plugins, err = webhookPlugins(map[string]map[string]any{
		"filename_skip": {"pattern": "extras", "match_mode": "literal", "filter_mode": "include", "directory_only": true, "case_sensitive": true},
		"skip_regex":    {"pattern": "sample"},
		"filename":      {"pattern": "first", "replacement": "second"},
		"replace_regex": {"pattern": "third", "replacement": "fourth"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"filename", "filename_skip", "replace_regex", "skip_regex"}, []string{plugins[0].Type, plugins[1].Type, plugins[2].Type, plugins[3].Type})
	assert.Equal(t, config.PluginConfig{Type: "filename_skip", Pattern: "extras", MatchMode: "literal", FilterMode: "include", DirectoryOnly: true, CaseSensitive: true}, plugins[1])
	_, err = webhookPlugins(map[string]map[string]any{"filename_skip": {"pattern": "[", "match_mode": "regex"}})
	require.ErrorContains(t, err, "pattern")
	_, err = webhookPlugins(map[string]map[string]any{"filename_skip": {"pattern": "x", "filter_mode": "keep"}})
	require.ErrorContains(t, err, "filter_mode")
	_, err = webhookPlugins(map[string]map[string]any{"filename_skip": {"pattern": "x", "directory_only": "true"}})
	require.ErrorContains(t, err, "boolean")
	assert.Equal(t, []string{"custom", "*.nfo", "*.jpg", "*.png"}, appendUnique([]string{"custom", "*.nfo"}, "*.nfo", "*.jpg", "*.png"))
}

func TestPublicWebhookBatchValidationAndCORS(t *testing.T) {
	application, err := New(config.Config{
		PublicURL: "https://media.example", WebhookToken: "public-cors-value",
		Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: t.TempDir()}},
		Tasks:    []config.TaskConfig{{ID: "movie", Name: "Movie Task", StorageID: "media", Source: "/", Destination: t.TempDir()}},
	})
	require.NoError(t, err)
	preflight := httptest.NewRequest(http.MethodOptions, "/webhook/public-cors-value", nil)
	preflight.Header.Set("Origin", "https://docs.example")
	preflight.Header.Set("Access-Control-Request-Method", "POST")
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, preflight)
	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, response.Header().Get("Access-Control-Allow-Headers"), "Content-Type")

	payload := `{"event":"cs_strm","strmtask":"movie,missing","savepath":"/"}`
	request := httptest.NewRequest(http.MethodPost, "/webhook/public-cors-value", strings.NewReader(payload))
	request.Header.Set("Origin", "https://docs.example")
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
	statuses := application.manager.Statuses()
	require.Len(t, statuses, 1)
	assert.Zero(t, statuses[0].Queued, "validation failure must not partially enqueue the valid task")

	request = httptest.NewRequest(http.MethodPost, "/webhook/wrong-value", strings.NewReader(`{"event":"a_task","task":{"name":"movie"}}`))
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestPublicWebhookOverridesAssetRetentionWithoutEnablingDelete(t *testing.T) {
	t.Run("false clears task retention for an explicitly deleting task", func(t *testing.T) {
		source, destination := t.TempDir(), t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(destination, "stale.nfo"), []byte("metadata"), 0o644))
		application, err := New(config.Config{
			PublicURL: "https://media.example", WebhookToken: "asset-false-value",
			Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: source}},
			Tasks: []config.TaskConfig{{ID: "movie", Name: "Movie", StorageID: "media", Source: "/", Destination: destination,
				SyncDelete: true, Incremental: true, CopyExt: []string{".nfo"}, KeepLocal: []string{"*.nfo"}}},
		})
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		require.NoError(t, application.Start(ctx))
		payload := `{"event":"a_task","task":{"name":"Movie","incremental":false,"keep_local_asset":false}}`
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/webhook/asset-false-value", strings.NewReader(payload)))
		require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
		require.Eventually(t, func() bool {
			_, statErr := os.Stat(filepath.Join(destination, "stale.nfo"))
			return os.IsNotExist(statErr)
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("false does not implicitly enable sync delete", func(t *testing.T) {
		source, destination := t.TempDir(), t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(destination, "stale.nfo"), []byte("metadata"), 0o644))
		application, err := New(config.Config{
			PublicURL: "https://media.example", WebhookToken: "asset-safe-value",
			Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: source}},
			Tasks: []config.TaskConfig{{ID: "movie", Name: "Movie", StorageID: "media", Source: "/", Destination: destination,
				SyncDelete: false, Incremental: true, CopyExt: []string{".nfo"}, KeepLocal: []string{"*.nfo"}}},
		})
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		require.NoError(t, application.Start(ctx))
		payload := `{"event":"a_task","task":{"name":"Movie","incremental":false,"keep_local_asset":false}}`
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/webhook/asset-safe-value", strings.NewReader(payload)))
		require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
		require.Eventually(t, func() bool {
			status := application.manager.Statuses()[0]
			return !status.Running && status.EndedAt.Unix() > 0
		}, time.Second, 10*time.Millisecond)
		_, statErr := os.Stat(filepath.Join(destination, "stale.nfo"))
		require.NoError(t, statErr)
	})
}

func TestTaskToolboxAPI(t *testing.T) {
	source, destination := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "movie.mkv"), []byte("video"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(destination, "movie.strm"), []byte("http://old/movie\n"), 0o644))
	application, err := New(config.Config{
		PublicURL: "https://media.example", WebhookToken: "tool-secret",
		Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: source}},
		Tasks:    []config.TaskConfig{{ID: "movie", StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"}, Incremental: true}},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, application.Start(ctx))
	call := func(method, target, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, req)
		return response
	}
	response := call(http.MethodPost, "/api/tasks/movie/replace-content", `{"from":"http://old","to":"https://new","preview":true}`)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"changed":1`)
	data, err := os.ReadFile(filepath.Join(destination, "movie.strm"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "http://old")
	response = call(http.MethodPost, "/api/tasks/movie/replace-content", `{"from":"http://old","to":"https://new","preview":false}`)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	data, err = os.ReadFile(filepath.Join(destination, "movie.strm"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "https://new")
	response = call(http.MethodPost, "/api/tasks/movie/full-overwrite", `{}`)
	assert.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	require.Eventually(t, func() bool {
		return !application.manager.Statuses()[0].Running && application.manager.Statuses()[0].Result.Created == 1
	}, time.Second, 10*time.Millisecond)
	response = call(http.MethodDelete, "/api/tasks/movie/generated", "")
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"removed":1`)
}

func TestCloudDrive2WebhookMappingAndTaskMatch(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "library", "movies", "New Movie"), 0o755))
	application, err := New(config.Config{
		PublicURL: "https://media.example", WebhookToken: "cd2-secret",
		Storages:     []config.StorageConfig{{ID: "media", Type: "local", Root: root}},
		Tasks:        []config.TaskConfig{{ID: "movies", StorageID: "media", Source: "/library/movies", Destination: destination}},
		Integrations: config.IntegrationConfig{CloudDrive2: map[string]string{"/": "fallback", "/115open": "media/library"}},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, application.Start(ctx))
	payload := `{"event_category":"FileSystem","event_name":"Changed","data":[{"action":"create","is_dir":"false","source_file":"/115open/movies/New Movie/video.mkv","destination_file":""}]}`
	request := httptest.NewRequest(http.MethodPost, "/webhook/cd2-secret/file_notify", bytes.NewBufferString(payload))
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"movies"`)
	require.Eventually(t, func() bool { return application.manager.Statuses()[0].EndedAt.After(time.Time{}) }, time.Second, 10*time.Millisecond)

	ignored := `{"data":[{"action":"create","is_dir":true,"source_file":"/unknown/path"}]}`
	request = httptest.NewRequest(http.MethodPost, "/webhook/cd2-secret/file_notify", bytes.NewBufferString(ignored))
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"ignored"`)
}

func TestCloudDrivePathMapping(t *testing.T) {
	storageID, remotePath, ok := mapCloudDrivePath("/115open/movies/demo", map[string]string{"/": "root", "/115open": "media/library"})
	assert.True(t, ok)
	assert.Equal(t, "media", storageID)
	assert.Equal(t, "/library/movies/demo", remotePath)
	assert.True(t, valueIsTrue("true"))
	assert.True(t, valueIsTrue(float64(1)))
	assert.False(t, valueIsTrue("false"))
}

func TestMoviePilotWebhookMappingAndTaskMatch(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "movie", "Demo"), 0o755))
	application, err := New(config.Config{
		PublicURL: "https://media.example", WebhookToken: "mp-secret",
		Storages:     []config.StorageConfig{{ID: "openlist", Type: "local", Root: root}},
		Tasks:        []config.TaskConfig{{ID: "movie", StorageID: "openlist", Source: "/movie", Destination: destination}},
		Integrations: config.IntegrationConfig{MoviePilot: map[string]string{"/media/movie": "openlist/movie"}},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, application.Start(ctx))
	payload := `{"type":"transfer.complete","data":{"transferinfo":{"target_diritem":{"path":"/media/movie/Demo"},"file_list_new":["/media/movie/Demo/video.mkv"]}}}`
	request := httptest.NewRequest(http.MethodPost, "/webhook/mp-secret/moviepilot", bytes.NewBufferString(payload))
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"movie"`)
	require.Eventually(t, func() bool { return application.manager.Statuses()[0].EndedAt.After(time.Time{}) }, time.Second, 10*time.Millisecond)

	request = httptest.NewRequest(http.MethodPost, "/webhook/mp-secret/moviepilot", bytes.NewBufferString(`{"type":"download.added","data":{}}`))
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), `"ignored"`)
}

func TestExternalPathMapping(t *testing.T) {
	storageID, remotePath, ok := mapExternalPath(`/media/movie/Demo/video.mkv`, map[string]string{"/media": "fallback", "/media/movie": "openlist/movie"})
	assert.True(t, ok)
	assert.Equal(t, "openlist", storageID)
	assert.Equal(t, "/movie/Demo/video.mkv", remotePath)
}

func TestTMDBSearchDetailsAndRenameAPI(t *testing.T) {
	tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/tv":
			_, _ = w.Write([]byte(`{"results":[{"id":9,"name":"示例剧","original_name":"Demo Series","first_air_date":"2025-01-02"}]}`))
		case "/tv/9":
			_, _ = w.Write([]byte(`{"id":9,"name":"示例剧","original_name":"Demo Series","first_air_date":"2025-01-02"}`))
		case "/movie/10":
			_, _ = w.Write([]byte(`{"id":10,"title":"示例电影","original_title":"Demo Movie","release_date":"2024-03-04"}`))
		case "/movie/11":
			_, _ = w.Write([]byte(`{"id":11,"title":"仅显示标题","release_date":"2023-05-06"}`))
		case "/tv/9/season/1/episode/2":
			_, _ = w.Write([]byte(`{"id":92,"name":"第二集","season_number":1,"episode_number":2}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer tmdbServer.Close()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "Demo.S1E2.mkv"), []byte("video"), 0o644))
	application, err := New(config.Config{
		PublicURL: "https://media.example", WebhookToken: "tmdb-secret",
		Storages: []config.StorageConfig{{ID: "media", Type: "local", Root: root}},
		TMDB:     config.TMDBConfig{APIKey: "key", BaseURL: tmdbServer.URL, Language: "zh-CN"},
	})
	require.NoError(t, err)
	call := func(method, target, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, req)
		return response
	}
	response := call(http.MethodGet, "/api/tmdb/search?query=Demo&type=tv&year=2025", "")
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "示例剧")
	response = call(http.MethodGet, "/api/tmdb/tv/9", "")
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "Demo Series")
	moviePayload := `{"path":"/","media_type":"movie","tmdb_id":10,"template":"{title} ({title_original}) ({year}){ext}","preview":true}`
	response = call(http.MethodPost, "/api/storages/media/tmdb-rename", moviePayload)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "示例电影 (Demo Movie) (2024).mkv")
	fallbackPayload := `{"path":"/","media_type":"movie","tmdb_id":11,"template":"{title_original} ({year}){ext}","preview":true}`
	response = call(http.MethodPost, "/api/storages/media/tmdb-rename", fallbackPayload)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "仅显示标题 (2023).mkv")
	payload := `{"path":"/","media_type":"tv","tmdb_id":9,"template":"{title} ({title_original}) - S{season:2}E{episode:2}{ext}","preview":true}`
	response = call(http.MethodPost, "/api/storages/media/tmdb-rename", payload)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "示例剧 (Demo Series) - S01E02.mkv")
	payload = strings.Replace(payload, `"preview":true`, `"preview":false`, 1)
	response = call(http.MethodPost, "/api/storages/media/tmdb-rename", payload)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	_, err = os.Stat(filepath.Join(root, "示例剧 (Demo Series) - S01E02.mkv"))
	require.NoError(t, err)
}

func TestTMDBPosterAPI(t *testing.T) {
	images := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/poster.jpg", r.URL.Path)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("jpeg-data"))
	}))
	defer images.Close()
	application, err := New(config.Config{PublicURL: "http://localhost", WebhookToken: "poster-secret", TMDB: config.TMDBConfig{APIKey: "test", BaseURL: images.URL, ImageBaseURL: images.URL}})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "/api/tmdb/poster?path=%2Fposter.jpg", nil)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "image/jpeg", response.Header().Get("Content-Type"))
	assert.Equal(t, "private, max-age=86400", response.Header().Get("Cache-Control"))
	assert.Equal(t, "jpeg-data", response.Body.String())
	request = httptest.NewRequest(http.MethodGet, "/api/tmdb/poster?path=..%2Fsecret.jpg", nil)
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadGateway, response.Code)
}

func TestConfigPersistenceAPIRedactsPreservesResetsAndRestores(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	disk := config.Config{Version: config.CurrentVersion, Listen: ":8024", PublicURL: "http://localhost:8024", WebhookToken: "original-secret", Admin: config.AdminConfig{Username: "admin", Password: "admin-secret"}, TMDB: config.TMDBConfig{APIKey: "tmdb-secret"}}
	require.NoError(t, config.Save(configPath, disk))
	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	application, err := New(cfg)
	require.NoError(t, err)
	call := func(method, target, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
		req.SetBasicAuth("admin", "admin-secret")
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, req)
		return response
	}
	response := call(http.MethodGet, "/api/config", "")
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.NotContains(t, response.Body.String(), "original-secret")
	assert.NotContains(t, response.Body.String(), "admin-secret")
	assert.Contains(t, response.Body.String(), config.RedactedSecret)

	redacted := disk.Redacted()
	redacted.PublicURL = "http://new.example"
	payload, err := json.Marshal(redacted)
	require.NoError(t, err)
	response = call(http.MethodPut, "/api/config", string(payload))
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"restart_required":true`)
	updated, err := config.LoadDisk(configPath)
	require.NoError(t, err)
	assert.Equal(t, "original-secret", updated.WebhookToken)
	assert.Equal(t, "admin-secret", updated.Admin.Password)
	assert.Equal(t, "http://new.example", updated.PublicURL)

	response = call(http.MethodPost, "/api/config/webhook-token", "")
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.NotContains(t, response.Body.String(), "original-secret")
	reset, err := config.LoadDisk(configPath)
	require.NoError(t, err)
	assert.NotEqual(t, "original-secret", reset.WebhookToken)
	assert.Len(t, reset.WebhookToken, 43)

	response = call(http.MethodPost, "/api/config/restore", "")
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	restored, err := config.LoadDisk(configPath)
	require.NoError(t, err)
	assert.Equal(t, "original-secret", restored.WebhookToken)
	assert.Equal(t, "http://new.example", restored.PublicURL)

	response = call(http.MethodPut, "/api/config", string(payload)+` {}`)
	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
}

func TestManagedConfigAPIUpdatesBusinessSettingsOnly(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	disk := config.Config{
		Version:      config.CurrentVersion,
		Listen:       ":8024",
		PublicURL:    "http://localhost:8024",
		WebhookToken: "webhook-secret",
		Admin:        config.AdminConfig{Username: "admin", Password: "admin-secret"},
		Storages:     []config.StorageConfig{{ID: "dav", Type: "webdav", Endpoint: "https://dav.example", Root: "/media", Password: "dav-secret"}},
	}
	require.NoError(t, config.Save(configPath, disk))
	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	application, err := New(cfg)
	require.NoError(t, err)
	call := func(method, target, payload string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, target, bytes.NewBufferString(payload))
		request.SetBasicAuth("admin", "admin-secret")
		if payload != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, request)
		return response
	}

	response := call(http.MethodGet, "/api/config/managed", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.NotContains(t, response.Body.String(), "webhook-secret")
	assert.NotContains(t, response.Body.String(), "admin-secret")
	assert.NotContains(t, response.Body.String(), "dav-secret")
	assert.Contains(t, response.Body.String(), `"admin_username":"admin"`)

	managed := disk.Redacted().Managed()
	managed.Storages = append(managed.Storages, config.StorageConfig{ID: "local", Type: "local", Root: "/media"})
	managed.Tasks = []config.TaskConfig{{ID: "movies", Name: "Movies", StorageID: "local", Source: "/movies", Destination: "/strm/movies", MediaExt: []string{".mkv"}}}
	payload, err := json.Marshal(managed)
	require.NoError(t, err)
	response = call(http.MethodPut, "/api/config/managed", string(payload))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Contains(t, response.Body.String(), `"restart_required":true`)

	updated, err := config.LoadDisk(configPath)
	require.NoError(t, err)
	assert.Equal(t, ":8024", updated.Listen)
	assert.Equal(t, "http://localhost:8024", updated.PublicURL)
	assert.Equal(t, "webhook-secret", updated.WebhookToken)
	assert.Equal(t, "admin-secret", updated.Admin.Password)
	assert.Equal(t, "dav-secret", updated.Storages[0].Password)
	assert.Len(t, updated.Storages, 2)
	assert.Len(t, updated.Tasks, 1)

	response = call(http.MethodPut, "/api/config/managed", `{"storages":[],"tasks":[],"listen":":9000"}`)
	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	unchanged, err := config.LoadDisk(configPath)
	require.NoError(t, err)
	assert.Equal(t, ":8024", unchanged.Listen)
	assert.Len(t, unchanged.Storages, 2)
}

func TestPublicWebhookOpenListRefresh(t *testing.T) {
	var refreshed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if r.URL.Path == "/api/fs/list" {
			if value, _ := body["refresh"].(bool); value {
				refreshed.Store(true)
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"content":[]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	application, err := New(config.Config{PublicURL: "http://localhost", WebhookToken: "refresh-secret", Storages: []config.StorageConfig{{ID: "openlist", Type: "openlist", Endpoint: server.URL}}, Tasks: []config.TaskConfig{{ID: "refresh", StorageID: "openlist", Source: "/media", Destination: t.TempDir()}}})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, application.Start(ctx))
	request := httptest.NewRequest(http.MethodPost, "/webhook/refresh-secret", bytes.NewBufferString(`{"event":"qas_strm","strmtask":"refresh","savepath":"/media/new","refresh":true}`))
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	assert.True(t, refreshed.Load())
}

func TestWebSavePublicUserscriptProtocol(t *testing.T) {
	var refreshPath atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if r.URL.Path == "/api/fs/list" {
			if refresh, _ := body["refresh"].(bool); refresh {
				refreshPath.Store(body["path"])
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"content":[]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	application, err := New(config.Config{
		PublicURL: "http://localhost", WebhookToken: "web-save-value",
		Storages: []config.StorageConfig{
			{ID: "quark-a", Type: "quark", Endpoint: server.URL},
			{ID: "quark-b", Type: "quark", Endpoint: server.URL},
			{ID: "other", Type: "115", Endpoint: server.URL},
		},
		Tasks: []config.TaskConfig{
			{ID: "root", Name: "Root", StorageID: "quark-a", Source: "/media", Destination: t.TempDir()},
			{ID: "movies", Name: "Movies", StorageID: "quark-a", Source: "/media/movies", Destination: t.TempDir()},
			{ID: "unrelated", Name: "Other", StorageID: "other", Source: "/media/movies", Destination: t.TempDir()},
		},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, application.Start(ctx))
	request := httptest.NewRequest(http.MethodPost, "/webhook/web-save-value", strings.NewReader(`{"event":"web_save","delay":0,"data":{"driver":"quark","savepath":"/media/movies/New Film"}}`))
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	assert.JSONEq(t, `{"success":true,"message":"matching task queued","task":{"name":"Movies","storage_path":"/media/movies/New Film"}}`, response.Body.String())
	assert.Equal(t, "/media/movies/New Film", refreshPath.Load())
	require.Eventually(t, func() bool {
		status := application.manager.Statuses()
		for _, item := range status {
			if item.TaskID == "movies" {
				return item.EndedAt.Unix() > 0
			}
		}
		return false
	}, time.Second, 10*time.Millisecond)
}

func TestWebSaveRejectsUnknownAmbiguousAndInvalidRequests(t *testing.T) {
	application, err := New(config.Config{
		PublicURL: "http://localhost", WebhookToken: "web-save-errors-value",
		Storages: []config.StorageConfig{
			{ID: "quark-a", Type: "quark", Endpoint: "http://127.0.0.1:1"},
			{ID: "quark-b", Type: "quark", Endpoint: "http://127.0.0.1:1"},
		},
		Tasks: []config.TaskConfig{
			{ID: "a", Name: "A", StorageID: "quark-a", Source: "/same", Destination: t.TempDir()},
			{ID: "b", Name: "B", StorageID: "quark-b", Source: "/same", Destination: t.TempDir()},
		},
	})
	require.NoError(t, err)
	tests := []struct {
		body   string
		status int
		text   string
	}{
		{`{"event":"web_save","data":{"driver":"quark","savepath":""}}`, http.StatusBadRequest, "path is invalid"},
		{`{"event":"web_save","data":{"driver":"unknown","savepath":"/same"}}`, http.StatusBadRequest, "unsupported"},
		{`{"event":"web_save","data":{"driver":"quark","savepath":"/same/movie"}}`, http.StatusConflict, "multiple"},
		{`{"event":"web_save","data":{"driver":"open115","savepath":"/other"}}`, http.StatusNotFound, "no task"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodPost, "/webhook/web-save-errors-value", strings.NewReader(test.body))
		response := httptest.NewRecorder()
		application.Handler().ServeHTTP(response, request)
		assert.Equal(t, test.status, response.Code, response.Body.String())
		assert.Contains(t, response.Body.String(), `"success":false`)
		assert.Contains(t, response.Body.String(), test.text)
	}
	for _, status := range application.manager.Statuses() {
		assert.Zero(t, status.Queued)
	}
}

func TestExtractTaskCoversAPI(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "movie.strm"), []byte("http://media.example/stream/local?sig=x"), 0o644))
	binary := filepath.Join(t.TempDir(), "fake-ffmpeg")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nfor last; do :; done\nprintf jpeg > \"$last\"\n"), 0o755))
	application, err := New(config.Config{PublicURL: "http://media.example", WebhookToken: "cover-secret", MediaTools: config.MediaToolsConfig{FFmpeg: binary, TimeoutSeconds: 2}, Storages: []config.StorageConfig{{ID: "local", Type: "local", Root: t.TempDir()}}, Tasks: []config.TaskConfig{{ID: "covers", StorageID: "local", Source: "/", Destination: root}}})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/tasks/covers/covers", bytes.NewBufferString(`{"position":"5s","preview":true}`))
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"planned":1`)
	request = httptest.NewRequest(http.MethodPost, "/api/tasks/covers/covers", bytes.NewBufferString(`{"position":"5s","preview":false}`))
	response = httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	_, err = os.Stat(filepath.Join(root, "movie.jpg"))
	require.NoError(t, err)
}

func TestHistoryAPIAndSSE(t *testing.T) {
	application, err := New(config.Config{PublicURL: "http://localhost", WebhookToken: "history-secret"})
	require.NoError(t, err)
	require.NoError(t, application.history.Record(history.Event{TaskID: "demo", Type: "completed", Created: 2}))
	request := httptest.NewRequest(http.MethodGet, "/api/history?task_id=demo&limit=10", nil)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"created":2`)
	server := httptest.NewServer(application.Handler())
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/history/stream?task_id=demo", nil)
	require.NoError(t, err)
	stream, err := server.Client().Do(request)
	require.NoError(t, err)
	defer stream.Body.Close()
	assert.Equal(t, "text/event-stream", stream.Header.Get("Content-Type"))
	assert.Equal(t, "no-store", stream.Header.Get("Cache-Control"))
	scanner := bufio.NewScanner(stream.Body)
	lines := make([]string, 0, 4)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if scanner.Text() == "" {
			break
		}
	}
	require.NoError(t, scanner.Err())
	text := strings.Join(lines, "\n")
	assert.Contains(t, text, "event: history")
	assert.Contains(t, text, `"task_id":"demo"`)
	cancel()
}

func quoteJSON(value string) string {
	var buffer bytes.Buffer
	buffer.WriteByte('"')
	for _, character := range value {
		if character == '\\' || character == '"' {
			buffer.WriteByte('\\')
		}
		buffer.WriteRune(character)
	}
	buffer.WriteByte('"')
	return buffer.String()
}
