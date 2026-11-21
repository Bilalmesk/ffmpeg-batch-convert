package web

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/doctor"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/watch"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg, path := s.currentConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":       s.version,
		"commit":        s.commit,
		"platform":      runtime.GOOS + "/" + runtime.GOARCH,
		"go":            runtime.Version(),
		"cpus":          runtime.NumCPU(),
		"startedAt":     s.startedAt,
		"uptimeSeconds": int(time.Since(s.startedAt).Seconds()),
		"address":       s.address,
		"configPath":    path,
		"config":        cfg,
		"tool": map[string]any{
			"ffmpeg":  s.tool.FFmpeg,
			"ffprobe": s.tool.FFprobe,
			"version": s.health.Version,
			"ok":      s.health.OK,
			"error":   s.health.Error,
		},
		"activeRun": s.manager.Active(),
		"runs":      s.manager.List(),
	})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, path := s.currentConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"config":     cfg,
		"configPath": path,
		"recipes":    config.Recipes(),
		"formats":    config.SupportedFormats,
		"presets":    config.SupportedPresets,
	})
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Config config.Config `json:"config"`
		Save   bool          `json:"save"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.storeConfig(req.Config, req.Save); err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	cfg, path := s.currentConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"config":     cfg,
		"configPath": path,
		"saved":      req.Save,
	})
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"formats":        config.SupportedFormats,
		"presets":        config.SupportedPresets,
		"extensions":     config.DefaultExtensions(),
		"recipes":        config.Recipes(),
		"modes":          []string{"remux", "encode", "rename"},
		"overwrite":      []string{"skip", "overwrite", "number", "fail"},
		"audioCodecs":    []string{"copy", "aac", "opus", "none"},
		"subtitleCodecs": []string{"auto", "copy", "mov_text", "none"},
	})
}

type scanRequest struct {
	Dir        string   `json:"dir"`
	Recursive  *bool    `json:"recursive"`
	Extensions []string `json:"extensions"`
	Mode       string   `json:"mode"`
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	var req scanRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Dir) == "" {
		writeError(w, http.StatusBadRequest, "dir is required")
		return
	}
	cfg, _ := s.currentConfig()
	recursive := cfg.Recursive
	if req.Recursive != nil {
		recursive = *req.Recursive
	}
	extensions := req.Extensions
	if len(extensions) == 0 {
		extensions = cfg.Extensions
	}
	scanCfg := cfg
	scanCfg.Extensions = extensions
	if req.Mode != "" {
		scanCfg.Mode = config.Mode(req.Mode)
	}
	extensions = engine.ScanExtensions(scanCfg)
	dir := req.Dir
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}

	candidates, err := engine.Scan(dir, recursive, extensions)
	if err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	const limit = 5000
	truncated := false
	if len(candidates) > limit {
		candidates = candidates[:limit]
		truncated = true
	}
	var total int64
	for _, candidate := range candidates {
		total += candidate.SizeBytes
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"root":       dir,
		"recursive":  recursive,
		"extensions": extensions,
		"count":      len(candidates),
		"totalBytes": total,
		"truncated":  truncated,
		"files":      candidates,
	})
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	if !s.health.OK {
		writeError(w, http.StatusServiceUnavailable, "FFmpeg is not available: %s", s.health.Error)
		return
	}
	var req struct {
		Dir        string         `json:"dir"`
		Files      []string       `json:"files"`
		Config     *config.Config `json:"config"`
		SaveConfig bool           `json:"saveConfig"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	cfg, _ := s.currentConfig()
	if req.Config != nil {
		cfg = *req.Config
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	if req.SaveConfig {
		if err := s.storeConfig(cfg, true); err != nil {
			writeError(w, http.StatusInternalServerError, "%v", err)
			return
		}
	} else {
		_ = s.storeConfig(cfg, false)
	}

	paths := req.Files
	if len(paths) == 0 && strings.TrimSpace(req.Dir) != "" {
		paths = []string{req.Dir}
	}
	if len(paths) == 0 {
		writeError(w, http.StatusBadRequest, "choose a source folder or select files first")
		return
	}

	snapshot, err := s.manager.Start(s.ctx, cfg, paths)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrBusy):
			writeError(w, http.StatusConflict, "%v", err)
		case errors.Is(err, engine.ErrNoFiles):
			writeError(w, http.StatusUnprocessableEntity, "%v", err)
		default:
			writeError(w, http.StatusInternalServerError, "%v", err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, snapshot)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"runs":   s.manager.List(),
		"active": s.manager.Active(),
	})
}

func (s *Server) handleRetryRun(w http.ResponseWriter, r *http.Request) {
	snapshot, err := s.manager.Retry(s.ctx, r.PathValue("id"))
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, snapshot)
	case errors.Is(err, engine.ErrNotFound):
		writeError(w, http.StatusNotFound, "run not found")
	case errors.Is(err, engine.ErrNoFailures):
		writeError(w, http.StatusBadRequest, "%v", err)
	case errors.Is(err, engine.ErrBusy):
		writeError(w, http.StatusConflict, "%v", err)
	default:
		writeError(w, http.StatusInternalServerError, "%v", err)
	}
}

