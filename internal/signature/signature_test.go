package signature

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateAndValid(t *testing.T) {
	signed := Create("secret", "media", "/movies/demo.mkv")
	assert.NotEmpty(t, signed)
	assert.True(t, Valid("secret", "media", "/movies/demo.mkv", signed))
	assert.False(t, Valid("secret", "media", "/movies/other.mkv", signed))
	assert.False(t, Valid("other-secret", "media", "/movies/demo.mkv", signed))
}
