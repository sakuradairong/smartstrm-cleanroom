package task

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
)

type fileState struct {
	size     int64
	modified int64
	mode     fs.FileMode
}

type pollWatcher struct {
	root     string
	interval time.Duration
	state    map[string]fileState
}

func newPollWatcher(local *storage.Local, source string) (*pollWatcher, error) {
	root := filepath.Join(local.Root(), filepath.FromSlash(storage.CleanRemote(source)))
	watcher := &pollWatcher{root: root, interval: 2 * time.Second}
	state, err := watcher.snapshot()
	if err != nil {
		return nil, err
	}
	watcher.state = state
	return watcher, nil
}

func (w *pollWatcher) run(ctx context.Context, changed func()) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state, err := w.snapshot()
			if err == nil && differentState(w.state, state) {
				w.state = state
				changed()
			}
		}
	}
}

func (w *pollWatcher) snapshot() (map[string]fileState, error) {
	result := make(map[string]fileState)
	err := filepath.WalkDir(w.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		result[path] = fileState{size: info.Size(), modified: info.ModTime().UnixNano(), mode: info.Mode()}
		return nil
	})
	if os.IsNotExist(err) {
		return result, nil
	}
	return result, err
}

func differentState(left, right map[string]fileState) bool {
	if len(left) != len(right) {
		return true
	}
	for key, value := range left {
		if right[key] != value {
			return true
		}
	}
	return false
}
