package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
)

func (m *Manager) runJob(ctx context.Context, run *Run, job *Job) {
	cfg := run.Config
	if job.Status != StatusPending {
		return
	}
	if ctx.Err() != nil {
		m.cancelJob(run, job)
		return
	}

	started := time.Now()
	m.setJob(run, job, "job.started", func() {
		job.Status = StatusRunning
		job.Stage = "starting"
		job.StartedAt = &started
		job.Progress = 0
	})

	if cfg.Mode == config.ModeRename {
		m.runRename(ctx, run, job)
		return
	}
	m.runConvert(ctx, run, job)
}

func (m *Manager) runConvert(ctx context.Context, run *Run, job *Job) {
	cfg := run.Config

	m.setJob(run, job, "job.updated", func() { job.Stage = "probing" })
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	info, err := mediainfo.Probe(probeCtx, m.tool.FFprobe, job.Input)
	cancel()
	if err != nil {
		m.failJob(run, job, fmt.Errorf("probe failed: %w", err))
		return
	}

	m.setJob(run, job, "job.updated", func() {
		job.DurationMs = info.DurationMs
		if info.Video != nil {
			job.Resolution = fmt.Sprintf("%dx%d", info.Video.Width, info.Video.Height)
			job.VideoCodec = info.Video.Codec
		}
		if len(info.Audios) > 0 {
			job.AudioCodec = info.Audios[0].Codec
		}
		job.Stage = "converting"
	})

	if cfg.DryRun {
		build := ffmpeg.Build(ffmpeg.BuildRequest{Cfg: cfg, Input: job.Input, Output: job.Output, Info: info})
		now := time.Now()
		m.setJob(run, job, "job.planned", func() {
			job.Status = StatusPlanned
			job.Stage = ""
			job.Progress = 100
			job.Command = build.Command
			job.Warnings = append(job.Warnings, build.Warnings...)
			job.FinishedAt = &now
		})
		m.log(run, "info", "Dry run: "+job.Name)
		return
	}

	if err := os.MkdirAll(filepath.Dir(job.Output), 0o755); err != nil {
		m.failJob(run, job, fmt.Errorf("create output folder: %w", err))
		return
	}

	ext := filepath.Ext(job.Output)
	base := strings.TrimSuffix(filepath.Base(job.Output), ext)
	temp := filepath.Join(filepath.Dir(job.Output), fmt.Sprintf(".%s.ffbatch-%s.part%s", base, job.ID, ext))
	_ = os.Remove(temp)

	build := ffmpeg.Build(ffmpeg.BuildRequest{Cfg: cfg, Input: job.Input, Output: temp, Info: info})
	displayCmd := strings.Replace(build.Command, temp, job.Output, 1)
	m.setJob(run, job, "job.updated", func() {
		job.Command = displayCmd
		job.Warnings = append(job.Warnings, build.Warnings...)
	})
	for _, warning := range build.Warnings {
		m.log(run, "warn", job.Name+": "+warning)
	}

	result, err := m.tool.Run(ctx, build.Args, ffmpeg.RunOptions{
		TotalMs: info.DurationMs,
		OnProgress: func(progress ffmpeg.Progress) {
			m.updateProgress(run, job, progress)
		},
	})
	if err != nil {
		_ = os.Remove(temp)
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			m.markCanceled(run, job)
			return
		}
		m.failJob(run, job, err)
		return
	}

	m.setJob(run, job, "job.updated", func() { job.Stage = "finalizing" })
	dest := job.Output
	appeared := fileExists(dest)
	switch {
	case appeared && cfg.Overwrite == config.OverwriteSkip:
		_ = os.Remove(temp)
		m.skipJob(run, job, "output appeared during the run")
		return
	case appeared && cfg.Overwrite == config.OverwriteFail:
		_ = os.Remove(temp)
		m.failJob(run, job, errors.New("output appeared during the run"))
		return
	case appeared && cfg.Overwrite == config.OverwriteNumbering:
		dest = uniquePath(dest, map[string]bool{}, true)
	}
	if err := replaceFile(temp, dest); err != nil {
		_ = os.Remove(temp)
		m.failJob(run, job, fmt.Errorf("finalise output: %w", err))
		return
	}

	size := statSize(dest)
	now := time.Now()
	m.setJob(run, job, "job.done", func() {
		job.Output = dest
		job.OutputBytes = size
		job.Status = StatusDone
		job.Stage = ""
		job.Progress = 100
		job.ETASeconds = 0
		job.FinishedAt = &now
	})

	if cfg.DeleteSource && !samePath(job.Input, dest) {
		if err := os.Remove(job.Input); err != nil {
			m.log(run, "warn", fmt.Sprintf("%s: converted, but the source could not be deleted: %v", job.Name, err))
			m.setJob(run, job, "job.updated", func() {
				job.Warnings = append(job.Warnings, "source could not be deleted: "+err.Error())
			})
		} else {
			m.setJob(run, job, "job.updated", func() { job.SourceRemoved = true })
		}
	}

	speed := ""
	if job.Speed > 0.01 {
		speed = fmt.Sprintf(" · %.1f×", job.Speed)
	}
	m.log(run, "info", fmt.Sprintf("✓ %s · %s · %s%s", job.Name, humanBytes(size), humanDuration(result.Duration), speed))
}

