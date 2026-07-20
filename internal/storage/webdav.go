package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/urlpolicy"
)

type WebDAV struct {
	endpoint *url.URL
	root     string
	username string
	password string
	client   *http.Client
}

type davMultiStatus struct {
	Responses []struct {
		Href string `xml:"href"`
		Prop struct {
			DisplayName string    `xml:"prop>displayname"`
			Length      string    `xml:"prop>getcontentlength"`
			Modified    string    `xml:"prop>getlastmodified"`
			Collection  *struct{} `xml:"prop>resourcetype>collection"`
		} `xml:"propstat"`
	} `xml:"response"`
}

func NewWebDAV(endpoint, root, username, password string) (*WebDAV, error) {
	parsed, err := urlpolicy.ParseHTTP(endpoint, false)
	if err != nil {
		return nil, fmt.Errorf("invalid WebDAV endpoint: %w", err)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	return &WebDAV{endpoint: parsed, root: CleanRemote(root), username: username, password: password, client: &http.Client{Timeout: 30 * time.Second}}, nil
}

func (w *WebDAV) List(ctx context.Context, remotePath string) ([]Entry, error) {
	req, err := w.request(ctx, "PROPFIND", remotePath, bytes.NewBufferString(`<?xml version="1.0"?><propfind xmlns="DAV:"><prop><displayname/><resourcetype/><getcontentlength/><getlastmodified/></prop></propfind>`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "1")
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, responseError(resp)
	}
	data, err := readBoundedResponseBody(resp, maximumMetadataResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("read WebDAV PROPFIND response: %w", err)
	}
	var result davMultiStatus
	if err := decodeStrictXML(data, &result); err != nil {
		return nil, fmt.Errorf("decode WebDAV PROPFIND response: %w", err)
	}
	entries := make([]Entry, 0, len(result.Responses))
	requested := CleanRemote(remotePath)
	selfPaths := map[string]struct{}{
		normalizeDAVPath(req.URL.Path):                 {},
		normalizeDAVPath(path.Join(w.root, requested)): {},
		normalizeDAVPath(requested):                    {},
	}
	for _, item := range result.Responses {
		hrefPath, err := resolveDAVHrefPath(item.Href, req.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid WebDAV href %q: %w", item.Href, err)
		}
		if _, isSelf := selfPaths[normalizeDAVPath(hrefPath)]; isSelf {
			continue
		}
		name := item.Prop.DisplayName
		if name == "" {
			name = path.Base(strings.TrimRight(hrefPath, "/"))
		}
		size, _ := strconv.ParseInt(item.Prop.Length, 10, 64)
		modified := parseDAVTime(item.Prop.Modified)
		entryPath, err := JoinRemote(requested, name)
		if err != nil {
			return nil, fmt.Errorf("invalid WebDAV entry name %q: %w", name, err)
		}
		entries = append(entries, Entry{Path: entryPath, Name: name, Size: size, IsDir: item.Prop.Collection != nil, ModTime: modified})
	}
	return entries, nil
}

func resolveDAVHrefPath(raw string, requested *url.URL) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	escaped := parsed.EscapedPath()
	if escaped == "" {
		escaped = "."
	}
	if !parsed.IsAbs() && !strings.HasPrefix(escaped, "/") {
		escaped = path.Join(requested.EscapedPath(), escaped)
	}
	decoded, err := url.PathUnescape(escaped)
	if err != nil {
		return "", err
	}
	return normalizeDAVPath(decoded), nil
}

func normalizeDAVPath(value string) string {
	cleaned := path.Clean("/" + value)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func parseDAVTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if parsed, err := http.ParseTime(value); err == nil {
		return parsed
	}
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func (w *WebDAV) Delete(ctx context.Context, remotePath string) error {
	req, err := w.request(ctx, http.MethodDelete, remotePath, nil)
	if err != nil {
		return err
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	return nil
}

func (w *WebDAV) Mkdir(ctx context.Context, remotePath string) error {
	req, err := w.request(ctx, "MKCOL", remotePath, nil)
	if err != nil {
		return err
	}
	return w.doMutation(req)
}

func (w *WebDAV) Rename(ctx context.Context, remotePath, newName string) error {
	if err := ValidateName(newName); err != nil {
		return err
	}
	destination, err := JoinRemote(path.Dir(CleanRemoteExact(remotePath)), newName)
	if err != nil {
		return err
	}
	return w.move(ctx, remotePath, destination)
}

func (w *WebDAV) Move(ctx context.Context, remotePath, destinationDirectory string) error {
	destination, err := JoinRemote(CleanRemote(destinationDirectory), path.Base(CleanRemoteExact(remotePath)))
	if err != nil {
		return err
	}
	return w.move(ctx, remotePath, destination)
}

func (w *WebDAV) move(ctx context.Context, remotePath, destinationPath string) error {
	req, err := w.request(ctx, "MOVE", remotePath, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Destination", w.url(destinationPath))
	req.Header.Set("Overwrite", "F")
	return w.doMutation(req)
}

func (w *WebDAV) doMutation(req *http.Request) error {
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	return nil
}

func (w *WebDAV) DirectURL(_ context.Context, remotePath string) (string, error) {
	return w.url(remotePath), nil
}

func (w *WebDAV) Stream(writer http.ResponseWriter, incoming *http.Request, remotePath string) error {
	method := http.MethodGet
	if incoming.Method == http.MethodHead {
		method = http.MethodHead
	}
	req, err := w.request(incoming.Context(), method, remotePath, nil)
	if err != nil {
		return err
	}
	for _, header := range []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match"} {
		if value := incoming.Header.Get(header); value != "" {
			req.Header.Set(header, value)
		}
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusNotModified && resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		return responseError(resp)
	}
	headers := []string{"Accept-Ranges", "Content-Disposition", "Content-Length", "Content-Range", "Content-Type", "ETag", "Last-Modified"}
	if resp.StatusCode == http.StatusNotModified || resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		headers = []string{"Accept-Ranges", "Content-Range", "ETag", "Last-Modified"}
	}
	for _, header := range headers {
		if values := resp.Header.Values(header); len(values) > 0 {
			writer.Header()[header] = values
		}
	}
	writer.WriteHeader(resp.StatusCode)
	if incoming.Method == http.MethodHead || resp.StatusCode == http.StatusNotModified || resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return nil
	}
	_, _ = io.Copy(writer, resp.Body)
	return nil
}

func (w *WebDAV) request(ctx context.Context, method, remotePath string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, w.url(remotePath), body)
	if err != nil {
		return nil, fmt.Errorf("create WebDAV request: %w", err)
	}
	if w.username != "" {
		req.SetBasicAuth(w.username, w.password)
	}
	return req, nil
}

func (w *WebDAV) url(remotePath string) string {
	copyURL := *w.endpoint
	copyURL.Path = path.Join(w.endpoint.Path, w.root, CleanRemoteExact(remotePath))
	return copyURL.String()
}

func responseError(resp *http.Response) error {
	return fmt.Errorf("remote returned HTTP status %d", resp.StatusCode)
}
