package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
)

func testReportManager() *Manager {
	return &Manager{
		history: []*Run{{
			ID:        "run-report",
			Status:    StatusDone,
			Mode:      config.ModeRemux,
			Config:    config.Default(),
			StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			Jobs: []*Job{{
				ID:          "job-001",
				Name:        "حلقة.ts",
				Input:       `D:\Input\حلقة.ts`,
				Output:      `D:\Output\حلقة.mp4`,
				Status:      StatusDone,
				SizeBytes:   100,
				OutputBytes: 80,
			}},
		}},
	}
}

func TestCSVReportHasExcelBOM(t *testing.T) {
	manager := testReportManager()
	data, filename, err := manager.Report("run-report", "csv")
	if err != nil {
		t.Fatal(err)
	}
	if filename != "ffbatch-run-report.csv" {
		t.Fatalf("filename = %q", filename)
	}
	if !strings.HasPrefix(string(data), "\xEF\xBB\xBFid,status,name") {
		t.Fatalf("CSV should start with a UTF-8 BOM and header: %q", string(data[:min(len(data), 40)]))
	}
	if !strings.Contains(string(data), "حلقة.ts") {
		t.Fatal("CSV should preserve Unicode names")
	}
}

func TestJSONReportContainsRun(t *testing.T) {
	manager := testReportManager()
	data, filename, err := manager.Report("run-report", "json")
	if err != nil {
		t.Fatal(err)
	}
	if filename != "ffbatch-run-report.json" {
		t.Fatalf("filename = %q", filename)
	}
	if !strings.Contains(string(data), `"id": "run-report"`) {
		t.Fatalf("unexpected JSON report: %s", data)
	}
	if _, _, err := manager.Report("missing", "json"); err == nil {
		t.Fatal("expected an error for an unknown run")
	}
}
