package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSPA(t *testing.T) *spa {
	t.Helper()
	fsys := fstest.MapFS{
		"index.html":           {Data: []byte("<html><body>app</body></html>")},
		"assets/app-abc123.js": {Data: []byte("console.log(1)")},
	}
	s, err := newSPA(fsys)
	require.NoError(t, err)
	return s
}

func TestSPAServesIndexWithNoCache(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestSPA(t).handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	body, _ := io.ReadAll(rec.Body)
	assert.Contains(t, string(body), "app")
}

func TestSPAServesAssetWithImmutableCache(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestSPA(t).handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
}

func TestSPAMissingAssetIs404(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestSPA(t).handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSPAUnknownPathFallsBackToIndex(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestSPA(t).handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/some/client/route", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
}

func TestSPATraversalIsNotServed(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestSPA(t).handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/../../app.go", nil))
	assert.NotEqual(t, http.StatusOK, rec.Code)
}

func TestSPAWithoutBuildReturns503(t *testing.T) {
	s, err := newSPA(fstest.MapFS{})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
}

func TestAppRootServesAuthenticatedSPA(t *testing.T) {
	application := &App{spaHandler: newTestSPA(t).handler()}
	application.handler = application.routes()
	rec := httptest.NewRecorder()
	application.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<body>app</body>")
	assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
}
