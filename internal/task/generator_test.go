package task

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/signature"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratorPreservesLiteralPercentSequencesInStreamURL(t *testing.T) {
	sourceRoot := t.TempDir()
	destination := t.TempDir()
	const name = "100% ready %23 %2F.mkv"
	require.NoError(t, os.WriteFile(filepath.Join(sourceRoot, name), []byte("percent-video"), 0o644))
	local, err := storage.NewLocal(sourceRoot)
	require.NoError(t, err)
	_, err = NewGenerator("http://localhost:8024", "percent-secret").Run(context.Background(), config.TaskConfig{
		ID: "percent", StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"},
	}, local, "")
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(destination, strings.TrimSuffix(name, ".mkv")+".strm"))
	require.NoError(t, err)
	parsed, err := url.Parse(strings.TrimSpace(string(content)))
	require.NoError(t, err)
	assert.Contains(t, parsed.RawQuery, "100%25+ready+%2523+%252F.mkv")
	assert.Equal(t, "/"+name, parsed.Query().Get("path"))
	assert.True(t, signature.Valid("percent-secret", "media", "/"+name, parsed.Query().Get("sig")))
}

func TestIllegalFilenameTrimsWholeNameAndSanitizesExtension(t *testing.T) {
	plugins := []config.PluginConfig{{Type: "illegal_filename", MaxBytes: 12}}
	name, changed := illegalFilename("  中文中文中文.mkv ", plugins)
	assert.True(t, changed)
	assert.Equal(t, "中文.mkv", name)
	assert.LessOrEqual(t, len([]byte(name)), 12)
	assert.True(t, utf8.ValidString(name))

	name, changed = illegalFilename("Movie.mk?v ", []config.PluginConfig{{Type: "illegal_filename", MaxBytes: 100}})
	assert.True(t, changed)
	assert.Equal(t, "Movie.mk_v", name)
	name, changed = illegalFilename("Clean.mkv", plugins)
	assert.False(t, changed)
	assert.Equal(t, "Clean.mkv", name)
}

