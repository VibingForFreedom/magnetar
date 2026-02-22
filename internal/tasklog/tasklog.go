package tasklog

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Entry represents the status of a single scheduled task.
type Entry struct {
	Name       string `json:"name"`
	Interval   string `json:"interval"`
	LastRun    int64  `json:"last_run"`
	NextRun    int64  `json:"next_run"`
	LastResult string `json:"last_result"`
	LastError  bool   `json:"last_error"`
}

// Persister provides key-value persistence for task timestamps.
type Persister interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// Registry is a thread-safe in-memory registry of scheduled task statuses.
type Registry struct {
	mu        sync.RWMutex
	entries   map[string]*Entry
	persister Persister
}

// New creates a new task registry.
func New() *Registry {
	return &Registry{
		entries: make(map[string]*Entry),
	}
}

// SetPersister attaches a persistence backend and hydrates existing entries
// from stored timestamps.
func (r *Registry) SetPersister(ctx context.Context, p Persister) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.persister = p

	for name, e := range r.entries {
		key := settingKey(name)
		val, err := p.GetSetting(ctx, key)
		if err != nil || val == "" {
			continue
		}
		ts, parseErr := strconv.ParseInt(val, 10, 64)
		if parseErr != nil {
			continue
		}
		e.LastRun = ts

		d, durErr := time.ParseDuration(e.Interval)
		if durErr == nil && d > 0 {
			e.NextRun = ts + int64(d.Seconds())
		}

		slog.Info("hydrated task timestamp", "task", name, "last_run", time.Unix(ts, 0).Format(time.RFC3339))
	}
}

// Register adds a task with the given name and interval description.
// If the task already exists, this is a no-op.
func (r *Registry) Register(name string, interval string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.entries[name]; ok {
		return
	}

	r.entries[name] = &Entry{
		Name:     name,
		Interval: interval,
	}
}

// Record updates a task after execution. It sets LastRun to now,
// stores the result string, and computes NextRun from the interval.
func (r *Registry) Record(name string, result string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[name]
	if !ok {
		return
	}

	now := time.Now().Unix()
	e.LastRun = now
	e.LastResult = result
	e.LastError = err != nil

	if err != nil {
		e.LastResult = err.Error()
	}

	d, parseErr := time.ParseDuration(e.Interval)
	if parseErr == nil && d > 0 {
		e.NextRun = now + int64(d.Seconds())
	} else {
		e.NextRun = 0
	}

	// Persist the timestamp (fire-and-forget)
	if r.persister != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if pErr := r.persister.SetSetting(ctx, settingKey(name), strconv.FormatInt(now, 10)); pErr != nil {
				slog.Error("failed to persist task timestamp", "task", name, "error", pErr)
			}
		}()
	}
}

// TimeSinceLastRun returns how long ago the named task last ran.
// Returns math.MaxInt64 duration if the task was never run or is unknown.
func (r *Registry) TimeSinceLastRun(name string) time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[name]
	if !ok || e.LastRun == 0 {
		return time.Duration(math.MaxInt64)
	}
	return time.Since(time.Unix(e.LastRun, 0))
}

// Snapshot returns a sorted copy of all registered task entries.
func (r *Registry) Snapshot() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		entries = append(entries, *e)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries
}

func settingKey(name string) string {
	return fmt.Sprintf("tasklog:%s:last_run", name)
}
