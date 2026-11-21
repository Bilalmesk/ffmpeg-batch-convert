package watch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
)

type fakeClock struct{ at time.Time }

func (c *fakeClock) now() time.Time          { return c.at }
func (c *fakeClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testConfig() config.Config {
	cfg := config.Default()
	cfg.Extensions = []string{"ts", "mp4"}
	cfg.Format = "mp4"
	cfg.Mode = config.ModeRemux
	return cfg
}

func newTestWatcher(cfg config.Config, clock *fakeClock) *Watcher {
	return New(Options{
		Interval: time.Hour,
		Settle:   10 * time.Second,
		Config:   func() config.Config { return cfg },
		Now:      clock.now,
	})
}

func TestAddIgnoresExistingByDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "old.ts"), "old")
	clock := &fakeClock{at: time.Now()}
	watcher := newTestWatcher(testConfig(), clock)

	if _, err := watcher.Add(dir, false, false); err != nil {
		t.Fatal(err)
	}
	clock.advance(time.Minute)
	batches, _ := watcher.collectAll(clock.now(), testConfig(), true)
	if len(batches) != 0 {
		t.Fatalf("existing files should be ignored, got %+v", batches)
	}
}

func TestConvertExisting(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "old.ts"), "old")
	clock := &fakeClock{at: time.Now()}
	watcher := newTestWatcher(testConfig(), clock)

	if _, err := watcher.Add(dir, false, true); err != nil {
		t.Fatal(err)
	}
	batches, _ := watcher.collectAll(clock.now(), testConfig(), true)
	if len(batches) != 0 {
		t.Fatal("files should wait for the settle delay")
	}
	clock.advance(10 * time.Second)
	batches, _ = watcher.collectAll(clock.now(), testConfig(), true)
	if len(batches) != 1 || len(batches[0].Files) != 1 {
		t.Fatalf("expected one ready file, got %+v", batches)
	}
}

func TestNewFileWaitsForSettle(t *testing.T) {
	dir := t.TempDir()
	clock := &fakeClock{at: time.Now()}
	watcher := newTestWatcher(testConfig(), clock)
	target, err := watcher.Add(dir, false, false)
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(dir, "new.ts"), "new")
	clock.advance(5 * time.Second)
	if batches, _ := watcher.collectAll(clock.now(), testConfig(), true); len(batches) != 0 {
		t.Fatal("file should still be settling")
	}
	clock.advance(10 * time.Second)
	batches, _ := watcher.collectAll(clock.now(), testConfig(), true)
	if len(batches) != 1 || batches[0].TargetID != target.ID || len(batches[0].Files) != 1 {
		t.Fatalf("expected a ready batch, got %+v", batches)
	}

	// It must not be launched twice.
	clock.advance(time.Minute)
	if batches, _ := watcher.collectAll(clock.now(), testConfig(), true); len(batches) != 0 {
		t.Fatalf("file launched twice: %+v", batches)
	}
}

func TestGrowingFileResetsSettle(t *testing.T) {
	dir := t.TempDir()
	clock := &fakeClock{at: time.Now()}
	watcher := newTestWatcher(testConfig(), clock)
	if _, err := watcher.Add(dir, false, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "growing.ts")
	writeFile(t, path, "part one")
	clock.advance(10 * time.Second)
	if batches, _ := watcher.collectAll(clock.now(), testConfig(), true); len(batches) != 0 {
		t.Fatal("first size should be recorded only")
	}
	writeFile(t, path, "part one plus more")
	clock.advance(5 * time.Second)
	if batches, _ := watcher.collectAll(clock.now(), testConfig(), true); len(batches) != 0 {
		t.Fatal("changed file should wait again")
	}
	clock.advance(10 * time.Second)
	if batches, _ := watcher.collectAll(clock.now(), testConfig(), true); len(batches) != 1 {
		t.Fatal("stable file should be ready")
	}
}

func TestPendingFilesStayQueuedWhileBusy(t *testing.T) {
	dir := t.TempDir()
	clock := &fakeClock{at: time.Now()}
	watcher := newTestWatcher(testConfig(), clock)
	if _, err := watcher.Add(dir, false, false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "new.ts"), "new")
	clock.advance(5 * time.Second)
	if batches, _ := watcher.collectAll(clock.now(), testConfig(), false); len(batches) != 0 {
		t.Fatalf("busy watcher should not launch: %+v", batches)
	}
	clock.advance(10 * time.Second)
	batches, _ := watcher.collectAll(clock.now(), testConfig(), false)
	if len(batches) != 0 {
		t.Fatalf("busy watcher should not launch: %+v", batches)
	}
	status := watcher.Snapshot()
	if len(status.Targets) != 1 || status.Targets[0].Pending != 1 {
		t.Fatalf("pending count = %+v", status.Targets)
	}

	batches, _ = watcher.collectAll(clock.now(), testConfig(), true)
	if len(batches) != 1 {
		t.Fatalf("queued file should launch when idle: %+v", batches)
	}
}