func TestGeneratorRenamesTrailingSpacesOnDirectoriesAndFiles(t *testing.T) {
	sourceRoot := t.TempDir()
	destination := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(sourceRoot, " Season 1 "), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceRoot, " Season 1 ", " 电影标题 .MKV "), []byte("video"), 0o644))
	local, err := storage.NewLocal(sourceRoot)
	require.NoError(t, err)
	result, err := NewGenerator("http://localhost", "secret").Run(context.Background(), config.TaskConfig{
		StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"},
		Plugins: []config.PluginConfig{{Type: "illegal_filename", MaxBytes: 240}},
	}, local, "")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
	assert.FileExists(t, filepath.Join(sourceRoot, "Season 1", "电影标题.MKV"))
	assert.FileExists(t, filepath.Join(destination, "Season 1", "电影标题.strm"))
	_, err = os.Stat(filepath.Join(sourceRoot, " Season 1 "))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestGeneratorCreatesCopiesFiltersAndSynchronizes(t *testing.T) {
	sourceRoot := t.TempDir()
	destination := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(sourceRoot, "shows", "Season 1"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceRoot, "shows", "Season 1", "Episode 01 WEB.mkv"), []byte("video"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceRoot, "shows", "poster.jpg"), []byte("poster"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceRoot, "shows", "sample.mkv"), []byte("sample"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(destination, "stale.strm"), []byte("old"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(destination, "keep.nfo"), []byte("metadata"), 0o644))
	local, err := storage.NewLocal(sourceRoot)
	require.NoError(t, err)
	cfg := config.TaskConfig{
		ID: "shows", StorageID: "media", Source: "/shows", Destination: destination,
		SyncDelete: true, MediaExt: []string{".mkv"}, CopyExt: []string{".jpg"},
		KeepLocal: []string{"*.nfo"},
		Plugins: []config.PluginConfig{
			{Type: "skip_regex", Pattern: `(?i)^sample`},
			{Type: "replace_regex", Pattern: ` WEB`, Replacement: ""},
		},
	}

	result, err := NewGenerator("http://localhost:8024", "token").Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, result.Copied)
	assert.Equal(t, 1, result.Removed)
	assert.Equal(t, 1, result.Skipped)
	strm, err := os.ReadFile(filepath.Join(destination, "Season 1", "Episode 01.strm"))
	require.NoError(t, err)
	assert.Contains(t, string(strm), "/stream/media?")
	assert.Contains(t, string(strm), "path=%2Fshows%2FSeason+1%2FEpisode+01+WEB.mkv")
	assert.Contains(t, string(strm), "&sig=")
	_, err = os.Stat(filepath.Join(destination, "poster.jpg"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(destination, "keep.nfo"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(destination, "stale.strm"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestFilenameSkipLiteralAndRegexModes(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	for _, name := range []string{"Movie.mkv", "TRAILER.mkv", "Sample-01.mkv", "sample-02.mkv"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("video"), 0o644))
	}
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	cfg := config.TaskConfig{
		StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"},
		Plugins: []config.PluginConfig{
			{Type: "filename_skip", Pattern: "trailer"},
			{Type: "filename_skip", Pattern: `^Sample-`, MatchMode: "regex", CaseSensitive: true},
		},
	}
	result, err := NewGenerator("http://localhost", "secret").Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Created)
	assert.Equal(t, 2, result.Skipped)
	_, err = os.Stat(filepath.Join(destination, "Movie.strm"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(destination, "sample-02.strm"))
	require.NoError(t, err, "case-sensitive regex must not exclude lowercase input")
}

func TestFilenameSkipIncludeMode(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	for _, name := range []string{"Movie.1080p.mkv", "Movie.720p.mkv", "Keep.1080P.mkv"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("video"), 0o644))
	}
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	cfg := config.TaskConfig{
		StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"},
		Plugins: []config.PluginConfig{{Type: "filename_skip", Pattern: "1080p", FilterMode: "include"}},
	}
	result, err := NewGenerator("http://localhost", "secret").Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Created)
	assert.Equal(t, 1, result.Skipped)
	_, err = os.Stat(filepath.Join(destination, "Movie.720p.strm"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestFilenameSkipDirectoryOnly(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "Extras", "nested"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(root, "Main"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "Extras", "nested", "behind.mkv"), []byte("video"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "Main", "episode.mkv"), []byte("video"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "Extras.mkv"), []byte("video"), 0o644))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	cfg := config.TaskConfig{
		StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"},
		Plugins: []config.PluginConfig{{Type: "filename_skip", Pattern: "extras", DirectoryOnly: true}},
	}
	_, err = NewGenerator("http://localhost", "secret").Preview(context.Background(), cfg, local, "/Extras/nested/behind.mkv")
	require.ErrorContains(t, err, "directory skipped")
	preview, err := NewGenerator("http://localhost", "secret").Preview(context.Background(), cfg, local, "/Main/episode.mkv")
	require.NoError(t, err)
	assert.Equal(t, "Main/episode.strm", preview.Target)
	result, err := NewGenerator("http://localhost", "secret").Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Created)
	assert.Equal(t, 1, result.Skipped)
	_, err = os.Stat(filepath.Join(destination, "Extras.strm"))
	require.NoError(t, err, "directory-only rules must not exclude files")
	_, err = os.Stat(filepath.Join(destination, "Main", "episode.strm"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(destination, "Extras", "nested", "behind.strm"))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestGeneratorRejectsOverrideOutsideTaskRoot(t *testing.T) {
	local, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	_, err = NewGenerator("http://localhost", "token").Run(context.Background(), config.TaskConfig{
		StorageID: "local", Source: "/shows", Destination: t.TempDir(),
	}, local, "/other")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside task source")
}

func TestGeneratorExtendedPlugins(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, `Bad:Name.iso`), []byte("video"), 0o644))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	cfg := config.TaskConfig{
		StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".iso"},
		Plugins: []config.PluginConfig{
			{Type: "illegal_filename", MaxBytes: 100},
			{Type: "infuse_iso"},
			{Type: "strm_replace", Pattern: `localhost`, Replacement: "media.example"},
			{Type: "request_delay", DelayMS: 1},
		},
	}
	result, err := NewGenerator("http://localhost", "secret").Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
	_, err = os.Stat(filepath.Join(root, "Bad_Name.iso"))
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(destination, "Bad_Name.iso.strm"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "media.example")

	destination2 := t.TempDir()
	cfg.Destination = destination2
	cfg.Plugins = []config.PluginConfig{{Type: "custom_strm_filename", Template: "{filename}.link.strm"}}
	_, err = NewGenerator("http://localhost", "secret").Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(destination2, "Bad_Name.iso.link.strm"))
	require.NoError(t, err)
}

