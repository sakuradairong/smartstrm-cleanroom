package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/cronexpr"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/history"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/signature"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/storage"
)

type RunRequest struct {
	Path   string
	Config *config.TaskConfig
}

type BatchRequest struct {
	TaskID string
	Path   string
	Config *config.TaskConfig
}

type Status struct {
	TaskID    string    `json:"task_id"`
	Running   bool      `json:"running"`
	Queued    int       `json:"queued"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Result    Result    `json:"result"`
	Error     string    `json:"error,omitempty"`
	Stopped   bool      `json:"stopped,omitempty"`
}

type runtime struct {
	config    config.TaskConfig
	queue     chan RunRequest
	operation sync.Mutex
	mu        sync.RWMutex
	status    Status
	cancel    context.CancelFunc
}

type Manager struct {
	storages    map[string]storage.Storage
	generator   *Generator
	tasks       map[string]*runtime
	watchers    []*pollWatcher
	ctx         context.Context
	history     *history.Store
	enqueueMu   sync.Mutex
	workers     sync.WaitGroup
	outstanding int
	delayed     int
}

func ValidateRuntime(tasks []config.TaskConfig, storages map[string]storage.Storage) error {
	for _, taskConfig := range tasks {
		if taskConfig.Watch {
			if _, ok := storages[taskConfig.StorageID].(*storage.Local); !ok {
				return fmt.Errorf("task %q: watch requires local storage", taskConfig.ID)
			}
		}
	}
	return nil
}

func (m *Manager) SetHistory(store *history.Store) { m.history = store }

func (m *Manager) record(event history.Event) {
	if m.history == nil {
		return
	}
	if err := m.history.Record(event); err != nil {
		slog.Error("persist task history", "task", event.TaskID, "type", event.Type, "error", err)
	}
}

func NewManager(tasks []config.TaskConfig, storages map[string]storage.Storage, generator *Generator) *Manager {
	runtimes := make(map[string]*runtime, len(tasks))
	for _, cfg := range tasks {
		cfg = cloneTaskConfig(cfg)
		runtimes[cfg.ID] = &runtime{config: cfg, queue: make(chan RunRequest, 32), status: Status{TaskID: cfg.ID}}
	}
	return &Manager{storages: storages, generator: generator, tasks: runtimes}
}

func (m *Manager) Start(ctx context.Context) error {
	start, err := m.PrepareStart()
	if err != nil {
		return err
	}
	start(ctx)
	return nil
}

func (m *Manager) PrepareStart() (func(context.Context), error) {
	if err := ValidateRuntime(m.taskConfigs(), m.storages); err != nil {
		return nil, err
	}
	schedules := make(map[string]cronexpr.Schedule)
	watchers := make(map[string]*pollWatcher)
	for id, rt := range m.tasks {
		if rt.config.Schedule != "" {
			parsed, err := cronexpr.Parse(rt.config.Schedule)
			if err != nil {
				return nil, fmt.Errorf("task %q schedule: %w", id, err)
			}
			schedules[id] = parsed
		}
		if rt.config.Watch {
			local := m.storages[rt.config.StorageID].(*storage.Local)
			watcher, err := newPollWatcher(local, rt.config.Source)
			if err != nil {
				return nil, fmt.Errorf("task %q watcher: %w", id, err)
			}
			watchers[id] = watcher
		}
	}
	return func(ctx context.Context) {
		m.ctx = ctx
		for id, rt := range m.tasks {
			id, rt := id, rt
			m.workers.Add(1)
			go func() {
				defer m.workers.Done()
				m.worker(ctx, rt)
			}()
			if schedule, ok := schedules[id]; ok {
				go m.schedule(ctx, id, schedule)
			}
			if rt.config.Watch {
				watcher := watchers[id]
				m.watchers = append(m.watchers, watcher)
				go watcher.run(ctx, func() {
					if err := m.Enqueue(id, ""); err != nil {
						slog.Warn("watcher could not enqueue task", "task", id, "error", err)
					}
				})
			}
		}
	}, nil
}

func (m *Manager) taskConfigs() []config.TaskConfig {
	configs := make([]config.TaskConfig, 0, len(m.tasks))
	for _, rt := range m.tasks {
		configs = append(configs, rt.config)
	}
	return configs
}

func (m *Manager) Wait() { m.workers.Wait() }

func (m *Manager) Enqueue(taskID, sourcePath string) error {
	return m.EnqueueConfig(taskID, sourcePath, nil)
}

func (m *Manager) EnqueueConfig(taskID, sourcePath string, override *config.TaskConfig) error {
	return m.EnqueueBatch([]BatchRequest{{TaskID: taskID, Path: sourcePath, Config: override}}, 0)
}

func (m *Manager) EnqueueAfter(taskID, sourcePath string, override *config.TaskConfig, delay time.Duration) error {
	return m.EnqueueBatch([]BatchRequest{{TaskID: taskID, Path: sourcePath, Config: override}}, delay)
}

func (m *Manager) EnqueueBatch(requests []BatchRequest, delay time.Duration) error {
	if len(requests) == 0 {
		return fmt.Errorf("no task requests")
	}
	prepared := make([]BatchRequest, len(requests))
	for index, request := range requests {
		if _, exists := m.tasks[request.TaskID]; !exists {
			return fmt.Errorf("unknown task %q", request.TaskID)
		}
		prepared[index] = request
		if request.Config != nil {
			copyConfig := cloneTaskConfig(*request.Config)
			prepared[index].Config = &copyConfig
		}
	}
	if delay > 24*time.Hour {
		return fmt.Errorf("delay must not exceed 24 hours")
	}
	if m.ctx != nil {
		select {
		case <-m.ctx.Done():
			return fmt.Errorf("task manager is stopped")
		default:
		}
	}
	if delay <= 0 {
		return m.enqueueBatch(prepared)
	}
	ctx := m.ctx
	if ctx == nil {
		return fmt.Errorf("task manager is not started")
	}
	m.enqueueMu.Lock()
	if m.ctx.Err() != nil {
		m.enqueueMu.Unlock()
		return fmt.Errorf("task manager is stopped")
	}
	m.delayed++
	m.enqueueMu.Unlock()
	time.AfterFunc(delay, func() {
		m.enqueueMu.Lock()
		defer m.enqueueMu.Unlock()
		m.delayed--
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := m.enqueueBatchLocked(prepared); err != nil {
			slog.Warn("delayed task batch enqueue failed", "tasks", len(prepared), "error", err)
		}
	})
	return nil
}

func (m *Manager) enqueueBatch(requests []BatchRequest) error {
	m.enqueueMu.Lock()
	defer m.enqueueMu.Unlock()
	return m.enqueueBatchLocked(requests)
}

func (m *Manager) enqueueBatchLocked(requests []BatchRequest) error {
	if m.ctx != nil && m.ctx.Err() != nil {
		return fmt.Errorf("task manager is stopped")
	}
	counts := make(map[string]int, len(requests))
	for _, request := range requests {
		counts[request.TaskID]++
	}
	for taskID, count := range counts {
		rt := m.tasks[taskID]
		if len(rt.queue)+count > cap(rt.queue) {
			return fmt.Errorf("task %q queue does not have capacity for the batch", taskID)
		}
	}
	for _, request := range requests {
		rt := m.tasks[request.TaskID]
		m.outstanding++
		rt.queue <- RunRequest{Path: request.Path, Config: request.Config}
		rt.mu.Lock()
		rt.status.Queued = len(rt.queue)
		rt.mu.Unlock()
		m.record(history.Event{TaskID: request.TaskID, Type: "queued", Path: request.Path})
	}
	return nil
}

func (m *Manager) ResolveTask(name string) (string, config.TaskConfig, error) {
	if rt, exists := m.tasks[name]; exists {
		return name, cloneTaskConfig(rt.config), nil
	}
	for id, rt := range m.tasks {
		if rt.config.Name == name {
			return id, cloneTaskConfig(rt.config), nil
		}
	}
	return "", config.TaskConfig{}, fmt.Errorf("unknown task %q", name)
}

func cloneTaskConfig(cfg config.TaskConfig) config.TaskConfig {
	cfg.MediaExt = append([]string(nil), cfg.MediaExt...)
	cfg.CopyExt = append([]string(nil), cfg.CopyExt...)
	cfg.Plugins = append([]config.PluginConfig(nil), cfg.Plugins...)
	cfg.KeepLocal = append([]string(nil), cfg.KeepLocal...)
	return cfg
}

func (m *Manager) MatchTasks(storageID, remotePath string) []string {
	cleaned := storage.CleanRemote(remotePath)
	result := make([]string, 0)
	for id, rt := range m.tasks {
		if rt.config.StorageID != storageID {
			continue
		}
		source := storage.CleanRemote(rt.config.Source)
		if cleaned == source || strings.HasPrefix(cleaned, strings.TrimRight(source, "/")+"/") {
			result = append(result, id)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := m.tasks[result[i]].config.Source, m.tasks[result[j]].config.Source
		if len(left) != len(right) {
			return len(left) > len(right)
		}
		return result[i] < result[j]
	})
	return result
}

func (m *Manager) FullOverwrite(taskID, sourcePath string) error {
	id, cfg, err := m.ResolveTask(taskID)
	if err != nil {
		return err
	}
	cfg.Incremental = false
	cfg.SyncDelete = false
	return m.EnqueueConfig(id, sourcePath, &cfg)
}

func (m *Manager) ReplaceContent(taskID, from, to string, preview bool) (ReplaceResult, error) {
	id, cfg, err := m.ResolveTask(taskID)
	if err != nil {
		return ReplaceResult{}, err
	}
	rt := m.tasks[id]
	if !rt.operation.TryLock() {
		return ReplaceResult{}, fmt.Errorf("task %q is running", taskID)
	}
	defer rt.operation.Unlock()
	return ReplaceSTRMContent(cfg.Destination, from, to, preview)
}

func (m *Manager) Clear(taskID string, preview bool) (ClearResult, error) {
	id, cfg, err := m.ResolveTask(taskID)
	if err != nil {
		return ClearResult{}, err
	}
	rt := m.tasks[id]
	if !rt.operation.TryLock() {
		return ClearResult{}, fmt.Errorf("task %q is running", taskID)
	}
	defer rt.operation.Unlock()
	return ClearGenerated(cfg.Destination, preview, func(data []byte) bool {
		parsed, err := url.Parse(strings.TrimSpace(string(data)))
		if err != nil {
			return false
		}
		base, err := url.Parse(strings.TrimRight(m.generator.publicURL, "/") + "/stream/")
		if err != nil {
			return false
		}
		if parsed.Scheme != base.Scheme || parsed.Host != base.Host || parsed.User != nil || parsed.Fragment != "" || parsed.Path != base.Path+cfg.StorageID {
			return false
		}
		q := parsed.Query()
		locator := q.Get("path")
		id := q.Get("id")
		if (locator == "") == (id == "") {
			return false
		}
		if id != "" {
			locator = "id:" + id
		}
		if !signature.Valid(m.generator.token, cfg.StorageID, locator, q.Get("sig")) {
			return false
		}
		if id != "" {
			provider, ok := m.storages[cfg.StorageID].(storage.FileIDStorage)
			if !ok {
				return false
			}
			locator, err = provider.ResolveFileID(context.Background(), id)
			if err != nil {
				return false
			}
		}
		root := storage.CleanRemote(cfg.Source)
		cleaned := storage.CleanRemoteExact(locator)
		return path.Base(parsed.Path) == cfg.StorageID && (cleaned == root || strings.HasPrefix(cleaned, strings.TrimRight(root, "/")+"/"))
	})
}

func (m *Manager) ExtractCovers(ctx context.Context, taskID string, options CoverOptions) (CoverResult, error) {
	id, cfg, err := m.ResolveTask(taskID)
	if err != nil {
		return CoverResult{}, err
	}
	rt := m.tasks[id]
	if !rt.operation.TryLock() {
		return CoverResult{}, fmt.Errorf("task %q is running", taskID)
	}
	defer rt.operation.Unlock()
	return ExtractVideoCovers(ctx, cfg.Destination, options)
}

func (m *Manager) Preview(ctx context.Context, taskID, sourcePath string) (Preview, error) {
	rt, ok := m.tasks[taskID]
	if !ok {
		return Preview{}, fmt.Errorf("unknown task %q", taskID)
	}
	if sourcePath == "" {
		return Preview{}, fmt.Errorf("preview source path must not be empty")
	}
	source, ok := m.storages[rt.config.StorageID]
	if !ok {
		return Preview{}, fmt.Errorf("storage %q is not available", rt.config.StorageID)
	}
	return m.generator.Preview(ctx, cloneTaskConfig(rt.config), source, sourcePath)
}

func (m *Manager) running(taskID string) bool {
	rt, exists := m.tasks[taskID]
	if !exists {
		return false
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.status.Running
}

func (m *Manager) Stop(taskID string) error {
	rt, exists := m.tasks[taskID]
	if !exists {
		return fmt.Errorf("unknown task %q", taskID)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if !rt.status.Running || rt.cancel == nil {
		return fmt.Errorf("task %q is not running", taskID)
	}
	rt.cancel()
	return nil
}

func (m *Manager) Statuses() []Status {
	result := make([]Status, 0, len(m.tasks))
	for _, rt := range m.tasks {
		rt.mu.RLock()
		status := rt.status
		status.Queued = len(rt.queue)
		rt.mu.RUnlock()
		result = append(result, status)
	}
	return result
}

func (m *Manager) Configs() []config.TaskConfig {
	result := make([]config.TaskConfig, 0, len(m.tasks))
	for _, rt := range m.tasks {
		result = append(result, rt.config)
	}
	return result
}

func (m *Manager) worker(ctx context.Context, rt *runtime) {
	for {
		select {
		case <-ctx.Done():
			return
		case request := <-rt.queue:
			rt.operation.Lock()
			runCtx, cancel := context.WithCancel(ctx)
			rt.mu.Lock()
			rt.status.Running = true
			rt.status.StartedAt = time.Now()
			rt.status.Error = ""
			rt.status.Stopped = false
			rt.status.Queued = len(rt.queue)
			rt.cancel = cancel
			rt.mu.Unlock()
			m.record(history.Event{TaskID: rt.config.ID, Type: "started", Path: request.Path})
			runConfig := rt.config
			if request.Config != nil {
				runConfig = *request.Config
			}
			result, err := m.generator.Run(runCtx, runConfig, m.storages[runConfig.StorageID], request.Path)
			cancel()
			rt.mu.Lock()
			rt.status.Running = false
			rt.status.EndedAt = time.Now()
			rt.status.Result = result
			rt.cancel = nil
			if errors.Is(err, context.Canceled) && ctx.Err() == nil {
				rt.status.Stopped = true
			} else if err != nil {
				rt.status.Error = err.Error()
				slog.Error("task failed", "task", rt.config.ID, "error", err)
			}
			rt.mu.Unlock()
			eventType, errorText := "completed", ""
			if errors.Is(err, context.Canceled) && ctx.Err() == nil {
				eventType = "stopped"
			} else if err != nil {
				eventType, errorText = "failed", err.Error()
			}
			m.record(history.Event{TaskID: rt.config.ID, Type: eventType, Path: request.Path, Created: result.Created, Copied: result.Copied, Removed: result.Removed, Skipped: result.Skipped, Failed: result.Failed, Error: errorText})
			rt.operation.Unlock()
			m.enqueueMu.Lock()
			m.outstanding--
			m.enqueueMu.Unlock()
		}
	}
}

func (m *Manager) schedule(ctx context.Context, taskID string, cfg cronexpr.Schedule) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	last := ""
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			key := now.Format("2006-01-02 15:04")
			if key != last && cfg.Matches(now) {
				last = key
				_ = m.Enqueue(taskID, "")
			}
		}
	}
}

// FreezeIdle reserves the idle manager for a configuration handover. Accepted
// work includes delayed and dequeued requests, not just visible channel items.
func (m *Manager) FreezeIdle() (func(), error) {
	m.enqueueMu.Lock()
	if m.outstanding != 0 || m.delayed != 0 {
		m.enqueueMu.Unlock()
		return nil, fmt.Errorf("tasks are running, queued or delayed; retry configuration after they finish")
	}
	locked := make([]*runtime, 0, len(m.tasks))
	release := func() {
		for _, rt := range locked {
			rt.operation.Unlock()
		}
		m.enqueueMu.Unlock()
	}
	for _, rt := range m.tasks {
		if !rt.operation.TryLock() {
			release()
			return nil, fmt.Errorf("task tools are running; retry configuration after they finish")
		}
		locked = append(locked, rt)
	}
	return release, nil
}
