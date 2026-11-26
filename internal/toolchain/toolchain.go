// Package toolchain detects optional media tools that FFBatch can delegate to.
// The core converter still works with only FFmpeg; every other tool is optional.
package toolchain

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Tool is one detected external program.
type Tool struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	Path         string   `json:"path,omitempty"`
	Version      string   `json:"version,omitempty"`
	Available    bool     `json:"available"`
	Capabilities []string `json:"capabilities,omitempty"`
	Hint         string   `json:"hint,omitempty"`
}

// Registry is a snapshot of all known tools.
type Registry struct {
	Tools map[string]Tool `json:"tools"`
}

// Get returns a tool by ID.
func (r Registry) Get(id string) (Tool, bool) {
	tool, ok := r.Tools[id]
	return tool, ok
}

// Available reports whether every named tool was found.
func (r Registry) Available(ids ...string) bool {
	for _, id := range ids {
		tool, ok := r.Tools[id]
		if !ok || !tool.Available {
			return false
		}
	}
	return true
}

// Path returns the resolved path for a tool, or an empty string.
func (r Registry) Path(id string) string {
	return r.Tools[id].Path
}

// List returns tools sorted by category and name.
func (r Registry) List() []Tool {
	out := make([]Tool, 0, len(r.Tools))
	for _, tool := range r.Tools {
		out = append(out, tool)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

type spec struct {
	id           string
	name         string
	category     string
	binaries     []string
	versionArgs  []string
	hint         string
	capabilities []string
}

var specs = []spec{
	{id: "ffmpeg", name: "FFmpeg", category: "Core", binaries: []string{"ffmpeg"}, versionArgs: []string{"-version"},
		hint: "winget install Gyan.FFmpeg", capabilities: []string{"remux", "encode", "audio"}},
	{id: "ffprobe", name: "FFprobe", category: "Core", binaries: []string{"ffprobe"}, versionArgs: []string{"-version"},
		hint: "Ships with FFmpeg", capabilities: []string{"probe"}},
	{id: "mkvmerge", name: "MKVToolNix mkvmerge", category: "Muxing", binaries: []string{"mkvmerge"}, versionArgs: []string{"--version"},
		hint: "winget install MoritzBunkus.MKVToolNix", capabilities: []string{"mux", "chapters", "tags", "attachments"}},
	{id: "mkvextract", name: "MKVToolNix mkvextract", category: "Muxing", binaries: []string{"mkvextract"}, versionArgs: []string{"--version"},
		hint: "winget install MoritzBunkus.MKVToolNix", capabilities: []string{"extract"}},
	{id: "mp4box", name: "GPAC MP4Box", category: "Packaging", binaries: []string{"MP4Box", "mp4box"}, versionArgs: []string{"-version"},
		hint: "winget install GPAC.GPAC", capabilities: []string{"mux", "dash", "hls", "fragmented-mp4"}},
	{id: "mp4fragment", name: "Bento4 mp4fragment", category: "Packaging", binaries: []string{"mp4fragment"}, versionArgs: []string{"--version"},
		hint: "Download Bento4 from bento4.com", capabilities: []string{"fragmented-mp4"}},
	{id: "mp4dash", name: "Bento4 mp4dash", category: "Packaging", binaries: []string{"mp4dash"}, versionArgs: []string{"--version"},
		hint: "Download Bento4 from bento4.com", capabilities: []string{"dash", "hls"}},
	{id: "mediainfo", name: "MediaInfo", category: "Analysis", binaries: []string{"mediainfo"}, versionArgs: []string{"--Version"},
		hint: "winget install MediaArea.MediaInfo", capabilities: []string{"metadata"}},
	{id: "yt-dlp", name: "yt-dlp", category: "Ingest", binaries: []string{"yt-dlp"}, versionArgs: []string{"--version"},
		hint: "winget install yt-dlp.yt-dlp", capabilities: []string{"download", "hls", "dash"}},
	{id: "streamlink", name: "Streamlink", category: "Ingest", binaries: []string{"streamlink"}, versionArgs: []string{"--version"},
		hint: "pip install streamlink", capabilities: []string{"live"}},
	{id: "n-m3u8dl-re", name: "N_m3u8DL-RE", category: "Ingest", binaries: []string{"N_m3u8DL-RE", "N_m3u8DL-RE.exe"}, versionArgs: []string{"--version"},
		hint: "Download from github.com/nilaoda/N_m3u8DL-RE", capabilities: []string{"hls", "dash"}},
	{id: "aria2c", name: "aria2", category: "Ingest", binaries: []string{"aria2c"}, versionArgs: []string{"--version"},
		hint: "winget install aria2.aria2", capabilities: []string{"downloader"}},
	{id: "sox", name: "SoX", category: "Audio", binaries: []string{"sox"}, versionArgs: []string{"--version"},
		hint: "winget install SoX.SoX", capabilities: []string{"audio-effects"}},
	{id: "opusenc", name: "opusenc", category: "Audio", binaries: []string{"opusenc"}, versionArgs: []string{"--version"},
		hint: "Install opus-tools", capabilities: []string{"opus"}},
	{id: "flac", name: "flac", category: "Audio", binaries: []string{"flac"}, versionArgs: []string{"--version"},
		hint: "winget install Xiph.FLAC", capabilities: []string{"flac"}},
	{id: "lame", name: "LAME", category: "Audio", binaries: []string{"lame"}, versionArgs: []string{"--version"},
		hint: "winget install LAME.LAME", capabilities: []string{"mp3"}},
	{id: "ab-av1", name: "ab-av1", category: "Quality", binaries: []string{"ab-av1"}, versionArgs: []string{"--version"},
		hint: "cargo install ab-av1", capabilities: []string{"crf-search", "vmaf"}},
	{id: "av1an", name: "Av1an", category: "Encoding", binaries: []string{"av1an"}, versionArgs: []string{"--version"},
		hint: "cargo install av1an", capabilities: []string{"chunked-encode", "scene-detection"}},
	{id: "vspipe", name: "VapourSynth vspipe", category: "Encoding", binaries: []string{"vspipe"}, versionArgs: []string{"--version"},
		hint: "winget install VapourSynth.VapourSynth", capabilities: []string{"frames", "restoration"}},
	{id: "x264", name: "x264", category: "Encoders", binaries: []string{"x264"}, versionArgs: []string{"--version"},
		hint: "Optional standalone encoder", capabilities: []string{"h264"}},
	{id: "x265", name: "x265", category: "Encoders", binaries: []string{"x265"}, versionArgs: []string{"--version"},
		hint: "Optional standalone encoder", capabilities: []string{"h265"}},
	{id: "rav1e", name: "rav1e", category: "Encoders", binaries: []string{"rav1e"}, versionArgs: []string{"--version"},
		hint: "cargo install rav1e", capabilities: []string{"av1"}},
	{id: "svt-av1", name: "SVT-AV1", category: "Encoders", binaries: []string{"SvtAv1EncApp"}, versionArgs: []string{"--version"},
		hint: "FFmpeg's libsvtav1 is usually enough", capabilities: []string{"av1"}},
	{id: "handbrake", name: "HandBrakeCLI", category: "Encoders", binaries: []string{"HandBrakeCLI"}, versionArgs: []string{"--version"},
		hint: "winget install HandBrake.HandBrake", capabilities: []string{"presets", "encode"}},
	{id: "gstreamer", name: "GStreamer", category: "Frameworks", binaries: []string{"gst-launch-1.0"}, versionArgs: []string{"--version"},
		hint: "winget install GStreamer.GStreamer", capabilities: []string{"pipelines", "live"}},
	{id: "melt", name: "MLT melt", category: "Frameworks", binaries: []string{"melt"}, versionArgs: []string{"--version"},
		hint: "Install MLT / Shotcut", capabilities: []string{"editing", "compositing"}},
}

// Detect resolves every known tool. Missing tools are reported, not treated as
// errors. overrides maps tool IDs to explicit executable paths.
func Detect(ctx context.Context, overrides map[string]string) Registry {
	registry := Registry{Tools: make(map[string]Tool, len(specs))}
	for _, item := range specs {
		registry.Tools[item.id] = build(ctx, item, overrides[item.id])
	}
	return registry
}

// Resolve detects a single tool and reports whether its ID is known.
func Resolve(ctx context.Context, id string, overrides map[string]string) (Tool, bool) {
	for _, item := range specs {
		if item.id == id {
			return build(ctx, item, overrides[item.id]), true
		}
	}
	return Tool{}, false
}

func build(ctx context.Context, item spec, override string) Tool {
	tool := Tool{
		ID:           item.id,
		Name:         item.name,
		Category:     item.category,
		Available:    false,
		Hint:         item.hint,
		Capabilities: append([]string(nil), item.capabilities...),
	}
	if path := resolve(item, override); path != "" {
		tool.Available = true
		tool.Path = path
		tool.Version = version(ctx, path, item.versionArgs)
		if item.id == "ffmpeg" {
			tool.Capabilities = ffmpegCapabilities(ctx, path, tool.Capabilities)
		}
	}
	return tool
}

func resolve(item spec, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if info, err := os.Stat(configured); err == nil {
			if !info.IsDir() {
				return configured
			}
			for _, binary := range item.binaries {
				candidate := filepath.Join(configured, binary+exeSuffix())
				if fileExists(candidate) {
					return candidate
				}
			}
			return ""
		}
		if path, err := exec.LookPath(configured); err == nil {
			return path
		}
		return ""
	}
	for _, binary := range item.binaries {
		if path, err := exec.LookPath(binary); err == nil {
			return path
		}
	}
	for _, candidate := range commonLocations(item) {
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func commonLocations(item spec) []string {
	var out []string
	home, _ := os.UserHomeDir()
	for _, binary := range item.binaries {
		name := binary + exeSuffix()
		out = append(out,
			filepath.Join(home, "scoop", "shims", name),
			filepath.Join(home, ".cargo", "bin", name),
			filepath.Join(home, "go", "bin", name),
		)
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			out = append(out,
				filepath.Join(local, "Microsoft", "WinGet", "Links", name),
				filepath.Join(local, "Programs", "Python", "Python313", "Scripts", name),
				filepath.Join(local, "Programs", "Python", "Python312", "Scripts", name),
				filepath.Join(local, "Programs", "Python", "Python311", "Scripts", name),
			)
		}
		if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
			for _, dir := range []string{
				"MKVToolNix", "GPAC", "Bento4", "MediaInfo", "HandBrake", "VapourSynth", "GStreamer", "MLT",
			} {
				out = append(out,
					filepath.Join(programFiles, dir, name),
					filepath.Join(programFiles, dir, "bin", name),
				)
			}
		}
		if programFilesX86 := os.Getenv("ProgramFiles(x86)"); programFilesX86 != "" {
			out = append(out, filepath.Join(programFilesX86, "MKVToolNix", name))
		}
		if runtime.GOOS != "windows" {
			out = append(out,
				filepath.Join("/usr", "bin", name),
				filepath.Join("/usr", "local", "bin", name),
				filepath.Join("/opt", "homebrew", "bin", name),
			)
		}
	}
	return out
}

func version(ctx context.Context, path string, args []string) string {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return "installed"
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line != "" {
			return line
		}
	}
	return "installed"
}

func ffmpegCapabilities(ctx context.Context, path string, base []string) []string {
	caps := append([]string(nil), base...)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, path, "-hide_banner", "-encoders").Output(); err == nil {
		text := string(out)
		if strings.Contains(text, "libsvtav1") || strings.Contains(text, "libaom-av1") || strings.Contains(text, "librav1e") {
			caps = append(caps, "av1")
		}
		if strings.Contains(text, "libx264") {
			caps = append(caps, "h264")
		}
		if strings.Contains(text, "libx265") {
			caps = append(caps, "h265")
		}
		if strings.Contains(text, "libopus") {
			caps = append(caps, "opus")
		}
		if strings.Contains(text, " flac ") {
			caps = append(caps, "flac")
		}
	}
	if out, err := exec.CommandContext(ctx, path, "-hide_banner", "-filters").Output(); err == nil {
		text := string(out)
		for _, name := range []string{"libvmaf", "ssim", "psnr"} {
			if strings.Contains(text, " "+name+" ") {
				caps = append(caps, name)
			}
		}
	}
	return dedupe(caps)
}

func dedupe(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
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
