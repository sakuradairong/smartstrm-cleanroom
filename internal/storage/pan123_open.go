package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/urlpolicy"
)

const (
	pan123OfficialEndpoint = "https://open-api.123pan.com"
	pan123PageSize         = 100
	pan123MaximumAncestors = 1024
	pan123MaximumRetries   = 3
)

type Pan123Open struct {
	endpoint     string
	clientID     string
	clientSecret string
	rootFileID   int64
	client       *http.Client

	tokenMu     sync.Mutex
	accessToken string
	tokenExpiry time.Time
	limiter     *pan123RateLimiter
}

type pan123RateLimiter struct {
	mu      sync.Mutex
	next    map[string]time.Time
	enabled bool
}

type pan123Envelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

type pan123File struct {
	FileID       int64  `json:"fileId"`
	FileIDUpper  int64  `json:"fileID"`
	Filename     string `json:"filename"`
	ParentFileID int64  `json:"parentFileId"`
	ParentUpper  int64  `json:"parentFileID"`
	Type         int    `json:"type"`
	Size         int64  `json:"size"`
	UpdateAt     string `json:"updateAt"`
	Trashed      int    `json:"trashed"`
}

func NewPan123Open(clientID, clientSecret, rootFileID string) (*Pan123Open, error) {
	driver, err := newPan123Open(pan123OfficialEndpoint, clientID, clientSecret, rootFileID, true)
	if err != nil {
		return nil, fmt.Errorf("create 123 Open driver: %w", err)
	}
	return driver, nil
}

func newPan123Open(endpoint, clientID, clientSecret, rootFileID string, rateLimit bool) (*Pan123Open, error) {
	if strings.TrimSpace(clientID) == "" {
		return nil, fmt.Errorf("123 Open client ID must not be empty")
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("123 Open client secret must not be empty")
	}
	parsed, err := urlpolicy.ParseHTTP(endpoint, false)
	if err != nil {
		return nil, fmt.Errorf("invalid 123 Open endpoint: %w", err)
	}
	rootID := int64(0)
	if rootFileID != "" {
		rootID, err = strconv.ParseInt(rootFileID, 10, 64)
		if err != nil || rootID < 0 {
			return nil, fmt.Errorf("123 Open root file ID must be a non-negative integer")
		}
	}
	return &Pan123Open{
		endpoint: strings.TrimRight(parsed.String(), "/"), clientID: clientID, clientSecret: clientSecret,
		rootFileID: rootID, client: &http.Client{Timeout: 30 * time.Second},
		limiter: &pan123RateLimiter{next: make(map[string]time.Time), enabled: rateLimit},
	}, nil
}

func (p *Pan123Open) List(ctx context.Context, remotePath string) ([]Entry, error) {
	parentID, err := p.resolvePath(ctx, remotePath)
	if err != nil {
		return nil, fmt.Errorf("resolve 123 Open directory: %w", err)
	}
	return p.listDirectory(ctx, parentID, CleanRemoteExact(remotePath))
}

func (p *Pan123Open) Refresh(ctx context.Context, remotePath string) error {
	_, err := p.List(ctx, remotePath)
	return err
}

func (p *Pan123Open) Mkdir(ctx context.Context, remotePath string) error {
	cleaned := CleanRemoteExact(remotePath)
	if cleaned == "/" {
		return fmt.Errorf("cannot create the mounted root")
	}
	name := path.Base(cleaned)
	if err := ValidateName(name); err != nil {
		return err
	}
	parentID, err := p.resolvePath(ctx, path.Dir(cleaned))
	if err != nil {
		return fmt.Errorf("resolve 123 Open parent directory: %w", err)
	}
	return p.call(ctx, http.MethodPost, "/upload/v1/file/mkdir", nil, map[string]any{
		"parentID": strconv.FormatInt(parentID, 10), "name": name,
	}, nil, 2)
}

