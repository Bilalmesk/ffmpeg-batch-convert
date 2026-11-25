// Package config defines the batch-conversion settings, their defaults,
// validation rules and on-disk representation.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
)

// FileName is the conventional project-local configuration file.
const FileName = "ffbatch.json"

// Mode selects what the batch runner does with each input file.
type Mode string

const (
	// ModeRemux rewrites the container without touching the codecs.
	ModeRemux Mode = "remux"
	// ModeEncode decodes and re-encodes the video stream.
	ModeEncode Mode = "encode"
	// ModeRename changes the file extension only.
	ModeRename Mode = "rename"
)

// OverwritePolicy controls what happens when an output file already exists.
type OverwritePolicy string

const (
	OverwriteSkip      OverwritePolicy = "skip"
	OverwriteReplace   OverwritePolicy = "overwrite"
	OverwriteNumbering OverwritePolicy = "number"
	OverwriteFail      OverwritePolicy = "fail"
)

// SupportedFormats are the output containers the runner can target.
var SupportedFormats = []string{"mp4", "mkv", "mov", "ts"}

// SupportedPresets mirrors the x264/x265 preset names.
var SupportedPresets = []string{
	"ultrafast", "superfast", "veryfast", "faster", "fast",
	"medium", "slow", "slower", "veryslow",
}

var supportedModes = map[Mode]bool{ModeRemux: true, ModeEncode: true, ModeRename: true}

var supportedOverwrite = map[OverwritePolicy]bool{
	OverwriteSkip: true, OverwriteReplace: true, OverwriteNumbering: true, OverwriteFail: true,
}

// DefaultExtensions is the set of source containers the legacy scripts handled,
// plus a few common siblings.
func DefaultExtensions() []string {
	return []string{"ts", "m2ts", "mkv", "mov", "avi", "wmv", "flv", "webm", "m4v", "mpg", "mpeg"}
}

// Config is the complete, JSON-serialisable settings document. It is kept flat
// so that partial config files and CLI overrides merge predictably.
type Config struct {
	// Tool discovery.
	FFmpegPath  string `json:"ffmpegPath"`
	FFprobePath string `json:"ffprobePath"`

	// Input selection.
	Mode       Mode     `json:"mode"`
	Recursive  bool     `json:"recursive"`
	Extensions []string `json:"extensions"`

	// Output selection.
	OutputDir    string          `json:"outputDir"`
	Format       string          `json:"format"`
	Overwrite    OverwritePolicy `json:"overwrite"`
	DeleteSource bool            `json:"deleteSource"`
	ArchiveDir   string          `json:"archiveDir"`
	DryRun       bool            `json:"dryRun"`
	Workers      int             `json:"workers"`

	// Verification and chain-of-custody.
	VerifyOutput bool `json:"verifyOutput"`
	HashFiles    bool `json:"hashFiles"`

	// Container / stream options.
	Faststart     bool   `json:"faststart"`
	Aspect        string `json:"aspect"`
	GenPTS        bool   `json:"genPts"`
	VideoCodec    string `json:"videoCodec"`
	CRF           int    `json:"crf"`
	Preset        string `json:"preset"`
	AudioCodec    string `json:"audioCodec"`
	AudioRate     string `json:"audioBitrate"`
	AudioTracks   string `json:"audioTracks"`
	AudioLanguage string `json:"audioLanguage"`
	Subtitle      string `json:"subtitleCodec"`
	TrimStart     string `json:"trimStart"`
	TrimEnd       string `json:"trimEnd"`

	// Diagnostics.
	LogFile string `json:"logFile"`
}

