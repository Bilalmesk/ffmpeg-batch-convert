// Package ytdlp builds and runs yt-dlp downloads for FFBatch ingest.
package ytdlp

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Request describes one download batch.
type Request struct {
	URLs         []string
	OutputDir    string
	Format       string
	AudioOnly    bool
	AudioFormat  string
	Cookies      string
	Downloader   string
	Playlist     bool
	WriteInfo    bool
	Archive      string
	ExtraArgs    []string
	RestrictName bool
}

// Args returns the yt-dlp argv without the executable.
func Args(req Request) ([]string, error) {
	if len(req.URLs) == 0 {
		return nil, fmt.Errorf("at least one URL is required")
	}
	if strings.TrimSpace(req.OutputDir) == "" {
		return nil, fmt.Errorf("output directory is required")
	}
	args := []string{
		"--newline",
		"--no-color",
		"--no-overwrites",
		"--paths", req.OutputDir,
		"-o", "%(title)s [%(id)s].%(ext)s",
	}
	if !req.Playlist {
		args = append(args, "--no-playlist")
	}
	if req.RestrictName {
		args = append(args, "--restrict-filenames")
	}
	if req.Format != "" {
		args = append(args, "-f", req.Format)
	}
	if req.AudioOnly {
		format := req.AudioFormat
		if format == "" {
			format = "flac"
		}
		args = append(args, "--extract-audio", "--audio-format", format, "--audio-quality", "0")
	}
	if req.Cookies != "" {
		args = append(args, "--cookies", req.Cookies)
	}
	if req.Downloader != "" {
		args = append(args, "--downloader", req.Downloader)
	}
	if req.WriteInfo {
		args = append(args, "--write-info-json")
	}
	if req.Archive != "" {
		args = append(args, "--download-archive", req.Archive)
	}
	args = append(args, req.ExtraArgs...)
	args = append(args, req.URLs...)
	return args, nil
}

// DefaultArchive returns the persistent download archive for a folder.
func DefaultArchive(outputDir string) string {
	return filepath.Join(outputDir, ".ffbatch-download-archive.txt")
}