func (p *Pan123Open) Rename(ctx context.Context, remotePath, newName string) error {
	if CleanRemoteExact(remotePath) == "/" {
		return fmt.Errorf("cannot rename the mounted root")
	}
	if err := ValidateName(newName); err != nil {
		return err
	}
	fileID, err := p.resolvePath(ctx, remotePath)
	if err != nil {
		return fmt.Errorf("resolve 123 Open path: %w", err)
	}
	return p.call(ctx, http.MethodPut, "/api/v1/file/name", nil, map[string]any{
		"fileId": fileID, "fileName": newName,
	}, nil, 1)
}

func (p *Pan123Open) Move(ctx context.Context, remotePath, destinationDirectory string) error {
	if CleanRemoteExact(remotePath) == "/" {
		return fmt.Errorf("cannot move the mounted root")
	}
	fileID, err := p.resolvePath(ctx, remotePath)
	if err != nil {
		return fmt.Errorf("resolve 123 Open path: %w", err)
	}
	parentID, err := p.resolvePath(ctx, destinationDirectory)
	if err != nil {
		return fmt.Errorf("resolve 123 Open destination: %w", err)
	}
	return p.call(ctx, http.MethodPost, "/api/v1/file/move", nil, map[string]any{
		"fileIDs": []int64{fileID}, "toParentFileID": parentID,
	}, nil, 1)
}

func (p *Pan123Open) Delete(ctx context.Context, remotePath string) error {
	if CleanRemoteExact(remotePath) == "/" {
		return fmt.Errorf("cannot delete the mounted root")
	}
	fileID, err := p.resolvePath(ctx, remotePath)
	if err != nil {
		return fmt.Errorf("resolve 123 Open path: %w", err)
	}
	return p.call(ctx, http.MethodPost, "/api/v1/file/trash", nil, map[string]any{
		"fileIDs": []int64{fileID},
	}, nil, 2)
}

func (p *Pan123Open) DirectURL(ctx context.Context, remotePath string) (string, error) {
	fileID, err := p.resolvePath(ctx, remotePath)
	if err != nil {
		return "", fmt.Errorf("resolve 123 Open path: %w", err)
	}
	var data struct {
		DownloadURL string `json:"downloadUrl"`
	}
	query := url.Values{"fileId": {strconv.FormatInt(fileID, 10)}}
	if err := p.call(ctx, http.MethodGet, "/api/v1/file/download_info", query, nil, &data, 5); err != nil {
		return "", fmt.Errorf("get 123 Open download URL: %w", err)
	}
	if data.DownloadURL == "" {
		return "", fmt.Errorf("123 Open returned an empty download URL")
	}
	parsed, err := urlpolicy.ParseHTTP(data.DownloadURL, true)
	if err != nil {
		return "", fmt.Errorf("123 Open returned an invalid download URL: %w", err)
	}
	return parsed.String(), nil
}

func (p *Pan123Open) FileID(ctx context.Context, remotePath string) (string, error) {
	fileID, err := p.resolvePath(ctx, remotePath)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(fileID, 10), nil
}

func (p *Pan123Open) ResolveFileID(ctx context.Context, value string) (string, error) {
	fileID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || fileID < 0 {
		return "", fmt.Errorf("invalid 123 Open file ID")
	}
	if fileID == p.rootFileID {
		return "/", nil
	}
	segments := make([]string, 0, 8)
	seen := make(map[int64]struct{})
	current := fileID
	for range pan123MaximumAncestors {
		if _, exists := seen[current]; exists {
			return "", fmt.Errorf("123 Open file ID is outside mounted root")
		}
		seen[current] = struct{}{}
		item, detailErr := p.fileDetail(ctx, current)
		if detailErr != nil {
			return "", fmt.Errorf("123 Open file ID is outside mounted root: %w", detailErr)
		}
		if item.ID() != 0 && item.ID() != current {
			return "", fmt.Errorf("123 Open file ID is outside mounted root: mismatched file detail")
		}
		if err := ValidateName(item.Filename); err != nil {
			return "", fmt.Errorf("123 Open returned an invalid filename")
		}
		segments = append(segments, item.Filename)
		parentID := item.ParentID()
		if parentID == p.rootFileID {
			slices.Reverse(segments)
			return CleanRemoteExact("/" + strings.Join(segments, "/")), nil
		}
		current = parentID
	}
	return "", fmt.Errorf("123 Open file ID exceeds ancestor limit")
}

