// Package engine plans batches, runs FFmpeg jobs concurrently and publishes
// live events for the CLI and the web dashboard.
package engine

import (
	"math"
	"os"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
)

// Status is the lifecycle state of a job or run.
type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusDone     Status = "done"
	StatusSkipped  Status = "skipped"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
	StatusPlanned  Status = "planned" // dry run only
	StatusPartial  Status = "partial" // run with mixed results
)

// Terminal reports whether a job in this state will not change again without
// user action.
func (s Status) Terminal() bool {
	switch s {
	case StatusDone, StatusSkipped, StatusFailed, StatusCanceled, StatusPlanned:
		return true
	default:
		return false
	}
}

// Job is one input file moving through the pipeline.
type Job struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Input     string      `json:"input"`
	Output    string      `json:"output"`
	Ext       string      `json:"ext"`
	Mode      config.Mode `json:"mode"`
	SizeBytes int64       `json:"sizeBytes"`

	Status     Status  `json:"status"`
	Stage      string  `json:"stage,omitempty"`
	Progress   float64 `json:"progress"`
	Speed      float64 `json:"speed,omitempty"`
	FPS        float64 `json:"fps,omitempty"`
	Bitrate    string  `json:"bitrate,omitempty"`
	ETASeconds int64   `json:"etaSeconds,omitempty"`

	OutTimeMs  int64 `json:"outTimeMs,omitempty"`
	DurationMs int64 `json:"durationMs,omitempty"`

	Resolution string `json:"resolution,omitempty"`
	VideoCodec string `json:"videoCodec,omitempty"`
	AudioCodec string `json:"audioCodec,omitempty"`

	Command  string   `json:"command,omitempty"`
	Warnings []string `json:"warnings,omitempty"`

	Error         string `json:"error,omitempty"`
	Reason        string `json:"reason,omitempty"`
	OutputBytes   int64  `json:"outputBytes,omitempty"`
	SourceRemoved bool   `json:"sourceRemoved,omitempty"`

	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// Clone returns a deep copy safe to serialise outside the manager lock.
func (j *Job) Clone() *Job {
	if j == nil {
		return nil
	}
	copy := *j
	if j.Warnings != nil {
		copy.Warnings = append([]string(nil), j.Warnings...)
	}
	if j.StartedAt != nil {
		t := *j.StartedAt
		copy.StartedAt = &t
	}
	if j.FinishedAt != nil {
		t := *j.FinishedAt
		copy.FinishedAt = &t
	}
	return &copy
}

// LogEntry is one human-readable activity line.
type LogEntry struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// Run groups all jobs created by one start request.
type Run struct {
	ID         string
	Status     Status
	Mode       config.Mode
	Root       string
	Config     config.Config
	Jobs       []*Job
	Log        []LogEntry
	StartedAt  time.Time
	FinishedAt *time.Time
	Error      string
	Canceling  bool

	logFile *os.File // owned by the execution goroutine
}

// RunSummary is the small, frequently-broadcast view of a run.
type RunSummary struct {
	ID        string      `json:"id"`
	Status    Status      `json:"status"`
	Mode      config.Mode `json:"mode"`
	Root      string      `json:"root"`
	Format    string      `json:"format"`
	OutputDir string      `json:"outputDir"`
	Workers   int         `json:"workers"`
	DryRun    bool        `json:"dryRun"`

	Total    int `json:"total"`
	Pending  int `json:"pending"`
	Running  int `json:"running"`
	Done     int `json:"done"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
	Canceled int `json:"canceled"`
	Planned  int `json:"planned"`

	Percent  float64 `json:"percent"`
	BytesIn  int64   `json:"bytesIn"`
	BytesOut int64   `json:"bytesOut"`

	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Canceling  bool       `json:"canceling,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// RunSnapshot is the full state sent to fresh web clients.
type RunSnapshot struct {
	RunSummary
	Config config.Config `json:"config"`
	Jobs   []*Job        `json:"jobs"`
	Log    []LogEntry    `json:"log,omitempty"`
}

// Event is one SSE/CLI update.
type Event struct {
	Type    string      `json:"type"`
	At      time.Time   `json:"at"`
	Run     *RunSummary `json:"run,omitempty"`
	Job     *Job        `json:"job,omitempty"`
	Jobs    []*Job      `json:"jobs,omitempty"`
	Message string      `json:"message,omitempty"`
	Level   string      `json:"level,omitempty"`
}

func summarize(run *Run) RunSummary {
	summary := RunSummary{
		ID:         run.ID,
		Status:     run.Status,
		Mode:       run.Mode,
		Root:       run.Root,
		Format:     run.Config.Format,
		OutputDir:  run.Config.OutputDir,
		Workers:    run.Config.Workers,
		DryRun:     run.Config.DryRun,
		StartedAt:  run.StartedAt,
		FinishedAt: run.FinishedAt,
		Canceling:  run.Canceling,
		Error:      run.Error,
		Total:      len(run.Jobs),
	}
	weighted := 0.0
	for _, job := range run.Jobs {
		summary.BytesIn += job.SizeBytes
		switch job.Status {
		case StatusPending:
			summary.Pending++
		case StatusRunning:
			summary.Running++
			weighted += clampPercent(job.Progress) / 100
		case StatusDone:
			summary.Done++
			summary.BytesOut += job.OutputBytes
			weighted++
		case StatusSkipped:
			summary.Skipped++
			weighted++
		case StatusFailed:
			summary.Failed++
			weighted++
		case StatusCanceled:
			summary.Canceled++
			weighted += clampPercent(job.Progress) / 100
		case StatusPlanned:
			summary.Planned++
			weighted++
		}
	}
	if summary.Total > 0 {
		summary.Percent = math.Round(weighted/float64(summary.Total)*1000) / 10
	}
	return summary
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func (r *Run) snapshot() *RunSnapshot {
	summary := summarize(r)
	snap := &RunSnapshot{
		RunSummary: summary,
		Config:     r.Config,
		Jobs:       make([]*Job, 0, len(r.Jobs)),
		Log:        append([]LogEntry(nil), r.Log...),
	}
	for _, job := range r.Jobs {
		snap.Jobs = append(snap.Jobs, job.Clone())
	}
	return snap
}

func (r *Run) summary() RunSummary {
	return summarize(r)
}

func runStatusAfter(run *Run) Status {
	s := summarize(run)
	if s.Canceled > 0 {
		return StatusCanceled
	}
	if s.Total == 0 {
		return StatusDone
	}
	if s.Failed == s.Total {
		return StatusFailed
	}
	if s.Failed > 0 {
		return StatusPartial
	}
	return StatusDone
}
