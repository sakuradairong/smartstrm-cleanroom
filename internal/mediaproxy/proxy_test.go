package mediaproxy

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
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

func TestReverseProxyPreservesMediaRequestAndRewritesOwnRedirect(t *testing.T) {
	var upstreamURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/base/Videos/42/stream", r.URL.Path)
		assert.Equal(t, "api_key=secret&static=true", r.URL.RawQuery)
		assert.Equal(t, "Bearer media-token", r.Header.Get("Authorization"))
		assert.NotContains(t, r.Header.Get("X-Forwarded-For"), "attacker")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "request-body", string(body))
		w.Header().Set("Location", upstreamURL+"/base/web/index.html?x=1")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()
	upstreamURL = upstream.URL
	handler, err := New(config.MediaProxyConfig{Upstream: upstream.URL + "/base"}, "", "")
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "http://proxy.example/Videos/42/stream?api_key=secret&static=true", strings.NewReader("request-body"))
	request.Header.Set("Authorization", "Bearer media-token")
	request.Header.Set("X-Forwarded-For", "attacker")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, http.StatusFound, response.Code)
	assert.Equal(t, "/base/web/index.html?x=1", response.Header().Get("Location"))
}

func TestReverseProxyKeepsExternalRedirectAndStreams(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "https://login.example/auth")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		_, _ = io.WriteString(w, "first\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer upstream.Close()
	handler, err := New(config.MediaProxyConfig{Upstream: upstream.URL}, "", "")
	require.NoError(t, err)
	proxy := httptest.NewServer(handler)
	defer proxy.Close()
	client := proxy.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Get(proxy.URL + "/redirect")
	require.NoError(t, err)
	response.Body.Close()
	assert.Equal(t, http.StatusTemporaryRedirect, response.StatusCode)
	assert.Equal(t, "https://login.example/auth", response.Header.Get("Location"))

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, proxy.URL+"/stream", nil)
	require.NoError(t, err)
	response, err = client.Do(request)
	require.NoError(t, err)
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "first\n", line)
	cancel()
	response.Body.Close()
}

func TestReverseProxyValidationAndSafeGatewayError(t *testing.T) {
	for _, value := range []string{"file:///tmp", "http://user:pass@example.test", "http://example.test?target=x", "javascript:alert(1)"} {
		_, err := New(config.MediaProxyConfig{Upstream: value}, "", "")
		require.Error(t, err, value)
	}
	handler, err := New(config.MediaProxyConfig{Upstream: "http://127.0.0.1:1"}, "", "")
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodGet, "http://proxy.example/test", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadGateway, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.JSONEq(t, `{"error":"media server unavailable"}`, response.Body.String())
	assert.NotContains(t, response.Body.String(), "127.0.0.1")
}

func TestReverseProxyTunnelsProtocolUpgrade(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		require.True(t, ok)
		connection, buffered, err := hijacker.Hijack()
		require.NoError(t, err)
		defer connection.Close()
		_, err = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: smartstrm-test\r\n\r\n")
		require.NoError(t, err)
		require.NoError(t, buffered.Flush())
		payload := make([]byte, 4)
		_, err = io.ReadFull(connection, payload)
		require.NoError(t, err)
		_, err = connection.Write(payload)
		require.NoError(t, err)
	}))
	defer upstream.Close()
	handler, err := New(config.MediaProxyConfig{Upstream: upstream.URL}, "", "")
	require.NoError(t, err)
	proxy := httptest.NewServer(handler)
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	connection, err := net.Dial("tcp", proxyURL.Host)
	require.NoError(t, err)
	defer connection.Close()
	_, err = io.WriteString(connection, "GET /socket HTTP/1.1\r\nHost: proxy.example\r\nConnection: Upgrade\r\nUpgrade: smartstrm-test\r\n\r\n")
	require.NoError(t, err)
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	require.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
	_, err = connection.Write([]byte("ping"))
	require.NoError(t, err)
	payload := make([]byte, 4)
	_, err = io.ReadFull(reader, payload)
	require.NoError(t, err)
	assert.Equal(t, "ping", string(payload))
}

