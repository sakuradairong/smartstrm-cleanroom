package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAllKeepsLegacy123AliasAndAddsOfficialDriver(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	storages, err := BuildAll([]config.StorageConfig{
		{ID: "legacy", Type: "123", Endpoint: server.URL},
		{ID: "official", Type: "123_open", ClientID: "id", ClientSecret: "secret", RootFileID: "777"},
	})
	require.NoError(t, err)
	assert.IsType(t, &OpenList{}, storages["legacy"])
	official, ok := storages["official"].(*Pan123Open)
	require.True(t, ok)
	assert.Equal(t, int64(777), official.rootFileID)
}

func TestPan123OpenOfficialAPI(t *testing.T) {
	var tokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/access_token" {
			tokenCalls.Add(1)
			assert.Equal(t, "open_platform", r.Header.Get("Platform"))
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "client-id", body["clientID"])
			assert.Equal(t, "client-secret", body["clientSecret"])
			writePan123Response(t, w, map[string]any{
				"accessToken": "official-token",
				"expiredAt":   time.Now().Add(time.Hour).Format(time.RFC3339),
			})
			return
		}

		assert.Equal(t, "Bearer official-token", r.Header.Get("Authorization"))
		assert.Equal(t, "open_platform", r.Header.Get("Platform"))
		switch r.URL.Path {
		case "/api/v2/file/list":
			parentID, err := strconv.ParseInt(r.URL.Query().Get("parentFileId"), 10, 64)
			require.NoError(t, err)
			assert.Equal(t, "100", r.URL.Query().Get("limit"))
			assert.Equal(t, "0", r.URL.Query().Get("lastFileId"))
			files := []map[string]any{}
			switch parentID {
			case 777:
				files = append(files, map[string]any{"fileId": 10, "filename": "movies", "parentFileId": 777, "type": 1, "updateAt": "2026-07-21 12:34:56"})
			case 10:
				files = append(files, map[string]any{"fileId": 20, "filename": "demo.mkv", "parentFileId": 10, "type": 0, "size": 42, "updateAt": "2026-07-21 12:35:56"})
			}
			writePan123Response(t, w, map[string]any{"lastFileId": -1, "fileList": files})
		case "/api/v1/file/detail":
			fileID := r.URL.Query().Get("fileID")
			switch fileID {
			case "20":
				writePan123Response(t, w, map[string]any{"fileID": 20, "filename": "demo.mkv", "parentFileID": 10, "type": 0})
			case "10":
				writePan123Response(t, w, map[string]any{"fileID": 10, "filename": "movies", "parentFileID": 777, "type": 1})
			default:
				writePan123Error(t, w, 404, "missing file secret")
			}
		case "/api/v1/file/download_info":
			assert.Equal(t, "20", r.URL.Query().Get("fileId"))
			writePan123Response(t, w, map[string]any{"downloadUrl": "https://cdn.example/demo.mkv?sign=abc&expires=123"})
		case "/upload/v1/file/mkdir":
			assert.Equal(t, http.MethodPost, r.Method)
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "777", body["parentID"])
			assert.Equal(t, "archive", body["name"])
			writePan123Response(t, w, map[string]any{"dirID": 30})
		case "/api/v1/file/name":
			assert.Equal(t, http.MethodPut, r.Method)
			assertPan123JSONBody(t, r, map[string]any{"fileId": float64(20), "fileName": "renamed.mkv"})
			writePan123Response(t, w, nil)
		case "/api/v1/file/move":
			assertPan123JSONBody(t, r, map[string]any{"fileIDs": []any{float64(20)}, "toParentFileID": float64(777)})
			writePan123Response(t, w, nil)
		case "/api/v1/file/trash":
			assertPan123JSONBody(t, r, map[string]any{"fileIDs": []any{float64(20)}})
			writePan123Response(t, w, nil)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	driver, err := newPan123Open(server.URL, "client-id", "client-secret", "777", false)
	require.NoError(t, err)
	entries, err := driver.List(context.Background(), "/movies")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, Entry{Path: "/movies/demo.mkv", Name: "demo.mkv", Size: 42, ModTime: time.Date(2026, 7, 21, 12, 35, 56, 0, time.FixedZone("UTC+8", 8*60*60))}, entries[0])

	fileID, err := driver.FileID(context.Background(), "/movies/demo.mkv")
	require.NoError(t, err)
	assert.Equal(t, "20", fileID)
	resolved, err := driver.ResolveFileID(context.Background(), fileID)
	require.NoError(t, err)
	assert.Equal(t, "/movies/demo.mkv", resolved)
	directURL, err := driver.DirectURL(context.Background(), "/movies/demo.mkv")
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/demo.mkv?sign=abc&expires=123", directURL)

	require.NoError(t, driver.Mkdir(context.Background(), "/archive"))
	require.NoError(t, driver.Rename(context.Background(), "/movies/demo.mkv", "renamed.mkv"))
	require.NoError(t, driver.Move(context.Background(), "/movies/demo.mkv", "/"))
	require.NoError(t, driver.Delete(context.Background(), "/movies/demo.mkv"))
	assert.Equal(t, int32(1), tokenCalls.Load(), "a valid token must be reused")
}

