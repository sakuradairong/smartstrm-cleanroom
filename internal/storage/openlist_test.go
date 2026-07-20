package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenListAPI(t *testing.T) {
	var refreshCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "token", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		switch r.URL.Path {
		case "/api/fs/list":
			assert.Equal(t, "/cloud/movies", body["path"])
			if refreshed, _ := body["refresh"].(bool); refreshed {
				refreshCalls.Add(1)
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"content":[{"name":"demo.mkv","size":42,"is_dir":false,"modified":"2025-01-02T03:04:05Z"}]}}`))
		case "/api/fs/get":
			_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"raw_url":"https://cdn.example/demo.mkv"}}`))
		case "/api/fs/remove":
			assert.Equal(t, "/cloud/movies", body["dir"])
			_, _ = w.Write([]byte(`{"code":200,"message":"success","data":null}`))
		case "/api/fs/mkdir":
			assert.Equal(t, "/cloud/archive", body["path"])
			_, _ = w.Write([]byte(`{"code":200,"message":"success","data":null}`))
		case "/api/fs/rename":
			assert.Equal(t, "/cloud/movies/demo.mkv", body["path"])
			assert.Equal(t, "renamed.mkv", body["name"])
			_, _ = w.Write([]byte(`{"code":200,"message":"success","data":null}`))
		case "/api/fs/move":
			assert.Equal(t, "/cloud/movies", body["src_dir"])
			assert.Equal(t, "/cloud/archive", body["dst_dir"])
			_, _ = w.Write([]byte(`{"code":200,"message":"success","data":null}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	openList, err := NewOpenList(server.URL, "/cloud", "token")
	require.NoError(t, err)
	entries, err := openList.List(context.Background(), "/movies")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.NoError(t, openList.Refresh(context.Background(), "/movies"))
	assert.Equal(t, int32(1), refreshCalls.Load())
	assert.Equal(t, "/movies/demo.mkv", entries[0].Path)
	directURL, err := openList.DirectURL(context.Background(), entries[0].Path)
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/demo.mkv", directURL)
	require.NoError(t, openList.Mkdir(context.Background(), "/archive"))
	require.NoError(t, openList.Rename(context.Background(), entries[0].Path, "renamed.mkv"))
	require.NoError(t, openList.Move(context.Background(), entries[0].Path, "/archive"))
	require.NoError(t, openList.Delete(context.Background(), entries[0].Path))
}

func TestOpenListEndpointSubpathAndValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/alist/api/fs/list", r.URL.Path)
		_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"content":[]}}`))
	}))
	defer server.Close()
	openList, err := NewOpenList(server.URL+"/alist/", "/", "")
	require.NoError(t, err)
	_, err = openList.List(context.Background(), "/")
	require.NoError(t, err)
	for _, endpoint := range []string{
		"ftp://example.test", "https://user:pass@example.test", "https://example.test?token=x", "https://example.test#fragment",
	} {
		_, err := NewOpenList(endpoint, "/", "")
		require.Error(t, err, endpoint)
	}
}

func TestOpenListDirectURLValidation(t *testing.T) {
	for _, test := range []struct {
		name, rawURL string
		valid        bool
	}{
		{name: "signed query", rawURL: "https://cdn.example/media.mkv?sign=abc&expires=123", valid: true},
		{name: "credentials", rawURL: "https://user:secret@cdn.example/media.mkv"},
		{name: "fragment", rawURL: "https://cdn.example/media.mkv#secret"},
		{name: "file scheme", rawURL: "file:///etc/passwd"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "message": "success", "data": map[string]any{"raw_url": test.rawURL}})
			}))
			defer server.Close()
			driver, err := NewOpenList(server.URL, "/", "")
			require.NoError(t, err)
			result, err := driver.DirectURL(context.Background(), "/media.mkv")
			if test.valid {
				require.NoError(t, err)
				assert.Equal(t, test.rawURL, result)
				return
			}
			require.ErrorContains(t, err, "invalid raw_url")
			assert.NotContains(t, err.Error(), test.rawURL)
		})
	}
}

func TestOpenListErrorMessageIsNotExposed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":403,"message":"token=remote-secret\nsecond-line","data":null}`))
	}))
	defer server.Close()
	driver, err := NewOpenList(server.URL, "/", "")
	require.NoError(t, err)
	_, err = driver.List(context.Background(), "/")
	require.EqualError(t, err, "list OpenList page 1: OpenList returned error code 403")
	assert.NotContains(t, err.Error(), "remote-secret")
}

func TestOpenListPreservesProviderTrailingSpaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		switch r.URL.Path {
		case "/api/fs/list":
			_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"content":[{"name":"movie.mkv ","size":5,"is_dir":false}]}}`))
		case "/api/fs/rename":
			assert.Equal(t, "/cloud/movies/movie.mkv ", request.Path)
			assert.Equal(t, "movie.mkv", request.Name)
			_, _ = w.Write([]byte(`{"code":200,"message":"success","data":null}`))
		}
	}))
	defer server.Close()
	openList, err := NewOpenList(server.URL, "/cloud", "token")
	require.NoError(t, err)
	entries, err := openList.List(context.Background(), "/movies")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "movie.mkv ", entries[0].Name)
	assert.Equal(t, "/movies/movie.mkv ", entries[0].Path)
	require.NoError(t, openList.Rename(context.Background(), entries[0].Path, "movie.mkv"))
}

func TestOpenListPaginatesLargeDirectoriesAndRefreshesOnce(t *testing.T) {
	const total = 450
	var mu sync.Mutex
	requests := make([]struct {
		Page    int
		Refresh bool
	}, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Path    string `json:"path"`
			Page    int    `json:"page"`
			PerPage int    `json:"per_page"`
			Refresh bool   `json:"refresh"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, "/cloud/library", request.Path)
		assert.Equal(t, openListPageSize, request.PerPage)
		mu.Lock()
		requests = append(requests, struct {
			Page    int
			Refresh bool
		}{Page: request.Page, Refresh: request.Refresh})
		mu.Unlock()
		start := (request.Page - 1) * request.PerPage
		end := min(start+request.PerPage, total)
		content := make([]openListItem, 0, max(0, end-start))
		for index := start; index < end; index++ {
			content = append(content, openListItem{Name: fmt.Sprintf("Episode-%03d.mkv", index), Size: int64(index + 1)})
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"code": 200, "message": "success", "data": map[string]any{"content": content, "total": total},
		}))
	}))
	defer server.Close()
	openList, err := NewOpenList(server.URL, "/cloud", "token")
	require.NoError(t, err)
	entries, err := openList.List(context.Background(), "/library")
	require.NoError(t, err)
	require.Len(t, entries, total)
	assert.Equal(t, "/library/Episode-000.mkv", entries[0].Path)
	assert.Equal(t, "/library/Episode-449.mkv", entries[449].Path)
	require.NoError(t, openList.Refresh(context.Background(), "/library"))
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, requests, 6)
	assert.Equal(t, []int{1, 2, 3, 1, 2, 3}, []int{requests[0].Page, requests[1].Page, requests[2].Page, requests[3].Page, requests[4].Page, requests[5].Page})
	assert.False(t, requests[0].Refresh)
	assert.False(t, requests[1].Refresh)
	assert.False(t, requests[2].Refresh)
	assert.True(t, requests[3].Refresh)
	assert.False(t, requests[4].Refresh)
	assert.False(t, requests[5].Refresh)
}

func TestOpenListRejectsBrokenPagination(t *testing.T) {
	t.Run("premature empty page", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var request struct {
				Page int `json:"page"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			content := make([]openListItem, 0)
			if request.Page == 1 {
				for index := 0; index < openListPageSize; index++ {
					content = append(content, openListItem{Name: fmt.Sprintf("file-%03d.mkv", index)})
				}
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"code": 200, "message": "success", "data": map[string]any{"content": content, "total": openListPageSize + 1},
			}))
		}))
		defer server.Close()
		openList, err := NewOpenList(server.URL, "/", "")
		require.NoError(t, err)
		_, err = openList.List(context.Background(), "/")
		require.ErrorContains(t, err, "ended at 200 of 201")
	})

	t.Run("duplicate page", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			content := make([]openListItem, 0, openListPageSize)
			for index := 0; index < openListPageSize; index++ {
				content = append(content, openListItem{Name: fmt.Sprintf("file-%03d.mkv", index)})
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"code": 200, "message": "success", "data": map[string]any{"content": content},
			}))
		}))
		defer server.Close()
		openList, err := NewOpenList(server.URL, "/", "")
		require.NoError(t, err)
		_, err = openList.List(context.Background(), "/")
		require.ErrorContains(t, err, "duplicate entry")
	})
}

func TestOpenListRejectsTrailingResponseData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"content":[]}} trailing`))
	}))
	defer server.Close()
	openList, err := NewOpenList(server.URL, "/", "")
	require.NoError(t, err)
	_, err = openList.List(context.Background(), "/")
	require.ErrorContains(t, err, "decode OpenList response")
}
