package quality

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
)

func TestMeasureIdenticalClip(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	tool, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("ffmpeg not available:", err)
	}
	dir := t.TempDir()
	clip := filepath.Join(dir, "clip.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool.FFmpeg,
		"-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=10",
		"-t", "1", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", clip)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate clip: %v\n%s", err, output)
	}

	tests := []struct {
		metric Metric
		min    float64
	}{
		{MetricVMAF, 99},
		{MetricSSIM, 0.999},
		{MetricPSNR, 50},
	}
	for _, test := range tests {
		t.Run(string(test.metric), func(t *testing.T) {
			measureCtx, measureCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer measureCancel()
			result, err := Measure(measureCtx, Request{
				Reference: clip, Candidate: clip, Metric: test.metric,
				RefWidth: 160, RefHeight: 120, DurationMs: 1000, FFmpeg: tool.FFmpeg,
			})
			if errors.Is(err, ErrUnsupported) {
				if test.metric == MetricVMAF {
					t.Skip("this FFmpeg build has no libvmaf")
				}
				t.Fatal(err)
			}
			if err != nil {
				if test.metric == MetricVMAF {
					t.Skipf("VMAF unavailable in this environment: %v", err)
				}
				t.Fatal(err)
			}
			if result.Infinite {
				return
			}
			if result.Score < test.min {
				t.Fatalf("%s score = %.4f, want >= %.4f", test.metric, result.Score, test.min)
			}
		})
	}
}
