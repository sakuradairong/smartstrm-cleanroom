package history

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorePersistsBoundsRestoresAndRedacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	store, err := New(path, 2)
	require.NoError(t, err)
	require.NoError(t, store.Record(Event{TaskID: "one", Type: "failed", Path: "/movie?token=secret", Error: "GET https://example/path?sig=signature&x=1"}))
	require.NoError(t, store.Record(Event{TaskID: "two", Type: "completed"}))
	require.NoError(t, store.Record(Event{TaskID: "one", Type: "completed", Created: 1}))
	events := store.Snapshot("", 10)
	require.Len(t, events, 2)
	assert.Equal(t, "one", events[0].TaskID)
	assert.Equal(t, "two", events[1].TaskID)
	restored, err := New(path, 2)
	require.NoError(t, err)
	events = restored.Snapshot("one", 10)
	require.Len(t, events, 1)
	assert.Equal(t, 1, events[0].Created)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "secret")
	assert.NotContains(t, string(data), "signature")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestStoreSubscriptionIsNonBlockingAndCloses(t *testing.T) {
	store, err := New("", 10)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	events := store.Subscribe(ctx)
	require.NoError(t, store.Record(Event{TaskID: "task", Type: "queued"}))
	select {
	case event := <-events:
		assert.Equal(t, "queued", event.Type)
	case <-time.After(time.Second):
		t.Fatal("subscription did not receive event")
	}
	cancel()
	select {
	case _, open := <-events:
		assert.False(t, open)
	case <-time.After(time.Second):
		t.Fatal("subscription did not close")
	}
}

func TestRedact(t *testing.T) {
	assert.NotContains(t, Redact("https://user:pass@example/a?api_key=secret&x=1"), "pass")
	assert.NotContains(t, Redact("https://example/a?api_key=secret&x=1"), "secret")
}
