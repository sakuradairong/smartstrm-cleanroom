package task

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/signature"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
)

func reviewWrite(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0644); err != nil {
		t.Fatal(err)
	}
}
func reviewLocal(t *testing.T, root string) *storage.Local {
	t.Helper()
	s, e := storage.NewLocal(root)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func TestRegressionDestinationSymlink(t *testing.T) {
	src, dst, outside := t.TempDir(), t.TempDir(), t.TempDir()
	reviewWrite(t, filepath.Join(src, "series", "film.mkv"), "media")
	if err := os.Symlink(outside, filepath.Join(dst, "series")); err != nil {
		t.Fatal(err)
	}
	_, err := NewGenerator("http://example.test", "secret").Run(context.Background(), config.TaskConfig{StorageID: "local", Source: "/", Destination: dst}, reviewLocal(t, src), "")
	if _, e := os.Stat(filepath.Join(outside, "film.strm")); e == nil {
		t.Fatalf("outside file was written through destination symlink; Run error=%v", err)
	}
}

func TestRegressionSharedDestinationDeletesOtherTask(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	reviewWrite(t, filepath.Join(src, "a", "alpha.mkv"), "a")
	reviewWrite(t, filepath.Join(src, "b", "beta.mkv"), "b")
	cfg := config.Config{PublicURL: "http://example.test", WebhookToken: "secret", Storages: []config.StorageConfig{{ID: "local", Type: "local", Root: src}}, Tasks: []config.TaskConfig{{ID: "a", StorageID: "local", Source: "/a", Destination: dst, SyncDelete: true}, {ID: "b", StorageID: "local", Source: "/b", Destination: dst, SyncDelete: true}}}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "destination overlaps task") {
		t.Fatalf("expected output conflict, got %v", err)
	}
}

func TestRegressionClearDoesNotDeleteOriginalMedia(t *testing.T) {
	root := t.TempDir()
	media := filepath.Join(root, "original.mkv")
	reviewWrite(t, media, "irreplaceable source")
	cfg := config.Config{PublicURL: "http://example.test", WebhookToken: "secret", Storages: []config.StorageConfig{{ID: "local", Type: "local", Root: root}}, Tasks: []config.TaskConfig{{ID: "same-root", StorageID: "local", Source: "/", Destination: root}}}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "destination overlaps local storage") {
		t.Fatalf("expected source/output conflict, got %v", err)
	}
}

type reviewCancelStorage struct {
	storage.Storage
	cancel context.CancelFunc
}

func (s reviewCancelStorage) List(ctx context.Context, p string) ([]storage.Entry, error) {
	entries, err := s.Storage.List(ctx, p)
	s.cancel()
	return entries, err
}
func TestRegressionCancelPreventsFurtherWritesAndDeletes(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	reviewWrite(t, filepath.Join(src, "film.mkv"), "media")
	reviewWrite(t, filepath.Join(dst, "stale.strm"), "keep")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := NewGenerator("http://example.test", "secret").Run(ctx, config.TaskConfig{StorageID: "local", Source: "/", Destination: dst, SyncDelete: true}, reviewCancelStorage{reviewLocal(t, src), cancel}, "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if result.Created > 0 || result.Removed > 0 {
		t.Fatalf("after cancellation: Created=%d Removed=%d error=%v", result.Created, result.Removed, err)
	}
}

func TestRegressionPreviewMatchesDirectoryReplacement(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	reviewWrite(t, filepath.Join(src, "old", "film.mkv"), "media")
	cfg := config.TaskConfig{StorageID: "local", Source: "/", Destination: dst, Plugins: []config.PluginConfig{{Type: "replace_regex", Pattern: "^old$", Replacement: "new"}}}
	g := NewGenerator("http://example.test", "secret")
	s := reviewLocal(t, src)
	preview, e := g.Preview(context.Background(), cfg, s, "/old/film.mkv")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = g.Run(context.Background(), cfg, s, ""); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(filepath.Join(dst, filepath.FromSlash(preview.Target)))
	if e != nil {
		t.Fatalf("preview target=%q does not exist after Run: %v", preview.Target, e)
	}
	if string(data) != preview.Content {
		t.Fatalf("preview content=%q; actual=%q", preview.Content, data)
	}
}

