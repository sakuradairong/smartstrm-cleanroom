package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestANiOpenFeedDirectoryDirectURLAndCache(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss xmlns:anime="https://resources.ani.rip"><channel>
<item><title>[ANi] Demo - 01.mp4</title><link>https://resources.ani.rip/2026-7/demo?d=mp4</link><pubDate>Fri, 17 Jul 2026 11:07:15 GMT</pubDate><anime:size>649.7 MB</anime:size></item>
<item><title>../unsafe.mp4</title><link>https://resources.ani.rip/2026-7/unsafe?d=mp4</link><anime:size>1 MB</anime:size></item>
<item><title>Bad URL.mp4</title><link>javascript:alert(1)</link><anime:size>1 MB</anime:size></item>
<item><title>Credential URL.mp4</title><link>https://user:secret@resources.ani.rip/2026-7/private.mp4</link><anime:size>1 MB</anime:size></item>
<item><title>Fragment URL.mp4</title><link>https://resources.ani.rip/2026-7/private.mp4#secret</link><anime:size>1 MB</anime:size></item>
</channel></rss>`))
	}))
	defer server.Close()
	driver, err := NewANiOpen(server.URL)
	require.NoError(t, err)
	root, err := driver.List(context.Background(), "/")
	require.NoError(t, err)
	require.Len(t, root, 1)
	assert.True(t, root[0].IsDir)
	assert.Equal(t, "2026-7", root[0].Name)
	items, err := driver.List(context.Background(), "/2026-7")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "[ANi] Demo - 01.mp4", items[0].Name)
	assert.Equal(t, int64(681259827), items[0].Size)
	link, err := driver.DirectURL(context.Background(), items[0].Path)
	require.NoError(t, err)
	assert.Equal(t, "https://resources.ani.rip/2026-7/demo?d=mp4", link)
	assert.Equal(t, int32(1), calls.Load(), "cached lists and links must not refetch")
	_, err = driver.List(context.Background(), "/missing")
	require.Error(t, err)
	assert.Contains(t, driver.Delete(context.Background(), items[0].Path).Error(), "read-only")
	assert.Contains(t, driver.Mkdir(context.Background(), "/new").Error(), "read-only")
}

func TestANiOpenValidationAndSize(t *testing.T) {
	_, err := NewANiOpen("file:///tmp/feed.xml")
	require.Error(t, err)
	_, err = NewANiOpen("https://user:pass@example.test/feed.xml")
	require.ErrorContains(t, err, "credentials")
	_, err = NewANiOpen("https://example.test/feed.xml#fragment")
	require.ErrorContains(t, err, "fragment")
	assert.Equal(t, int64(1536), parseANiSize("1.5 KB"))
	assert.Equal(t, int64(0), parseANiSize("unknown"))
	assert.Equal(t, int64(0), parseANiSize("-1 MB"))
}

func TestANiOpenAllowsEndpointQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "mirror=1", r.URL.RawQuery)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel></channel></rss>`))
	}))
	defer server.Close()
	driver, err := NewANiOpen(server.URL + "/feed.xml?mirror=1")
	require.NoError(t, err)
	_, err = driver.List(context.Background(), "/")
	require.NoError(t, err)
}

func TestANiOpenRejectsTrailingFeedData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss><channel></channel></rss> trailing`))
	}))
	defer server.Close()
	driver, err := NewANiOpen(server.URL)
	require.NoError(t, err)
	_, err = driver.List(context.Background(), "/")
	require.ErrorContains(t, err, "decode ANi Open feed")
}

func TestANiOpenHTTPStatusDoesNotExposeReasonPhrase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	driver, err := NewANiOpen(server.URL)
	require.NoError(t, err)
	_, err = driver.List(context.Background(), "/")
	require.EqualError(t, err, "ANi Open returned HTTP status 403")
}
