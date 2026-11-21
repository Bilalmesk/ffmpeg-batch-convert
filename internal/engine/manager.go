package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
)

var (
	// ErrBusy is returned when another batch is already running.
	ErrBusy = errors.New("another batch is already running")
	// ErrNotFound is returned for an unknown run id.
	ErrNotFound = errors.New("run not found")
	// ErrNotRunning is returned when cancelling a run that is not active.
	ErrNotRunning = errors.New("run is not active")
	// ErrNoFailures is returned when retrying a run that has no failed jobs.
	ErrNoFailures = errors.New("run has no failed jobs")
)

const historyLimit = 30

// Manager owns the single active batch, its history and event subscribers.
type Manager struct {
	mu      sync.Mutex
	startMu sync.Mutex
	tool    ffmpeg.Tool

	active  *Run
	cancel  context.CancelFunc
	history []*Run

	subs    map[int]chan Event
	nextSub int
	runWG   sync.WaitGroup
}

// NewManager creates a manager around a resolved FFmpeg toolchain.
func NewManager(tool ffmpeg.Tool) *Manager {
	return &Manager{
		tool: tool,
		subs: make(map[int]chan Event),
	}
}

// Tool exposes the resolved toolchain.
func (m *Manager) Tool() ffmpeg.Tool { return m.tool }

// Start validates settings, scans the inputs, plans jobs and launches workers.
// The supplied context should outlive the caller (the web server passes its
// own context), because cancelling it stops the batch.
func (m *Manager) Start(ctx context.Context, cfg config.Config, paths []string) (*RunSnapshot, error) {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	m.startMu.Lock()
	defer m.startMu.Unlock()

	m.mu.Lock()
	if m.active != nil {
		m.mu.Unlock()
		return nil, ErrBusy
	}
	m.mu.Unlock()

	files, root, warnings, err := collectInputs(cfg, paths)
	if err != nil {
		return nil, err
	}
	jobs, planWarnings, err := Plan(cfg, files)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, planWarnings...)

	run := &Run{
		ID:        newRunID(),
		Status:    StatusRunning,
		Mode:      cfg.Mode,
		Root:      root,
		Config:    cfg,
		Jobs:      jobs,
		StartedAt: time.Now(),
	}
	run.logFile = openRunLog(cfg.LogFile)

	runCtx, cancel := context.WithCancel(ctx)

	m.mu.Lock()
	if m.active != nil {
		m.mu.Unlock()
		cancel()
		run.closeLog()
		return nil, ErrBusy
	}
	m.active = run
	m.cancel = cancel
	m.history = append([]*Run{run}, m.history...)
	if len(m.history) > historyLimit {
		m.history = m.history[:historyLimit]
	}
	snapshot := run.snapshot()
	m.mu.Unlock()

	m.broadcast(Event{Type: "run.started", At: time.Now(), Run: &snapshot.RunSummary, Jobs: snapshot.Jobs})
	for _, warning := range warnings {
		m.log(run, "warn", warning)
	}
	m.log(run, "info", fmt.Sprintf("%s batch started: %d file(s), %d worker(s), %s → %s",
		titleCase(string(cfg.Mode)), len(jobs), cfg.Workers, upperOr(cfg.Format, "MP4"), verbFor(cfg.Mode)))

	m.runWG.Add(1)
	go func() {
		defer m.runWG.Done()
		m.execute(runCtx, run)
	}()

	return snapshot, nil
}

func (m *Manager) execute(ctx context.Context, run *Run) {
	defer func() {
		m.mu.Lock()
		if ctx.Err() != nil {
			run.Status = StatusCanceled
		} else {
			run.Status = runStatusAfter(run)
		}
		now := time.Now()
		run.FinishedAt = &now
		run.Canceling = false
		m.mu.Unlock()

		summary := m.summaryOf(run)
		if summary.Canceled > 0 {
			m.log(run, "warn", fmt.Sprintf("Run cancelled: %d done, %d skipped, %d failed", summary.Done, summary.Skipped, summary.Failed))
		} else {
			m.log(run, "info", fmt.Sprintf("Run finished: %d done, %d skipped, %d failed in %s",
				summary.Done, summary.Skipped, summary.Failed, humanDuration(time.Since(run.StartedAt))))
		}

		m.mu.Lock()
		if m.active == run {
			m.active = nil
			m.cancel = nil
		}
		final := run.snapshot()
		run.closeLog()
		m.mu.Unlock()

		m.broadcast(Event{Type: "run.finished", At: time.Now(), Run: &final.RunSummary, Jobs: final.Jobs})
	}()

	pending := 0
	for _, job := range run.Jobs {
		if job.Status == StatusPending {
			pending++
		}
	}
	if pending == 0 {
		m.log(run, "info", "Nothing to do: every file is already converted or was skipped")
		return
	}

	workers := run.Config.Workers
	if workers > pending {
		workers = pending
	}
	jobsCh := make(chan *Job)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobsCh {
				if ctx.Err() != nil {
					m.cancelJob(run, job)
					continue
				}
				m.runJob(ctx, run, job)
			}
		}()
	}

	stopped := false
	for _, job := range run.Jobs {
		if job.Status != StatusPending {
			continue
		}
		if stopped {
			m.cancelJob(run, job)
			continue
		}
		select {
		case jobsCh <- job:
		case <-ctx.Done():
			stopped = true
			m.cancelJob(run, job)
		}
	}
	close(jobsCh)
	wg.Wait()
}

