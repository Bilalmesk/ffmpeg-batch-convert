package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
)

func TestRunIsInactiveWhenFinishedLogIsEmitted(t *testing.T) {
	manager := NewManager(ffmpeg.Tool{})
	_, events, _ := manager.Subscribe()

	run := &Run{
		ID:        "run-race",
		Status:    StatusRunning,
		Mode:      config.ModeRemux,
		Config:    config.Default(),
		StartedAt: time.Now(),
		Jobs: []*Job{{
			ID:     "job-001",
			Status: StatusSkipped,
			Reason: "already converted",
		}},
	}
	manager.mu.Lock()
	manager.active = run
	manager.history = []*Run{run}
	manager.mu.Unlock()

	done := make(chan struct{})
	go func() {
		manager.execute(context.Background(), run)
		close(done)
	}()

	timeout := time.After(5 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for the finished log")
		case event := <-events:
			if event.Type != "log" || !strings.Contains(event.Message, "Run finished") {
				continue
			}
			if active := manager.Active(); active != nil {
				t.Fatalf("run is still active when the finished log is emitted: %+v", active)
			}
			<-done
			return
		}
	}
}
