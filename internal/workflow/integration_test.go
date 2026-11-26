package workflow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
)

func generateClip(t *testing.T, tool ffmpeg.Tool, dir, ext string) string {
	t.Helper()
	path := filepath.Join(dir, "clip."+ext)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool.FFmpeg,
		"-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-t", "1", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-shortest", path)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate clip: %v\n%s", err, data)
	}
	return path
}

func testOptions(t *testing.T, input string) Options {
	t.Helper()
	tool, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("ffmpeg not available:", err)
	}
	return Options{Input: input, OutputDir: filepath.Join(t.TempDir(), "out"), FFmpeg: tool, Log: func(string, string) {}}
}

func TestBuiltinTSRepair(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	procTool, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("ffmpeg not available:", err)
	}
	input := generateClip(t, procTool, dir, "ts")
	opts := testOptions(t, input)
	wf, _ := Builtin("ts-repair")
	report, err := Run(context.Background(), wf, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "done" {
		t.Fatalf("status = %s", report.Status)
	}
	output := filepath.Join(opts.OutputDir, "clip.mp4")
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("output missing: %v", err)
	}
	info, err := mediainfo.Probe(context.Background(), opts.FFmpeg.FFprobe, output)
	if err != nil || info.Video == nil {
		t.Fatalf("probe output: %v", err)
	}
}

func TestBuiltinAudioAndHLS(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	procTool, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("ffmpeg not available:", err)
	}
	input := generateClip(t, procTool, dir, "mp4")

	t.Run("audio", func(t *testing.T) {
		opts := testOptions(t, input)
		wf := Workflow{Name: "test-audio", Steps: []Step{
			{ID: "probe", Type: "probe"},
			{ID: "audio", Type: "audio", Params: map[string]any{"format": "flac", "normalize": true, "output": "{{outputDir}}/{{base}}.flac"}},
		}}
		if _, err := Run(context.Background(), wf, opts); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(opts.OutputDir, "clip.flac")); err != nil {
			t.Fatalf("audio output missing: %v", err)
		}
	})

	t.Run("hls", func(t *testing.T) {
		opts := testOptions(t, input)
		wf := Workflow{Name: "test-hls", Steps: []Step{
			{ID: "probe", Type: "probe"},
			{ID: "encode", Type: "convert", Params: map[string]any{
				"mode": "encode", "videoCodec": "h264", "crf": 28, "preset": "veryfast",
				"audioCodec": "aac", "output": "{{work}}/mezz.mp4",
			}},
			{ID: "package", Type: "package", Params: map[string]any{
				"input": "{{steps.encode.output}}", "out": "{{outputDir}}/hls", "format": "hls", "segment": 1,
			}},
		}}
		report, err := Run(context.Background(), wf, opts)
		if err != nil {
			t.Fatal(err)
		}
		if report.Status != "done" {
			t.Fatalf("status = %s", report.Status)
		}
		if _, err := os.Stat(filepath.Join(opts.OutputDir, "hls", "index.m3u8")); err != nil {
			t.Fatalf("manifest missing: %v", err)
		}
	})
}

func TestBuiltinMKVIfAvailable(t *testing.T) {
	if _, err := exec.LookPath("mkvmerge"); err != nil {
		t.Skip("mkvmerge not available")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	procTool, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("ffmpeg not available:", err)
	}
	input := generateClip(t, procTool, dir, "mp4")
	opts := testOptions(t, input)
	wf, _ := Builtin("mkv-preserve")
	if _, err := Run(context.Background(), wf, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(opts.OutputDir, "clip.mkv")); err != nil {
		t.Fatalf("mkv output missing: %v", err)
	}
}

func TestAV1ConvertQualityAndCopy(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	procTool, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("ffmpeg not available:", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	encoders, err := procTool.Encoders(ctx)
	cancel()
	if err != nil || !encoders["libsvtav1"] {
		t.Skip("libsvtav1 not available")
	}
	dir := t.TempDir()
	input := generateClip(t, procTool, dir, "mp4")
	opts := testOptions(t, input)
	wf := Workflow{Name: "test-av1", Steps: []Step{
		{ID: "probe", Type: "probe"},
		{ID: "encode", Type: "convert", Params: map[string]any{
			"mode": "encode", "format": "mp4", "videoCodec": "av1", "av1Encoder": "svt",
			"crf": 40, "preset": "veryfast", "audioCodec": "aac", "output": "{{work}}/av1.mp4",
		}},
		{ID: "quality", Type: "quality", Params: map[string]any{
			"candidate": "{{steps.encode.output}}", "metric": "ssim", "min": 0.80,
		}},
		{ID: "copy", Type: "copy", Params: map[string]any{
			"from": "{{steps.encode.output}}", "to": "{{outputDir}}/final.mp4",
		}},
	}}
	report, err := Run(context.Background(), wf, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "done" {
		t.Fatalf("status = %s", report.Status)
	}
	if _, err := os.Stat(filepath.Join(opts.OutputDir, "final.mp4")); err != nil {
		t.Fatalf("final output missing: %v", err)
	}
}
