// Package ffmpeg locates the FFmpeg binaries, builds conversion arguments and
// runs processes while parsing their machine-readable progress stream.
package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Tool is a resolved pair of FFmpeg executables.
type Tool struct {
	FFmpeg  string
	FFprobe string
	Version string
}

// Locate resolves the configured binary paths, falling back to PATH and the
// usual Windows package-manager locations.
func Locate(ffmpegPath, ffprobePath string) (Tool, error) {
	ff, err := resolveBinary(ffmpegPath, "ffmpeg")
	if err != nil {
		return Tool{}, err
	}
	fp, err := resolveBinary(ffprobePath, "ffprobe")
	if err != nil {
		// ffprobe normally sits next to ffmpeg; try that before giving up.
		sibling := filepath.Join(filepath.Dir(ff), "ffprobe"+exeSuffix())
		if fileExists(sibling) {
			fp = sibling
		} else {
			return Tool{}, fmt.Errorf("ffprobe not found: %w", err)
		}
	}
	return Tool{FFmpeg: ff, FFprobe: fp}, nil
}

func resolveBinary(configured, name string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if info, err := os.Stat(configured); err == nil {
			if info.IsDir() {
				candidate := filepath.Join(configured, name+exeSuffix())
				if fileExists(candidate) {
					return candidate, nil
				}
				return "", fmt.Errorf("%s not found in %s", name, configured)
			}
			return configured, nil
		}
		if p, err := exec.LookPath(configured); err == nil {
			return p, nil
		}
		// "ffmpeg" is the default; a missing default may still be installed in
		// a well-known location. Any other explicit path must be honoured.
		if !strings.EqualFold(configured, name) {
			return "", fmt.Errorf("%s was not found (configured as %q)", name, configured)
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, candidate := range commonLocations(name) {
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s was not found on PATH; install FFmpeg or set %sPath in ffbatch.json", name, name)
}

func commonLocations(name string) []string {
	binary := name + exeSuffix()
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, "scoop", "shims", binary),
		filepath.Join(home, "scoop", "apps", name, "current", "bin", binary),
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		candidates = append(candidates,
			filepath.Join(local, "Microsoft", "WinGet", "Links", binary),
			filepath.Join(local, "Programs", name, "bin", binary),
		)
	}
	if programData := os.Getenv("ProgramData"); programData != "" {
		candidates = append(candidates, filepath.Join(programData, "chocolatey", "bin", binary))
	}
	if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
		candidates = append(candidates,
			filepath.Join(programFiles, name, "bin", binary),
			filepath.Join(programFiles, "ffmpeg", "bin", binary),
		)
	}
	candidates = append(candidates,
		filepath.Join(`C:\`, "ffmpeg", "bin", binary),
		filepath.Join(`C:\`, "tools", "ffmpeg", "bin", binary),
	)
	if runtime.GOOS != "windows" {
		candidates = append(candidates,
			filepath.Join("/usr", "bin", binary),
			filepath.Join("/usr", "local", "bin", binary),
			filepath.Join("/opt", "homebrew", "bin", binary),
		)
	}
	return candidates
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// DetectVersion runs `ffmpeg -version` and records the first line plus the
// build configuration date when available.
func (t *Tool) DetectVersion(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, t.FFmpeg, "-version").Output()
	if err != nil {
		return
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		t.Version = strings.TrimSpace(lines[0])
	}
}

// Encoders returns the set of encoder names advertised by the build.
func (t Tool) Encoders(ctx context.Context) (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, t.FFmpeg, "-hide_banner", "-encoders").Output()
	if err != nil {
		return nil, err
	}
	encoders := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "V") && !strings.HasPrefix(fields[0], "A") && !strings.HasPrefix(fields[0], "S") {
			continue
		}
		encoders[fields[1]] = true
	}
	return encoders, nil
}

// Health describes whether the toolchain can actually run.
type Health struct {
	OK      bool   `json:"ok"`
	FFmpeg  string `json:"ffmpeg"`
	FFprobe string `json:"ffprobe"`
	Version string `json:"version"`
	Error   string `json:"error,omitempty"`
}

// Check verifies both executables by running them.
func (t Tool) Check(ctx context.Context) Health {
	health := Health{FFmpeg: t.FFmpeg, FFprobe: t.FFprobe}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, t.FFmpeg, "-version").Output(); err != nil {
		health.Error = "ffmpeg failed: " + firstLine(err.Error())
		return health
	} else if lines := strings.Split(string(out), "\n"); len(lines) > 0 {
		health.Version = strings.TrimSpace(lines[0])
	}
	if _, err := exec.CommandContext(ctx, t.FFprobe, "-version").Output(); err != nil {
		health.Error = "ffprobe failed: " + firstLine(err.Error())
		return health
	}
	health.OK = true
	return health
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
