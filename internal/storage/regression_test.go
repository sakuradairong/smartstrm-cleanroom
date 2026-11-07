package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRegressionLocalRenameConflict(t *testing.T) {
	root := t.TempDir()
	for n, s := range map[string]string{"a.mkv": "source", "b.mkv": "existing"} {
		if e := os.WriteFile(filepath.Join(root, n), []byte(s), 0644); e != nil {
			t.Fatal(e)
		}
	}
	s, e := NewLocal(root)
	if e != nil {
		t.Fatal(e)
	}
	err := s.Rename(context.Background(), "/a.mkv", "b.mkv")
	if !os.IsExist(err) {
		t.Fatalf("expected destination-exists error, got %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "a.mkv")); err != nil || string(data) != "source" {
		t.Fatalf("source lost: %q, %v", data, err)
	}
	data, e := os.ReadFile(filepath.Join(root, "b.mkv"))
	if e != nil {
		t.Fatal(e)
	}
	if string(data) != "existing" {
		t.Fatalf("existing destination overwritten: data=%q, Rename error=%v", data, err)
	}
}

func TestRegressionWebDAVLongTransfer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		_, _ = io.WriteString(w, "first")
		w.(http.Flusher).Flush()
		select {
		case <-time.After(200 * time.Millisecond):
			_, _ = io.WriteString(w, "last!")
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	s, e := NewWebDAV(server.URL, "/", "", "")
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("production client timeout=%v; use 30ms for bounded reproduction", s.client.Timeout)
	s.client.Timeout = 30 * time.Millisecond
	response := httptest.NewRecorder()
	err := s.Stream(response, httptest.NewRequest("GET", "http://example.test/stream/local", nil), "/movie.mkv")
	if response.Body.Len() != 10 {
		t.Fatalf("truncated response: status=%d declared=%s actual=%d Stream error=%v", response.Code, response.Header().Get("Content-Length"), response.Body.Len(), err)
	}
}

func TestWebDAVTruncatedTransferAbortsWithoutAppendingError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "10")
		_, _ = io.WriteString(w, "first")
		w.(http.Flusher).Flush()
	}))
	defer upstream.Close()
	s, err := NewWebDAV(upstream.URL, "/", "", "")
	require.NoError(t, err)
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.Stream(w, r, "/movie.mkv"); err != nil {
			http.Error(w, "unexpected JSON error", 500)
		}
	}))
	defer downstream.Close()
	resp, err := http.Get(downstream.URL)
	// The server may close before headers are flushed; otherwise the body must
	// report truncation. Both paths signal a failed transfer to the player.
	if err != nil {
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.Error(t, err)
	require.NotContains(t, string(data), "unexpected JSON error")
	require.LessOrEqual(t, len(data), 5)
}

func TestWebDAVDownstreamCancellationCancelsUpstream(t *testing.T) {
	entered := make(chan struct{})
	canceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		_, _ = io.WriteString(w, "first")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()
	s, err := NewWebDAV(upstream.URL, "/", "", "")
	require.NoError(t, err)
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Force the first chunk through the downstream server's write buffer.
		w.(http.Flusher).Flush()
		_ = s.Stream(w, r, "/movie.mkv")
	}))
	defer downstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", downstream.URL, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream did not receive stream request")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream did not observe cancellation")
	}
}
