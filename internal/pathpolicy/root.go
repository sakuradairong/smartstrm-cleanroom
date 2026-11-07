package pathpolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AbsoluteNonRoot validates a filesystem scope without requiring it to exist.
// Callers that resolve symlinks must validate the resolved path again.
func AbsoluteNonRoot(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("must not be empty")
	}
	cleaned := filepath.Clean(value)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("must be an absolute path")
	}
	if cleaned == string(filepath.Separator) {
		return "", fmt.Errorf("must not be the filesystem root")
	}
	return cleaned, nil
}

// Canonical also handles a not-yet-created destination by resolving its
// nearest existing ancestor before comparing filesystem scopes.
func Canonical(value string) (string, error) {
	current, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	var suffix []string
	for {
		_, err = os.Lstat(current)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	for i := len(suffix) - 1; i >= 0; i-- {
		current = filepath.Join(current, suffix[i])
	}
	return current, nil
}

func Overlap(a, b string) bool {
	contains := func(root, value string) bool {
		rel, err := filepath.Rel(root, value)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return contains(a, b) || contains(b, a)
}