// Default returns settings that reproduce the safe behaviour of the original
// scripts: stream copy into MP4, skip existing outputs, never delete sources.
func Default() Config {
	return Config{
		FFmpegPath:   "ffmpeg",
		FFprobePath:  "ffprobe",
		Mode:         ModeRemux,
		Recursive:    false,
		Extensions:   DefaultExtensions(),
		Format:       "mp4",
		Overwrite:    OverwriteSkip,
		Workers:      defaultWorkers(),
		VerifyOutput: true,
		Faststart:    true,
		Aspect:       "",
		GenPTS:       true,
		VideoCodec:   "h264",
		CRF:          22,
		Preset:       "fast",
		AudioCodec:   "copy",
		AudioRate:    "192k",
		AudioTracks:  "all",
		Subtitle:     "auto",
	}
}

func defaultWorkers() int {
	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}
	if n > 4 {
		n = 4
	}
	return n
}

// Normalize lower-cases enumerations and cleans user-supplied lists.
func (c *Config) Normalize() {
	c.Mode = Mode(strings.ToLower(strings.TrimSpace(string(c.Mode))))
	c.Format = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(c.Format)), ".")
	c.Overwrite = OverwritePolicy(strings.ToLower(strings.TrimSpace(string(c.Overwrite))))
	c.VideoCodec = strings.ToLower(strings.TrimSpace(c.VideoCodec))
	c.Preset = strings.ToLower(strings.TrimSpace(c.Preset))
	c.AudioCodec = strings.ToLower(strings.TrimSpace(c.AudioCodec))
	c.AudioRate = strings.TrimSpace(c.AudioRate)
	c.AudioTracks = strings.ToLower(strings.TrimSpace(c.AudioTracks))
	c.AudioLanguage = strings.ToLower(strings.TrimSpace(c.AudioLanguage))
	c.Subtitle = strings.ToLower(strings.TrimSpace(c.Subtitle))
	c.Aspect = strings.TrimSpace(c.Aspect)
	c.TrimStart = strings.TrimSpace(c.TrimStart)
	c.TrimEnd = strings.TrimSpace(c.TrimEnd)
	c.OutputDir = strings.TrimSpace(c.OutputDir)
	c.ArchiveDir = strings.TrimSpace(c.ArchiveDir)
	c.LogFile = strings.TrimSpace(c.LogFile)
	c.FFmpegPath = strings.TrimSpace(c.FFmpegPath)
	c.FFprobePath = strings.TrimSpace(c.FFprobePath)

	if c.FFmpegPath == "" {
		c.FFmpegPath = "ffmpeg"
	}
	if c.FFprobePath == "" {
		c.FFprobePath = "ffprobe"
	}
	if c.VideoCodec == "" {
		c.VideoCodec = "h264"
	}
	if c.Preset == "" {
		c.Preset = "fast"
	}
	if c.AudioCodec == "" {
		c.AudioCodec = "copy"
	}
	if c.AudioRate == "" {
		c.AudioRate = "192k"
	}
	if c.AudioTracks == "" {
		c.AudioTracks = "all"
	}
	if c.Subtitle == "" {
		c.Subtitle = "auto"
	}
	if c.Workers <= 0 {
		c.Workers = defaultWorkers()
	}
	if c.Extensions == nil {
		c.Extensions = DefaultExtensions()
	}

	seen := make(map[string]bool, len(c.Extensions))
	exts := make([]string, 0, len(c.Extensions))
	for _, ext := range c.Extensions {
		ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
		ext = strings.TrimSpace(ext)
		if ext == "" || seen[ext] {
			continue
		}
		seen[ext] = true
		exts = append(exts, ext)
	}
	c.Extensions = exts
}

var audioRatePattern = regexp.MustCompile(`^[1-9][0-9]{1,4}k$`)
var aspectPattern = regexp.MustCompile(`^[1-9][0-9]{0,3}:[1-9][0-9]{0,3}$`)
var extPattern = regexp.MustCompile(`^[a-z0-9]{1,10}$`)
var audioLanguagePattern = regexp.MustCompile(`^[a-z]{2,3}(-[a-z]{2,4})?$`)