// Cancel stops the active run. It is safe to call more than once.
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	if m.active == nil || m.active.ID != id {
		m.mu.Unlock()
		return ErrNotRunning
	}
	if m.active.Canceling {
		m.mu.Unlock()
		return nil
	}
	m.active.Canceling = true
	cancel := m.cancel
	summary := m.active.summary()
	run := m.active
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	m.log(run, "warn", "Cancellation requested; stopping FFmpeg…")
	m.broadcast(Event{Type: "run.canceling", At: time.Now(), Run: &summary})
	return nil
}

// Retry starts a new run containing only the failed jobs of an existing run.
func (m *Manager) Retry(ctx context.Context, id string) (*RunSnapshot, error) {
	m.mu.Lock()
	var run *Run
	for _, candidate := range m.history {
		if candidate.ID == id {
			run = candidate
			break
		}
	}
	if run == nil {
		m.mu.Unlock()
		return nil, ErrNotFound
	}
	cfg := run.Config
	files := failedInputs(run)
	m.mu.Unlock()

	if len(files) == 0 {
		return nil, ErrNoFailures
	}
	return m.Start(ctx, cfg, files)
}

func failedInputs(run *Run) []string {
	files := make([]string, 0)
	seen := make(map[string]bool)
	for _, job := range run.Jobs {
		if job.Status != StatusFailed || job.Input == "" {
			continue
		}
		key := pathKey(job.Input)
		if seen[key] {
			continue
		}
		seen[key] = true
		files = append(files, job.Input)
	}
	return files
}

// List returns summaries newest-first.
func (m *Manager) List() []RunSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RunSummary, 0, len(m.history))
	for _, run := range m.history {
		out = append(out, run.summary())
	}
	return out
}

// Active returns the running batch summary, if any.
func (m *Manager) Active() *RunSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return nil
	}
	summary := m.active.summary()
	return &summary
}

// Snapshot returns the complete state of one run.
func (m *Manager) Snapshot(id string) (*RunSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, run := range m.history {
		if run.ID == id {
			return run.snapshot(), true
		}
	}
	return nil, false
}

// Latest returns the newest run snapshot, if any.
func (m *Manager) Latest() (*RunSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.history) == 0 {
		return nil, false
	}
	return m.history[0].snapshot(), true
}

// Subscribe registers a listener and atomically returns the current state so a
// client cannot miss events between the snapshot and the stream.
func (m *Manager) Subscribe() (int, <-chan Event, *RunSnapshot) {
	ch := make(chan Event, 512)
	m.mu.Lock()
	id := m.nextSub
	m.nextSub++
	m.subs[id] = ch
	var snapshot *RunSnapshot
	if len(m.history) > 0 {
		snapshot = m.history[0].snapshot()
	}
	m.mu.Unlock()
	return id, ch, snapshot
}

// Unsubscribe removes a listener. The channel is intentionally not closed, so a
// broadcast racing with removal cannot panic.
func (m *Manager) Unsubscribe(id int) {
	m.mu.Lock()
	delete(m.subs, id)
	m.mu.Unlock()
}

// Shutdown cancels the active run and waits for workers, bounded by ctx.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	cancel := m.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		m.runWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) broadcast(event Event) {
	m.mu.Lock()
	listeners := make([]chan Event, 0, len(m.subs))
	for _, ch := range m.subs {
		listeners = append(listeners, ch)
	}
	m.mu.Unlock()
	for _, ch := range listeners {
		select {
		case ch <- event:
		default:
			// A slow client must never stall conversions.
		}
	}
}

func (m *Manager) log(run *Run, level, message string) {
	entry := LogEntry{At: time.Now(), Level: level, Message: message}

	m.mu.Lock()
	run.Log = append(run.Log, entry)
	if len(run.Log) > 1000 {
		run.Log = append([]LogEntry(nil), run.Log[len(run.Log)-1000:]...)
	}
	if run.logFile != nil {
		fmt.Fprintf(run.logFile, "%s [%s] %s\n", entry.At.Format("2006-01-02 15:04:05"), upperOr(level, "INFO"), message)
	}
	summary := run.summary()
	m.mu.Unlock()

	m.broadcast(Event{Type: "log", At: entry.At, Run: &summary, Message: message, Level: level})
}

func (m *Manager) summaryOf(run *Run) RunSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	return run.summary()
}

func (m *Manager) setJob(run *Run, job *Job, eventType string, mutate func()) {
	m.mu.Lock()
	mutate()
	summary := run.summary()
	copy := job.Clone()
	m.mu.Unlock()

	m.broadcast(Event{Type: eventType, At: time.Now(), Run: &summary, Job: copy})
}

func (m *Manager) cancelJob(run *Run, job *Job) {
	if job.Status != StatusPending && job.Status != StatusRunning {
		return
	}
	m.setJob(run, job, "job.canceled", func() {
		now := time.Now()
		job.Status = StatusCanceled
		job.Stage = ""
		job.Error = "cancelled"
		job.ETASeconds = 0
		job.Speed = 0
		job.FinishedAt = &now
	})
}

func newRunID() string {
	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return "run-" + time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(suffix)
}

func openRunLog(path string) *os.File {
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil
	}
	return file
}

func (r *Run) closeLog() {
	if r.logFile != nil {
		_ = r.logFile.Close()
		r.logFile = nil
	}
}