func (s *Server) handleWatchStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.watcher.Snapshot())
}

func (s *Server) handleWatchAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Dir             string         `json:"dir"`
		Recursive       *bool          `json:"recursive"`
		ConvertExisting bool           `json:"convertExisting"`
		Config          *config.Config `json:"config"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Dir) == "" {
		writeError(w, http.StatusBadRequest, "dir is required")
		return
	}
	if req.Config != nil {
		if err := s.storeConfig(*req.Config, false); err != nil {
			writeError(w, http.StatusBadRequest, "%v", err)
			return
		}
	}
	cfg, _ := s.currentConfig()
	recursive := cfg.Recursive
	if req.Recursive != nil {
		recursive = *req.Recursive
	}
	target, err := s.watcher.Add(req.Dir, recursive, req.ConvertExisting)
	if err != nil {
		switch {
		case errors.Is(err, watch.ErrExists), errors.Is(err, watch.ErrOverlap):
			writeError(w, http.StatusConflict, "%v", err)
		case errors.Is(err, watch.ErrNotDir):
			writeError(w, http.StatusBadRequest, "%v", err)
		default:
			writeError(w, http.StatusBadRequest, "%v", err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, target)
}

func (s *Server) handleWatchRemove(w http.ResponseWriter, r *http.Request) {
	if err := s.watcher.Remove(r.PathValue("id")); err != nil {
		if errors.Is(err, watch.ErrNotFound) {
			writeError(w, http.StatusNotFound, "%v", err)
			return
		}
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleWatchPatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	target, err := s.watcher.SetEnabled(r.PathValue("id"), *req.Enabled)
	if err != nil {
		if errors.Is(err, watch.ErrNotFound) {
			writeError(w, http.StatusNotFound, "%v", err)
			return
		}
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := s.manager.Snapshot(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	err := s.manager.Cancel(r.PathValue("id"))
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case errors.Is(err, engine.ErrNotRunning):
		writeError(w, http.StatusConflict, "this run is not active")
	default:
		writeError(w, http.StatusInternalServerError, "%v", err)
	}
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}
	data, filename, err := s.manager.Report(r.PathValue("id"), format)
	if err != nil {
		if errors.Is(err, engine.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeError(w, http.StatusBadRequest, "%v", err)
		return
	}
	contentType := "application/json; charset=utf-8"
	if strings.EqualFold(format, "csv") {
		contentType = "text/csv; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	cfg, path := s.currentConfig()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	report := doctor.Run(ctx, s.tool, cfg, path, s.toolError)
	writeJSON(w, http.StatusOK, report)
}

type dirEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	current := r.URL.Query().Get("path")
	if current == "" {
		home, _ := os.UserHomeDir()
		current = home
	}
	if abs, err := filepath.Abs(current); err == nil {
		current = abs
	}
	info, err := os.Stat(current)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot open folder: %v", err)
		return
	}
	if !info.IsDir() {
		current = filepath.Dir(current)
	}

	entries, err := os.ReadDir(current)
	if err != nil {
		writeError(w, http.StatusForbidden, "cannot read folder: %v", err)
		return
	}
	dirs := make([]dirEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "$RECYCLE.BIN" || name == "System Volume Information" {
			continue
		}
		dirs = append(dirs, dirEntry{Name: name, Path: filepath.Join(current, name), IsDir: true})
	}
	sort.SliceStable(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	parent := filepath.Dir(current)
	if parent == current {
		parent = ""
	}

	cfg, _ := s.currentConfig()
	places := []dirEntry{}
	if home, err := os.UserHomeDir(); err == nil {
		places = append(places, dirEntry{Name: "Home", Path: home, IsDir: true})
		if _, err := os.Stat(filepath.Join(home, "Videos")); err == nil {
			places = append(places, dirEntry{Name: "Videos", Path: filepath.Join(home, "Videos"), IsDir: true})
		}
	}
	if wd, err := os.Getwd(); err == nil {
		places = append(places, dirEntry{Name: "Working folder", Path: wd, IsDir: true})
	}
	if cfg.OutputDir != "" {
		if abs, err := filepath.Abs(cfg.OutputDir); err == nil {
			if _, err := os.Stat(abs); err == nil {
				places = append(places, dirEntry{Name: "Output folder", Path: abs, IsDir: true})
			}
		}
	}

	roots := rootEntries()
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    current,
		"parent":  parent,
		"roots":   roots,
		"places":  places,
		"entries": dirs,
	})
}

func rootEntries() []dirEntry {
	var roots []dirEntry
	if runtime.GOOS == "windows" {
		for letter := 'A'; letter <= 'Z'; letter++ {
			path := string(letter) + `:\`
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				roots = append(roots, dirEntry{Name: string(letter) + ":", Path: path, IsDir: true})
			}
		}
		return roots
	}
	roots = append(roots, dirEntry{Name: "/", Path: "/", IsDir: true})
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, dirEntry{Name: "Home", Path: home, IsDir: true})
	}
	return roots
}

func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Path = strings.TrimSpace(req.Path)
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	info, err := os.Stat(req.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "path not found: %v", err)
		return
	}
	if err := reveal(absPath(req.Path), info.IsDir()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not open file manager: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func absPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}
