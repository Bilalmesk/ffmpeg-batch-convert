package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
)

func testServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ffbatch.json")
	manager := engine.NewManager(ffmpeg.Tool{})
	server, err := New(Options{
		Config:     config.Default(),
		ConfigPath: path,
		Manager:    manager,
		Tool:       ffmpeg.Tool{},
		Version:    "test-version",
		Commit:     "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	return server, dir
}

func request(t *testing.T, handler http.Handler, method, path string, body any, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestStatusAndConfig(t *testing.T) {
	server, _ := testServer(t)
	handler := server.Handler()

	rec := request(t, handler, http.MethodGet, "/api/status", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d: %s", rec.Code, rec.Body.String())
	}
	var status map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status["version"] != "test-version" {
		t.Fatalf("version = %v", status["version"])
	}
	if status["config"] == nil {
		t.Fatal("status should include the current config")
	}

	rec = request(t, handler, http.MethodGet, "/api/config", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("config status = %d", rec.Code)
	}
	var payload struct {
		Config  config.Config   `json:"config"`
		Recipes []config.Recipe `json:"recipes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Config.Format != "mp4" || len(payload.Recipes) == 0 {
		t.Fatalf("unexpected config payload: %+v", payload)
	}
}

func TestCapabilitiesIncludesAV1(t *testing.T) {
	server, _ := testServer(t)
	rec := request(t, server.Handler(), http.MethodGet, "/api/capabilities", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload struct {
		VideoCodecs []string `json:"videoCodecs"`
		AV1Encoders []string `json:"av1Encoders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.VideoCodecs) == 0 || payload.VideoCodecs[len(payload.VideoCodecs)-1] != "av1" {
		t.Fatalf("video codecs = %v", payload.VideoCodecs)
	}
	if len(payload.AV1Encoders) != 3 {
		t.Fatalf("av1 encoders = %v", payload.AV1Encoders)
	}
}

func TestStaticIndex(t *testing.T) {
	server, _ := testServer(t)
	rec := request(t, server.Handler(), http.MethodGet, "/", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "FFBatch") {
		t.Fatal("index page missing brand")
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("content type = %q", contentType)
	}
}

func TestScanEndpoint(t *testing.T) {
	server, dir := testServer(t)
	if err := os.WriteFile(filepath.Join(dir, "a.ts"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := request(t, server.Handler(), http.MethodPost, "/api/scan", map[string]any{"dir": dir}, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("scan status = %d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 {
		t.Fatalf("count = %d, want 1", result.Count)
	}
}

func TestScanEndpointRenameIncludesMP4(t *testing.T) {
	server, dir := testServer(t)
	if err := os.WriteFile(filepath.Join(dir, "a.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := request(t, server.Handler(), http.MethodPost, "/api/scan", map[string]any{
		"dir":  dir,
		"mode": "rename",
	}, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("scan status = %d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 {
		t.Fatalf("rename scan count = %d, want 1", result.Count)
	}
}

func TestMutatingRequestsRequireJSON(t *testing.T) {
	server, _ := testServer(t)
	rec := request(t, server.Handler(), http.MethodPost, "/api/scan", map[string]any{"dir": "."}, "")
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
}

func TestCrossOriginBlocked(t *testing.T) {
	server, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/scan", strings.NewReader(`{"dir":"."}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestBrowseEndpoint(t *testing.T) {
	server, dir := testServer(t)
	if err := os.Mkdir(filepath.Join(dir, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := request(t, server.Handler(), http.MethodGet, "/api/fs?path="+dir, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("browse status = %d: %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Path    string     `json:"path"`
		Entries []dirEntry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Path != dir || len(result.Entries) != 1 || result.Entries[0].Name != "child" {
		t.Fatalf("unexpected browse result: %+v", result)
	}
}

func TestStartRunWithoutFFmpegReturns503(t *testing.T) {
	server, dir := testServer(t)
	rec := request(t, server.Handler(), http.MethodPost, "/api/runs", map[string]any{
		"dir":    dir,
		"config": config.Default(),
	}, "application/json")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
}

func TestUnknownRunReturns404(t *testing.T) {
	server, _ := testServer(t)
	rec := request(t, server.Handler(), http.MethodGet, "/api/runs/run-nope", nil, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	rec = request(t, server.Handler(), http.MethodPost, "/api/runs/run-nope/retry", nil, "application/json")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("retry status = %d, want 404", rec.Code)
	}
}

func TestWatchEndpoints(t *testing.T) {
	server, dir := testServer(t)
	handler := server.Handler()

	rec := request(t, handler, http.MethodPost, "/api/watch", map[string]any{
		"dir":             dir,
		"recursive":       false,
		"convertExisting": false,
	}, "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("watch add status = %d: %s", rec.Code, rec.Body.String())
	}
	var target struct {
		ID      string `json:"id"`
		Dir     string `json:"dir"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &target); err != nil {
		t.Fatal(err)
	}
	if target.ID == "" || !target.Enabled {
		t.Fatalf("unexpected target: %+v", target)
	}

	rec = request(t, handler, http.MethodGet, "/api/watch", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("watch status = %d", rec.Code)
	}
	var status struct {
		Targets []struct {
			ID string `json:"id"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Targets) != 1 || status.Targets[0].ID != target.ID {
		t.Fatalf("watched targets = %+v", status.Targets)
	}

	rec = request(t, handler, http.MethodPatch, "/api/watch/"+target.ID, map[string]any{"enabled": false}, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("watch patch status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = request(t, handler, http.MethodDelete, "/api/watch/"+target.ID, nil, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("watch delete status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWatchRejectsOverlap(t *testing.T) {
	server, dir := testServer(t)
	child := filepath.Join(dir, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	rec := request(t, handler, http.MethodPost, "/api/watch", map[string]any{"dir": dir}, "application/json")
	if rec.Code != http.StatusCreated {
		t.Fatalf("first watch status = %d: %s", rec.Code, rec.Body.String())
	}
	rec = request(t, handler, http.MethodPost, "/api/watch", map[string]any{"dir": child}, "application/json")
	if rec.Code != http.StatusConflict {
		t.Fatalf("overlap status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestPutConfig(t *testing.T) {
	server, dir := testServer(t)
	cfg := config.Default()
	cfg.Workers = 2
	cfg.Format = "mkv"
	rec := request(t, server.Handler(), http.MethodPut, "/api/config", map[string]any{
		"config": cfg,
		"save":   true,
	}, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("put config status = %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "ffbatch.json")); err != nil {
		t.Fatalf("config was not saved: %v", err)
	}
	current, _ := server.currentConfig()
	if current.Workers != 2 || current.Format != "mkv" {
		t.Fatalf("server config not updated: %+v", current)
	}
}

func TestInvalidConfigRejected(t *testing.T) {
	server, _ := testServer(t)
	cfg := config.Default()
	cfg.Workers = 999
	rec := request(t, server.Handler(), http.MethodPut, "/api/config", map[string]any{
		"config": cfg,
	}, "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}
