package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebDAVListDeleteAndAuthenticatedRangeStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "user" || password != "password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case "PROPFIND":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><multistatus xmlns="DAV:">
<response><href>/dav/media/movies/</href><propstat><prop><displayname>movies</displayname><resourcetype><collection/></resourcetype></prop></propstat></response>
<response><href>/dav/media/movies/demo.mkv</href><propstat><prop><displayname>demo.mkv</displayname><getcontentlength>10</getcontentlength><getlastmodified>Wed, 21 Oct 2015 07:28:00 GMT</getlastmodified><resourcetype/></prop></propstat></response>
</multistatus>`)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case "MKCOL":
			assert.Equal(t, "/dav/media/archive", r.URL.Path)
			w.WriteHeader(http.StatusCreated)
		case "MOVE":
			assert.Equal(t, "F", r.Header.Get("Overwrite"))
			assert.Contains(t, r.Header.Get("Destination"), "/dav/media/")
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			assert.Equal(t, "bytes=0-3", r.Header.Get("Range"))
			w.Header().Set("Content-Range", "bytes 0-3/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "0123")
		}
	}))
	defer server.Close()

	dav, err := NewWebDAV(server.URL+"/dav", "/media", "user", "password")
	require.NoError(t, err)
	entries, err := dav.List(context.Background(), "/movies")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "demo.mkv", entries[0].Name)
	assert.EqualValues(t, 10, entries[0].Size)
	assert.False(t, entries[0].IsDir)
	require.NoError(t, dav.Mkdir(context.Background(), "/archive"))
	require.NoError(t, dav.Rename(context.Background(), "/movies/demo.mkv", "renamed.mkv"))
	require.NoError(t, dav.Move(context.Background(), "/movies/demo.mkv", "/archive"))
	require.NoError(t, dav.Delete(context.Background(), "/movies/demo.mkv"))

	request := httptest.NewRequest(http.MethodGet, "/stream", nil)
	request.Header.Set("Range", "bytes=0-3")
	response := httptest.NewRecorder()
	require.NoError(t, dav.Stream(response, request, "/movies/demo.mkv"))
	assert.Equal(t, http.StatusPartialContent, response.Code)
	assert.Equal(t, "0123", response.Body.String())
	assert.Equal(t, "bytes 0-3/10", response.Header().Get("Content-Range"))
}

func TestWebDAVEndpointValidation(t *testing.T) {
	for _, endpoint := range []string{
		"ftp://example.test", "https://user:pass@example.test", "https://example.test/dav?token=x", "https://example.test/dav#fragment",
	} {
		_, err := NewWebDAV(endpoint, "/", "", "")
		require.Error(t, err, endpoint)
	}
}

func TestWebDAVStreamRedactsErrorsAndHandlesBodylessResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "session=upstream-secret")
		switch r.URL.Path {
		case "/dav/error.mkv":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "token=upstream-secret")
		case "/dav/range.mkv":
			w.Header().Set("Content-Range", "bytes */10")
			w.Header().Set("Content-Length", "15")
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			_, _ = io.WriteString(w, "upstream-secret")
		case "/dav/head.mkv":
			assert.Equal(t, http.MethodHead, r.Method)
			w.Header().Set("Content-Length", "10")
			w.Header().Set("Content-Type", "video/mp4")
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()
	dav, err := NewWebDAV(server.URL+"/dav", "/", "", "")
	require.NoError(t, err)

	response := httptest.NewRecorder()
	err = dav.Stream(response, httptest.NewRequest(http.MethodGet, "/stream", nil), "/error.mkv")
	require.EqualError(t, err, "remote returned HTTP status 401")
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Empty(t, response.Body.String())
	assert.Empty(t, response.Header().Get("Set-Cookie"))

	response = httptest.NewRecorder()
	require.NoError(t, dav.Stream(response, httptest.NewRequest(http.MethodGet, "/stream", nil), "/range.mkv"))
	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, response.Code)
	assert.Equal(t, "bytes */10", response.Header().Get("Content-Range"))
	assert.Empty(t, response.Header().Get("Content-Length"))
	assert.Empty(t, response.Header().Get("Set-Cookie"))
	assert.Empty(t, response.Body.String())

	response = httptest.NewRecorder()
	require.NoError(t, dav.Stream(response, httptest.NewRequest(http.MethodHead, "/stream", nil), "/head.mkv"))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "10", response.Header().Get("Content-Length"))
	assert.Equal(t, "video/mp4", response.Header().Get("Content-Type"))
	assert.Empty(t, response.Header().Get("Set-Cookie"))
	assert.Empty(t, response.Body.String())
}

func TestWebDAVPreservesProviderTrailingSpaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><multistatus xmlns="DAV:">
<response><href>/dav/media/movies/</href><propstat><prop><displayname>movies</displayname><resourcetype><collection/></resourcetype></prop></propstat></response>
<response><href>/dav/media/movies/movie.mkv%20</href><propstat><prop><displayname></displayname><getcontentlength>5</getcontentlength><resourcetype/></prop></propstat></response>
</multistatus>`)
		case "MOVE":
			assert.Equal(t, "/dav/media/movies/movie.mkv ", r.URL.Path)
			assert.Contains(t, r.Header.Get("Destination"), "/dav/media/movies/movie.mkv")
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer server.Close()
	dav, err := NewWebDAV(server.URL+"/dav", "/media", "", "")
	require.NoError(t, err)
	entries, err := dav.List(context.Background(), "/movies")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "movie.mkv ", entries[0].Name)
	assert.Equal(t, "/movies/movie.mkv ", entries[0].Path)
	require.NoError(t, dav.Rename(context.Background(), entries[0].Path, "movie.mkv"))
}

