package pathpolicy

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAtomicFilePinsParentDuringReplacement(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0755); err != nil {
		t.Fatal(err)
	}
	pinned := filepath.Join(root, "pinned")
	err := AtomicFile(context.Background(), filepath.Join(parent, "film.strm"), func(w io.Writer) error {
		if err := os.Rename(parent, pinned); err != nil {
			return err
		}
		if err := os.Symlink(outside, parent); err != nil {
			return err
		}
		_, err := io.WriteString(w, "signed URL")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "film.strm")); !os.IsNotExist(err) {
		t.Fatalf("escaped pinned parent: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(pinned, "film.strm"))
	if err != nil || string(data) != "signed URL" {
		t.Fatalf("pinned output=%q: %v", data, err)
	}
	if err := AtomicFile(context.Background(), filepath.Join(parent, "second.strm"), func(w io.Writer) error { _, err := io.WriteString(w, "bad"); return err }); err == nil {
		t.Fatal("accepted a symlink parent")
	}
}

func TestAtomicFileCreatesParentsAndCleansFailedWrites(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "nested", "deeper", "film.strm")
	write := func(w io.Writer) error { _, err := io.WriteString(w, "original"); return err }
	if err := AtomicFile(context.Background(), target, write); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("copy failed")
	if err := AtomicFile(context.Background(), target, func(w io.Writer) error { _, _ = io.WriteString(w, "partial"); return wantErr }); !errors.Is(err, wantErr) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := AtomicFile(ctx, target, func(w io.Writer) error { cancel(); _, err := io.WriteString(w, "canceled"); return err }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "original" {
		t.Fatalf("existing output changed: %q %v", data, err)
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v %v", entries, err)
	}
}

func TestConcurrentRenameNeverReplacesWinner(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, name := range []string{"a", "b"} {
		workers.Add(1)
		go func(name string) {
			defer workers.Done()
			<-start
			results <- RenameNoReplace(filepath.Join(root, name), filepath.Join(root, "target"))
		}(name)
	}
	close(start)
	workers.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if os.IsExist(err) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	winner, err := os.ReadFile(filepath.Join(root, "target"))
	if err != nil {
		t.Fatal(err)
	}
	loser := "a"
	if string(winner) == "a" {
		loser = "b"
	} else if string(winner) != "b" {
		t.Fatalf("unexpected winner %q", winner)
	}
	data, err := os.ReadFile(filepath.Join(root, loser))
	if err != nil || string(data) != loser {
		t.Fatalf("loser lost: %q %v", data, err)
	}
}

func TestWalkPinsEnumeratedParentAndRejectsSymlinkReads(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	parent := filepath.Join(root, "parent")
	if err := os.Mkdir(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "keep.strm"), []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "keep.strm"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "skip")); err != nil {
		t.Fatal(err)
	}
	visited := 0
	err := WalkFiles(context.Background(), root, func(fd *os.File, current string, entry os.DirEntry) error {
		visited++
		if err := os.Rename(parent, filepath.Join(root, "pinned")); err != nil {
			return err
		}
		if err := os.Symlink(outside, parent); err != nil {
			return err
		}
		data, err := ReadFileAt(fd, entry.Name(), 64)
		if err != nil {
			return err
		}
		if string(data) != "owned" {
			t.Fatalf("read escaped parent: %q", data)
		}
		if err := os.Symlink(filepath.Join(outside, "keep.strm"), filepath.Join(root, "pinned", "link")); err != nil {
			return err
		}
		if _, err := ReadFileAt(fd, "link", 64); err == nil {
			t.Fatal("followed leaf symlink")
		}
		return RemoveFileAt(fd, entry.Name())
	})
	if err != nil || visited != 1 {
		t.Fatalf("visits=%d: %v", visited, err)
	}
	if data, err := os.ReadFile(filepath.Join(outside, "keep.strm")); err != nil || string(data) != "outside" {
		t.Fatalf("outside changed: %q %v", data, err)
	}
}