func (p *Pan123Open) resolvePath(ctx context.Context, remotePath string) (int64, error) {
	cleaned := CleanRemoteExact(remotePath)
	if cleaned == "/" {
		return p.rootFileID, nil
	}
	current := p.rootFileID
	segments := strings.Split(strings.TrimPrefix(cleaned, "/"), "/")
	for index, segment := range segments {
		files, err := p.listFiles(ctx, current)
		if err != nil {
			return 0, err
		}
		found := false
		for _, item := range files {
			if item.Trashed == 0 && item.Filename == segment {
				if index < len(segments)-1 && item.Type != 1 {
					return 0, fmt.Errorf("%q is not a directory", segment)
				}
				current = item.ID()
				found = true
				break
			}
		}
		if !found {
			return 0, fmt.Errorf("path component %q was not found", segment)
		}
	}
	return current, nil
}

func (p *Pan123Open) listDirectory(ctx context.Context, parentID int64, remotePath string) ([]Entry, error) {
	files, err := p.listFiles(ctx, parentID)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(files))
	for _, item := range files {
		if item.Trashed != 0 {
			continue
		}
		entryPath, err := JoinRemote(remotePath, item.Filename)
		if err != nil {
			return nil, fmt.Errorf("invalid 123 Open entry name")
		}
		entries = append(entries, Entry{Path: entryPath, Name: item.Filename, Size: item.Size, IsDir: item.Type == 1, ModTime: item.ModTime()})
	}
	return entries, nil
}

func (p *Pan123Open) listFiles(ctx context.Context, parentID int64) ([]pan123File, error) {
	files := make([]pan123File, 0)
	seenFiles := make(map[int64]struct{})
	seenCursors := make(map[int64]struct{})
	cursor := int64(0)
	for {
		if _, exists := seenCursors[cursor]; exists {
			return nil, fmt.Errorf("123 Open listing made no progress")
		}
		seenCursors[cursor] = struct{}{}
		var data struct {
			LastFileID int64        `json:"lastFileId"`
			FileList   []pan123File `json:"fileList"`
		}
		query := url.Values{
			"parentFileId": {strconv.FormatInt(parentID, 10)},
			"limit":        {strconv.Itoa(pan123PageSize)},
			"lastFileId":   {strconv.FormatInt(cursor, 10)},
			"trashed":      {"false"},
		}
		if err := p.call(ctx, http.MethodGet, "/api/v2/file/list", query, nil, &data, 3); err != nil {
			return nil, fmt.Errorf("list 123 Open files: %w", err)
		}
		for _, item := range data.FileList {
			fileID := item.ID()
			if fileID <= 0 {
				return nil, fmt.Errorf("123 Open returned an invalid file ID")
			}
			if _, exists := seenFiles[fileID]; exists {
				return nil, fmt.Errorf("123 Open listing returned duplicate file ID")
			}
			seenFiles[fileID] = struct{}{}
			files = append(files, item)
			if len(files) > maximumOpenListEntries {
				return nil, fmt.Errorf("123 Open listing exceeds %d entries", maximumOpenListEntries)
			}
		}
		if data.LastFileID == -1 {
			return files, nil
		}
		if len(data.FileList) == 0 || data.LastFileID <= 0 {
			return nil, fmt.Errorf("123 Open listing ended without a final cursor")
		}
		cursor = data.LastFileID
	}
}

func (p *Pan123Open) fileDetail(ctx context.Context, fileID int64) (pan123File, error) {
	var item pan123File
	query := url.Values{"fileID": {strconv.FormatInt(fileID, 10)}}
	if err := p.call(ctx, http.MethodGet, "/api/v1/file/detail", query, nil, &item, 5); err != nil {
		return pan123File{}, err
	}
	return item, nil
}

