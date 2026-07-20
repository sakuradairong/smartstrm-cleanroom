package history

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Event struct {
	ID        uint64    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	TaskID    string    `json:"task_id"`
	Type      string    `json:"type"`
	Path      string    `json:"path,omitempty"`
	Created   int       `json:"created,omitempty"`
	Copied    int       `json:"copied,omitempty"`
	Removed   int       `json:"removed,omitempty"`
	Skipped   int       `json:"skipped,omitempty"`
	Failed    int       `json:"failed,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type Store struct {
	mu          sync.RWMutex
	path        string
	max         int
	events      []Event
	nextID      uint64
	subscribers map[chan Event]struct{}
}

var secretParameter = regexp.MustCompile(`(?i)([?&](?:sig|token|password|api_key|authorization)=)[^&\s]+`)

func New(path string, max int) (*Store, error) {
	if max <= 0 {
		max = 2000
	}
	store := &Store{path: path, max: max, subscribers: make(map[chan Event]struct{})}
	if path == "" {
		return store, nil
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("decode history: %w", err)
		}
		store.events = append(store.events, event)
		if event.ID >= store.nextID {
			store.nextID = event.ID + 1
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(store.events) > max {
		store.events = append([]Event(nil), store.events[len(store.events)-max:]...)
		if err := store.rewriteLocked(); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *Store) Record(event Event) error {
	s.mu.Lock()
	event.ID = s.nextID
	s.nextID++
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	event.Path = Redact(event.Path)
	event.Error = Redact(event.Error)
	s.events = append(s.events, event)
	if len(s.events) > s.max {
		s.events = append([]Event(nil), s.events[len(s.events)-s.max:]...)
	}
	if err := s.persistLocked(event); err != nil {
		s.mu.Unlock()
		return err
	}
	for subscriber := range s.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *Store) Snapshot(taskID string, limit int) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > s.max {
		limit = 100
	}
	result := make([]Event, 0, limit)
	for index := len(s.events) - 1; index >= 0 && len(result) < limit; index-- {
		event := s.events[index]
		if taskID == "" || event.TaskID == taskID {
			result = append(result, event)
		}
	}
	return result
}

func (s *Store) Subscribe(ctx context.Context) <-chan Event {
	channel := make(chan Event, 32)
	s.mu.Lock()
	s.subscribers[channel] = struct{}{}
	s.mu.Unlock()
	go func() { <-ctx.Done(); s.mu.Lock(); delete(s.subscribers, channel); close(channel); s.mu.Unlock() }()
	return channel
}

func (s *Store) persistLocked(event Event) error {
	if s.path == "" {
		return nil
	}
	if len(s.events) == s.max && event.ID > s.nextID-uint64(s.max) {
		return s.rewriteLocked()
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	err = encoder.Encode(event)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (s *Store) rewriteLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".history-*.jsonl")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	encoder := json.NewEncoder(temporary)
	for _, event := range s.events {
		if err := encoder.Encode(event); err != nil {
			temporary.Close()
			return err
		}
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.path)
}

func Redact(value string) string {
	redacted := secretParameter.ReplaceAllString(value, "$1[redacted]")
	if parsed, err := url.Parse(redacted); err == nil && parsed.User != nil {
		parsed.User = nil
		redacted = parsed.String()
	}
	return strings.TrimSpace(redacted)
}