func TestGeneratorNamesCopiedAssetsForMatchingMedia(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"Movie.MKV":         "video",
		"Movie.nfo":         "metadata",
		"Movie-poster.jpg":  "poster",
		"Already.mp4":       "video",
		"Already.mp4.nfo":   "metadata",
		"Unmatched-fan.jpg": "unmatched",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o644))
	}
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	destination := t.TempDir()
	result, err := NewGenerator("http://localhost", "secret").Run(context.Background(), config.TaskConfig{
		StorageID: "media", Source: "/", Destination: destination,
		MediaExt: []string{".mkv", ".mp4"}, CopyExt: []string{".nfo", ".jpg"},
	}, local, "")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Created)
	assert.Equal(t, 4, result.Copied)
	assert.FileExists(t, filepath.Join(destination, "Movie.MKV.nfo"))
	assert.FileExists(t, filepath.Join(destination, "Movie.MKV-poster.jpg"))
	assert.FileExists(t, filepath.Join(destination, "Already.mp4.nfo"), "an existing media extension must not be inserted twice")
	assert.FileExists(t, filepath.Join(destination, "Unmatched-fan.jpg"))
	assert.NoFileExists(t, filepath.Join(destination, "Movie.nfo"))
	assert.Equal(t, "Dual.nfo", copiedAssetName("Dual.nfo", []string{"Dual.mkv", "Dual.mp4"}), "ambiguous same-stem media must retain the source asset name")
	assert.Equal(t, "电影.MKV.zh-CN.srt", copiedAssetName("电影.zh-CN.srt", []string{"电影.MKV"}))
	assert.Equal(t, "movie.MKV.nfo", copiedAssetName("movie.nfo", []string{"Movie.MKV"}), "matching is case-insensitive while source asset casing is retained")
}

func TestGeneratorRejectsCopiedAssetTargetCollisionsBeforeWriting(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Movie.MKV", "Movie.nfo", "Movie.MKV.nfo"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(name), 0o644))
	}
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	destination := t.TempDir()
	_, err = NewGenerator("http://localhost", "secret").Run(context.Background(), config.TaskConfig{
		StorageID: "media", Source: "/", Destination: destination,
		MediaExt: []string{".mkv"}, CopyExt: []string{".nfo"},
	}, local, "")
	require.ErrorContains(t, err, "filename collision")
	entries, readErr := os.ReadDir(destination)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "directory target preflight must reject collisions before any writes")
}

func TestGeneratorPreviewMatchesRunWithoutWrites(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	original := filepath.Join(root, `Bad:Name.mkv`)
	require.NoError(t, os.WriteFile(original, []byte("video"), 0o644))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	cfg := config.TaskConfig{
		StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"},
		Plugins: []config.PluginConfig{
			{Type: "illegal_filename", MaxBytes: 100},
			{Type: "custom_strm_filename", Template: "{filename}.link.strm"},
			{Type: "strm_replace", Pattern: `localhost`, Replacement: "media.example"},
			{Type: "request_delay", DelayMS: 100},
		},
	}
	generator := NewGenerator("http://localhost", "secret")
	preview, err := generator.Preview(context.Background(), cfg, local, "/Bad:Name.mkv")
	require.NoError(t, err)
	assert.Equal(t, "/Bad:Name.mkv", preview.Source)
	assert.Equal(t, "Bad_Name.mkv.link.strm", preview.Target)
	assert.Contains(t, preview.Content, "http://media.example/stream/media?")
	assert.Contains(t, preview.Content, "path=%2FBad_Name.mkv")
	assert.True(t, strings.HasSuffix(preview.Content, "\n"))
	_, err = os.Stat(original)
	require.NoError(t, err, "preview must not rename the source")
	entries, err := os.ReadDir(destination)
	require.NoError(t, err)
	assert.Empty(t, entries, "preview must not write the destination")

	result, err := generator.Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
	data, err := os.ReadFile(filepath.Join(destination, preview.Target))
	require.NoError(t, err)
	assert.Equal(t, preview.Content, string(data))
	_, err = os.Stat(filepath.Join(root, "Bad_Name.mkv"))
	require.NoError(t, err)
}

