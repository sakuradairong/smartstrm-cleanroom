package pathpolicy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// OpenDirectory walks from / using directory descriptors. No path component
// may be a symlink, including when a parent is replaced during the operation.
func OpenDirectory(name string, create bool) (*os.File, error) {
	if !filepath.IsAbs(name) {
		return nil, fmt.Errorf("directory must be absolute")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(filepath.Clean(name), "/"), "/") {
		if part == "" {
			continue
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e == unix.ENOENT && create {
			e = unix.Mkdirat(fd, part, 0755)
			if e == nil || e == unix.EEXIST {
				next, e = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		unix.Close(fd)
		if e != nil {
			return nil, fmt.Errorf("open safe directory %q: %w", name, e)
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), name), nil
}

func AtomicFile(ctx context.Context, target string, write func(io.Writer) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := OpenDirectory(filepath.Dir(target), true)
	if err != nil {
		return err
	}
	defer dir.Close()
	random := make([]byte, 12)
	if _, err = rand.Read(random); err != nil {
		return err
	}
	temporary := ".smartstrm-copy-" + hex.EncodeToString(random)
	fd, err := unix.Openat(int(dir.Fd()), temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), temporary)
	defer unix.Unlinkat(int(dir.Fd()), temporary, 0)
	if err = write(file); err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = file.Chmod(0644)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return unix.Renameat(int(dir.Fd()), temporary, int(dir.Fd()), filepath.Base(target))
}

// WalkFiles never follows symlinks. Operations in visit are relative to the
// same pinned parent descriptor used to enumerate the entry.
func WalkFiles(ctx context.Context, root string, visit func(*os.File, string, os.DirEntry) error) error {
	dir, err := OpenDirectory(root, false)
	if err != nil {
		return err
	}
	defer dir.Close()
	var walk func(*os.File, string) error
	walk = func(parent *os.File, current string) error {
		entries, e := parent.ReadDir(-1)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if e = ctx.Err(); e != nil {
				return e
			}
			if entry.IsDir() && strings.HasPrefix(entry.Name(), captureDirectoryPrefix) {
				continue
			}
			if entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := filepath.Join(current, entry.Name())
			if entry.IsDir() {
				fd, e := unix.Openat(int(parent.Fd()), entry.Name(), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
				if e != nil {
					return e
				}
				child := os.NewFile(uintptr(fd), name)
				e = walk(child, name)
				child.Close()
				if e != nil {
					return e
				}
			} else if e = visit(parent, name, entry); e != nil {
				return e
			}
		}
		return nil
	}
	return walk(dir, filepath.Clean(root))
}

func ReadFileAt(parent *os.File, name string, maximum int64) ([]byte, error) {
	fd, err := unix.Openat(int(parent.Fd()), filepath.Base(name), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, fmt.Errorf("file must be regular and at most %d bytes", maximum)
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("file exceeds limit")
	}
	return data, err
}

func RemoveFileAt(parent *os.File, name string) error {
	return unix.Unlinkat(int(parent.Fd()), filepath.Base(name), 0)
}

func SetDirectoryTime(name string, modified time.Time) error {
	dir, err := OpenDirectory(name, true)
	if err != nil {
		return err
	}
	defer dir.Close()
	value := unix.NsecToTimeval(modified.UnixNano())
	return unix.Futimes(int(dir.Fd()), []unix.Timeval{value, value})
}

func RenameNoReplace(source, target string) error {
	from, err := OpenDirectory(filepath.Dir(source), false)
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := OpenDirectory(filepath.Dir(target), false)
	if err != nil {
		return err
	}
	defer to.Close()
	return unix.Renameat2(int(from.Fd()), filepath.Base(source), int(to.Fd()), filepath.Base(target), unix.RENAME_NOREPLACE)
}