// Validate returns an error describing every invalid setting.
func (c Config) Validate() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if !supportedModes[c.Mode] {
		add("mode must be one of remux, encode or rename (got %q)", c.Mode)
	}
	if !contains(SupportedFormats, c.Format) {
		add("format must be one of %s (got %q)", strings.Join(SupportedFormats, ", "), c.Format)
	}
	if !supportedOverwrite[c.Overwrite] {
		add("overwrite must be one of skip, overwrite, number or fail (got %q)", c.Overwrite)
	}
	if c.Workers < 1 || c.Workers > 32 {
		add("workers must be between 1 and 32 (got %d)", c.Workers)
	}
	if len(c.Extensions) == 0 {
		add("extensions must contain at least one extension")
	}
	for _, ext := range c.Extensions {
		if !extPattern.MatchString(ext) {
			add("invalid extension %q (letters and digits only, e.g. \"mkv\")", ext)
		}
	}
	if c.Mode == ModeEncode {
		if c.VideoCodec != "h264" && c.VideoCodec != "h265" && c.VideoCodec != "hevc" {
			add("video codec must be h264 or h265 (got %q)", c.VideoCodec)
		}
		if c.CRF < 0 || c.CRF > 51 {
			add("crf must be between 0 and 51 (got %d)", c.CRF)
		}
		if !contains(SupportedPresets, c.Preset) {
			add("preset must be one of %s (got %q)", strings.Join(SupportedPresets, ", "), c.Preset)
		}
	}
	if !contains([]string{"copy", "aac", "opus", "none"}, c.AudioCodec) {
		add("audio codec must be copy, aac, opus or none (got %q)", c.AudioCodec)
	}
	if c.AudioCodec == "aac" || c.AudioCodec == "opus" {
		if !audioRatePattern.MatchString(c.AudioRate) {
			add("audio bitrate must look like 192k (got %q)", c.AudioRate)
		}
	}
	if !contains([]string{"all", "first", "preferred"}, c.AudioTracks) {
		add("audioTracks must be all, first or preferred (got %q)", c.AudioTracks)
	}
	if c.AudioTracks == "preferred" && !audioLanguagePattern.MatchString(c.AudioLanguage) {
		add("audioLanguage must be a 2-3 letter code such as ara or eng (got %q)", c.AudioLanguage)
	}
	trimStart, startErr := units.ParseTimecode(c.TrimStart)
	if startErr != nil {
		add("trimStart: %v", startErr)
	}
	trimEnd, endErr := units.ParseTimecode(c.TrimEnd)
	if endErr != nil {
		add("trimEnd: %v", endErr)
	}
	if c.TrimEnd != "" && trimEnd <= 0 {
		add("trimEnd must be greater than zero")
	}
	if c.TrimStart != "" && c.TrimEnd != "" && trimEnd <= trimStart {
		add("trimEnd must be later than trimStart")
	}
	if !contains([]string{"auto", "copy", "mov_text", "none"}, c.Subtitle) {
		add("subtitle codec must be auto, copy, mov_text or none (got %q)", c.Subtitle)
	}
	if c.Aspect != "" && !aspectPattern.MatchString(c.Aspect) {
		add("aspect must look like 16:9 (got %q)", c.Aspect)
	}
	if c.ArchiveDir != "" && c.DeleteSource {
		add("deleteSource and archiveDir are mutually exclusive; choose one")
	}
	if c.Mode == ModeRename && c.ArchiveDir != "" {
		add("archiveDir is not supported in rename mode")
	}
	if c.Mode == ModeRename && c.DeleteSource {
		// Renaming already moves the source; the flag has no additional effect.
		// Keep it permitted for config-file compatibility.
		_ = c.DeleteSource
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// ExtFor returns the normalised output extension (with leading dot).
func (c Config) ExtFor() string {
	return "." + strings.TrimPrefix(c.Format, ".")
}

// ResolvePath determines which configuration file should be used.
//
// Priority: explicit flag, FFBATCH_CONFIG, ./ffbatch.json, user config dir.
func ResolvePath(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	if env := os.Getenv("FFBATCH_CONFIG"); env != "" {
		return filepath.Abs(env)
	}
	if wd, err := os.Getwd(); err == nil {
		local := filepath.Join(wd, FileName)
		if _, err := os.Stat(local); err == nil {
			return local, nil
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "ffbatch", FileName), nil
	}
	return filepath.Abs(FileName)
}

// Load reads path over the built-in defaults. A missing file is not an error.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	// Windows editors love UTF-8 BOMs; encoding/json does not.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.Normalize()
	return cfg, nil
}

