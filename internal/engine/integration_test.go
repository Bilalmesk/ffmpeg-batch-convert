package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
)

// makeClip produces a tiny real transport stream so the full convert,
// verify and archive pipeline can run in CI.
func makeClip(t *testing.T, tool ffmpeg.Tool, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "clip.ts")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool.FFmpeg,
		"-y", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=10",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-t", "2", "-c:v", "mpeg4", "-c:a", "aac", "-shortest", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate clip: %v\n%s", err, output)
	}
	return path
}

func waitForRun(t *testing.T, manager *Manager, events <-chan Event, id string) *RunSnapshot {
	t.Helper()
	deadline := time.After(2 * time.Minute)
	for {
		select {
		case event := <-events:
			if event.Type != "run.finished" {
				continue
			}
			if event.Run != nil && event.Run.ID != id {
				continue
			}
			snapshot, ok := manager.Snapshot(id)
			if !ok {
				t.Fatal("run disappeared")
			}
			return snapshot
		case <-deadline:
			t.Fatal("timed out waiting for the run")
		}
	}
}

func TestPipelineVerifiesHashesAndArchives(t *testing.T) {
	tool, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("ffmpeg not available:", err)
	}
	dir := t.TempDir()
	source := makeClip(t, tool, dir)
	archive := filepath.Join(dir, "archive")

	cfg := config.Default()
	cfg.Extensions = []string{"ts"}
	cfg.OutputDir = filepath.Join(dir, "out")
	cfg.ArchiveDir = archive
	cfg.Workers = 1
	cfg.VerifyOutput = false // forced on because the source is archived
	cfg.HashFiles = false    // forced on for the same reason

	manager := NewManager(tool)
	subID, events, _ := manager.Subscribe()
	defer manager.Unsubscribe(subID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	started, err := manager.Start(ctx, cfg, []string{source})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := waitForRun(t, manager, events, started.ID)

	if snapshot.Done != 1 || snapshot.Failed != 0 {
		t.Fatalf("done=%d failed=%d: %+v", snapshot.Done, snapshot.Failed, snapshot.Jobs)
	}
	job := snapshot.Jobs[0]
	if !job.Verified || job.Verification != "passed" {
		t.Fatalf("output was not verified: %+v", job)
	}
	if len(job.SourceSHA256) != 64 || len(job.OutputSHA256) != 64 {
		t.Fatalf("hashes were not recorded: %+v", job)
	}
	if job.SourceArchivedTo == "" {
		t.Fatal("source was not archived")
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source still exists after archive: %v", err)
	}
	if _, err := os.Stat(job.Output); err != nil {
		t.Fatalf("output missing: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(job.SourceArchivedTo), "clip.ts") {
		t.Fatalf("archived path = %q", job.SourceArchivedTo)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(job.Output), "."+strings.TrimSuffix(filepath.Base(job.Output), filepath.Ext(job.Output))+".ffbatch-"+job.ID+".part"+filepath.Ext(job.Output))); !os.IsNotExist(err) {
		t.Fatal("temporary part file was left behind")
	}
}
