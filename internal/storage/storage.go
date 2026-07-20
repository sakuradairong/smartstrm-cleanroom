package storage

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
)

const maximumMetadataResponseBytes int64 = 16 << 20

type Entry struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	IsDir   bool      `json:"is_dir"`
	ModTime time.Time `json:"modified"`
}

func readBoundedResponseBody(resp *http.Response, maximum int64) ([]byte, error) {
	if maximum < 0 {
		return nil, fmt.Errorf("response size limit must not be negative")
	}
	if resp.ContentLength > maximum {
		return nil, fmt.Errorf("response body exceeds %d bytes", maximum)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maximum+1))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("response body exceeds %d bytes", maximum)
	}
	return data, nil
}

func decodeStrictXML(data []byte, target any) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	foundRoot := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if !foundRoot {
				return fmt.Errorf("XML document has no root element")
			}
			return nil
		}
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if foundRoot {
				return fmt.Errorf("XML document has trailing element %q", value.Name.Local)
			}
			foundRoot = true
			if err := decoder.DecodeElement(target, &value); err != nil {
				return err
			}
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return fmt.Errorf("XML document has trailing character data")
			}
		}
	}
}

type Storage interface {
	List(ctx context.Context, remotePath string) ([]Entry, error)
	Mkdir(ctx context.Context, remotePath string) error
	Rename(ctx context.Context, remotePath, newName string) error
	Move(ctx context.Context, remotePath, destinationDirectory string) error
	Delete(ctx context.Context, remotePath string) error
	DirectURL(ctx context.Context, remotePath string) (string, error)
}

type HTTPStreamer interface {
	Stream(w http.ResponseWriter, r *http.Request, remotePath string) error
}

type Refresher interface {
	Refresh(ctx context.Context, remotePath string) error
}

// FileIDStorage exposes a provider's stable file identifier. Implementations
// must not use a mutable path as the identifier.
type FileIDStorage interface {
	FileID(ctx context.Context, remotePath string) (string, error)
	ResolveFileID(ctx context.Context, fileID string) (string, error)
}

func BuildAll(configs []config.StorageConfig) (map[string]Storage, error) {
	result := make(map[string]Storage, len(configs))
	for _, cfg := range configs {
		var instance Storage
		var err error
		switch strings.ToLower(cfg.Type) {
		case "local":
			instance, err = NewLocal(cfg.Root)
		case "webdav":
			instance, err = NewWebDAV(cfg.Endpoint, cfg.Root, cfg.Username, cfg.Password)
		case "openlist", "quark", "115", "189", "123":
			// Domestic-cloud aliases intentionally use OpenList's documented API.
			// This keeps provider credentials in OpenList instead of this service.
			instance, err = NewOpenList(cfg.Endpoint, cfg.Root, cfg.Token)
		case "ani_open", "ani-open":
			instance, err = NewANiOpen(cfg.Endpoint)
		default:
			err = fmt.Errorf("unsupported storage type %q", cfg.Type)
		}
		if err != nil {
			return nil, fmt.Errorf("storage %q: %w", cfg.ID, err)
		}
		result[cfg.ID] = instance
	}
	return result, nil
}

func CleanRemote(value string) string {
	cleaned := path.Clean("/" + strings.TrimSpace(value))
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

// CleanRemoteExact normalizes separators and traversal without changing valid
// leading or trailing spaces inside a provider-returned path segment.
func CleanRemoteExact(value string) string {
	cleaned := path.Clean("/" + value)
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func JoinRemote(parent, name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	return path.Join(CleanRemoteExact(parent), name), nil
}

func ValidateName(name string) error {
	if name == "" || name == "." || name == ".." || path.Base(name) != name || strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, '\x00') {
		return fmt.Errorf("invalid file name %q", name)
	}
	return nil
}
