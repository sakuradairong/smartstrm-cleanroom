//go:build !linux

package pathpolicy

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// Fail closed instead of silently falling back to path-based mutations.
func unsupported() error                                              { return fmt.Errorf("safe filesystem mutations currently require Linux") }
func OpenDirectory(string, bool) (*os.File, error)                    { return nil, unsupported() }
func AtomicFile(context.Context, string, func(io.Writer) error) error { return unsupported() }
func WalkFiles(context.Context, string, func(*os.File, string, os.DirEntry) error) error {
	return unsupported()
}
func ReadFileAt(*os.File, string, int64) ([]byte, error) { return nil, unsupported() }
func RemoveFileAt(*os.File, string) error                { return unsupported() }
func SetDirectoryTime(string, time.Time) error           { return unsupported() }
func RenameNoReplace(string, string) error               { return unsupported() }
