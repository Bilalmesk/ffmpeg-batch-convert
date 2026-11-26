package packager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeInput(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.mp4")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuildFFmpegHLS(t *testing.T) {
	input := writeInput(t)
	out := filepath.Join(t.TempDir(), "hls")
	plan, err := Build(Request{Input: input, OutputDir: out, Format: FormatHLS, SegmentSeconds: 6, Tool: ToolFFmpeg, FFmpeg: "ffmpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 1 || plan.Manifest != filepath.Join(out, "index.m3u8") {
		t.Fatalf("plan = %+v", plan)
	}
	joined := strings.Join(plan.Commands[0].Args, " ")
	for _, want := range []string{"-f hls", "-hls_time 6", "segment_%05d.ts", "index.m3u8"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}

func TestBuildFFmpegDASH(t *testing.T) {
	input := writeInput(t)
	out := filepath.Join(t.TempDir(), "dash")
	plan, err := Build(Request{Input: input, OutputDir: out, Format: FormatDASH, SegmentSeconds: 4, Tool: ToolFFmpeg, FFmpeg: "ffmpeg"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Commands[0].Args, " ")
	for _, want := range []string{"-f dash", "-seg_duration 4", "manifest.mpd", "init-stream$RepresentationID$.m4s", "chunk-stream$RepresentationID$"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}

func TestBuildMP4BoxAndBento4(t *testing.T) {
	input := writeInput(t)
	out := filepath.Join(t.TempDir(), "mp4box")
	plan, err := Build(Request{Input: input, OutputDir: out, Format: FormatDASH, SegmentSeconds: 5, Tool: ToolMP4Box, MP4Box: "MP4Box"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 1 || !strings.Contains(strings.Join(plan.Commands[0].Args, " "), "-dash 5000") {
		t.Fatalf("mp4box plan = %+v", plan)
	}

	out = filepath.Join(t.TempDir(), "bento")
	plan, err = Build(Request{Input: input, OutputDir: out, Format: FormatHLS, SegmentSeconds: 5, Tool: ToolBento4, MP4Fragment: "mp4fragment", MP4Dash: "mp4dash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 2 || plan.Manifest != filepath.Join(out, "master.m3u8") {
		t.Fatalf("bento plan = %+v", plan)
	}
	if !strings.Contains(strings.Join(plan.Commands[1].Args, " "), "--hls") {
		t.Fatalf("bento args = %v", plan.Commands[1].Args)
	}
}

func TestBuildRejectsBadSegment(t *testing.T) {
	input := writeInput(t)
	if _, err := Build(Request{Input: input, OutputDir: t.TempDir(), SegmentSeconds: 0, Tool: ToolFFmpeg}); err != nil {
		t.Fatalf("zero should default, got %v", err)
	}
	if _, err := Build(Request{Input: input, OutputDir: t.TempDir(), SegmentSeconds: 90, Tool: ToolFFmpeg}); err == nil {
		t.Fatal("expected an error for a long segment")
	}
}