func TestGeneratorPreviewRejectsInvalidSources(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "shows"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "shows", "notes.txt"), []byte("notes"), 0o644))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	cfg := config.TaskConfig{StorageID: "media", Source: "/shows", Destination: t.TempDir(), MediaExt: []string{".mkv"}}
	generator := NewGenerator("http://localhost", "secret")
	for _, sourcePath := range []string{"/outside.mkv", "/shows", "/shows/notes.txt", "/shows/missing.mkv"} {
		_, err = generator.Preview(context.Background(), cfg, local, sourcePath)
		require.Error(t, err, sourcePath)
	}
}

func TestGeneratorPreviewRejectsSiblingTargetCollision(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "one.mkv"), []byte("one"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "two.mkv"), []byte("two"), 0o644))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	cfg := config.TaskConfig{
		StorageID: "media", Source: "/", Destination: t.TempDir(), MediaExt: []string{".mkv"},
		Plugins: []config.PluginConfig{{Type: "custom_strm_filename", Template: "same.strm"}},
	}
	_, err = NewGenerator("http://localhost", "secret").Preview(context.Background(), cfg, local, "/one.mkv")
	require.ErrorContains(t, err, "collision")
}

func TestRequestDelayIsCancelable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := pluginDelay(ctx, []config.PluginConfig{{Type: "request_delay", DelayMS: 1000}})
	require.ErrorIs(t, err, context.Canceled)
}

func TestDirectoryTimeCheckSkipsUnchangedSubtree(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	remoteDirectory := filepath.Join(root, "shows")
	require.NoError(t, os.Mkdir(remoteDirectory, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(remoteDirectory, "episode.mkv"), []byte("video"), 0o644))
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(remoteDirectory, old, old))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	cfg := config.TaskConfig{StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"}, DirTimeCheck: true, SyncDelete: true}
	generator := NewGenerator("http://localhost", "secret")
	first, err := generator.Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 1, first.Created)
	second, err := generator.Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 0, second.Created)
	assert.Equal(t, 1, second.Skipped)
	_, err = os.Stat(filepath.Join(destination, "shows", "episode.strm"))
	require.NoError(t, err, "sync delete must preserve skipped unchanged subtrees")
}

func TestCustomSTRMFilenameRejectsCollisionBeforeWrite(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "one.mkv"), []byte("one"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "two.mkv"), []byte("two"), 0o644))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	cfg := config.TaskConfig{StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"}, Plugins: []config.PluginConfig{{Type: "custom_strm_filename", Template: "same.strm"}}}
	_, err = NewGenerator("http://localhost", "secret").Run(context.Background(), cfg, local, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collision")
	entries, readErr := os.ReadDir(destination)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "preflight collision must not write any conflicting STRM")
}

func TestCustomSTRMFilenameSyncDeleteTracksRenameAndDelete(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	sourcePath := filepath.Join(root, "old.mkv")
	require.NoError(t, os.WriteFile(sourcePath, []byte("video"), 0o644))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	cfg := config.TaskConfig{StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"}, SyncDelete: true, Plugins: []config.PluginConfig{{Type: "custom_strm_filename", Template: "prefix-{name}.link.strm"}}}
	generator := NewGenerator("http://localhost", "secret")
	first, err := generator.Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 1, first.Created)
	oldTarget := filepath.Join(destination, "prefix-old.link.strm")
	_, err = os.Stat(oldTarget)
	require.NoError(t, err)
	require.NoError(t, os.Rename(sourcePath, filepath.Join(root, "new.mkv")))
	second, err := generator.Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 1, second.Created)
	assert.Equal(t, 1, second.Removed)
	_, err = os.Stat(oldTarget)
	require.ErrorIs(t, err, os.ErrNotExist)
	newTarget := filepath.Join(destination, "prefix-new.link.strm")
	_, err = os.Stat(newTarget)
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(root, "new.mkv")))
	third, err := generator.Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 1, third.Removed)
	_, err = os.Stat(newTarget)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCustomSTRMFilenameIncrementalPreservesTarget(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "movie.mkv"), []byte("video"), 0o644))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	cfg := config.TaskConfig{StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"}, Incremental: true, SyncDelete: true, Plugins: []config.PluginConfig{{Type: "custom_strm_filename", Template: "{filename}.custom.strm"}}}
	generator := NewGenerator("http://localhost", "secret")
	_, err = generator.Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	second, err := generator.Run(context.Background(), cfg, local, "")
	require.NoError(t, err)
	assert.Equal(t, 1, second.Skipped)
	assert.Equal(t, 0, second.Removed)
	_, err = os.Stat(filepath.Join(destination, "movie.mkv.custom.strm"))
	require.NoError(t, err)
}

