package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/urlpolicy"
)

type OpenList struct {
	endpoint string
	root     string
	token    string
	client   *http.Client
}

type openListResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

const openListPageSize = 200
const maximumOpenListEntries = 100000

type openListItem struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	IsDir    bool      `json:"is_dir"`
	Modified time.Time `json:"modified"`
}

func NewOpenList(endpoint, root, token string) (*OpenList, error) {
	parsed, err := urlpolicy.ParseHTTP(endpoint, false)
	if err != nil {
		return nil, fmt.Errorf("invalid OpenList endpoint: %w", err)
	}
	return &OpenList{endpoint: strings.TrimRight(parsed.String(), "/"), root: CleanRemote(root), token: token, client: &http.Client{Timeout: 30 * time.Second}}, nil
}

func (o *OpenList) List(ctx context.Context, remotePath string) ([]Entry, error) {
	return o.list(ctx, remotePath, false)
}

func (o *OpenList) Refresh(ctx context.Context, remotePath string) error {
	_, err := o.list(ctx, remotePath, true)
	return err
}

func (o *OpenList) list(ctx context.Context, remotePath string, refresh bool) ([]Entry, error) {
	entries := make([]Entry, 0)
	seen := make(map[string]struct{})
	for page := 1; ; page++ {
		var data struct {
			Content []openListItem `json:"content"`
			Total   *int           `json:"total"`
		}
		payload := map[string]any{"path": o.fullPath(remotePath), "page": page, "per_page": openListPageSize, "refresh": refresh && page == 1}
		if err := o.call(ctx, "/api/fs/list", payload, &data); err != nil {
			return nil, fmt.Errorf("list OpenList page %d: %w", page, err)
		}
		if data.Total != nil && (*data.Total < 0 || *data.Total > maximumOpenListEntries) {
			return nil, fmt.Errorf("OpenList listing total %d is outside 0-%d", *data.Total, maximumOpenListEntries)
		}
		if len(data.Content) == 0 {
			if data.Total != nil && len(entries) < *data.Total {
				return nil, fmt.Errorf("OpenList listing ended at %d of %d entries", len(entries), *data.Total)
			}
			return entries, nil
		}
		for _, item := range data.Content {
			entryPath, err := JoinRemote(remotePath, item.Name)
			if err != nil {
				return nil, fmt.Errorf("invalid OpenList entry name %q: %w", item.Name, err)
			}
			if _, exists := seen[entryPath]; exists {
				return nil, fmt.Errorf("OpenList listing made no progress: duplicate entry %q on page %d", entryPath, page)
			}
			seen[entryPath] = struct{}{}
			entries = append(entries, Entry{Path: entryPath, Name: item.Name, Size: item.Size, IsDir: item.IsDir, ModTime: item.Modified})
			if len(entries) > maximumOpenListEntries {
				return nil, fmt.Errorf("OpenList listing exceeds %d entries", maximumOpenListEntries)
			}
		}
		if data.Total != nil {
			if len(entries) > *data.Total {
				return nil, fmt.Errorf("OpenList listing returned %d entries but total is %d", len(entries), *data.Total)
			}
			if len(entries) == *data.Total {
				return entries, nil
			}
		} else if len(data.Content) < openListPageSize {
			return entries, nil
		}
	}
}

func (o *OpenList) Delete(ctx context.Context, remotePath string) error {
	cleaned := CleanRemoteExact(remotePath)
	return o.call(ctx, "/api/fs/remove", map[string]any{"dir": path.Dir(o.fullPath(cleaned)), "names": []string{path.Base(cleaned)}}, nil)
}

func (o *OpenList) Mkdir(ctx context.Context, remotePath string) error {
	return o.call(ctx, "/api/fs/mkdir", map[string]any{"path": o.fullPath(remotePath)}, nil)
}

func (o *OpenList) Rename(ctx context.Context, remotePath, newName string) error {
	if err := ValidateName(newName); err != nil {
		return err
	}
	return o.call(ctx, "/api/fs/rename", map[string]any{"path": o.fullPath(remotePath), "name": newName}, nil)
}

func (o *OpenList) Move(ctx context.Context, remotePath, destinationDirectory string) error {
	cleaned := CleanRemoteExact(remotePath)
	return o.call(ctx, "/api/fs/move", map[string]any{
		"src_dir": o.fullPath(path.Dir(cleaned)), "dst_dir": o.fullPath(destinationDirectory), "names": []string{path.Base(cleaned)},
	}, nil)
}

func (o *OpenList) DirectURL(ctx context.Context, remotePath string) (string, error) {
	var data struct {
		RawURL string `json:"raw_url"`
	}
	if err := o.call(ctx, "/api/fs/get", map[string]any{"path": o.fullPath(remotePath)}, &data); err != nil {
		return "", err
	}
	if data.RawURL == "" {
		return "", fmt.Errorf("OpenList returned an empty raw_url")
	}
	parsed, err := urlpolicy.ParseHTTP(data.RawURL, true)
	if err != nil {
		return "", fmt.Errorf("OpenList returned an invalid raw_url: %w", err)
	}
	return parsed.String(), nil
}

func (o *OpenList) fullPath(remotePath string) string {
	return path.Join(o.root, CleanRemoteExact(remotePath))
}

func (o *OpenList) call(ctx context.Context, apiPath string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint+apiPath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.token != "" {
		req.Header.Set("Authorization", o.token)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	data, err := readBoundedResponseBody(resp, maximumMetadataResponseBytes)
	if err != nil {
		return fmt.Errorf("read OpenList response: %w", err)
	}
	var envelope openListResponse
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode OpenList response: %w", err)
	}
	if envelope.Code != 200 {
		return fmt.Errorf("OpenList returned error code %d", envelope.Code)
	}
	if target != nil {
		return json.Unmarshal(envelope.Data, target)
	}
	return nil
}
