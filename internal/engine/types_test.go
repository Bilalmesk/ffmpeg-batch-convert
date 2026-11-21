package engine

import (
	"testing"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
)

func TestSummarizeProgress(t *testing.T) {
	run := &Run{
		ID:        "run-test",
		Status:    StatusCanceled,
		Mode:      config.ModeRemux,
		StartedAt: time.Now(),
		Jobs: []*Job{
			{Status: StatusDone, SizeBytes: 100, OutputBytes: 80, Progress: 100},
			{Status: StatusCanceled, SizeBytes: 100, Progress: 50},
			{Status: StatusPending, SizeBytes: 100},
		},
	}
	summary := summarize(run)
	if summary.Total != 3 || summary.Done != 1 || summary.Canceled != 1 || summary.Pending != 1 {
		t.Fatalf("unexpected counts: %+v", summary)
	}
	if summary.Percent != 50 {
		t.Fatalf("percent = %v, want 50 (done 1 + cancelled 0.5 of 3)", summary.Percent)
	}
	if summary.BytesIn != 300 || summary.BytesOut != 80 {
		t.Fatalf("bytes = %d/%d", summary.BytesIn, summary.BytesOut)
	}
}
