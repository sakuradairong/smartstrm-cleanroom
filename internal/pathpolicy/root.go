package pathpolicy

import (
	"fmt"
	"path/filepath"
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
