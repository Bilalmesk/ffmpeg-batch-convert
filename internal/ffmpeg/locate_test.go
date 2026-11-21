package ffmpeg

import (
	"path/filepath"
	"testing"
)

func TestLocateRejectsExplicitMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "definitely-no-ffmpeg.exe")
	if _, err := Locate(missing, ""); err == nil {
		t.Fatal("expected an error for an explicit missing ffmpeg path")
	}
}

func TestLocateRejectsDirectoryWithoutBinary(t *testing.T) {
	dir := t.TempDir()
	if _, err := Locate(dir, ""); err == nil {
		t.Fatal("expected an error when the configured folder has no ffmpeg binary")
	}
}

func TestSummarizeStderrKeepsLastLines(t *testing.T) {
	stderr := "line one\nline two\n\nline three\n"
	got := SummarizeStderr(stderr, 2)
	if got != "line two | line three" {
		t.Fatalf("SummarizeStderr = %q", got)
	}
}

func TestContainers(t *testing.T) {
	if !isMP4("mp4") || !isMP4("MOV") || isMP4("mkv") {
		t.Fatal("isMP4 classification is wrong")
	}
	if isFaststartContainer("ts") || !isFaststartContainer("mp4") {
		t.Fatal("faststart classification is wrong")
	}
}
