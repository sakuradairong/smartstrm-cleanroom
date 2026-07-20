package storage

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadBoundedResponseBody(t *testing.T) {
	t.Run("exact boundary", func(t *testing.T) {
		response := &http.Response{Body: io.NopCloser(bytes.NewReader([]byte("12345678"))), ContentLength: 8}
		data, err := readBoundedResponseBody(response, 8)
		require.NoError(t, err)
		assert.Equal(t, "12345678", string(data))
	})

	t.Run("known oversized length", func(t *testing.T) {
		response := &http.Response{Body: io.NopCloser(bytes.NewReader(nil)), ContentLength: 9}
		_, err := readBoundedResponseBody(response, 8)
		require.ErrorContains(t, err, "exceeds 8 bytes")
	})

	t.Run("unknown oversized length", func(t *testing.T) {
		response := &http.Response{Body: io.NopCloser(bytes.NewReader([]byte("123456789"))), ContentLength: -1}
		_, err := readBoundedResponseBody(response, 8)
		require.ErrorContains(t, err, "exceeds 8 bytes")
	})

	t.Run("negative limit", func(t *testing.T) {
		response := &http.Response{Body: io.NopCloser(bytes.NewReader(nil))}
		_, err := readBoundedResponseBody(response, -1)
		require.ErrorContains(t, err, "must not be negative")
	})
}

func TestResponseErrorDoesNotExposeRemoteText(t *testing.T) {
	response := &http.Response{
		Status:     "599 upstream-secret reason",
		StatusCode: 599,
		Body:       io.NopCloser(bytes.NewReader([]byte("token=body-secret\nnext-line"))),
	}
	err := responseError(response)
	require.EqualError(t, err, "remote returned HTTP status 599")
	assert.NotContains(t, err.Error(), "upstream-secret")
	assert.NotContains(t, err.Error(), "body-secret")
}
