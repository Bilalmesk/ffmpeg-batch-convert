package muxer

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
)

func TestMKVMergeRealClip(t *testing.T) {
	mkvmerge, err := exec.LookPath("mkvmerge")
	if err != nil {
		t.Skip("mkvmerge not available")
	}
	tool, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "clip.mp4")
	output := filepath.Join(dir, "clip.mkv")
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

	args, command, err := Build(mkvmerge, Request{Input: input, Output: output, Title: "Test"})
	if err != nil {
		t.Fatal(err)
	}
	if command == "" {
		t.Fatal("empty command")
	}
	run := exec.CommandContext(ctx, mkvmerge, args...)
	if data, err := run.CombinedOutput(); err != nil {
		t.Fatalf("mkvmerge: %v\n%s", err, data)
	}
	info, err := mediainfo.Probe(ctx, tool.FFprobe, output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video == nil || len(info.Audios) != 1 {
		t.Fatalf("unexpected muxed media: %+v", info)
	}
}
