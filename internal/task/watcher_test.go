package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestPollWatcherDetectsChange(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "watch"), 0o755))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	watcher, err := newPollWatcher(local, "/watch")
	require.NoError(t, err)
	watcher.interval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := make(chan struct{}, 1)
	go watcher.run(ctx, func() { changed <- struct{}{} })
	require.NoError(t, os.WriteFile(filepath.Join(root, "watch", "new.mkv"), []byte("video"), 0o644))
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("watcher did not detect the file change")
	}
}
