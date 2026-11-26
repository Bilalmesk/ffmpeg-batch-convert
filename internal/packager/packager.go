// Package packager builds HLS/DASH packaging plans. FFmpeg is the default
// backend; MP4Box and Bento4 are optional advanced backends.
package packager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Format is the streaming package format.
type Format string

const (
	FormatHLS  Format = "hls"
	FormatDASH Format = "dash"
)

// Tool selects the packaging backend.
type Tool string

const (
	ToolFFmpeg Tool = "ffmpeg"
	ToolMP4Box Tool = "mp4box"
	ToolBento4 Tool = "bento4"
)

// Command is one external process in a packaging plan.
type Command struct {
	Binary string
	Args   []string
}

// Request describes a packaging job.
type Request struct {
	Input          string
	OutputDir      string
	Format         Format
	SegmentSeconds int
	Tool           Tool
	FFmpeg         string
	MP4Box         string
	MP4Fragment    string
	MP4Dash        string
}

// Plan is an ordered list of commands plus expected outputs.
type Plan struct {
	Commands []Command
	Manifest string
	Notes    []string
}

// Build validates a request and returns a command plan.
func Build(req Request) (Plan, error) {
	if strings.TrimSpace(req.Input) == "" {
		return Plan{}, fmt.Errorf("input is required")
	}
	info, err := os.Stat(req.Input)
	if err != nil {
		return Plan{}, fmt.Errorf("input: %w", err)
	}
	if info.IsDir() {
		return Plan{}, fmt.Errorf("input must be a file")
	}
	if strings.TrimSpace(req.OutputDir) == "" {
		return Plan{}, fmt.Errorf("output directory is required")
	}
	if req.SegmentSeconds == 0 {
		req.SegmentSeconds = 6
	}
	if req.SegmentSeconds < 1 || req.SegmentSeconds > 60 {
		return Plan{}, fmt.Errorf("segment seconds must be between 1 and 60")
	}
	switch req.Format {
	case FormatHLS, FormatDASH:
	case "":
		req.Format = FormatHLS
	default:
		return Plan{}, fmt.Errorf("format must be hls or dash")
	}
	if req.Tool == "" || req.Tool == "auto" {
		req.Tool = ToolFFmpeg
	}

	outputDir, err := filepath.Abs(req.OutputDir)
	if err != nil {
		return Plan{}, err
	}
	input, err := filepath.Abs(req.Input)
	if err != nil {
		return Plan{}, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Plan{}, fmt.Errorf("create output directory: %w", err)
	}

	switch req.Tool {
	case ToolFFmpeg:
		return buildFFmpeg(input, outputDir, req)
	case ToolMP4Box:
		return buildMP4Box(input, outputDir, req)
	case ToolBento4:
		return buildBento4(input, outputDir, req)
	default:
		return Plan{}, fmt.Errorf("tool must be ffmpeg, mp4box or bento4")
	}
}

func buildFFmpeg(input, outputDir string, req Request) (Plan, error) {
	ffmpeg := req.FFmpeg
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-i", input,
		"-map", "0:v:0", "-map", "0:a?",
		"-c", "copy",
	}
	plan := Plan{}
	switch req.Format {
	case FormatHLS:
		plan.Manifest = filepath.Join(outputDir, "index.m3u8")
		args = append(args,
			"-f", "hls",
			"-hls_time", fmt.Sprintf("%d", req.SegmentSeconds),
			"-hls_playlist_type", "vod",
			"-hls_flags", "independent_segments",
			"-hls_segment_filename", filepath.Join(outputDir, "segment_%05d.ts"),
			plan.Manifest,
		)
	case FormatDASH:
		plan.Manifest = filepath.Join(outputDir, "manifest.mpd")
		args = append(args,
			"-f", "dash",
			"-seg_duration", fmt.Sprintf("%d", req.SegmentSeconds),
			"-use_template", "1",
			"-use_timeline", "1",
			"-init_seg_name", filepath.Join(outputDir, "init-stream$RepresentationID$.m4s"),
			"-media_seg_name", filepath.Join(outputDir, "chunk-stream$RepresentationID$-$Number%05d$.m4s"),
			"-adaptation_sets", "id=0,streams=v id=1,streams=a",
			plan.Manifest,
		)
	}
	plan.Commands = []Command{{Binary: ffmpeg, Args: args}}
	plan.Notes = append(plan.Notes, "FFmpeg stream-copy packaging; re-encode first if the source codecs are not HLS/DASH-friendly")
	return plan, nil
}

func buildMP4Box(input, outputDir string, req Request) (Plan, error) {
	mp4box := req.MP4Box
	if mp4box == "" {
		return Plan{}, fmt.Errorf("MP4Box path is required")
	}
	manifest := filepath.Join(outputDir, "manifest.mpd")
	if req.Format == FormatHLS {
		manifest = filepath.Join(outputDir, "master.m3u8")
	}
	ms := fmt.Sprintf("%d", req.SegmentSeconds*1000)
	args := []string{
		"-dash", ms,
		"-frag", ms,
		"-rap",
		"-profile", "live",
		"-out", manifest,
		input,
	}
	return Plan{
		Commands: []Command{{Binary: mp4box, Args: args}},
		Manifest: manifest,
		Notes:    []string{"MP4Box packaging is optional; verify the manifest in your player"},
	}, nil
}

func buildBento4(input, outputDir string, req Request) (Plan, error) {
	if req.MP4Fragment == "" || req.MP4Dash == "" {
		return Plan{}, fmt.Errorf("mp4fragment and mp4dash paths are required")
	}
	fragmented := filepath.Join(outputDir, "fragmented.mp4")
	plan := Plan{
		Commands: []Command{{Binary: req.MP4Fragment, Args: []string{input, fragmented}}},
		Manifest: filepath.Join(outputDir, "stream.mpd"),
		Notes:    []string{"Bento4 packaging is optional; mp4dash writes the final manifests"},
	}
	dashArgs := []string{"--output-dir", outputDir}
	if req.Format == FormatHLS {
		dashArgs = append(dashArgs, "--hls")
		plan.Manifest = filepath.Join(outputDir, "master.m3u8")
	}
	dashArgs = append(dashArgs, fragmented)
	plan.Commands = append(plan.Commands, Command{Binary: req.MP4Dash, Args: dashArgs})
	return plan, nil
}

// CommandString renders a plan command for display.
func CommandString(command Command) string {
	parts := make([]string, 0, len(command.Args)+1)
	parts = append(parts, quote(command.Binary))
	for _, arg := range command.Args {
		parts = append(parts, quote(arg))
	}
	return strings.Join(parts, " ")
}

func quote(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\"") {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return value
}
