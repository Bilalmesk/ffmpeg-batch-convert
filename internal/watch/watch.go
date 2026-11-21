// Package watch monitors folders for new media files and launches batches when
// files have stopped changing.
package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
)

var (
	// ErrNotFound means the watched folder id is unknown.
	ErrNotFound = errors.New("watched folder not found")
	// ErrExists means the exact folder is already watched.
	ErrExists = errors.New("folder is already watched")
	// ErrOverlap means a folder is inside another watched folder (or vice versa).
	ErrOverlap = errors.New("folder overlaps an existing watched folder")
	// ErrNotDir means the path is not usable as a folder.
	ErrNotDir = errors.New("path is not a folder")
)

// Event is one watcher activity line.
type Event struct {
	At       time.Time `json:"at"`
	Level    string    `json:"level"`
	TargetID string    `json:"targetId,omitempty"`
	Message  string    `json:"message"`
}

// Batch is a set of ready files for one watched folder.
type Batch struct {
	TargetID string
	Dir      string
	Files    []string
}

// Target is the serialisable snapshot of a watched folder.
type Target struct {
	ID          string     `json:"id"`
	Dir         string     `json:"dir"`
	Recursive   bool       `json:"recursive"`
	Enabled     bool       `json:"enabled"`
	AddedAt     time.Time  `json:"addedAt"`
	LastScanAt  *time.Time `json:"lastScanAt,omitempty"`
	LastStartAt *time.Time `json:"lastStartAt,omitempty"`
	Pending     int        `json:"pending"`
	Tracked     int        `json:"tracked"`
	LastError   string     `json:"lastError,omitempty"`
}

// Status is the dashboard view of the watcher.
type Status struct {
	Running         bool     `json:"running"`
	IntervalSeconds int      `json:"intervalSeconds"`
	SettleSeconds   int      `json:"settleSeconds"`
	Targets         []Target `json:"targets"`
	Events          []Event  `json:"events"`
}

type fileState struct {
	size      int64
	mod       time.Time
	firstSeen time.Time
	launched  bool
}

type watchedFolder struct {
	Target
	files       map[string]*fileState
	nextAttempt time.Time
}

// Options configures a Watcher. Launch must return when the run has been
// started; in the CLI it may block until the run has finished.
type Options struct {
	Interval time.Duration
	Settle   time.Duration
	Config   func() config.Config
	Busy     func() bool
	Launch   func(ctx context.Context, cfg config.Config, files []string) error
	OnEvent  func(Event)
	Now      func() time.Time
}

