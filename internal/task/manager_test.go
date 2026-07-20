package task

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/cronexpr"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/history"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSchedule(t *testing.T) {
	schedule, err := cronexpr.Parse("*/15 1-3 * * 1,3,5")
	require.NoError(t, err)
	assert.True(t, schedule.Matches(time.Date(2025, time.January, 6, 2, 30, 0, 0, time.UTC)))
	assert.False(t, schedule.Matches(time.Date(2025, time.January, 6, 2, 31, 0, 0, time.UTC)))
	assert.False(t, schedule.Matches(time.Date(2025, time.January, 7, 2, 30, 0, 0, time.UTC)))

	schedule, err = cronexpr.Parse("30 2 7 * 1")
	require.NoError(t, err)
	assert.True(t, schedule.Matches(time.Date(2025, time.January, 6, 2, 30, 0, 0, time.UTC)), "weekday match uses standard cron OR semantics")
	assert.True(t, schedule.Matches(time.Date(2025, time.January, 7, 2, 30, 0, 0, time.UTC)), "day-of-month match uses standard cron OR semantics")

	_, err = cronexpr.Parse("60 * * * *")
	require.Error(t, err)
}

func TestManagerPreflightsSchedulesBeforeStartingWorkers(t *testing.T) {
	instance := &cancelStorage{}
	manager := NewManager(
		[]config.TaskConfig{
			{ID: "valid", StorageID: "media", Destination: t.TempDir()},
			{ID: "invalid", StorageID: "media", Destination: t.TempDir(), Schedule: "60 * * * *"},
		},
		map[string]storage.Storage{"media": instance},
		NewGenerator("http://localhost", "secret"),
	)
	err := manager.Start(context.Background())
	require.ErrorContains(t, err, "schedule")
	assert.Nil(t, manager.ctx)
	assert.Empty(t, manager.watchers)
	require.NoError(t, manager.Enqueue("valid", ""))
	time.Sleep(20 * time.Millisecond)
	assert.Zero(t, instance.calls.Load(), "schedule preflight failure must not leave a worker running")
}

type cancelStorage struct{ calls atomic.Int32 }

func (s *cancelStorage) List(ctx context.Context, _ string) ([]storage.Entry, error) {
	if s.calls.Add(1) == 1 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return nil, nil
}
func (*cancelStorage) Mkdir(context.Context, string) error          { return nil }
func (*cancelStorage) Rename(context.Context, string, string) error { return nil }
func (*cancelStorage) Move(context.Context, string, string) error   { return nil }
func (*cancelStorage) Delete(context.Context, string) error         { return nil }
func (*cancelStorage) DirectURL(context.Context, string) (string, error) {
	return "", fmt.Errorf("not used")
}

