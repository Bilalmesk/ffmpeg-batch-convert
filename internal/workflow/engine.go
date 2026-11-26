package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
)

// Options configures one workflow run.
type Options struct {
	Input     string
	OutputDir string
	WorkDir   string
	Overwrite bool
	KeepWork  bool
	Vars      map[string]any
	ToolPaths map[string]string
	FFmpeg    ffmpeg.Tool
	Log       func(level, message string)
}

// StepReport records one step's outcome.
type StepReport struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Status     string    `json:"status"` // done | skipped | failed
	Output     string    `json:"output,omitempty"`
	Path       string    `json:"path,omitempty"`
	Manifest   string    `json:"manifest,omitempty"`
	Score      float64   `json:"score,omitempty"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
}

// Report is the machine-readable result of a workflow run.
type Report struct {
	Workflow   string       `json:"workflow"`
	Input      string       `json:"input"`
	OutputDir  string       `json:"outputDir"`
	WorkDir    string       `json:"workDir,omitempty"`
	Status     string       `json:"status"` // done | partial | failed
	Steps      []StepReport `json:"steps"`
	StartedAt  time.Time    `json:"startedAt"`
	FinishedAt time.Time    `json:"finishedAt"`
}

// Run executes a workflow sequentially.
func Run(ctx context.Context, wf Workflow, opts Options) (*Report, error) {
	if err := wf.Validate(); err != nil {
		return nil, err
	}
	input, err := filepath.Abs(opts.Input)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(input)
	if err != nil {
		return nil, fmt.Errorf("input: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("input must be a file")
	}
	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = filepath.Dir(input)
	}
	outputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output folder: %w", err)
	}
	workDir := opts.WorkDir
	tempWork := false
	if workDir == "" {
		workDir, err = os.MkdirTemp("", "ffbatch-workflow-*")
		if err != nil {
			return nil, err
		}
		tempWork = true
	} else {
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			return nil, err
		}
	}
	if tempWork && !opts.KeepWork {
		defer os.RemoveAll(workDir)
	}
	logf := opts.Log
	if logf == nil {
		logf = func(string, string) {}
	}

	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	state := &runState{
		Input:      input,
		OutputDir:  outputDir,
		WorkDir:    workDir,
		Base:       base,
		Ext:        strings.TrimPrefix(strings.ToLower(filepath.Ext(input)), "."),
		Overwrite:  opts.Overwrite,
		ToolPaths:  opts.ToolPaths,
		Steps:      make(map[string]*stepResult, len(wf.Steps)),
		LastOutput: input,
		Vars:       opts.Vars,
	}
	report := &Report{
		Workflow:  wf.Name,
		Input:     input,
		OutputDir: outputDir,
		WorkDir:   workDir,
		Status:    "done",
		StartedAt: time.Now(),
	}
	failed := false
	for _, step := range wf.Steps {
		stepReport := StepReport{ID: step.ID, Type: step.Type, StartedAt: time.Now()}
		if step.When != nil {
			ok, err := step.When.Eval(buildContext(state))
			if err != nil {
				stepReport.Status = "failed"
				stepReport.Error = err.Error()
				stepReport.FinishedAt = time.Now()
				report.Steps = append(report.Steps, stepReport)
				report.Status = "failed"
				report.FinishedAt = time.Now()
				return report, err
			}
			if !ok {
				stepReport.Status = "skipped"
				stepReport.FinishedAt = time.Now()
				report.Steps = append(report.Steps, stepReport)
				logf("info", fmt.Sprintf("skip %s (%s): condition is false", step.ID, step.Type))
				continue
			}
		}
		logf("step", fmt.Sprintf("%s (%s)", step.ID, step.Type))
		params, err := resolveParams(step.Params, buildContext(state))
		if err != nil {
			stepReport.Status = "failed"
			stepReport.Error = err.Error()
			stepReport.FinishedAt = time.Now()
			report.Steps = append(report.Steps, stepReport)
			report.Status = "failed"
			report.FinishedAt = time.Now()
			return report, err
		}
		var result *stepResult
		attempts := step.Retries + 1
		for attempt := 1; attempt <= attempts; attempt++ {
			result, err = runStep(ctx, step, params, state, opts.FFmpeg)
			if err == nil {
				break
			}
			if attempt < attempts {
				logf("warn", fmt.Sprintf("%s failed (attempt %d/%d): %v", step.ID, attempt, attempts, err))
			}
		}
		stepReport.FinishedAt = time.Now()
		if err != nil {
			stepReport.Status = "failed"
			stepReport.Error = err.Error()
			report.Steps = append(report.Steps, stepReport)
			if step.OnError == "continue" {
				failed = true
				logf("warn", fmt.Sprintf("%s failed, continuing: %v", step.ID, err))
				continue
			}
			report.Status = "failed"
			report.FinishedAt = time.Now()
			return report, fmt.Errorf("%s: %w", step.ID, err)
		}
		if result == nil {
			result = &stepResult{}
		}
		state.Steps[step.ID] = result
		if result.Output != "" {
			state.LastOutput = result.Output
		}
		stepReport.Status = "done"
		stepReport.Output = result.Output
		stepReport.Path = result.Path
		stepReport.Manifest = result.Manifest
		stepReport.Score = result.Score
		report.Steps = append(report.Steps, stepReport)
		if result.Output != "" {
			logf("done", result.Output)
		} else if result.Manifest != "" {
			logf("done", result.Manifest)
		} else if result.Path != "" && result.Path != state.Input {
			logf("done", result.Path)
		}
	}
	report.FinishedAt = time.Now()
	if failed {
		report.Status = "partial"
	}
	return report, nil
}
