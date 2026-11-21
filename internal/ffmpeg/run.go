package ffmpeg

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// RunOptions configures progress reporting for one FFmpeg invocation.
type RunOptions struct {
	TotalMs          int64
	ProgressInterval time.Duration
	OnProgress       func(Progress)
}

// RunResult captures the outcome of an FFmpeg process.
type RunResult struct {
	Duration time.Duration
	Stderr   string
	ExitCode int
}

// Run executes FFmpeg, parses `-progress pipe:1` output and returns a compact
// error when the process fails. Cancelling ctx kills the process.
func (t Tool) Run(ctx context.Context, args []string, opts RunOptions) (RunResult, error) {
	started := time.Now()
	result := RunResult{ExitCode: -1}

	cmd := exec.CommandContext(ctx, t.FFmpeg, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, fmt.Errorf("capture ffmpeg output: %w", err)
	}
	stderr := &tailBuffer{max: 64 << 10}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return result, fmt.Errorf("start ffmpeg: %w", err)
	}

	interval := opts.ProgressInterval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	parser := NewParser(opts.TotalMs)
	lastEmit := time.Time{}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		sample, complete := parser.Feed(scanner.Text())
		if !complete || opts.OnProgress == nil {
			continue
		}
		now := time.Now()
		if sample.Done || lastEmit.IsZero() || now.Sub(lastEmit) >= interval {
			opts.OnProgress(sample)
			lastEmit = now
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	result.Duration = time.Since(started)
	result.Stderr = stderr.String()

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	} else if waitErr == nil {
		result.ExitCode = 0
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if waitErr != nil {
		message := SummarizeStderr(result.Stderr, 3)
		if message == "" {
			message = waitErr.Error()
		}
		return result, fmt.Errorf("ffmpeg failed: %s", message)
	}
	if scanErr != nil {
		return result, fmt.Errorf("read ffmpeg progress: %w", scanErr)
	}
	return result, nil
}

// SummarizeStderr keeps the last meaningful lines of FFmpeg's error output.
func SummarizeStderr(stderr string, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	kept := make([]string, 0, maxLines)
	for i := len(lines) - 1; i >= 0 && len(kept) < maxLines; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "frame=") || strings.HasPrefix(line, "size=") {
			continue
		}
		kept = append([]string{line}, kept...)
	}
	message := strings.Join(kept, " | ")
	if len(message) > 1200 {
		message = message[:1200] + "..."
	}
	return message
}

type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-b.max:]...)
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
