package mediaproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/signature"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
)

const maxPlaybackInfoBytes = 8 << 20

func New(cfg config.MediaProxyConfig, publicURL, signingSecret string) (http.Handler, error) {
	upstream, err := url.Parse(strings.TrimRight(cfg.Upstream, "/"))
	if err != nil || (upstream.Scheme != "http" && upstream.Scheme != "https") || upstream.Host == "" || upstream.User != nil || upstream.RawQuery != "" || upstream.Fragment != "" {
		return nil, fmt.Errorf("invalid media proxy upstream")
	}
	var streamBase *url.URL
	if cfg.RewritePlaybackInfo {
		if cfg.ServerType != "emby" && cfg.ServerType != "jellyfin" {
			return nil, fmt.Errorf("playback info rewriting requires an emby or jellyfin server type")
		}
		streamBase, err = url.Parse(strings.TrimRight(publicURL, "/") + "/stream/")
		if err != nil || (streamBase.Scheme != "http" && streamBase.Scheme != "https") || streamBase.Host == "" || streamBase.User != nil || streamBase.RawQuery != "" || streamBase.Fragment != "" {
			return nil, fmt.Errorf("invalid SmartStrm public URL")
		}
		if signingSecret == "" {
			return nil, fmt.Errorf("playback info rewriting requires a signing secret")
		}
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	proxy.FlushInterval = -1
	originalDirector := proxy.Director
	proxy.Director = func(request *http.Request) {
		request.Header.Del("Forwarded")
		request.Header.Del("X-Forwarded-For")
		request.Header.Del("X-Forwarded-Host")
		request.Header.Del("X-Forwarded-Proto")
		if streamBase != nil && isPlaybackInfoPath(request.URL.Path) {
			request.Header.Set("Accept-Encoding", "identity")
		}
		originalDirector(request)
		request.Host = upstream.Host
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		if streamBase != nil {
			if err := rewritePlaybackInfoResponse(response, streamBase, signingSecret); err != nil {
				return err
			}
		}
		location := response.Header.Get("Location")
		if location == "" {
			return nil
		}
		parsed, err := url.Parse(location)
		if err != nil {
			return fmt.Errorf("invalid upstream redirect: %w", err)
		}
		if parsed.IsAbs() && strings.EqualFold(parsed.Scheme, upstream.Scheme) && strings.EqualFold(parsed.Host, upstream.Host) {
			parsed.Scheme, parsed.Host, parsed.User = "", "", nil
			response.Header.Set("Location", parsed.String())
		}
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"media server unavailable"}`))
	}
	return proxy, nil
}

func rewritePlaybackInfoResponse(response *http.Response, streamBase *url.URL, signingSecret string) error {
	if response.Request == nil || !isPlaybackInfoPath(response.Request.URL.Path) || response.StatusCode != http.StatusOK {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || response.Header.Get("Content-Encoding") != "" {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxPlaybackInfoBytes+1))
	closeErr := response.Body.Close()
	if err != nil {
		wrapped := fmt.Errorf("read playback info response: %w", err)
		return wrapped
	}
	if closeErr != nil {
		wrapped := fmt.Errorf("close playback info response: %w", closeErr)
		return wrapped
	}
	if len(data) > maxPlaybackInfoBytes {
		tooLarge := fmt.Errorf("playback info response exceeds %d bytes", maxPlaybackInfoBytes)
		return tooLarge
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		replaceResponseBody(response, data)
		return nil
	}
	var sources []map[string]json.RawMessage
	if err := json.Unmarshal(document["MediaSources"], &sources); err != nil {
		replaceResponseBody(response, data)
		return nil
	}
	changed := false
	for _, source := range sources {
		var remote, directPlay bool
		var sourcePath string
		if json.Unmarshal(source["IsRemote"], &remote) != nil || !remote || json.Unmarshal(source["SupportsDirectPlay"], &directPlay) != nil || !directPlay || json.Unmarshal(source["Path"], &sourcePath) != nil {
			continue
		}
		if !isSignedSmartStrmURL(sourcePath, streamBase, signingSecret) {
			continue
		}
		encodedPath, err := json.Marshal(sourcePath)
		if err != nil {
			wrapped := fmt.Errorf("encode direct stream URL: %w", err)
			return wrapped
		}
		source["DirectStreamUrl"] = encodedPath
		source["AddApiKeyToDirectStreamUrl"] = json.RawMessage("false")
		changed = true
	}
	if !changed {
		replaceResponseBody(response, data)
		return nil
	}
	document["MediaSources"], err = json.Marshal(sources)
	if err != nil {
		wrapped := fmt.Errorf("encode playback media sources: %w", err)
		return wrapped
	}
	rewritten, err := json.Marshal(document)
	if err != nil {
		wrapped := fmt.Errorf("encode playback info response: %w", err)
		return wrapped
	}
	replaceResponseBody(response, rewritten)
	response.Header.Del("Content-MD5")
	response.Header.Del("ETag")
	response.Header.Set("Cache-Control", "no-store")
	return nil
}

func replaceResponseBody(response *http.Response, data []byte) {
	response.Body = io.NopCloser(bytes.NewReader(data))
	response.ContentLength = int64(len(data))
	response.Header.Set("Content-Length", strconv.Itoa(len(data)))
}

func isPlaybackInfoPath(value string) bool {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	return len(parts) >= 3 && strings.EqualFold(parts[len(parts)-3], "Items") && parts[len(parts)-2] != "" && strings.EqualFold(parts[len(parts)-1], "PlaybackInfo")
}

func isSignedSmartStrmURL(value string, streamBase *url.URL, signingSecret string) bool {
	candidate, err := url.Parse(value)
	if err != nil || candidate.Scheme == "" || candidate.Host == "" || candidate.User != nil || candidate.Fragment != "" {
		return false
	}
	if !strings.EqualFold(candidate.Scheme, streamBase.Scheme) || !strings.EqualFold(candidate.Host, streamBase.Host) || !strings.HasPrefix(candidate.Path, streamBase.Path) {
		return false
	}
	storageID := strings.TrimPrefix(candidate.Path, streamBase.Path)
	if storageID == "" || strings.Contains(storageID, "/") {
		return false
	}
	query := candidate.Query()
	remotePath, fileID := query.Get("path"), query.Get("id")
	if (remotePath == "") == (fileID == "") || len(fileID) > 1024 {
		return false
	}
	locator := storage.CleanRemoteExact(remotePath)
	if fileID != "" {
		locator = "id:" + fileID
	}
	return signature.Valid(signingSecret, storageID, locator, query.Get("sig"))
}

func Server(cfg config.MediaProxyConfig, handler http.Handler) *http.Server {
	return &http.Server{Addr: cfg.Listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second}
}
