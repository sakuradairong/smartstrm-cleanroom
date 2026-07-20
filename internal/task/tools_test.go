package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceSTRMContentAndClear(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "nested"), 0o755))
	strm := filepath.Join(root, "nested", "movie.strm")
	require.NoError(t, os.WriteFile(strm, []byte("http://old/media\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "poster.jpg"), []byte("image"), 0o644))
	preview, err := ReplaceSTRMContent(root, "http://old", "https://new", true)
	require.NoError(t, err)
	assert.Equal(t, ReplaceResult{Scanned: 1, Changed: 1}, preview)
	data, err := os.ReadFile(strm)
	require.NoError(t, err)
	assert.Contains(t, string(data), "http://old")
	applied, err := ReplaceSTRMContent(root, "http://old", "https://new", false)
	require.NoError(t, err)
	assert.Equal(t, 1, applied.Changed)
	data, err = os.ReadFile(strm)
	require.NoError(t, err)
	assert.Contains(t, string(data), "https://new")
	removed, err := ClearGenerated(root)
	require.NoError(t, err)
	assert.Equal(t, 2, removed)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestExtractVideoCoversPreviewAndApply(t *testing.T) {
	root := t.TempDir()
	strm := filepath.Join(root, "movie.strm")
	require.NoError(t, os.WriteFile(strm, []byte("http://media.example/stream/local?path=%2Fmovie.mkv&sig=signed\n"), 0o644))
	logPath := filepath.Join(t.TempDir(), "args.log")
	binary := filepath.Join(t.TempDir(), "fake-ffmpeg")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FFMPEG_ARGS_LOG\"\nfor last; do :; done\nprintf 'jpeg' > \"$last\"\n"
	require.NoError(t, os.WriteFile(binary, []byte(script), 0o755))
	t.Setenv("FFMPEG_ARGS_LOG", logPath)
	options := CoverOptions{Binary: binary, PublicURL: "http://media.example", Position: 12 * time.Second, Timeout: time.Second, Preview: true}
	preview, err := ExtractVideoCovers(context.Background(), root, options)
	require.NoError(t, err)
	assert.Equal(t, 1, preview.Planned)
	assert.Equal(t, 0, preview.Created)
	_, err = os.Stat(filepath.Join(root, "movie.jpg"))
	require.ErrorIs(t, err, os.ErrNotExist)
	options.Preview = false
	result, err := ExtractVideoCovers(context.Background(), root, options)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
	data, err := os.ReadFile(filepath.Join(root, "movie.jpg"))
	require.NoError(t, err)
	assert.Equal(t, "jpeg", string(data))
	arguments, err := os.ReadFile(logPath)
	require.NoError(t, err)
	text := string(arguments)
	assert.Contains(t, text, "-nostdin\n")
	assert.Contains(t, text, "http,https,tcp,tls,crypto\n")
	assert.Contains(t, text, "12.000\n")
	assert.Contains(t, text, "http://media.example/stream/local?path=%2Fmovie.mkv&sig=signed\n")
}

func TestExtractVideoCoversRejectsUnsafeInputAndSymlink(t *testing.T) {
	root := t.TempDir()
	strm := filepath.Join(root, "movie.strm")
	require.NoError(t, os.WriteFile(strm, []byte("http://metadata.invalid/private\n"), 0o644))
	_, err := ExtractVideoCovers(context.Background(), root, CoverOptions{Binary: "does-not-matter", PublicURL: "http://media.example", Timeout: time.Second, Preview: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configured public origin")
	require.NoError(t, os.WriteFile(strm, []byte("http://media.example/stream/local?sig=x\n"), 0o644))
	target := filepath.Join(root, "target.jpg")
	require.NoError(t, os.WriteFile(target, []byte("keep"), 0o644))
	require.NoError(t, os.Symlink(target, filepath.Join(root, "movie.jpg")))
	_, err = ExtractVideoCovers(context.Background(), root, CoverOptions{Binary: "does-not-matter", PublicURL: "http://media.example", Timeout: time.Second, Preview: true, Overwrite: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "regular file")
	data, readErr := os.ReadFile(target)
	require.NoError(t, readErr)
	assert.True(t, strings.Contains(string(data), "keep"))
}

func TestExtractVideoCoversTimeout(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "movie.strm"), []byte("http://media.example/stream/local?sig=x"), 0o644))
	binary := filepath.Join(t.TempDir(), "slow-ffmpeg")
	require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\nsleep 2\n"), 0o755))
	_, err := ExtractVideoCovers(context.Background(), root, CoverOptions{Binary: binary, PublicURL: "http://media.example", Timeout: 20 * time.Millisecond})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestExtractVideoCoversSubpathBoundary(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	require.NoError(t, os.Mkdir(child, 0o755))
	resolved, err := safeToolSubroot(root, "/child")
	require.NoError(t, err)
	assert.Equal(t, child, resolved)
	_, err = safeToolSubroot(root, "../outside")
	require.Error(t, err)
	external := t.TempDir()
	require.NoError(t, os.Symlink(external, filepath.Join(root, "link")))
	_, err = safeToolSubroot(root, "link")
	require.Error(t, err)
}

func TestTaskToolsRejectUnsafeRoots(t *testing.T) {
	_, err := ReplaceSTRMContent("/", "a", "b", false)
	require.Error(t, err)
	_, err = ClearGenerated("relative")
	require.Error(t, err)
}