func (m *Manager) runRename(ctx context.Context, run *Run, job *Job) {
	cfg := run.Config
	if ctx.Err() != nil {
		m.markCanceled(run, job)
		return
	}

	dest := job.Output
	if fileExists(dest) {
		switch cfg.Overwrite {
		case config.OverwriteSkip:
			m.skipJob(run, job, "output already exists")
			return
		case config.OverwriteFail:
			m.failJob(run, job, errors.New("output already exists"))
			return
		case config.OverwriteNumbering:
			dest = uniquePath(dest, map[string]bool{}, true)
		}
	}

	command := fmt.Sprintf("move %q -> %q", job.Input, dest)
	if cfg.DryRun {
		now := time.Now()
		m.setJob(run, job, "job.planned", func() {
			job.Status = StatusPlanned
			job.Stage = ""
			job.Progress = 100
			job.Command = command
			job.FinishedAt = &now
		})
		m.log(run, "info", "Dry run: "+job.Name)
		return
	}

	m.setJob(run, job, "job.updated", func() {
		job.Stage = "renaming"
		job.Command = command
	})
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		m.failJob(run, job, fmt.Errorf("create output folder: %w", err))
		return
	}
	if err := moveFile(job.Input, dest, cfg.Overwrite == config.OverwriteReplace); err != nil {
		m.failJob(run, job, fmt.Errorf("rename: %w", err))
		return
	}

	size := statSize(dest)
	now := time.Now()
	m.setJob(run, job, "job.done", func() {
		job.Output = dest
		job.OutputBytes = size
		job.Status = StatusDone
		job.Stage = ""
		job.Progress = 100
		job.SourceRemoved = true
		job.FinishedAt = &now
	})
	m.log(run, "info", fmt.Sprintf("✓ %s · renamed to %s", job.Name, filepath.Base(dest)))
}

func (m *Manager) updateProgress(run *Run, job *Job, progress ffmpeg.Progress) {
	m.mu.Lock()
	job.Progress = clampPercent(progress.Percent)
	job.Speed = progress.Speed
	job.FPS = progress.FPS
	job.Bitrate = progress.Bitrate
	job.ETASeconds = progress.ETASeconds
	job.OutTimeMs = progress.OutTimeMs
	summary := run.summary()
	copy := job.Clone()
	m.mu.Unlock()
	m.broadcast(Event{Type: "job.progress", At: time.Now(), Run: &summary, Job: copy})
}

func (m *Manager) failJob(run *Run, job *Job, err error) {
	now := time.Now()
	m.setJob(run, job, "job.failed", func() {
		job.Status = StatusFailed
		job.Stage = ""
		job.Error = err.Error()
		job.FinishedAt = &now
	})
	m.log(run, "error", fmt.Sprintf("✗ %s: %s", job.Name, err.Error()))
}

func (m *Manager) skipJob(run *Run, job *Job, reason string) {
	now := time.Now()
	m.setJob(run, job, "job.skipped", func() {
		job.Status = StatusSkipped
		job.Stage = ""
		job.Reason = reason
		job.FinishedAt = &now
	})
	m.log(run, "warn", fmt.Sprintf("Skipped %s: %s", job.Name, reason))
}

func (m *Manager) markCanceled(run *Run, job *Job) {
	now := time.Now()
	m.setJob(run, job, "job.canceled", func() {
		job.Status = StatusCanceled
		job.Stage = ""
		job.Error = "cancelled"
		job.ETASeconds = 0
		job.Speed = 0
		job.FinishedAt = &now
	})
	m.log(run, "warn", fmt.Sprintf("Cancelled %s", job.Name))
}

func replaceFile(src, dst string) error {
	firstErr := os.Rename(src, dst)
	if firstErr == nil {
		return nil
	}
	// Windows cannot rename over an existing file.
	if fileExists(dst) {
		if err := os.Remove(dst); err != nil {
			return err
		}
		if err := os.Rename(src, dst); err == nil {
			return nil
		}
	}
	return fmt.Errorf("could not move %s to %s: %w", filepath.Base(src), filepath.Base(dst), firstErr)
}

// moveFile renames across folders, falling back to copy+delete across volumes.
func moveFile(src, dst string, overwrite bool) error {
	if !overwrite && fileExists(dst) {
		return fmt.Errorf("%s already exists", filepath.Base(dst))
	}
	if overwrite {
		_ = os.Remove(dst)
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	temp := dst + ".ffbatch-copy.part"
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		_ = in.Close()
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = in.Close()
		_ = os.Remove(temp)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = in.Close()
		_ = os.Remove(temp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = in.Close()
		_ = os.Remove(temp)
		return err
	}
	if err := in.Close(); err != nil {
		_ = os.Remove(temp)
		return err
	}
	if err := replaceFile(temp, dst); err != nil {
		_ = os.Remove(temp)
		return err
	}
	_ = os.Chtimes(dst, info.ModTime(), info.ModTime())
	if err := os.Remove(src); err != nil {
		return err
	}
	return nil
}

func statSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
