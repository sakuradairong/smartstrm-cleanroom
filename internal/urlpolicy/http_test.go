package urlpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHTTP(t *testing.T) {
	for _, value := range []string{"http://example.test", "https://example.test/sub/path"} {
		parsed, err := ParseHTTP(value, false)
		require.NoError(t, err)
		assert.Equal(t, value, parsed.String())
	}
	_, err := ParseHTTP("https://example.test/feed.xml?mirror=1", true)
	require.NoError(t, err)
	for _, value := range []string{
		"", "ftp://example.test", "https://user:pass@example.test", "https://example.test/base?token=x", "https://example.test/base#fragment",
	} {
		_, err := ParseHTTP(value, false)
		require.Error(t, err, value)
	}
}
