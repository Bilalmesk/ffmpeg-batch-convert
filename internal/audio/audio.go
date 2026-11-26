// Package audio builds audio-only extraction and conversion commands.
package audio

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Format is an audio output format.
type Format string

const (
	FormatFLAC Format = "flac"
	FormatOpus Format = "opus"
	FormatMP3  Format = "mp3"
	FormatM4A  Format = "m4a"
	FormatWAV  Format = "wav"
)

// Formats lists the supported audio outputs.
var Formats = []string{"flac", "opus", "mp3", "m4a", "wav"}

// Options configures one extraction.
type Options struct {
	Format    Format
	Bitrate   string
	Track     int
	Normalize bool
}

// Ext returns the output extension, including the dot.
func Ext(format Format) string {
	return "." + string(format)
}

// Build returns the FFmpeg argv for one audio extraction.
func Build(ffmpeg, input, output string, opts Options) ([]string, error) {
	if strings.TrimSpace(input) == "" {
		return nil, fmt.Errorf("input is required")
	}
	if strings.TrimSpace(output) == "" {
		return nil, fmt.Errorf("output is required")
	}
	if opts.Track < 0 {
		return nil, fmt.Errorf("audio track must be zero or greater")
	}
	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-loglevel", "error",
		"-i", input,
		"-map", fmt.Sprintf("0:a:%d?", opts.Track),
		"-vn", "-sn", "-dn",
	}
	switch opts.Format {
	case FormatFLAC:
		args = append(args, "-c:a", "flac")
	case FormatOpus:
		args = append(args, "-c:a", "libopus", "-b:a", bitrate(opts.Bitrate, "128k"))
	case FormatMP3:
		args = append(args, "-c:a", "libmp3lame", "-b:a", bitrate(opts.Bitrate, "192k"))
	case FormatM4A:
		args = append(args, "-c:a", "aac", "-b:a", bitrate(opts.Bitrate, "192k"))
	case FormatWAV:
		args = append(args, "-c:a", "pcm_s16le")
	default:
		return nil, fmt.Errorf("format must be one of %s", strings.Join(Formats, ", "))
	}
	if opts.Normalize {
		args = append(args, "-af", "loudnorm=I=-16:TP=-1.5:LRA=11")
	}
	args = append(args, output)
	return args, nil
}

// OutputPath returns the default output path for an input.
func OutputPath(input, outputDir string, format Format) string {
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	if outputDir == "" {
		outputDir = filepath.Dir(input)
	}
	return filepath.Join(outputDir, base+Ext(format))
}

func bitrate(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if _, err := strconv.Atoi(strings.TrimSuffix(strings.ToLower(value), "k")); err != nil {
		return fallback
	}
	if !strings.HasSuffix(strings.ToLower(value), "k") {
		value += "k"
	}
	return value
}

// ParseFormat validates a CLI format name.
func ParseFormat(value string) (Format, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "flac":
		return FormatFLAC, true
	case "opus":
		return FormatOpus, true
	case "mp3":
		return FormatMP3, true
	case "m4a", "aac":
		return FormatM4A, true
	case "wav":
		return FormatWAV, true
	default:
		return "", false
	}
}
