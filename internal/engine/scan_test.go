package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
)

func touch(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanRespectsRecursionAndExtensions(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "a.ts"), 10)
	touch(t, filepath.Join(dir, "b.mkv"), 20)
	touch(t, filepath.Join(dir, "notes.txt"), 30)
	touch(t, filepath.Join(dir, "sub", "c.ts"), 40)
	touch(t, filepath.Join(dir, "sub", "skip.ffbatch-tmp.mp4"), 50)
	touch(t, filepath.Join(dir, ".git", "d.ts"), 60)

	flat, err := Scan(dir, false, []string{"ts", "mkv"})
	if err != nil {
		t.Fatal(err)
	}
	if len(flat) != 2 {
		t.Fatalf("flat scan = %d files, want 2: %+v", len(flat), flat)
	}

	recursive, err := Scan(dir, true, []string{"ts", "mkv"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recursive) != 3 {
		t.Fatalf("recursive scan = %d files, want 3: %+v", len(recursive), recursive)
	}
	for _, candidate := range recursive {
		if strings.Contains(candidate.Name, "ffbatch") {
			t.Fatalf("temporary file should be ignored: %s", candidate.Name)
		}
	}
}

func TestPlanSkipsExistingOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "a.ts")
	touch(t, input, 10)
	touch(t, filepath.Join(dir, "a.mp4"), 10)

	cfg := config.Default()
	jobs, _, err := Plan(cfg, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].Status != StatusSkipped || jobs[0].Reason != "output already exists" {
		t.Fatalf("unexpected job: %+v", jobs[0])
	}
}

func TestPlanNumbersExistingOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "a.ts")
	touch(t, input, 10)
	touch(t, filepath.Join(dir, "a.mp4"), 10)

	cfg := config.Default()
	cfg.Overwrite = config.OverwriteNumbering
	jobs, _, err := Plan(cfg, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].Status != StatusPending {
		t.Fatalf("expected pending, got %s", jobs[0].Status)
	}
	if want := filepath.Join(dir, "a (1).mp4"); jobs[0].Output != want {
		t.Fatalf("output = %q, want %q", jobs[0].Output, want)
	}
}

func TestPlanDisambiguatesCollisions(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.ts")
	second := filepath.Join(dir, "a.mkv")
	touch(t, first, 10)
	touch(t, second, 10)

	cfg := config.Default()
	jobs, warnings, err := Plan(cfg, []string{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].Output == jobs[1].Output {
		t.Fatalf("outputs must differ: %q", jobs[0].Output)
	}
	if !strings.Contains(jobs[1].Output, "a (1)") {
		t.Fatalf("second output should be numbered: %s", jobs[1].Output)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a collision warning")
	}
}

func TestPlanSkipsSameContainerInRemux(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "a.mp4")
	touch(t, input, 10)

	cfg := config.Default()
	cfg.Mode = config.ModeRemux
	jobs, _, err := Plan(cfg, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].Status != StatusSkipped {
		t.Fatalf("remuxing MP4 to MP4 should be skipped: %+v", jobs[0])
	}
}

func TestPlanRefusesEncodeOverSource(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "a.mp4")
	touch(t, input, 10)

	cfg := config.Default()
	cfg.Mode = config.ModeEncode
	jobs, _, err := Plan(cfg, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].Status != StatusFailed || !strings.Contains(jobs[0].Error, "overwrite the source") {
		t.Fatalf("encode over source should fail: %+v", jobs[0])
	}
}

func TestPlanRenameChangesExtension(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "episode.mp4")
	touch(t, input, 10)

	cfg := config.Default()
	cfg.Mode = config.ModeRename
	cfg.Format = "ts"
	jobs, _, err := Plan(cfg, []string{input})
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].Status != StatusPending {
		t.Fatalf("expected pending: %+v", jobs[0])
	}
	if want := filepath.Join(dir, "episode.ts"); jobs[0].Output != want {
		t.Fatalf("output = %q, want %q", jobs[0].Output, want)
	}
}

func TestUniquePathAvoidsDiskAndReservations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	touch(t, path, 1)
	touch(t, filepath.Join(dir, "clip (1).mp4"), 1)

	got := uniquePath(path, map[string]bool{pathKey(filepath.Join(dir, "clip (2).mp4")): true}, true)
	if want := filepath.Join(dir, "clip (3).mp4"); got != want {
		t.Fatalf("uniquePath = %q, want %q", got, want)
	}
}

func TestScanExtensionsAddsMP4ForRename(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeRename
	cfg.Format = "ts"
	exts := ScanExtensions(cfg)
	found := false
	for _, ext := range exts {
		if ext == "mp4" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rename scan should include mp4: %v", exts)
	}
	cfg.Mode = config.ModeRemux
	for _, ext := range ScanExtensions(cfg) {
		if ext == "mp4" {
			t.Fatal("remux scan should keep the configured extensions")
		}
	}
}

func TestCollectInputsDeduplicates(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "a.ts")
	touch(t, input, 10)

	cfg := config.Default()
	files, _, _, err := collectInputs(cfg, []string{input, input, dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one de-duplicated file, got %v", files)
	}
}