func (p *Pan123Open) call(ctx context.Context, method, apiPath string, query url.Values, payload, target any, qps int) error {
	var rejectedAuthorization string
	for attempt := 0; attempt < pan123MaximumRetries; attempt++ {
		token, err := p.getAccessToken(ctx, rejectedAuthorization)
		if err != nil {
			return err
		}
		rejectedAuthorization = ""
		if err := p.limiter.Wait(ctx, apiPath, qps); err != nil {
			return err
		}
		code, err := p.request(ctx, method, apiPath, query, payload, target, token)
		if err != nil {
			return err
		}
		switch code {
		case 0:
			return nil
		case http.StatusUnauthorized:
			rejectedAuthorization = token
		case http.StatusTooManyRequests:
			if err := waitContext(ctx, 500*time.Millisecond); err != nil {
				return err
			}
		default:
			return fmt.Errorf("123 Open returned error code %d", code)
		}
	}
	return fmt.Errorf("123 Open request failed after %d attempts", pan123MaximumRetries)
}

func (p *Pan123Open) request(ctx context.Context, method, apiPath string, query url.Values, payload, target any, token string) (int, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, p.endpoint+apiPath, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Platform", "open_platform")
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return http.StatusUnauthorized, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return http.StatusTooManyRequests, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, responseError(resp)
	}
	data, err := readBoundedResponseBody(resp, maximumMetadataResponseBytes)
	if err != nil {
		return 0, fmt.Errorf("read 123 Open response: %w", err)
	}
	var envelope pan123Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return 0, fmt.Errorf("decode 123 Open response: %w", err)
	}
	if envelope.Code != 0 {
		return envelope.Code, nil
	}
	if target != nil {
		if len(envelope.Data) == 0 || bytes.Equal(envelope.Data, []byte("null")) {
			return 0, fmt.Errorf("123 Open returned empty response data")
		}
		if err := json.Unmarshal(envelope.Data, target); err != nil {
			return 0, fmt.Errorf("decode 123 Open response data: %w", err)
		}
	}
	return 0, nil
}

func (p *Pan123Open) getAccessToken(ctx context.Context, rejectedToken string) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()
	if p.accessToken != "" && time.Until(p.tokenExpiry) > time.Minute && (rejectedToken == "" || p.accessToken != rejectedToken) {
		return p.accessToken, nil
	}
	body, err := json.Marshal(map[string]string{"clientID": p.clientID, "clientSecret": p.clientSecret})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/api/v1/access_token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Platform", "open_platform")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", responseError(resp)
	}
	data, err := readBoundedResponseBody(resp, maximumMetadataResponseBytes)
	if err != nil {
		return "", fmt.Errorf("read 123 Open token response: %w", err)
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			AccessToken string `json:"accessToken"`
			ExpiredAt   string `json:"expiredAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", fmt.Errorf("decode 123 Open token response: %w", err)
	}
	if envelope.Code != 0 {
		return "", fmt.Errorf("123 Open token endpoint returned error code %d", envelope.Code)
	}
	expires, err := time.Parse(time.RFC3339, envelope.Data.ExpiredAt)
	if err != nil || envelope.Data.AccessToken == "" {
		return "", fmt.Errorf("123 Open token endpoint returned invalid token data")
	}
	p.accessToken = envelope.Data.AccessToken
	p.tokenExpiry = expires
	return p.accessToken, nil
}

func (l *pan123RateLimiter) Wait(ctx context.Context, key string, qps int) error {
	if !l.enabled || qps <= 0 {
		return nil
	}
	interval := time.Second / time.Duration(qps)
	l.mu.Lock()
	now := time.Now()
	ready := l.next[key]
	if ready.Before(now) {
		ready = now
	}
	l.next[key] = ready.Add(interval)
	l.mu.Unlock()
	return waitContext(ctx, time.Until(ready))
}

func waitContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (f pan123File) ID() int64 {
	if f.FileID != 0 {
		return f.FileID
	}
	return f.FileIDUpper
}

func (f pan123File) ParentID() int64 {
	if f.ParentFileID != 0 {
		return f.ParentFileID
	}
	return f.ParentUpper
}

func (f pan123File) ModTime() time.Time {
	location := time.FixedZone("UTC+8", 8*60*60)
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", f.UpdateAt, location)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
