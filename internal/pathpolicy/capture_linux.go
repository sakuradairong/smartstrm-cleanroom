package pathpolicy

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const captureDirectoryPrefix = ".smartstrm-clear-"

// CapturedFile owns a private namespace for an entry removed from its public
// name. The caller must keep parent open and call Close to restore an entry
// that was not deleted. Writers must not modify the private namespace or the
// captured inode through existing descriptors or hard links.
type CapturedFile struct {
	parent    *os.File
	directory *os.File
	name      string
	pending   bool
}

func CaptureFileAt(parent *os.File, name string) (*CapturedFile, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	temporary := captureDirectoryPrefix + hex.EncodeToString(random)
	parentFD := int(parent.Fd())
	if err := unix.Mkdirat(parentFD, temporary, 0o700); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(parentFD, temporary, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		cleanupErr := unix.Unlinkat(parentFD, temporary, unix.AT_REMOVEDIR)
		return nil, errors.Join(err, cleanupErr)
	}
	captured := &CapturedFile{
		parent:    parent,
		directory: os.NewFile(uintptr(fd), filepath.Join(parent.Name(), temporary)),
		name:      filepath.Base(name),
	}
	if err := unix.Renameat2(parentFD, captured.name, fd, "entry", unix.RENAME_NOREPLACE); err != nil {
		return nil, errors.Join(err, captured.Close())
	}
	captured.pending = true
	return captured, nil
}

// RemoveIfUnchanged compares the captured bytes with the ownership-validated
// snapshot. It never unlinks the public name, which may now refer to a new file.
func (c *CapturedFile) RemoveIfUnchanged(expected []byte, maximum int64) (bool, error) {
	data, err := ReadFileAt(c.directory, "entry", maximum)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(data, expected) {
		return false, nil
	}
	if err := unix.Unlinkat(int(c.directory.Fd()), "entry", 0); err != nil {
		return false, err
	}
	c.pending = false
	return true, nil
}

// Close restores an undeleted entry without replacing a newly published file.
// A failed restore retains recovery material; cleanup only removes empty dirs.
func (c *CapturedFile) Close() error {
	var restoreErr error
	if c.pending {
		restoreErr = unix.Renameat2(int(c.directory.Fd()), "entry", int(c.parent.Fd()), c.name, unix.RENAME_NOREPLACE)
		if restoreErr == nil {
			c.pending = false
		} else {
			restoreErr = fmt.Errorf("restore %q: %w; captured entry retained in %q under pinned parent %q", c.name, restoreErr, filepath.Base(c.directory.Name()), c.parent.Name())
		}
	}
	closeErr := c.directory.Close()
	var cleanupErr error
	if !c.pending {
		cleanupErr = unix.Unlinkat(int(c.parent.Fd()), filepath.Base(c.directory.Name()), unix.AT_REMOVEDIR)
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("remove empty capture directory %q: %w", c.directory.Name(), cleanupErr)
		}
	}
	return errors.Join(restoreErr, closeErr, cleanupErr)
}