func TestPan123OpenRefreshesExpiredTokenOnceAcrossConcurrentCalls(t *testing.T) {
	var tokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/access_token":
			tokenCalls.Add(1)
			time.Sleep(20 * time.Millisecond)
			writePan123Response(t, w, map[string]any{"accessToken": "shared-token", "expiredAt": time.Now().Add(time.Hour).Format(time.RFC3339)})
		case "/api/v2/file/list":
			assert.Equal(t, "Bearer shared-token", r.Header.Get("Authorization"))
			writePan123Response(t, w, map[string]any{"lastFileId": -1, "fileList": []any{}})
		}
	}))
	defer server.Close()
	driver, err := newPan123Open(server.URL, "id", "secret", "0", false)
	require.NoError(t, err)

	errors := make(chan error, 8)
	for range 8 {
		go func() {
			_, err := driver.List(context.Background(), "/")
			errors <- err
		}()
	}
	for range 8 {
		require.NoError(t, <-errors)
	}
	assert.Equal(t, int32(1), tokenCalls.Load())
}

func TestPan123OpenPaginatesWithOfficialCursor(t *testing.T) {
	var cursors []string
	server := newPan123TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v2/file/list", r.URL.Path)
		cursor := r.URL.Query().Get("lastFileId")
		cursors = append(cursors, cursor)
		if cursor == "0" {
			writePan123Response(t, w, map[string]any{
				"lastFileId": 11,
				"fileList":   []map[string]any{{"fileId": 11, "filename": "first.mkv", "type": 0}},
			})
			return
		}
		writePan123Response(t, w, map[string]any{
			"lastFileId": -1,
			"fileList":   []map[string]any{{"fileId": 12, "filename": "second.mkv", "type": 0}},
		})
	})
	defer server.Close()
	driver, err := newPan123Open(server.URL, "id", "secret", "0", false)
	require.NoError(t, err)
	entries, err := driver.List(context.Background(), "/")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, []string{"0", "11"}, cursors)
	assert.Equal(t, []string{"first.mkv", "second.mkv"}, []string{entries[0].Name, entries[1].Name})
}

func TestPan123OpenRefreshesRejectedTokenOnceAcrossConcurrentCalls(t *testing.T) {
	var tokenCalls atomic.Int32
	var staleRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/access_token":
			call := tokenCalls.Add(1)
			credential := fmt.Sprintf("credential-%d", call)
			if call > 1 {
				credential = fmt.Sprintf("credential-%d", call)
				time.Sleep(20 * time.Millisecond)
			}
			writePan123Response(t, w, map[string]any{"accessToken": credential, "expiredAt": time.Now().Add(time.Hour).Format(time.RFC3339)})
		case "/api/v2/file/list":
			if r.Header.Get("Authorization") == "Bearer credential-1" {
				staleRequests.Add(1)
				writePan123Error(t, w, http.StatusUnauthorized, "expired")
				return
			}
			assert.Equal(t, "Bearer credential-2", r.Header.Get("Authorization"))
			writePan123Response(t, w, map[string]any{"lastFileId": -1, "fileList": []any{}})
		}
	}))
	defer server.Close()
	driver, err := newPan123Open(server.URL, "id", "secret", "0", false)
	require.NoError(t, err)
	_, err = driver.getAccessToken(context.Background(), "")
	require.NoError(t, err)

	errors := make(chan error, 8)
	for range 8 {
		go func() {
			_, err := driver.List(context.Background(), "/")
			errors <- err
		}()
	}
	for range 8 {
		require.NoError(t, <-errors)
	}
	assert.GreaterOrEqual(t, staleRequests.Load(), int32(1))
	assert.Equal(t, int32(2), tokenCalls.Load(), "concurrent 401 responses must share one refreshed token")
}