func TestGeneratorFileIDMode(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "Episode.mp4"), []byte("video"), 0o644))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	source := stableIDTestStorage{Storage: local, fileID: "provider-file-123", remotePath: "/Episode.mp4"}
	destination := t.TempDir()
	cfg := config.TaskConfig{ID: "stable", StorageID: "stable", Source: "/", Destination: destination, FileIDMode: true, MediaExt: []string{".mp4"}}
	generator := NewGenerator("http://localhost", "secret")
	preview, err := generator.Preview(context.Background(), cfg, source, "/Episode.mp4")
	require.NoError(t, err)
	assert.Equal(t, "Episode.strm", preview.Target)
	assert.Contains(t, preview.Content, "?id=")
	assert.NotContains(t, preview.Content, "path=")
	_, err = NewGenerator("http://localhost", "secret").Run(context.Background(), cfg, source, "")
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(destination, "Episode.strm"))
	require.NoError(t, err)
	assert.Equal(t, preview.Content, string(content))
	assert.Contains(t, string(content), "?id=")
	assert.NotContains(t, string(content), "path=")
	assert.NotContains(t, string(content), "Episode.mp4")
}

func TestGeneratorContinuesAfterEntryFailureAndSkipsSyncDelete(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Bad.mp4", "Good.mp4"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("video"), 0o644))
	}
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	source := selectiveFileIDTestStorage{Storage: local, failures: map[string]error{"/Bad.mp4": assert.AnError}}
	destination := t.TempDir()
	stale := filepath.Join(destination, "Stale.strm")
	require.NoError(t, os.WriteFile(stale, []byte("keep"), 0o644))
	result, err := NewGenerator("http://localhost", "secret").Run(context.Background(), config.TaskConfig{
		ID: "partial", StorageID: "stable", Source: "/", Destination: destination,
		FileIDMode: true, SyncDelete: true, MediaExt: []string{".mp4"},
	}, source, "")
	require.ErrorContains(t, err, `build STRM content for "/Bad.mp4"`)
	assert.Equal(t, 2, result.Scanned)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, result.Failed)
	assert.Zero(t, result.Removed)
	assert.FileExists(t, filepath.Join(destination, "Good.strm"))
	assert.NoFileExists(t, filepath.Join(destination, "Bad.strm"))
	assert.FileExists(t, stale, "sync deletion must not run after any entry failure")
}

func TestEntryFailureDetailsAreBounded(t *testing.T) {
	failures := &entryFailures{}
	result := &Result{}
	for i := 0; i < maximumEntryFailureDetails+5; i++ {
		failures.add(result, fmt.Errorf("entry-%d", i))
	}
	err := failures.err()
	require.Error(t, err)
	assert.Equal(t, maximumEntryFailureDetails+5, result.Failed)
	assert.ErrorContains(t, err, "5 additional errors omitted")
	assert.NotContains(t, err.Error(), fmt.Sprintf("entry-%d", maximumEntryFailureDetails))
	large := &entryFailures{}
	large.add(&Result{}, fmt.Errorf("provider: %s", strings.Repeat("界", maximumEntryFailureDetailBytes)))
	assert.LessOrEqual(t, len(large.details[0]), maximumEntryFailureDetailBytes)
	assert.True(t, utf8.ValidString(large.details[0]))
	assert.True(t, strings.HasSuffix(large.details[0], "..."))
}

type stableIDTestStorage struct {
	storage.Storage
	fileID     string
	remotePath string
}

type selectiveFileIDTestStorage struct {
	storage.Storage
	failures map[string]error
}

func (s selectiveFileIDTestStorage) FileID(_ context.Context, remotePath string) (string, error) {
	if err := s.failures[remotePath]; err != nil {
		return "", err
	}
	return "id:" + remotePath, nil
}

func (selectiveFileIDTestStorage) ResolveFileID(_ context.Context, fileID string) (string, error) {
	return strings.TrimPrefix(fileID, "id:"), nil
}

func (s stableIDTestStorage) FileID(context.Context, string) (string, error) { return s.fileID, nil }
func (s stableIDTestStorage) ResolveFileID(context.Context, string) (string, error) {
	return s.remotePath, nil
}