func TestOverlappingFoldersRejected(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{at: time.Now()}
	watcher := newTestWatcher(testConfig(), clock)
	if _, err := watcher.Add(parent, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := watcher.Add(parent, true, false); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate add error = %v", err)
	}
	if _, err := watcher.Add(child, true, false); !errors.Is(err, ErrOverlap) {
		t.Fatalf("overlap error = %v", err)
	}
}

func TestDisabledTargetDoesNotLaunch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.ts"), "a")
	clock := &fakeClock{at: time.Now()}
	watcher := newTestWatcher(testConfig(), clock)
	target, err := watcher.Add(dir, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := watcher.SetEnabled(target.ID, false); err != nil {
		t.Fatal(err)
	}
	clock.advance(time.Minute)
	if batches, _ := watcher.collectAll(clock.now(), testConfig(), true); len(batches) != 0 {
		t.Fatalf("disabled target launched: %+v", batches)
	}
}

func TestOutputFolderAndSameFormatIgnored(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig()
	cfg.OutputDir = filepath.Join(dir, "converted")
	cfg.Recursive = true
	writeFile(t, filepath.Join(dir, "source.ts"), "source")
	writeFile(t, filepath.Join(dir, "already.mp4"), "mp4")
	writeFile(t, filepath.Join(dir, "converted", "output.ts"), "output")

	clock := &fakeClock{at: time.Now()}
	watcher := newTestWatcher(cfg, clock)
	target, err := watcher.Add(dir, true, true)
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(10 * time.Second)
	batches, _ := watcher.collectAll(clock.now(), cfg, true)
	if len(batches) != 1 {
		t.Fatalf("expected one batch, got %+v", batches)
	}
	if len(batches[0].Files) != 1 || filepath.Base(batches[0].Files[0]) != "source.ts" {
		t.Fatalf("wrong files queued: %+v", batches[0].Files)
	}
	if target.Tracked != 1 {
		t.Fatalf("tracked = %d, want 1", target.Tracked)
	}
}

func TestRequeueBacksOff(t *testing.T) {
	dir := t.TempDir()
	clock := &fakeClock{at: time.Now()}
	watcher := newTestWatcher(testConfig(), clock)
	target, err := watcher.Add(dir, false, false)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "a.ts")
	writeFile(t, path, "a")
	clock.advance(5 * time.Second)
	if batches, _ := watcher.collectAll(clock.now(), testConfig(), true); len(batches) != 0 {
		t.Fatal("file should be settling after first sight")
	}
	clock.advance(10 * time.Second)
	batches, _ := watcher.collectAll(clock.now(), testConfig(), true)
	if len(batches) != 1 {
		t.Fatal("expected a batch")
	}

	watcher.requeue(batches[0], errors.New("no ffprobe"))
	clock.advance(5 * time.Second)
	if delayed, _ := watcher.collectAll(clock.now(), testConfig(), true); len(delayed) != 0 {
		t.Fatal("requeued target should back off")
	}
	clock.advance(30 * time.Second)
	if retried, _ := watcher.collectAll(clock.now(), testConfig(), true); len(retried) != 1 {
		t.Fatal("requeued file should become ready after backoff")
	}
	if target.ID == "" {
		t.Fatal("target id missing")
	}
}

func TestOnlyOneBatchReturnedPerTick(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	writeFile(t, filepath.Join(dirA, "a.ts"), "a")
	writeFile(t, filepath.Join(dirB, "b.ts"), "b")
	clock := &fakeClock{at: time.Now()}
	watcher := newTestWatcher(testConfig(), clock)
	if _, err := watcher.Add(dirA, false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := watcher.Add(dirB, false, true); err != nil {
		t.Fatal(err)
	}
	clock.advance(10 * time.Second)
	first, _ := watcher.collectAll(clock.now(), testConfig(), true)
	if len(first) != 1 {
		t.Fatalf("expected one batch per tick, got %d", len(first))
	}
	second, _ := watcher.collectAll(clock.now(), testConfig(), true)
	if len(second) != 1 || second[0].TargetID == first[0].TargetID {
		t.Fatalf("expected the other folder on the next tick, got %+v then %+v", first, second)
	}
}

func TestLoopLaunchesReadyFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.ts"), "a")
	launched := make(chan []string, 1)
	watcher := New(Options{
		Interval: 30 * time.Millisecond,
		Settle:   0,
		Config:   func() config.Config { return testConfig() },
		Launch: func(ctx context.Context, cfg config.Config, files []string) error {
			launched <- files
			return nil
		},
	})
	if _, err := watcher.Add(dir, false, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watcher.Start(ctx)
	defer watcher.Stop()

	select {
	case files := <-launched:
		if len(files) != 1 {
			t.Fatalf("launched %v", files)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("watcher did not launch a batch")
	}
}

func TestStopBeforeStart(t *testing.T) {
	watcher := New(Options{})
	watcher.Stop() // must not panic or block
}
