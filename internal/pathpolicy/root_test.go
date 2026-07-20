package pathpolicy

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAbsoluteNonRoot(t *testing.T) {
	cleaned, err := AbsoluteNonRoot("/media/../media/library")
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean("/media/library"), cleaned)
	for _, value := range []string{"", ".", "media", "/", "//"} {
		_, err := AbsoluteNonRoot(value)
		require.Error(t, err, value)
	}
}
