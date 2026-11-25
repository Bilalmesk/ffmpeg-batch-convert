package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/hashutil"
)

func writeManifest(t *testing.T, snapshot *engine.RunSnapshot) string {
	t.Helper()
	data, err := json.Marshal(engine.ReportDoc{
		Schema:      engine.ReportSchema,
		Tool:        "ffbatch test",
		GeneratedAt: time.Now(),
		Run:         snapshot,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerifyManifest(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "out.mp4")
	if err := os.WriteFile(output, []byte("converted"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := hashutil.SHA256(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	outputInfo, _ := os.Stat(output)

	snapshot := &engine.RunSnapshot{
		Jobs: []*engine.Job{
			{
				ID: "j1", Name: "good", Status: engine.StatusDone,
				Input: filepath.Join(dir, "source.ts"), Output: output,
				OutputBytes: outputInfo.Size(), OutputSHA256: digest,
			},
			{
				ID: "j2", Name: "skipped", Status: engine.StatusSkipped,
			},
			{
				ID: "j3", Name: "missing", Status: engine.StatusDone,
				Output: filepath.Join(dir, "missing.mp4"), OutputSHA256: "abc",
			},
		},
	}
	results, err := verifyManifestFile(context.Background(), writeManifest(t, snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if results[0].Status != "pass" {
		t.Fatalf("good output status = %q: %s", results[0].Status, results[0].Detail)
	}
	if results[1].Status != "skip" {
		t.Fatalf("skipped job status = %q", results[1].Status)
	}
	if results[2].Status != "fail" {
		t.Fatalf("missing output status = %q", results[2].Status)
	}
}

func TestVerifyManifestDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "out.mp4")
	if err := os.WriteFile(output, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := hashutil.SHA256(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(output)
	if err := os.WriteFile(output, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}

	snapshot := &engine.RunSnapshot{Jobs: []*engine.Job{{
		ID: "j1", Name: "tampered", Status: engine.StatusDone,
		Output: output, OutputBytes: info.Size(), OutputSHA256: digest,
	}}}
	results, err := verifyManifestFile(context.Background(), writeManifest(t, snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "fail" {
		t.Fatalf("tampered result = %+v", results)
	}
}

func TestVerifyManifestChecksArchivedSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.ts")
	archive := filepath.Join(dir, "archive", "source.ts")
	if err := os.WriteFile(source, []byte("original media"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceDigest, err := hashutil.SHA256(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, archive); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "out.mp4")
	if err := os.WriteFile(output, []byte("converted"), 0o644); err != nil {
		t.Fatal(err)
	}
	outputDigest, err := hashutil.SHA256(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	outputInfo, _ := os.Stat(output)

	snapshot := &engine.RunSnapshot{Jobs: []*engine.Job{{
		ID: "j1", Name: "archived", Status: engine.StatusDone,
		Input: source, Output: output, OutputBytes: outputInfo.Size(),
		OutputSHA256: outputDigest, SourceSHA256: sourceDigest,
		SourceArchivedTo: archive, SourceRemoved: true,
	}}}
	manifest := writeManifest(t, snapshot)
	results, err := verifyManifestFile(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Status != "pass" || !strings.Contains(results[0].Detail, "archived") {
		t.Fatalf("archived result = %+v", results[0])
	}

	if err := os.WriteFile(archive, []byte("tampered media"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err = verifyManifestFile(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Status != "fail" || !strings.Contains(results[0].Detail, "source SHA-256 mismatch") {
		t.Fatalf("tampered archive result = %+v", results[0])
	}
}

func TestVerifyManifestRejectsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, []byte(`{"run":{"jobs":[]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyManifestFile(context.Background(), path); err == nil {
		t.Fatal("expected an error for a manifest with no jobs")
	}
}
