package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
)

func TestArchiveSourcePreservesTree(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source", "shows", "episode.ts")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "archive")
	cfg := config.Default()
	cfg.ArchiveDir = archive
	run := &Run{Root: filepath.Join(root, "source")}
	job := &Job{Input: source}

	dest, err := archiveSource(run, job, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(archive, "shows", "episode.ts")
	if dest != want {
		t.Fatalf("archive dest = %q, want %q", dest, want)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("archived file missing: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
}

func TestArchiveSourceRefusesSamePath(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "episode.ts")
	if err := os.WriteFile(source, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.ArchiveDir = root
	run := &Run{}
	if _, err := archiveSource(run, &Job{Input: source}, cfg); err == nil {
		t.Fatal("expected an error when the archive folder is the source folder")
	}
}

func TestArchiveSourceFallsBackWhenOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "elsewhere")
	source := filepath.Join(sourceDir, "episode.ts")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.ArchiveDir = filepath.Join(dir, "archive")
	run := &Run{Root: filepath.Join(dir, "different-root")}
	dest, err := archiveSource(run, &Job{Input: source}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cfg.ArchiveDir, "episode.ts"); dest != want {
		t.Fatalf("archive dest = %q, want %q", dest, want)
	}
}

func TestSourceStillMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.ts")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !sourceStillMatches(path, info.Size(), info.ModTime()) {
		t.Fatal("unchanged source should match")
	}
	if sourceStillMatches(path, info.Size()+1, info.ModTime()) {
		t.Fatal("size change should be detected")
	}
	if sourceStillMatches(path, info.Size(), info.ModTime().Add(time.Second)) {
		t.Fatal("mtime change should be detected")
	}
}

func TestEstimatedBytes(t *testing.T) {
	cfg := config.Default()
	const size = int64(1 << 30)
	remux := estimatedBytes(cfg, size)
	if remux <= uint64(size) {
		t.Fatalf("remux estimate %d should exceed the source", remux)
	}
	cfg.Mode = config.ModeEncode
	encode := estimatedBytes(cfg, size)
	if encode <= uint64(size/2) {
		t.Fatalf("encode estimate %d should include headroom", encode)
	}
	if estimatedBytes(cfg, 0) != 0 {
		t.Fatal("zero-size sources need no space estimate")
	}
}
