package storage

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/pathpolicy"
)

type Local struct {
	root string
}

func NewLocal(root string) (*Local, error) {
	abs, err := pathpolicy.AbsoluteNonRoot(root)
	if err != nil {
		return nil, fmt.Errorf("invalid local root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	canonical, err = pathpolicy.AbsoluteNonRoot(canonical)
	if err != nil {
		return nil, fmt.Errorf("invalid resolved local root: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root is not a directory")
	}
	return &Local{root: canonical}, nil
}

func (l *Local) Root() string                               { return l.root }
func (l *Local) FilePath(remotePath string) (string, error) { return l.resolve(remotePath) }

func (l *Local) List(_ context.Context, remotePath string) ([]Entry, error) {
	localPath, err := l.resolve(remotePath)
	if err != nil {
		return nil, err
	}
	items, err := os.ReadDir(localPath)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		entryPath, err := JoinRemote(remotePath, item.Name())
		if err != nil {
			return nil, fmt.Errorf("invalid local entry name %q: %w", item.Name(), err)
		}
		resolvedEntry, err := l.resolve(entryPath)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolvedEntry)
		if err != nil {
			continue
		}
		entries = append(entries, Entry{
			Path: entryPath, Name: item.Name(),
			Size: info.Size(), IsDir: info.IsDir(), ModTime: info.ModTime(),
		})
	}
	return entries, nil
}

func (l *Local) Delete(_ context.Context, remotePath string) error {
	localPath, err := l.resolveEntry(remotePath)
	if err != nil {
		return err
	}
	if localPath == l.root {
		return fmt.Errorf("refusing to delete storage root")
	}
	return os.RemoveAll(localPath)
}

func (l *Local) Mkdir(_ context.Context, remotePath string) error {
	target, err := l.resolveNew(remotePath)
	if err != nil {
		return err
	}
	return os.Mkdir(target, 0o755)
}

func (l *Local) Rename(_ context.Context, remotePath, newName string) error {
	if err := ValidateName(newName); err != nil {
		return err
	}
	source, err := l.resolveEntry(remotePath)
	if err != nil {
		return err
	}
	if source == l.root {
		return fmt.Errorf("refusing to rename storage root")
	}
	targetPath, err := JoinRemote(path.Dir(CleanRemoteExact(remotePath)), newName)
	if err != nil {
		return err
	}
	target, err := l.resolveNew(targetPath)
	if err != nil {
		return err
	}
	return os.Rename(source, target)
}

func (l *Local) Move(_ context.Context, remotePath, destinationDirectory string) error {
	source, err := l.resolveEntry(remotePath)
	if err != nil {
		return err
	}
	if source == l.root {
		return fmt.Errorf("refusing to move storage root")
	}
	destination, err := l.resolve(destinationDirectory)
	if err != nil {
		return err
	}
	info, err := os.Stat(destination)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("destination is not a directory")
	}
	target := filepath.Join(destination, filepath.Base(source))
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("destination already contains %q", filepath.Base(source))
	}
	return os.Rename(source, target)
}

func (l *Local) DirectURL(_ context.Context, remotePath string) (string, error) {
	localPath, err := l.resolve(remotePath)
	if err != nil {
		return "", err
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(localPath)}).String(), nil
}

func (l *Local) Stream(writer http.ResponseWriter, request *http.Request, remotePath string) error {
	localPath, err := l.resolve(remotePath)
	if err != nil {
		return err
	}
	http.ServeFile(writer, request, localPath)
	return nil
}

func (l *Local) resolve(remotePath string) (string, error) {
	relative := strings.TrimPrefix(CleanRemoteExact(remotePath), "/")
	resolved := filepath.Join(l.root, filepath.FromSlash(relative))
	if canonical, evalErr := filepath.EvalSymlinks(resolved); evalErr == nil {
		resolved = canonical
	}
	rel, err := filepath.Rel(l.root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes storage root")
	}
	return resolved, nil
}

func (l *Local) resolveNew(remotePath string) (string, error) {
	cleaned := CleanRemoteExact(remotePath)
	if cleaned == "/" {
		return "", fmt.Errorf("storage root already exists")
	}
	if err := ValidateName(filepath.Base(cleaned)); err != nil {
		return "", err
	}
	parent, err := l.resolve(filepath.ToSlash(filepath.Dir(cleaned)))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(parent)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("parent is not a directory")
	}
	return filepath.Join(parent, filepath.Base(cleaned)), nil
}

func (l *Local) resolveEntry(remotePath string) (string, error) {
	cleaned := CleanRemoteExact(remotePath)
	if cleaned == "/" {
		return l.root, nil
	}
	parent, err := l.resolve(path.Dir(cleaned))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(parent)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("parent is not a directory")
	}
	return filepath.Join(parent, filepath.Base(cleaned)), nil
}
