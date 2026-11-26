package packager

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFFmpegPackagingRealClip(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "clip.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	gen := exec.CommandContext(ctx, ffmpegPath,
		"-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-t", "2", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-shortest", input)
	if data, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate clip: %v\n%s", err, data)
	}

	for _, format := range []Format{FormatHLS, FormatDASH} {
		t.Run(string(format), func(t *testing.T) {
			out := filepath.Join(dir, string(format))
			plan, err := Build(Request{
				Input: input, OutputDir: out, Format: format,
				SegmentSeconds: 1, Tool: ToolFFmpeg, FFmpeg: ffmpegPath,
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, command := range plan.Commands {
				run := exec.CommandContext(ctx, command.Binary, command.Args...)
				if data, err := run.CombinedOutput(); err != nil {
					t.Fatalf("%s: %v\n%s", command.Binary, err, data)
				}
			}
			if _, err := os.Stat(plan.Manifest); err != nil {
				t.Fatalf("manifest missing: %v", err)
			}
			entries, err := os.ReadDir(out)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) < 2 {
				t.Fatalf("expected segments next to the manifest, got %d entries", len(entries))
			}
		})
	}
}