// Save writes the configuration atomically as indented JSON.
func Save(path string, cfg Config) error {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		// Windows cannot rename over an existing file.
		if removeErr := os.Remove(path); removeErr == nil {
			if retry := os.Rename(tmp, path); retry == nil {
				return nil
			}
		}
		_ = os.Remove(tmp)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

// Recipe is a one-click workflow shown in the web UI and documented in the CLI.
type Recipe struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Mode        Mode            `json:"mode"`
	Format      string          `json:"format"`
	VideoCodec  string          `json:"videoCodec"`
	CRF         int             `json:"crf"`
	Preset      string          `json:"preset"`
	AudioCodec  string          `json:"audioCodec"`
	Faststart   bool            `json:"faststart"`
	Aspect      string          `json:"aspect"`
	GenPTS      bool            `json:"genPts"`
	Overwrite   OverwritePolicy `json:"overwrite"`
}

// Recipes returns the built-in workflows that mirror the legacy batch files.
func Recipes() []Recipe {
	return []Recipe{
		{
			ID: "remux-mp4", Name: "Remux to MP4",
			Description: "Stream-copy video and audio into MP4 with faststart. The classic convert.bat.",
			Mode:        ModeRemux, Format: "mp4", Faststart: true, Aspect: "16:9", Overwrite: OverwriteSkip,
		},
		{
			ID: "ts-seeking-fix", Name: "TS seeking fix",
			Description: "Remux transport streams with regenerated timestamps and a faststart index.",
			Mode:        ModeRemux, Format: "mp4", Faststart: true, Aspect: "16:9", GenPTS: true, Overwrite: OverwriteSkip,
		},
		{
			ID: "encode-mp4", Name: "Re-encode to MP4",
			Description: "H.264 CRF 22 with the fast preset, mp4 faststart and the original audio.",
			Mode:        ModeEncode, Format: "mp4", VideoCodec: "h264", CRF: 22, Preset: "fast",
			AudioCodec: "copy", Faststart: true, Aspect: "16:9", Overwrite: OverwriteSkip,
		},
		{
			ID: "encode-hevc", Name: "Re-encode to HEVC",
			Description: "H.265 CRF 24 for smaller archives. Slower, and needs HEVC-capable players.",
			Mode:        ModeEncode, Format: "mp4", VideoCodec: "h265", CRF: 24, Preset: "medium",
			AudioCodec: "copy", Faststart: true, Overwrite: OverwriteSkip,
		},
		{
			ID: "rename-ts", Name: "MP4 to TS",
			Description: "Rename the container extension only - no re-muxing, exactly like mp4tots.bat.",
			Mode:        ModeRename, Format: "ts", Overwrite: OverwriteSkip,
		},
	}
}

// ApplyRecipe overlays a recipe on a copy of the configuration.
func (c Config) ApplyRecipe(id string) (Config, bool) {
	for _, r := range Recipes() {
		if r.ID != id {
			continue
		}
		c.Mode = r.Mode
		c.Format = r.Format
		c.Overwrite = r.Overwrite
		c.Faststart = r.Faststart
		c.Aspect = r.Aspect
		c.GenPTS = r.GenPTS
		if r.VideoCodec != "" {
			c.VideoCodec = r.VideoCodec
			c.CRF = r.CRF
			c.Preset = r.Preset
			c.AudioCodec = r.AudioCodec
		}
		return c, true
	}
	return c, false
}