func TestManagerStopsOnlyCurrentRunAndContinuesQueue(t *testing.T) {
	instance := &cancelStorage{}
	manager := NewManager([]config.TaskConfig{{ID: "task", StorageID: "storage", Destination: t.TempDir()}}, map[string]storage.Storage{"storage": instance}, NewGenerator("http://localhost", "secret"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, manager.Start(ctx))
	require.NoError(t, manager.Enqueue("task", ""))
	require.Eventually(t, func() bool { return manager.Statuses()[0].Running }, time.Second, 5*time.Millisecond)
	require.NoError(t, manager.Stop("task"))
	require.Eventually(t, func() bool {
		status := manager.Statuses()[0]
		return !status.Running && status.Stopped && instance.calls.Load() == 1
	}, 3*time.Second, 5*time.Millisecond)
	require.NoError(t, manager.Enqueue("task", ""))
	require.Eventually(t, func() bool {
		status := manager.Statuses()[0]
		return !status.Running && !status.Stopped && status.Queued == 0 && instance.calls.Load() == 2
	}, 3*time.Second, 5*time.Millisecond)
	status := manager.Statuses()[0]
	assert.Empty(t, status.Error)
	assert.EqualValues(t, 2, instance.calls.Load())
	require.Error(t, manager.Stop("task"))
}

func TestMatchTasksPrefersMostSpecificPath(t *testing.T) {
	manager := NewManager([]config.TaskConfig{
		{ID: "all", StorageID: "media", Source: "/library", Destination: t.TempDir()},
		{ID: "movies", StorageID: "media", Source: "/library/movies", Destination: t.TempDir()},
		{ID: "other", StorageID: "other", Source: "/library/movies", Destination: t.TempDir()},
	}, map[string]storage.Storage{}, NewGenerator("http://localhost", "secret"))
	assert.Equal(t, []string{"movies", "all"}, manager.MatchTasks("media", "/library/movies/demo"))
}

func TestManagerRecordsTaskLifecycle(t *testing.T) {
	root, destination := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "movie.mkv"), []byte("video"), 0o644))
	local, err := storage.NewLocal(root)
	require.NoError(t, err)
	store, err := history.New("", 20)
	require.NoError(t, err)
	manager := NewManager([]config.TaskConfig{{ID: "history", StorageID: "media", Source: "/", Destination: destination, MediaExt: []string{".mkv"}}}, map[string]storage.Storage{"media": local}, NewGenerator("http://localhost", "secret"))
	manager.SetHistory(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, manager.Start(ctx))
	require.NoError(t, manager.Enqueue("history", ""))
	require.Eventually(t, func() bool { return len(store.Snapshot("history", 10)) >= 3 }, time.Second, 10*time.Millisecond)
	events := store.Snapshot("history", 10)
	assert.Equal(t, "completed", events[0].Type)
	assert.Equal(t, "started", events[1].Type)
	assert.Equal(t, "queued", events[2].Type)
	assert.Equal(t, 1, events[0].Created)
}

func TestManagerBatchEnqueueIsAtomicAndCopiesOverrides(t *testing.T) {
	manager := NewManager([]config.TaskConfig{
		{ID: "one", StorageID: "media", Destination: t.TempDir(), KeepLocal: []string{"original"}},
		{ID: "two", StorageID: "media", Destination: t.TempDir()},
	}, map[string]storage.Storage{}, NewGenerator("http://localhost", "secret"))
	override := config.TaskConfig{ID: "one", StorageID: "media", Destination: t.TempDir(), KeepLocal: []string{"override"}}
	err := manager.EnqueueBatch([]BatchRequest{
		{TaskID: "one", Config: &override},
		{TaskID: "missing"},
	}, 0)
	require.ErrorContains(t, err, "unknown task")
	statuses := manager.Statuses()
	assert.Equal(t, 0, statuses[0].Queued+statuses[1].Queued)

	require.NoError(t, manager.EnqueueBatch([]BatchRequest{{TaskID: "one", Config: &override}, {TaskID: "two"}}, 0))
	override.KeepLocal[0] = "mutated"
	request := <-manager.tasks["one"].queue
	require.NotNil(t, request.Config)
	assert.Equal(t, []string{"override"}, request.Config.KeepLocal)
	resolvedID, resolved, err := manager.ResolveTask("one")
	require.NoError(t, err)
	assert.Equal(t, "one", resolvedID)
	resolved.KeepLocal[0] = "changed"
	_, resolvedAgain, err := manager.ResolveTask("one")
	require.NoError(t, err)
	assert.Equal(t, []string{"original"}, resolvedAgain.KeepLocal)
}

func TestManagerBatchCapacityFailureDoesNotPartiallyEnqueue(t *testing.T) {
	manager := NewManager([]config.TaskConfig{
		{ID: "full", StorageID: "media", Destination: t.TempDir()},
		{ID: "empty", StorageID: "media", Destination: t.TempDir()},
	}, map[string]storage.Storage{}, NewGenerator("http://localhost", "secret"))
	for index := 0; index < cap(manager.tasks["full"].queue); index++ {
		require.NoError(t, manager.Enqueue("full", fmt.Sprintf("/%d", index)))
	}
	err := manager.EnqueueBatch([]BatchRequest{{TaskID: "empty"}, {TaskID: "full"}}, 0)
	require.ErrorContains(t, err, "capacity")
	assert.Equal(t, 0, len(manager.tasks["empty"].queue))
	assert.Equal(t, cap(manager.tasks["full"].queue), len(manager.tasks["full"].queue))
}
