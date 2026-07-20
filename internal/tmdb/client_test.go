package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientSearchAndDetails(t *testing.T) {
	var detailCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "key", r.URL.Query().Get("api_key"))
		assert.Equal(t, "zh-CN", r.URL.Query().Get("language"))
		switch r.URL.Path {
		case "/search/multi":
			assert.Equal(t, "Demo", r.URL.Query().Get("query"))
			_, _ = w.Write([]byte(`{"results":[{"id":1,"media_type":"movie","title":"电影","original_title":"Movie","release_date":"2025-01-02","poster_path":"/poster.jpg","vote_average":8.5},{"id":2,"media_type":"person","name":"ignored"},{"id":3,"media_type":"tv","name":"剧集","original_name":"Series","first_air_date":"2024-02-03"},{"id":4,"media_type":"movie","title":"仅显示标题"}]}`))
		case "/movie/1":
			detailCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":1,"title":"电影","original_title":"Movie","release_date":"2025-01-02","overview":"overview","vote_average":8.5}`))
		case "/tv/3/season/1/episode/2":
			_, _ = w.Write([]byte(`{"id":32,"name":"第二集","air_date":"2024-02-10","season_number":1,"episode_number":2}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(config.TMDBConfig{APIKey: "key", Language: "zh-CN", BaseURL: server.URL})
	require.NoError(t, err)
	results, err := client.Search(context.Background(), "Demo", "multi", 0)
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "电影", results[0].Title)
	assert.Equal(t, "剧集", results[1].Title)
	assert.Equal(t, "仅显示标题", results[2].OriginalTitle)
	details, err := client.Details(context.Background(), "movie", 1)
	require.NoError(t, err)
	assert.Equal(t, "Movie", details.OriginalTitle)
	assert.Equal(t, 8.5, details.VoteAverage)
	_, err = client.Details(context.Background(), "movie", 1)
	require.NoError(t, err)
	assert.Equal(t, int32(1), detailCalls.Load(), "details should be served from cache")
	episode, err := client.Episode(context.Background(), 3, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, "第二集", episode.Name)
}

func TestCacheExpires(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"id":1,"title":"Demo"}`))
	}))
	defer server.Close()
	client, err := New(config.TMDBConfig{APIKey: "key", BaseURL: server.URL, CacheMinutes: 1})
	require.NoError(t, err)
	client.cacheTTL = time.Millisecond
	_, err = client.Details(context.Background(), "movie", 1)
	require.NoError(t, err)
	time.Sleep(3 * time.Millisecond)
	_, err = client.Details(context.Background(), "movie", 1)
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
}

func TestDetailsOriginalTitleFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/1":
			_, _ = w.Write([]byte(`{"id":1,"title":"Movie Display"}`))
		case "/tv/2":
			_, _ = w.Write([]byte(`{"id":2,"name":"TV Display"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(config.TMDBConfig{APIKey: "key", BaseURL: server.URL})
	require.NoError(t, err)
	movie, err := client.Details(context.Background(), "movie", 1)
	require.NoError(t, err)
	assert.Equal(t, "Movie Display", movie.OriginalTitle)
	tv, err := client.Details(context.Background(), "tv", 2)
	require.NoError(t, err)
	assert.Equal(t, "TV Display", tv.OriginalTitle)
}

func TestClientRequiresKeyAndValidType(t *testing.T) {
	client, err := New(config.TMDBConfig{})
	require.NoError(t, err)
	_, err = client.Search(context.Background(), "Demo", "movie", 2025)
	require.Error(t, err)
	client.apiKey = "key"
	_, err = client.Details(context.Background(), "person", 1)
	require.Error(t, err)
}

func TestTMDBImageValidationAndLimits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/poster.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg"))
		case "/redirect.jpg":
			http.Redirect(w, r, "/poster.jpg", http.StatusFound)
		case "/text.jpg":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("not an image"))
		case "/empty.png":
			w.Header().Set("Content-Type", "image/png")
		case "/large.webp":
			w.Header().Set("Content-Type", "image/webp")
			_, _ = w.Write(make([]byte, 5<<20+1))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(config.TMDBConfig{APIKey: "test", BaseURL: server.URL, ImageBaseURL: server.URL})
	require.NoError(t, err)
	data, contentType, err := client.Image(context.Background(), "/poster.jpg")
	require.NoError(t, err)
	assert.Equal(t, []byte("jpeg"), data)
	assert.Equal(t, "image/jpeg", contentType)
	for _, imagePath := range []string{"../poster.jpg", "/dir/poster.jpg", "poster.svg", "poster.jpg?x=1", ""} {
		_, _, err = client.Image(context.Background(), imagePath)
		require.Error(t, err, imagePath)
	}
	for _, imagePath := range []string{"redirect.jpg", "text.jpg", "empty.png", "large.webp"} {
		_, _, err = client.Image(context.Background(), imagePath)
		require.Error(t, err, imagePath)
	}
	_, err = New(config.TMDBConfig{APIKey: "test", ImageBaseURL: "file:///tmp"})
	require.ErrorContains(t, err, "image base URL")
}
