package storage

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalLifecycleAndContainment(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "movies"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "movies", "demo.mkv"), []byte("video"), 0o644))
	local, err := NewLocal(root)
	require.NoError(t, err)

	entries, err := local.List(context.Background(), "/movies")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "/movies/demo.mkv", entries[0].Path)
	assert.EqualValues(t, 5, entries[0].Size)

	_, err = local.resolve("../../etc/passwd")
	require.NoError(t, err, "remote paths are normalized below the configured root")
	assert.Error(t, local.Delete(context.Background(), "/"))
	require.NoError(t, local.Mkdir(context.Background(), "/archive"))
	require.NoError(t, local.Rename(context.Background(), "/movies/demo.mkv", "renamed.mkv"))
	require.NoError(t, local.Move(context.Background(), "/movies/renamed.mkv", "/archive"))
	_, err = os.Stat(filepath.Join(root, "archive", "renamed.mkv"))
	require.NoError(t, err)
	require.NoError(t, local.Delete(context.Background(), "/archive/renamed.mkv"))
}

func TestLocalRejectsUnsafeRoots(t *testing.T) {
	for _, root := range []string{"", ".", "media", "/"} {
		_, err := NewLocal(root)
		require.Error(t, err, root)
	}
	link := filepath.Join(t.TempDir(), "root-link")
	require.NoError(t, os.Symlink("/", link))
	_, err := NewLocal(link)
	require.ErrorContains(t, err, "resolved local root")
}

func TestLocalSymlinkContainmentAndMutationSemantics(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outFile := filepath.Join(outside, "outside.mkv")
	require.NoError(t, os.WriteFile(outFile, []byte("outside-secret"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(outside, "outside-dir"), 0o755))
	require.NoError(t, os.Symlink(outFile, filepath.Join(root, "escape-file")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "outside-dir"), filepath.Join(root, "escape-dir")))
	insideFile := filepath.Join(root, "inside.mkv")
	require.NoError(t, os.WriteFile(insideFile, []byte("inside-media"), 0o644))
	require.NoError(t, os.Symlink(insideFile, filepath.Join(root, "inside-link")))
	require.NoError(t, os.Mkdir(filepath.Join(root, "archive"), 0o755))

	local, err := NewLocal(root)
	require.NoError(t, err)
	entries, err := local.List(context.Background(), "/")
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	assert.NotContains(t, names, "escape-file")
	assert.NotContains(t, names, "escape-dir")
	assert.Contains(t, names, "inside-link")
	_, err = local.FilePath("/escape-file")
	require.ErrorContains(t, err, "escapes storage root")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/stream", nil)
	require.ErrorContains(t, local.Stream(recorder, request, "/escape-file"), "escapes storage root")
	assert.Empty(t, recorder.Body.String())

	require.NoError(t, local.Delete(context.Background(), "/escape-file"))
	content, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Equal(t, "outside-secret", string(content))
	require.NoError(t, local.Rename(context.Background(), "/escape-dir", "renamed-escape"))
	_, err = os.Lstat(filepath.Join(root, "renamed-escape"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(outside, "outside-dir"))
	require.NoError(t, err)

	require.NoError(t, local.Move(context.Background(), "/inside-link", "/archive"))
	content, err = os.ReadFile(insideFile)
	require.NoError(t, err)
	assert.Equal(t, "inside-media", string(content))
	linkTarget, err := os.Readlink(filepath.Join(root, "archive", "inside-link"))
	require.NoError(t, err)
	assert.Equal(t, insideFile, linkTarget)
}