func TestWebDAVKeepsFirstFileAndParsesUTCTimes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "PROPFIND", r.Method)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><multistatus xmlns="DAV:">
<response><href>First%20Episode.mkv</href><propstat><prop><displayname>First Episode.mkv</displayname><getcontentlength>11</getcontentlength><getlastmodified>2025-01-02T03:04:05.123Z</getlastmodified><resourcetype/></prop></propstat></response>
<response><href>/dav/media/movies/Second.mkv</href><propstat><prop><displayname>Second.mkv</displayname><getcontentlength>12</getcontentlength><getlastmodified></getlastmodified><resourcetype/></prop></propstat></response>
<response><href>`+serverURLForRequest(r)+`/dav/media/movies/</href><propstat><prop><displayname>movies</displayname><resourcetype><collection/></resourcetype></prop></propstat></response>
</multistatus>`)
	}))
	defer server.Close()
	dav, err := NewWebDAV(server.URL+"/dav", "/media", "", "")
	require.NoError(t, err)
	entries, err := dav.List(context.Background(), "/movies")
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "/movies/First Episode.mkv", entries[0].Path)
	assert.Equal(t, time.Date(2025, time.January, 2, 3, 4, 5, 123000000, time.UTC), entries[0].ModTime)
	assert.Equal(t, "/movies/Second.mkv", entries[1].Path)
	assert.True(t, entries[1].ModTime.IsZero())
}

func TestWebDAVRejectsInvalidHrefEncoding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><multistatus xmlns="DAV:"><response><href>/dav/media/%ZZ.mkv</href><propstat><prop><displayname>bad.mkv</displayname><resourcetype/></prop></propstat></response></multistatus>`)
	}))
	defer server.Close()
	dav, err := NewWebDAV(server.URL+"/dav", "/media", "", "")
	require.NoError(t, err)
	_, err = dav.List(context.Background(), "/movies")
	require.ErrorContains(t, err, "invalid WebDAV href")
}

func TestWebDAVRejectsTrailingResponseData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><multistatus xmlns="DAV:"></multistatus> trailing`)
	}))
	defer server.Close()
	dav, err := NewWebDAV(server.URL, "/", "", "")
	require.NoError(t, err)
	_, err = dav.List(context.Background(), "/")
	require.ErrorContains(t, err, "decode WebDAV PROPFIND response")
}

func serverURLForRequest(r *http.Request) string {
	return "http://" + r.Host
}
