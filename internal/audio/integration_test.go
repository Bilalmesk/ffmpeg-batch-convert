package audio

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
)

func TestExtractAudioRealClip(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	tool, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("ffmpeg not available:", err)
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "clip.mp4")
	output := filepath.Join(dir, "clip.flac")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	gen := exec.CommandContext(ctx, tool.FFmpeg,
		"-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-t", "1", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-shortest", input)
	if data, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate clip: %v\n%s", err, data)
	}

	args, err := Build(tool.FFmpeg, input, output, Options{Format: FormatFLAC})
	if err != nil {
		t.Fatal(err)
	}
	run := exec.CommandContext(ctx, tool.FFmpeg, args...)
	if data, err := run.CombinedOutput(); err != nil {
		t.Fatalf("extract audio: %v\n%s", err, data)
	}
	info, err := mediainfo.ProbeAny(ctx, tool.FFprobe, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Audios) != 1 || info.Video != nil {
		t.Fatalf("unexpected audio output: %+v", info)
	}
}