func TestPlaybackInfoRewritesOnlySignedSmartStrmRemoteSources(t *testing.T) {
	signingSecret := testSigningValue()
	signedURL := "https://smart.example/base/stream/cloud?path=%2Fmovie.mkv&sig=" + url.QueryEscape(signature.Create(signingSecret, "cloud", "/movie.mkv"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "identity", r.Header.Get("Accept-Encoding"))
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-MD5", "stale")
		w.Header().Set("ETag", `"stale"`)
		_, _ = io.WriteString(w, `{"PlaySessionId":"session","FutureField":{"kept":true},"MediaSources":[`+
			`{"Id":"smart","Path":"`+signedURL+`","IsRemote":true,"SupportsDirectPlay":true,"DirectStreamUrl":"/Videos/1/stream","AddApiKeyToDirectStreamUrl":true,"Unknown":42},`+
			`{"Id":"external","Path":"https://cdn.example/movie.mkv","IsRemote":true,"SupportsDirectPlay":true,"DirectStreamUrl":"/Videos/2/stream"},`+
			`{"Id":"unsigned","Path":"https://smart.example/base/stream/cloud?path=%2Fother.mkv","IsRemote":true,"SupportsDirectPlay":true,"DirectStreamUrl":"/Videos/3/stream"},`+
			`{"Id":"incompatible","Path":"`+signedURL+`","IsRemote":true,"SupportsDirectPlay":false,"DirectStreamUrl":"/Videos/4/stream"}]}`)
	}))
	defer upstream.Close()
	handler, err := New(config.MediaProxyConfig{Upstream: upstream.URL, ServerType: "jellyfin", RewritePlaybackInfo: true}, "https://smart.example/base", signingSecret)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "http://proxy.example/Items/item-1/PlaybackInfo?UserId=user", strings.NewReader(`{"DeviceProfile":{}}`))
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Empty(t, response.Header().Get("ETag"))
	assert.Empty(t, response.Header().Get("Content-MD5"))
	var document struct {
		PlaySessionID string `json:"PlaySessionId"`
		FutureField   struct {
			Kept bool `json:"kept"`
		} `json:"FutureField"`
		MediaSources []struct {
			ID                         string `json:"Id"`
			DirectStreamURL            string `json:"DirectStreamUrl"`
			AddAPIKeyToDirectStreamURL bool   `json:"AddApiKeyToDirectStreamUrl"`
			Unknown                    int    `json:"Unknown"`
		} `json:"MediaSources"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&document))
	assert.Equal(t, "session", document.PlaySessionID)
	assert.True(t, document.FutureField.Kept)
	assert.Equal(t, signedURL, document.MediaSources[0].DirectStreamURL)
	assert.False(t, document.MediaSources[0].AddAPIKeyToDirectStreamURL)
	assert.Equal(t, 42, document.MediaSources[0].Unknown)
	assert.Equal(t, "/Videos/2/stream", document.MediaSources[1].DirectStreamURL)
	assert.Equal(t, "/Videos/3/stream", document.MediaSources[2].DirectStreamURL)
	assert.Equal(t, "/Videos/4/stream", document.MediaSources[3].DirectStreamURL)
}

func TestPlaybackInfoRewriteLeavesOtherResponsesUntouched(t *testing.T) {
	payload := `{"MediaSources":[{"Path":"https://smart.example/stream/s?path=%2Fx&sig=s","IsRemote":true,"SupportsDirectPlay":true}]}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(strings.ToLower(r.URL.Path), "playbackinfo") {
			w.Header().Set("Content-Type", "text/plain")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = io.WriteString(w, payload)
	}))
	defer upstream.Close()
	handler, err := New(config.MediaProxyConfig{Upstream: upstream.URL, ServerType: "emby", RewritePlaybackInfo: true}, "https://smart.example", testSigningValue())
	require.NoError(t, err)
	for _, target := range []string{"http://proxy.example/Items/1/PlaybackInfo", "http://proxy.example/Items"} {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, payload, response.Body.String())
	}
}

func TestPlaybackInfoRewriteRejectsOversizedResponseSafely(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, strings.Repeat(" ", maxPlaybackInfoBytes+1))
	}))
	defer upstream.Close()
	handler, err := New(config.MediaProxyConfig{Upstream: upstream.URL, ServerType: "emby", RewritePlaybackInfo: true}, "https://smart.example", testSigningValue())
	require.NoError(t, err)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://proxy.example/Items/1/PlaybackInfo", nil))
	assert.Equal(t, http.StatusBadGateway, response.Code)
	assert.JSONEq(t, `{"error":"media server unavailable"}`, response.Body.String())
	assert.NotContains(t, response.Body.String(), "8388608")
}

func TestPlaybackInfoRewriteValidation(t *testing.T) {
	for _, cfg := range []config.MediaProxyConfig{
		{Upstream: "http://emby.example", RewritePlaybackInfo: true},
		{Upstream: "http://emby.example", ServerType: "plex", RewritePlaybackInfo: true},
	} {
		_, err := New(cfg, "https://smart.example", testSigningValue())
		require.Error(t, err)
	}
	_, err := New(config.MediaProxyConfig{Upstream: "http://emby.example", ServerType: "emby", RewritePlaybackInfo: true}, "http://user:password@smart.example", testSigningValue())
	require.Error(t, err)
	_, err = New(config.MediaProxyConfig{Upstream: "http://emby.example", ServerType: "emby", RewritePlaybackInfo: true}, "https://smart.example?unexpected=1", testSigningValue())
	require.Error(t, err)
	_, err = New(config.MediaProxyConfig{Upstream: "http://emby.example", ServerType: "emby", RewritePlaybackInfo: true}, "https://smart.example", "")
	require.Error(t, err)
}

func TestSignedSmartStrmURLValidation(t *testing.T) {
	secret := testSigningValue()
	streamBase, err := url.Parse("https://smart.example/base/stream/")
	require.NoError(t, err)
	pathSignature := signature.Create(secret, "cloud", "/folder/movie.mkv")
	fileIDSignature := signature.Create(secret, "cloud", "id:stable-123")
	assert.True(t, isSignedSmartStrmURL("https://smart.example/base/stream/cloud?path=%2Ffolder%2Fmovie.mkv&sig="+url.QueryEscape(pathSignature), streamBase, secret))
	assert.True(t, isSignedSmartStrmURL("https://smart.example/base/stream/cloud?id=stable-123&sig="+url.QueryEscape(fileIDSignature), streamBase, secret))
	for _, candidate := range []string{
		"https://smart.example/base/stream/cloud?path=%2Ffolder%2Fmovie.mkv&sig=forged",
		"https://smart.example/base/stream/cloud?path=%2Fmovie.mkv&id=stable-123&sig=x",
		"https://other.example/base/stream/cloud?path=%2Ffolder%2Fmovie.mkv&sig=" + url.QueryEscape(pathSignature),
		"https://smart.example/base/stream/cloud/child?path=%2Ffolder%2Fmovie.mkv&sig=" + url.QueryEscape(pathSignature),
		"https://smart.example/base/stream/cloud?path=%2Ffolder%2Fmovie.mkv&sig=" + url.QueryEscape(pathSignature) + "#fragment",
	} {
		assert.False(t, isSignedSmartStrmURL(candidate, streamBase, secret), candidate)
	}
}

func testSigningValue() string {
	return strings.Join([]string{"test", "signing", "value"}, "-")
}