func TestClearOnlyRemovesVerifiedTaskSTRMs(t *testing.T) {
	src, dst, outside := t.TempDir(), t.TempDir(), t.TempDir()
	reviewWrite(t, filepath.Join(src, "films", "film.mkv"), "media")
	reviewWrite(t, filepath.Join(src, "other", "other.mkv"), "media")
	g := NewGenerator("http://example.test/base", "secret")
	cfg := config.TaskConfig{ID: "films", StorageID: "local", Source: "/films", Destination: dst}
	source := reviewLocal(t, src)
	if _, err := g.Run(context.Background(), cfg, source, ""); err != nil {
		t.Fatal(err)
	}
	other := cfg
	other.Source = "/other"
	if _, err := g.Run(context.Background(), other, source, ""); err != nil {
		t.Fatal(err)
	}
	preserved := map[string]string{"original.mkv": "media", "poster.jpg": "poster", "sub.srt": "subtitle", "unknown.strm": "https://other.test/video", "nested/unknown.strm": "not a signature", "old-key.strm": "http://example.test/base/stream/local?path=%2Ffilms%2Ffilm.mkv&sig=wrong"}
	for name, content := range preserved {
		reviewWrite(t, filepath.Join(dst, name), content)
	}
	reviewWrite(t, filepath.Join(outside, "keep.strm"), "outside")
	if err := os.Symlink(outside, filepath.Join(dst, "linked")); err != nil {
		t.Fatal(err)
	}
	otherContent, err := os.ReadFile(filepath.Join(dst, "other.strm"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager([]config.TaskConfig{cfg}, map[string]storage.Storage{"local": source}, g)
	result, err := m.Clear("films", false)
	if err != nil || result.Removed != 1 {
		t.Fatalf("removed=%d err=%v", result.Removed, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "film.strm")); !os.IsNotExist(err) {
		t.Fatalf("owned STRM remains: %v", err)
	}
	preserved["other.strm"] = string(otherContent)
	for name, want := range preserved {
		got, err := os.ReadFile(filepath.Join(dst, name))
		if err != nil || string(got) != want {
			t.Fatalf("preserved %s: %q, %v", name, got, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(outside, "keep.strm")); err != nil || string(data) != "outside" {
		t.Fatalf("outside changed: %q, %v", data, err)
	}
}

type replacingFileIDStorage struct {
	storage.Storage
	replace func() error
}

func (s replacingFileIDStorage) FileID(context.Context, string) (string, error) {
	return "film-id", nil
}

func (s replacingFileIDStorage) ResolveFileID(context.Context, string) (string, error) {
	if err := s.replace(); err != nil {
		return "", err
	}
	return "/films/film.mkv", nil
}

func TestClearPreservesChangesDuringOwnershipVerification(t *testing.T) {
	for _, change := range []string{"replacement", "overwrite"} {
		t.Run(change, func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			current := filepath.Join(dst, "film.strm")
			query := url.Values{
				"id":  {"film-id"},
				"sig": {signature.Create("secret", "local", "id:film-id")},
			}
			reviewWrite(t, current, "http://example.test/stream/local?"+query.Encode())
			const manual = "https://manual.example/keep\n"
			source := replacingFileIDStorage{
				Storage: reviewLocal(t, src),
				replace: func() error {
					if change == "overwrite" {
						return os.WriteFile(current, []byte(manual), 0o644)
					}
					replacement := filepath.Join(dst, "replacement.tmp")
					if err := os.WriteFile(replacement, []byte(manual), 0o644); err != nil {
						return err
					}
					return os.Rename(replacement, current)
				},
			}
			cfg := config.TaskConfig{ID: "films", StorageID: "local", Source: "/films", Destination: dst}
			manager := NewManager(
				[]config.TaskConfig{cfg},
				map[string]storage.Storage{"local": source},
				NewGenerator("http://example.test", "secret"),
			)

			result, err := manager.Clear("films", false)

			if err != nil {
				t.Fatal(err)
			}
			if result != (ClearResult{Scanned: 1, Kept: 1}) {
				t.Fatalf("changed file was not retained: %+v", result)
			}
			data, err := os.ReadFile(current)
			if err != nil || string(data) != manual {
				t.Fatalf("manual file changed: %q, %v", data, err)
			}
			entries, err := os.ReadDir(dst)
			if err != nil || len(entries) != 1 || entries[0].Name() != "film.strm" {
				t.Fatalf("unexpected recovery files: %v, %v", entries, err)
			}
		})
	}
}
