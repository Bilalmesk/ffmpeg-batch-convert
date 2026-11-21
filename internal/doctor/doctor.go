// Package doctor inspects the local FFmpeg installation and configuration.
package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/version"
)

// Check is one diagnostic result.
type Check struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"` // pass | warn | fail
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

// Report is the complete environment snapshot.
type Report struct {
	GeneratedAt time.Time       `json:"generatedAt"`
	Version     string          `json:"version"`
	Platform    string          `json:"platform"`
	GoVersion   string          `json:"goVersion"`
	CPUs        int             `json:"cpus"`
	ConfigPath  string          `json:"configPath"`
	ConfigOK    bool            `json:"configOk"`
	Health      ffmpeg.Health   `json:"health"`
	Checks      []Check         `json:"checks"`
	Encoders    map[string]bool `json:"encoders"`
}

// Run executes every check. It never returns an error: problems are reported as
// failed checks so the dashboard can display them. toolErr carries a binary
// discovery error so the report can explain an explicit bad path.
func Run(ctx context.Context, tool ffmpeg.Tool, cfg config.Config, configPath, toolErr string) Report {
	report := Report{
		GeneratedAt: time.Now(),
		Version:     version.Version,
		Platform:    runtime.GOOS + "/" + runtime.GOARCH,
		GoVersion:   runtime.Version(),
		CPUs:        runtime.NumCPU(),
		ConfigPath:  configPath,
		Encoders:    map[string]bool{},
	}

	health := ffmpeg.Health{OK: false, Error: "FFmpeg is not installed or not on PATH"}
	if tool.FFmpeg != "" {
		health = tool.Check(ctx)
	} else if toolErr != "" {
		health.Error = toolErr
	}
	report.Health = health
	if health.OK {
		report.Checks = append(report.Checks, Check{
			ID: "ffmpeg", Label: "FFmpeg and FFprobe", Status: "pass",
			Detail: health.Version,
		})
	} else {
		report.Checks = append(report.Checks, Check{
			ID: "ffmpeg", Label: "FFmpeg and FFprobe", Status: "fail",
			Detail: health.Error,
			Hint:   "Install FFmpeg (winget install Gyan.FFmpeg) and reopen the terminal.",
		})
	}

	if tool.FFmpeg == "" {
		report.Checks = append(report.Checks, Check{
			ID: "encoders", Label: "Encoders", Status: "fail",
			Detail: "cannot inspect encoders until FFmpeg is available",
		})
	} else if encoders, err := tool.Encoders(ctx); err == nil {
		report.Encoders = encoders
		required := []struct{ name, label string }{
			{"libx264", "H.264 encoder"},
			{"aac", "AAC audio encoder"},
		}
		optional := []struct{ name, label string }{
			{"libx265", "H.265 encoder"},
			{"libopus", "Opus audio encoder"},
		}
		for _, item := range required {
			if encoders[item.name] {
				report.Checks = append(report.Checks, Check{ID: "enc-" + item.name, Label: item.label, Status: "pass", Detail: item.name + " available"})
			} else {
				report.Checks = append(report.Checks, Check{
					ID: "enc-" + item.name, Label: item.label, Status: "fail", Detail: item.name + " missing from this FFmpeg build",
					Hint: "Use a full FFmpeg build such as gyan.dev or BtbN.",
				})
			}
		}
		for _, item := range optional {
			if encoders[item.name] {
				report.Checks = append(report.Checks, Check{ID: "enc-" + item.name, Label: item.label, Status: "pass", Detail: item.name + " available"})
			} else {
				report.Checks = append(report.Checks, Check{
					ID: "enc-" + item.name, Label: item.label, Status: "warn", Detail: item.name + " missing; HEVC or Opus output will fail",
					Hint: "Optional. Install a full FFmpeg build to enable it.",
				})
			}
		}
	}

	if err := cfg.Validate(); err != nil {
		report.Checks = append(report.Checks, Check{
			ID: "config", Label: "Configuration", Status: "fail", Detail: err.Error(),
			Hint: "Run `ffbatch config init` to write a fresh ffbatch.json.",
		})
	} else {
		report.ConfigOK = true
		if _, err := os.Stat(configPath); err == nil {
			report.Checks = append(report.Checks, Check{
				ID: "config", Label: "Configuration", Status: "pass", Detail: configPath,
			})
		} else {
			report.Checks = append(report.Checks, Check{
				ID: "config", Label: "Configuration", Status: "pass",
				Detail: "Using built-in defaults (" + configPath + " does not exist yet)",
			})
		}
	}

	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir, _ = os.Getwd()
	}
	if err := checkWritable(outputDir); err != nil {
		report.Checks = append(report.Checks, Check{
			ID: "output", Label: "Output folder", Status: "fail",
			Detail: outputDir + ": " + err.Error(),
			Hint:   "Choose a folder you can write to, or leave the output folder empty.",
		})
	} else {
		report.Checks = append(report.Checks, Check{
			ID: "output", Label: "Output folder", Status: "pass", Detail: outputDir,
		})
	}

	if cfg.LogFile != "" {
		dir := filepath.Dir(cfg.LogFile)
		if err := checkWritable(dir); err != nil {
			report.Checks = append(report.Checks, Check{
				ID: "log", Label: "Log file", Status: "warn",
				Detail: cfg.LogFile + ": " + err.Error(),
			})
		} else {
			report.Checks = append(report.Checks, Check{
				ID: "log", Label: "Log file", Status: "pass", Detail: cfg.LogFile,
			})
		}
	}

	report.Checks = append(report.Checks, Check{
		ID: "workers", Label: "Concurrency", Status: "pass",
		Detail: fmt.Sprintf("%d worker(s) on %d logical CPU(s)", cfg.Workers, runtime.NumCPU()),
	})
	return report
}

func checkWritable(dir string) error {
	if dir == "" {
		dir = "."
	}
	// The target may not exist yet. Walk up to the nearest existing folder,
	// because the runner will create intermediate folders with MkdirAll.
	probeDir := dir
	for {
		info, err := os.Stat(probeDir)
		if err == nil {
			if info.IsDir() {
				break
			}
			probeDir = filepath.Dir(probeDir)
			continue
		}
		parent := filepath.Dir(probeDir)
		if parent == probeDir {
			return err
		}
		probeDir = parent
	}
	file, err := os.CreateTemp(probeDir, ".ffbatch-write-test-*")
	if err != nil {
		return err
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
	return nil
}
