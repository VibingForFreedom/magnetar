package tasklog

import (
	"sort"
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

// Registry is a thread-safe in-memory registry of scheduled task statuses.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*Entry
}

// New creates a new task registry.
func New() *Registry {
	return &Registry{
		entries: make(map[string]*Entry),
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
