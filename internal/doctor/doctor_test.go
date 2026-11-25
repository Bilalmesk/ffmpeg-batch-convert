package doctor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
)

func TestRunWithoutFFmpegReportsToolError(t *testing.T) {
	cfg := config.Default()
	report := Run(context.Background(), ffmpeg.Tool{}, cfg, filepath.Join(t.TempDir(), "ffbatch.json"), "ffmpeg was not found (configured as \"nope\")")
	if report.Health.OK {
		t.Fatal("health should be false")
	}
	if report.Health.Error != "ffmpeg was not found (configured as \"nope\")" {
		t.Fatalf("health error = %q", report.Health.Error)
	}
	found := false
	for _, check := range report.Checks {
		if check.ID == "ffmpeg" && check.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a failing FFmpeg check")
	}
}

func TestRunValidConfig(t *testing.T) {
	cfg := config.Default()
	report := Run(context.Background(), ffmpeg.Tool{}, cfg, filepath.Join(t.TempDir(), "ffbatch.json"), "")
	if !report.ConfigOK {
		t.Fatalf("config should be valid: %+v", report.Checks)
	}
}

func TestRunChecksArchiveFolder(t *testing.T) {
	cfg := config.Default()
	cfg.ArchiveDir = t.TempDir()
	report := Run(context.Background(), ffmpeg.Tool{}, cfg, filepath.Join(t.TempDir(), "ffbatch.json"), "")
	for _, check := range report.Checks {
		if check.ID == "archive" {
			if check.Status != "pass" {
				t.Fatalf("archive check = %+v", check)
			}
			return
		}
	}
	t.Fatal("expected an archive folder check")
}
