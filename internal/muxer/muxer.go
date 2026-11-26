// Package muxer builds MKVToolNix commands for track-preserving MKV muxing.
package muxer

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
)

// Option is one ID:value pair used by mkvmerge (languages, track names, ...).
type Option struct {
	ID    string
	Value string
}

// Request describes one mkvmerge invocation.
type Request struct {
	Input       string
	Output      string
	Title       string
	Chapters    string
	NoAudio     bool
	NoVideo     bool
	NoSubtitles bool
	NoChapters  bool
	Languages   []Option
	TrackNames  []Option
}

// Build returns the mkvmerge argv and a printable command.
func Build(mkvmerge string, req Request) ([]string, string, error) {
	if strings.TrimSpace(req.Input) == "" {
		return nil, "", fmt.Errorf("input is required")
	}
	if strings.TrimSpace(req.Output) == "" {
		return nil, "", fmt.Errorf("output is required")
	}
	if !strings.EqualFold(filepath.Ext(req.Output), ".mkv") {
		return nil, "", fmt.Errorf("mkvmerge output must end in .mkv (got %s)", filepath.Ext(req.Output))
	}
	if mkvmerge == "" {
		mkvmerge = "mkvmerge"
	}
	args := []string{"-o", req.Output}
	if req.Title != "" {
		args = append(args, "--title", req.Title)
	}
	if req.Chapters != "" {
		args = append(args, "--chapters", req.Chapters)
	}
	if req.NoAudio {
		args = append(args, "--no-audio")
	}
	if req.NoVideo {
		args = append(args, "--no-video")
	}
	if req.NoSubtitles {
		args = append(args, "--no-subtitles")
	}
	if req.NoChapters {
		args = append(args, "--no-chapters")
	}
	for _, option := range req.Languages {
		args = append(args, "--language", option.ID+":"+option.Value)
	}
	for _, option := range req.TrackNames {
		args = append(args, "--track-name", option.ID+":"+option.Value)
	}
	args = append(args, req.Input)
	return args, ffmpeg.CommandString(mkvmerge, args), nil
}

// ParseOptions turns "0:ara,1:eng" into ID/value pairs.
func ParseOptions(value string) ([]Option, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	out := make([]Option, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, val, ok := strings.Cut(part, ":")
		if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(val) == "" {
			return nil, fmt.Errorf("expected ID:value, got %q", part)
		}
		out = append(out, Option{ID: strings.TrimSpace(id), Value: strings.TrimSpace(val)})
	}
	return out, nil
}
