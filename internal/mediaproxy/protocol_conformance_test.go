package mediaproxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/signature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin the rewrite against the publicly documented Emby/Jellyfin
// playback contract. References (public, checked at the time of writing):
//
//   - Jellyfin MediaSourceInfo fields:
//     MediaBrowser.Model/Dto/MediaSourceInfo.cs
//   - Jellyfin PlaybackInfo routes (GET and POST):
//     Jellyfin.Api/Controllers/MediaInfoController.cs
//   - Jellyfin client direct-play selection, which uses MediaSource.Path and
//     requires Protocol == "Http":
//     jellyfin-web src/components/playback/playbackmanager.js
//
// Jellyfin's MediaSourceInfo has no DirectStreamUrl or
// AddApiKeyToDirectStreamUrl field; those are Emby-only. Jellyfin clients reach
// a remote source through Path instead, so the rewrite must leave Path intact
// and must not depend on the Emby-only fields to work on Jellyfin.

func conformanceUpstream(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func rewriteOnce(t *testing.T, serverType, upstreamURL, publicURL, secret, requestPath string) map[string]any {
	t.Helper()
	handler, err := New(config.MediaProxyConfig{
		Upstream: upstreamURL, ServerType: serverType, RewritePlaybackInfo: true,
	}, publicURL, secret)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "http://proxy.example"+requestPath, strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	var document map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&document))
	return document
}

// Jellyfin clients play a remote source from MediaSource.Path. The rewrite must
// never disturb it, otherwise direct play breaks on Jellyfin even though the
// Emby-only fields look correct.
func TestConformancePathIsPreservedForJellyfinDirectPlay(t *testing.T) {
	secret := testSigningValue()
	signed := "https://smart.example/stream/cloud?path=%2Fmovie.mkv&sig=" +
		url.QueryEscape(signature.Create(secret, "cloud", "/movie.mkv"))
	upstream := conformanceUpstream(t, `{"MediaSources":[`+
		`{"Id":"a","Path":"`+signed+`","Protocol":"Http","IsRemote":true,"SupportsDirectPlay":true}]}`)

	document := rewriteOnce(t, "jellyfin", upstream.URL, "https://smart.example", secret, "/Items/item-1/PlaybackInfo")
	sources := document["MediaSources"].([]any)
	source := sources[0].(map[string]any)

	assert.Equal(t, signed, source["Path"], "Jellyfin direct play reads Path; it must survive the rewrite")
	assert.Equal(t, "Http", source["Protocol"], "Jellyfin requires Protocol==Http for direct play")
	assert.Equal(t, true, source["SupportsDirectPlay"], "the rewrite must not clear the direct play flag")
}

// The Emby-only hint fields are additive. They must be set for Emby without
// removing anything Jellyfin depends on.
func TestConformanceEmbyHintFieldsAreAdditive(t *testing.T) {
	secret := testSigningValue()
	signed := "https://smart.example/stream/cloud?path=%2Fmovie.mkv&sig=" +
		url.QueryEscape(signature.Create(secret, "cloud", "/movie.mkv"))
	upstream := conformanceUpstream(t, `{"MediaSources":[`+
		`{"Id":"a","Path":"`+signed+`","Protocol":"Http","IsRemote":true,"SupportsDirectPlay":true,`+
		`"SupportsDirectStream":true,"SupportsTranscoding":true,"RequiredHttpHeaders":{},"MediaStreams":[]}]}`)

	document := rewriteOnce(t, "emby", upstream.URL, "https://smart.example", secret, "/Items/item-1/PlaybackInfo")
	source := document["MediaSources"].([]any)[0].(map[string]any)

	assert.Equal(t, signed, source["DirectStreamUrl"], "Emby direct stream URL must point at the signed address")
	assert.Equal(t, false, source["AddApiKeyToDirectStreamUrl"], "the signed URL already authenticates; Emby must not append its API key")
	// Fields the client still needs must be untouched.
	assert.Equal(t, true, source["SupportsDirectStream"])
	assert.Equal(t, true, source["SupportsTranscoding"])
	assert.NotNil(t, source["MediaStreams"], "stream metadata must survive for client capability checks")
	assert.NotNil(t, source["RequiredHttpHeaders"], "Jellyfin checks RequiredHttpHeaders length before direct play")
}

// Jellyfin exposes PlaybackInfo as both GET and POST. Both must be rewritten.
func TestConformanceBothPlaybackInfoMethodsAreRewritten(t *testing.T) {
	secret := testSigningValue()
	signed := "https://smart.example/stream/cloud?path=%2Fmovie.mkv&sig=" +
		url.QueryEscape(signature.Create(secret, "cloud", "/movie.mkv"))
	upstream := conformanceUpstream(t, `{"MediaSources":[`+
		`{"Id":"a","Path":"`+signed+`","Protocol":"Http","IsRemote":true,"SupportsDirectPlay":true}]}`)
	handler, err := New(config.MediaProxyConfig{
		Upstream: upstream.URL, ServerType: "emby", RewritePlaybackInfo: true,
	}, "https://smart.example", secret)
	require.NoError(t, err)

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "http://proxy.example/Items/item-1/PlaybackInfo", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, method)
		var document map[string]any
		require.NoError(t, json.NewDecoder(response.Body).Decode(&document))
		source := document["MediaSources"].([]any)[0].(map[string]any)
		assert.Equal(t, signed, source["DirectStreamUrl"], "%s PlaybackInfo must be rewritten", method)
	}
}

// Emby and Jellyfin both serve PlaybackInfo under a configurable base path.
func TestConformanceBasePathedPlaybackInfoIsRewritten(t *testing.T) {
	secret := testSigningValue()
	signed := "https://smart.example/stream/cloud?path=%2Fmovie.mkv&sig=" +
		url.QueryEscape(signature.Create(secret, "cloud", "/movie.mkv"))
	upstream := conformanceUpstream(t, `{"MediaSources":[`+
		`{"Id":"a","Path":"`+signed+`","Protocol":"Http","IsRemote":true,"SupportsDirectPlay":true}]}`)

	document := rewriteOnce(t, "emby", upstream.URL, "https://smart.example", secret,
		"/jellyfin/Items/item-1/PlaybackInfo")
	source := document["MediaSources"].([]any)[0].(map[string]any)
	assert.Equal(t, signed, source["DirectStreamUrl"], "a reverse-proxied base path must still be rewritten")
}