func TestPan123OpenRejectsUnsafeDownloadAndRedactsProviderErrors(t *testing.T) {
	t.Run("unsafe download", func(t *testing.T) {
		server := newPan123TestServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v2/file/list":
				writePan123Response(t, w, map[string]any{"lastFileId": -1, "fileList": []map[string]any{{"fileId": 1, "filename": "movie.mkv", "type": 0}}})
			case "/api/v1/file/download_info":
				writePan123Response(t, w, map[string]any{"downloadUrl": "https://user:download-secret@cdn.example/movie.mkv"})
			}
		})
		defer server.Close()
		driver, err := newPan123Open(server.URL, "id", "secret", "0", false)
		require.NoError(t, err)
		_, err = driver.DirectURL(context.Background(), "/movie.mkv")
		require.ErrorContains(t, err, "invalid download URL")
		assert.NotContains(t, err.Error(), "download-secret")
	})

	t.Run("provider error", func(t *testing.T) {
		server := newPan123TestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writePan123Error(t, w, 403, "clientSecret=provider-secret\nsecond-line")
		})
		defer server.Close()
		driver, err := newPan123Open(server.URL, "id", "secret", "0", false)
		require.NoError(t, err)
		_, err = driver.List(context.Background(), "/")
		require.ErrorContains(t, err, "error code 403")
		assert.NotContains(t, err.Error(), "provider-secret")
	})
}

func TestPan123OpenRejectsFileIDOutsideMountedRoot(t *testing.T) {
	server := newPan123TestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/file/detail" {
			writePan123Response(t, w, map[string]any{"fileID": 20, "filename": "outside.mkv", "parentFileID": 999, "type": 0})
		}
	})
	defer server.Close()
	driver, err := newPan123Open(server.URL, "id", "secret", "777", false)
	require.NoError(t, err)
	_, err = driver.ResolveFileID(context.Background(), "20")
	require.ErrorContains(t, err, "outside mounted root")
}

func TestPan123OpenRefusesMutatingMountedRoot(t *testing.T) {
	driver, err := newPan123Open("https://open-api.123pan.com", "id", "secret", "777", false)
	require.NoError(t, err)
	for name, operation := range map[string]func() error{
		"delete": func() error { return driver.Delete(context.Background(), "/") },
		"rename": func() error { return driver.Rename(context.Background(), "/", "renamed") },
		"move":   func() error { return driver.Move(context.Background(), "/", "/archive") },
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, operation(), "mounted root")
		})
	}
}

func newPan123TestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/access_token" {
			writePan123Response(t, w, map[string]any{"accessToken": "token", "expiredAt": time.Now().Add(time.Hour).Format(time.RFC3339)})
			return
		}
		handler(w, r)
	}))
}

func assertPan123JSONBody(t *testing.T, r *http.Request, expected map[string]any) {
	t.Helper()
	var actual map[string]any
	require.NoError(t, json.NewDecoder(r.Body).Decode(&actual))
	assert.Equal(t, expected, actual)
}

func writePan123Response(t *testing.T, w http.ResponseWriter, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok", "data": data}))
}

func writePan123Error(t *testing.T, w http.ResponseWriter, code int, message string) {
	t.Helper()
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"code": code, "message": message, "data": nil}))
}
