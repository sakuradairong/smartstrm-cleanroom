package pathpolicy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestCapturedFileLeavesNewPublicEntryUntouched(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "film.strm")
	require.NoError(t, os.WriteFile(current, []byte("owned"), 0o644))
	parent, err := OpenDirectory(root, false)
	require.NoError(t, err)
	defer parent.Close()
	captured, err := CaptureFileAt(parent, "film.strm")
	require.NoError(t, err)

	writeErr := os.WriteFile(current, []byte("new public file"), 0o644)
	removed, removeErr := captured.RemoveIfUnchanged([]byte("owned"), 64)
	closeErr := captured.Close()

	require.NoError(t, writeErr)
	require.NoError(t, removeErr)
	require.True(t, removed)
	require.NoError(t, closeErr)
	data, err := os.ReadFile(current)
	require.NoError(t, err)
	require.Equal(t, "new public file", string(data))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestCapturedFileRetainsRecoveryOnRestoreConflict(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "film.strm")
	require.NoError(t, os.WriteFile(current, []byte("changed"), 0o644))
	parent, err := OpenDirectory(root, false)
	require.NoError(t, err)
	defer parent.Close()
	captured, err := CaptureFileAt(parent, "film.strm")
	require.NoError(t, err)

	writeErr := os.WriteFile(current, []byte("new public file"), 0o644)
	removed, removeErr := captured.RemoveIfUnchanged([]byte("owned"), 64)
	closeErr := captured.Close()

	require.NoError(t, writeErr)
	require.NoError(t, removeErr)
	require.False(t, removed)
	require.ErrorIs(t, closeErr, os.ErrExist)
	data, err := os.ReadFile(current)
	require.NoError(t, err)
	require.Equal(t, "new public file", string(data))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		require.True(t, strings.HasPrefix(entry.Name(), captureDirectoryPrefix))
		require.ErrorContains(t, closeErr, entry.Name())
		info, err := entry.Info()
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
		data, err := os.ReadFile(filepath.Join(root, entry.Name(), "entry"))
		require.NoError(t, err)
		require.Equal(t, "changed", string(data))
	}
	var visited []string
	require.NoError(t, WalkFiles(context.Background(), root, func(_ *os.File, name string, _ os.DirEntry) error {
		visited = append(visited, filepath.Base(name))
		return nil
	}))
	require.Equal(t, []string{"film.strm"}, visited)
}

func TestCapturedFileRejectsUnsafePayloadAndRestoresIt(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo", "directory", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			current := filepath.Join(root, "film.strm")
			external := filepath.Join(outside, "keep")
			require.NoError(t, os.WriteFile(external, []byte("outside"), 0o644))
			switch kind {
			case "symlink":
				require.NoError(t, os.Symlink(external, current))
			case "fifo":
				require.NoError(t, unix.Mkfifo(current, 0o600))
			case "directory":
				require.NoError(t, os.Mkdir(current, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(current, "keep.strm"), []byte("keep"), 0o644))
			case "oversize":
				require.NoError(t, os.WriteFile(current, []byte(strings.Repeat("x", 65)), 0o644))
			}
			before, err := os.Lstat(current)
			require.NoError(t, err)
			parent, err := OpenDirectory(root, false)
			require.NoError(t, err)
			defer parent.Close()
			captured, err := CaptureFileAt(parent, "film.strm")
			require.NoError(t, err)

			removed, removeErr := captured.RemoveIfUnchanged([]byte("owned"), 64)
			closeErr := captured.Close()

			require.False(t, removed)
			require.Error(t, removeErr)
			require.NoError(t, closeErr)
			after, err := os.Lstat(current)
			require.NoError(t, err)
			require.True(t, os.SameFile(before, after))
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			data, err := os.ReadFile(external)
			require.NoError(t, err)
			require.Equal(t, "outside", string(data))
			if kind == "directory" {
				data, err := os.ReadFile(filepath.Join(current, "keep.strm"))
				require.NoError(t, err)
				require.Equal(t, "keep", string(data))
			}
		})
	}
}

func TestCapturedFileRestoresThroughPinnedParent(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	original, pinned := filepath.Join(root, "parent"), filepath.Join(root, "pinned")
	require.NoError(t, os.Mkdir(original, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(original, "film.strm"), []byte("changed"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "film.strm"), []byte("outside"), 0o644))
	parent, err := OpenDirectory(original, false)
	require.NoError(t, err)
	defer parent.Close()
	captured, err := CaptureFileAt(parent, "film.strm")
	require.NoError(t, err)

	renameErr := os.Rename(original, pinned)
	linkErr := os.Symlink(outside, original)
	removed, removeErr := captured.RemoveIfUnchanged([]byte("owned"), 64)
	closeErr := captured.Close()

	require.NoError(t, renameErr)
	require.NoError(t, linkErr)
	require.False(t, removed)
	require.NoError(t, removeErr)
	require.NoError(t, closeErr)
	data, err := os.ReadFile(filepath.Join(pinned, "film.strm"))
	require.NoError(t, err)
	require.Equal(t, "changed", string(data))
	data, err = os.ReadFile(filepath.Join(outside, "film.strm"))
	require.NoError(t, err)
	require.Equal(t, "outside", string(data))
	entries, err := os.ReadDir(pinned)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}