// Watcher owns the watched folders and the polling loop.
type Watcher struct {
	mu      sync.Mutex
	opts    Options
	targets []*watchedFolder
	events  []Event
	seq     int

	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// New creates a watcher with sane defaults.
func New(opts Options) *Watcher {
	if opts.Interval <= 0 {
		opts.Interval = 5 * time.Second
	}
	if opts.Settle < 0 {
		opts.Settle = 0
	}
	if opts.Config == nil {
		opts.Config = func() config.Config { return config.Default() }
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Busy == nil {
		opts.Busy = func() bool { return false }
	}
	return &Watcher{opts: opts}
}

// Add starts watching dir. Existing files are ignored unless convertExisting is
// true, in which case they are queued after the settle delay.
func (w *Watcher) Add(dir string, recursive, convertExisting bool) (Target, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Target{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Target{}, fmt.Errorf("open folder: %w", err)
	}
	if !info.IsDir() {
		return Target{}, ErrNotDir
	}

	cfg := w.opts.Config()
	cfg.Normalize()
	candidates, err := engine.Scan(abs, recursive, engine.ScanExtensions(cfg))
	if err != nil {
		return Target{}, err
	}

	now := w.now()
	w.mu.Lock()
	for _, existing := range w.targets {
		if sameFolder(existing.Dir, abs) {
			w.mu.Unlock()
			return Target{}, ErrExists
		}
		if foldersOverlap(existing.Dir, abs) {
			w.mu.Unlock()
			return Target{}, fmt.Errorf("%w: %s and %s", ErrOverlap, existing.Dir, abs)
		}
	}
	w.seq++
	t := &watchedFolder{
		Target: Target{
			ID:        fmt.Sprintf("watch-%03d-%x", w.seq, now.UnixNano()),
			Dir:       abs,
			Recursive: recursive,
			Enabled:   true,
			AddedAt:   now,
		},
		files: make(map[string]*fileState, len(candidates)),
	}
	for _, candidate := range candidates {
		if ignoreFile(candidate.Path, cfg) {
			continue
		}
		t.files[candidate.Path] = &fileState{
			size:      candidate.SizeBytes,
			mod:       candidate.ModTime,
			firstSeen: now,
			launched:  !convertExisting,
		}
	}
	t.Pending = pendingCount(t)
	w.targets = append(w.targets, t)
	event := w.pushEventLocked("info", t.ID, fmt.Sprintf("Watching %s", abs))
	snapshot := w.targetSnapshotLocked(t)
	w.mu.Unlock()

	w.announce(event)
	return snapshot, nil
}

// Remove stops watching a folder. Files and existing runs are untouched.
func (w *Watcher) Remove(id string) error {
	w.mu.Lock()
	index := -1
	var removed *watchedFolder
	for i, t := range w.targets {
		if t.ID == id {
			index = i
			removed = t
			break
		}
	}
	if index < 0 {
		w.mu.Unlock()
		return ErrNotFound
	}
	w.targets = append(w.targets[:index], w.targets[index+1:]...)
	event := w.pushEventLocked("info", id, "Stopped watching "+removed.Dir)
	w.mu.Unlock()

	w.announce(event)
	return nil
}

// SetEnabled pauses or resumes a watched folder.
func (w *Watcher) SetEnabled(id string, enabled bool) (Target, error) {
	w.mu.Lock()
	t := w.findLocked(id)
	if t == nil {
		w.mu.Unlock()
		return Target{}, ErrNotFound
	}
	t.Enabled = enabled
	message := "Paused " + t.Dir
	level := "warn"
	if enabled {
		message = "Resumed " + t.Dir
		level = "info"
	}
	event := w.pushEventLocked(level, id, message)
	snapshot := w.targetSnapshotLocked(t)
	w.mu.Unlock()

	w.announce(event)
	return snapshot, nil
}

// Snapshot returns the current dashboard view.
func (w *Watcher) Snapshot() Status {
	running := w.opts.Busy()
	w.mu.Lock()
	defer w.mu.Unlock()
	status := Status{
		Running:         running,
		IntervalSeconds: int(w.opts.Interval.Seconds()),
		SettleSeconds:   int(w.opts.Settle.Seconds()),
		Targets:         make([]Target, 0, len(w.targets)),
	}
	for _, t := range w.targets {
		status.Targets = append(status.Targets, w.targetSnapshotLocked(t))
	}
	const eventLimit = 150
	start := 0
	if len(w.events) > eventLimit {
		start = len(w.events) - eventLimit
	}
	for i := len(w.events) - 1; i >= start; i-- {
		status.Events = append(status.Events, w.events[i])
	}
	return status
}

// Start launches the polling loop. Calling Start twice is a no-op.
func (w *Watcher) Start(parent context.Context) {
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	w.started = true
	w.cancel = cancel
	w.done = done
	w.mu.Unlock()

	go w.loop(ctx, done)
}

// Stop ends the polling loop and waits for it to exit. It is safe to call Stop
// more than once or before Start.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	cancel := w.cancel
	done := w.done
	w.started = false
	w.cancel = nil
	w.done = nil
	w.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (w *Watcher) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(w.opts.Interval)
	defer ticker.Stop()

	for {
		w.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Watcher) tick(ctx context.Context) {
	busy := w.opts.Busy()
	cfg := w.opts.Config()
	cfg.Normalize()
	batches, events := w.collectAll(w.now(), cfg, !busy)
	for _, event := range events {
		w.announce(event)
	}
	for _, batch := range batches {
		if ctx.Err() != nil {
			return
		}
		w.mu.Lock()
		event := w.pushEventLocked("info", batch.TargetID,
			fmt.Sprintf("Starting a batch of %d %s in %s", len(batch.Files), countWord(len(batch.Files), "file", "files"), batch.Dir))
		w.mu.Unlock()
		w.announce(event)

		if err := w.opts.Launch(ctx, cfg, batch.Files); err != nil {
			w.requeue(batch, err)
		}
	}
}

// collectAll scans every enabled folder. When allowLaunch is false it only
// updates state, so a busy engine can finish without losing ready files.
func (w *Watcher) collectAll(now time.Time, cfg config.Config, allowLaunch bool) ([]Batch, []Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var batches []Batch
	var events []Event
	collectedBatch := false

	for _, t := range w.targets {
		if !t.Enabled || now.Before(t.nextAttempt) {
			continue
		}
		scanAt := now
		t.LastScanAt = &scanAt
		candidates, err := engine.Scan(t.Dir, t.Recursive, engine.ScanExtensions(cfg))
		if err != nil {
			message := err.Error()
			if t.LastError != message {
				t.LastError = message
				events = append(events, w.pushEventLocked("error", t.ID, "Scan failed: "+message))
			}
			continue
		}
		if t.LastError != "" {
			t.LastError = ""
			events = append(events, w.pushEventLocked("info", t.ID, "Folder is readable again: "+t.Dir))
		}

		seen := make(map[string]bool, len(candidates))
		var ready []string
		for _, candidate := range candidates {
			if ignoreFile(candidate.Path, cfg) {
				continue
			}
			seen[candidate.Path] = true
			state := t.files[candidate.Path]
			if state == nil {
				state = &fileState{firstSeen: now}
				t.files[candidate.Path] = state
			}
			if state.size != candidate.SizeBytes || !state.mod.Equal(candidate.ModTime) {
				state.size = candidate.SizeBytes
				state.mod = candidate.ModTime
				state.firstSeen = now
				state.launched = false
			}
			if state.launched || candidate.SizeBytes <= 0 {
				continue
			}
			if now.Sub(state.firstSeen) < w.opts.Settle {
				continue
			}
			if allowLaunch {
				ready = append(ready, candidate.Path)
			}
		}
		for path := range t.files {
			if !seen[path] {
				delete(t.files, path)
			}
		}
		if len(ready) > 0 && allowLaunch && !collectedBatch {
			sort.Strings(ready)
			for _, path := range ready {
				if state := t.files[path]; state != nil {
					state.launched = true
				}
			}
			startAt := now
			t.LastStartAt = &startAt
			batches = append(batches, Batch{TargetID: t.ID, Dir: t.Dir, Files: ready})
			events = append(events, w.pushEventLocked("info", t.ID,
				fmt.Sprintf("%d new %s ready in %s", len(ready), countWord(len(ready), "file", "files"), t.Dir)))
			collectedBatch = true
		}
		t.Pending = pendingCount(t)
	}
	return batches, events
}

func (w *Watcher) requeue(batch Batch, err error) {
	now := w.now()
	level := "error"
	delay := 30 * time.Second
	if errors.Is(err, engine.ErrBusy) {
		level = "warn"
		delay = 10 * time.Second
	}

	w.mu.Lock()
	t := w.findLocked(batch.TargetID)
	if t == nil {
		w.mu.Unlock()
		return
	}
	t.LastError = err.Error()
	t.nextAttempt = now.Add(delay)
	for _, path := range batch.Files {
		if state := t.files[path]; state != nil {
			state.launched = false
		}
	}
	t.Pending = pendingCount(t)
	event := w.pushEventLocked(level, t.ID,
		fmt.Sprintf("Could not start a batch in %s: %v", t.Dir, err))
	w.mu.Unlock()
	w.announce(event)
}

func (w *Watcher) findLocked(id string) *watchedFolder {
	for _, t := range w.targets {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (w *Watcher) targetSnapshotLocked(t *watchedFolder) Target {
	snapshot := t.Target
	snapshot.Tracked = len(t.files)
	snapshot.Pending = pendingCount(t)
	return snapshot
}

func countWord(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func pendingCount(t *watchedFolder) int {
	count := 0
	for _, state := range t.files {
		if !state.launched {
			count++
		}
	}
	return count
}

func (w *Watcher) pushEventLocked(level, targetID, message string) Event {
	event := Event{At: w.now(), Level: level, TargetID: targetID, Message: message}
	w.events = append(w.events, event)
	if len(w.events) > 300 {
		w.events = append([]Event(nil), w.events[len(w.events)-300:]...)
	}
	return event
}

func (w *Watcher) announce(event Event) {
	if w.opts.OnEvent != nil {
		w.opts.OnEvent(event)
	}
}

func (w *Watcher) now() time.Time {
	return w.opts.Now()
}

// ignoreFile excludes files that would be produced by the configured output.
func ignoreFile(path string, cfg config.Config) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == strings.ToLower(cfg.Format) {
		return true
	}
	if cfg.OutputDir != "" {
		if out, err := filepath.Abs(cfg.OutputDir); err == nil && isUnder(path, out) {
			return true
		}
	}
	return false
}

func sameFolder(a, b string) bool {
	return cleanKey(a) == cleanKey(b)
}

func foldersOverlap(a, b string) bool {
	return isUnder(a, b) || isUnder(b, a)
}

func isUnder(path, dir string) bool {
	path = cleanKey(path)
	dir = cleanKey(dir)
	if path == dir {
		return true
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func cleanKey(path string) string {
	cleaned := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned
}
