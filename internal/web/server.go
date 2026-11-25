// Package web serves the local FFmpeg dashboard and its JSON API.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/watch"
)

//go:embed assets/*
var assetFS embed.FS

// Options configures a dashboard server.
type Options struct {
	Address string
	Config  config.Config
	// Options for the dashboard watcher.
	WatchInterval time.Duration
	WatchSettle   time.Duration

	ConfigPath string
	Manager    *engine.Manager
	Tool       ffmpeg.Tool
	ToolError  string
	Version    string
	Commit     string
	// Token optionally requires a shared secret before serving the dashboard.
	Token string
}

// Server is the local dashboard.
type Server struct {
	cfgMu      sync.RWMutex
	cfg        config.Config
	configPath string
	manager    *engine.Manager
	tool       ffmpeg.Tool
	health     ffmpeg.Health
	version    string
	commit     string
	startedAt  time.Time
	address    string
	toolError  string
	token      string
	watcher    *watch.Watcher

	ctx    context.Context
	cancel context.CancelFunc
	http   *http.Server
}

// New creates a server. It never fails on a missing FFmpeg binary; the
// dashboard shows the problem instead.
func New(opts Options) (*Server, error) {
	cfg := opts.Config
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		cfg:        cfg,
		configPath: opts.ConfigPath,
		manager:    opts.Manager,
		tool:       opts.Tool,
		version:    opts.Version,
		commit:     opts.Commit,
		startedAt:  time.Now(),
		address:    opts.Address,
		toolError:  opts.ToolError,
		token:      strings.TrimSpace(opts.Token),
		ctx:        ctx,
		cancel:     cancel,
	}
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer checkCancel()
	if opts.Tool.FFmpeg == "" {
		message := opts.ToolError
		if message == "" {
			message = "FFmpeg is not installed or not on PATH"
		}
		s.health = ffmpeg.Health{OK: false, Error: message}
	} else {
		s.health = opts.Tool.Check(checkCtx)
	}

	s.watcher = watch.New(watch.Options{
		Interval: opts.WatchInterval,
		Settle:   opts.WatchSettle,
		Config: func() config.Config {
			cfg, _ := s.currentConfig()
			return cfg
		},
		Busy: func() bool { return s.manager.Active() != nil },
		Launch: func(launchCtx context.Context, cfg config.Config, files []string) error {
			if !s.health.OK {
				return fmt.Errorf("FFmpeg is not available: %s", s.health.Error)
			}
			_, err := s.manager.Start(launchCtx, cfg, files)
			return err
		},
	})
	return s, nil
}

// StartWatcher starts monitoring the registered watch folders.
func (s *Server) StartWatcher() {
	s.watcher.Start(s.ctx)
}

// Listen binds the HTTP listener and prepares the server.
func (s *Server) Listen() (net.Listener, error) {
	address := s.address
	if address == "" {
		address = "127.0.0.1:8737"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	s.address = listener.Addr().String()
	s.http = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return listener, nil
}

// Serve blocks while serving requests.
func (s *Server) Serve(listener net.Listener) error {
	if s.http == nil {
		return errors.New("server is not initialised")
	}
	err := s.http.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops the dashboard and detaches the run context.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.watcher != nil {
		s.watcher.Stop()
	}
	s.cancel()
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

// URL returns the browsable address for a listener.
func URL(listener net.Listener) string {
	address := listener.Addr().String()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "http://" + address
	}
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// Handler exposes the routing table for tests and embedders.
func (s *Server) Handler() http.Handler { return s.routes() }

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	mux.HandleFunc("GET /api/capabilities", s.handleCapabilities)
	mux.HandleFunc("POST /api/scan", s.handleScan)
	mux.HandleFunc("GET /api/fs", s.handleBrowse)
	mux.HandleFunc("POST /api/reveal", s.handleReveal)
	mux.HandleFunc("GET /api/doctor", s.handleDoctor)

	mux.HandleFunc("GET /api/watch", s.handleWatchStatus)
	mux.HandleFunc("POST /api/watch", s.handleWatchAdd)
	mux.HandleFunc("DELETE /api/watch/{id}", s.handleWatchRemove)
	mux.HandleFunc("PATCH /api/watch/{id}", s.handleWatchPatch)

	mux.HandleFunc("GET /api/runs", s.handleListRuns)
	mux.HandleFunc("POST /api/runs", s.handleStartRun)
	mux.HandleFunc("GET /api/runs/{id}", s.handleGetRun)
	mux.HandleFunc("POST /api/runs/{id}/cancel", s.handleCancelRun)
	mux.HandleFunc("POST /api/runs/{id}/retry", s.handleRetryRun)
	mux.HandleFunc("GET /api/runs/{id}/report", s.handleReport)
	mux.HandleFunc("GET /api/events", s.handleEvents)

	sub, err := fs.Sub(assetFS, "assets")
	if err != nil {
		panic(err)
	}
	assets := http.FileServer(http.FS(sub))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/index.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			data, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.Error(w, "dashboard assets missing", http.StatusInternalServerError)
				return
			}
			_, _ = w.Write(data)
		default:
			w.Header().Set("Cache-Control", "no-cache")
			assets.ServeHTTP(w, r)
		}
	})

	return s.auth(s.security(mux))
}

func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
				writeError(w, http.StatusUnsupportedMediaType, "requests must use application/json")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && !sameOrigin(origin, r.Host) {
				writeError(w, http.StatusForbidden, "cross-origin request blocked")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(origin, host string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Host, host)
}

func (s *Server) currentConfig() (config.Config, string) {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg, s.configPath
}

func (s *Server) storeConfig(cfg config.Config, save bool) error {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return err
	}
	_, path := s.currentConfig()
	if save && path != "" {
		if err := config.Save(path, cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}
	s.cfgMu.Lock()
	s.cfg = cfg
	s.cfgMu.Unlock()
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func writeError(w http.ResponseWriter, status int, format string, args ...any) {
	writeJSON(w, status, map[string]string{"error": fmt.Sprintf(format, args...)})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: %v", err)
		return false
	}
	return true
}
