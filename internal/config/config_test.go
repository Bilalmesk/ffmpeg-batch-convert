package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	cfg := Default()
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should validate: %v", err)
	}
	if cfg.Workers < 1 {
		t.Fatalf("workers should default to at least 1, got %d", cfg.Workers)
	}
	if cfg.AudioTracks != "all" {
		t.Fatalf("audioTracks default = %q, want all", cfg.AudioTracks)
	}
	if !cfg.VerifyOutput {
		t.Fatal("verifyOutput should default to true")
	}
	if cfg.Format != "mp4" || cfg.Mode != ModeRemux {
		t.Fatalf("unexpected defaults: mode=%s format=%s", cfg.Mode, cfg.Format)
	}
	if cfg.AV1Encoder != "svt" {
		t.Fatalf("av1Encoder default = %q, want svt", cfg.AV1Encoder)
	}
}

func TestNormalize(t *testing.T) {
	cfg := Default()
	cfg.Mode = "  REMUX "
	cfg.Format = ".MP4"
	cfg.Overwrite = " SKIP "
	cfg.Extensions = []string{".MKV", "mkv", " TS ", ""}
	cfg.VideoCodec = "H264"
	cfg.Normalize()

	if cfg.Mode != ModeRemux || cfg.Format != "mp4" || cfg.Overwrite != OverwriteSkip {
		t.Fatalf("normalisation failed: %+v", cfg)
	}
	if got, want := strings.Join(cfg.Extensions, ","), "mkv,ts"; got != want {
		t.Fatalf("extensions = %q, want %q", got, want)
	}
	if cfg.VideoCodec != "h264" {
		t.Fatalf("video codec = %q", cfg.VideoCodec)
	}
	cfg.VideoCodec = "HEVC"
	cfg.Normalize()
	if cfg.VideoCodec != "h265" {
		t.Fatalf("hevc alias = %q, want h265", cfg.VideoCodec)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"mode", func(c *Config) { c.Mode = "stream" }, "mode must be"},
		{"format", func(c *Config) { c.Format = "avi" }, "format must be"},
		{"webm output", func(c *Config) { c.Format = "webm" }, "format must be"},
		{"workers", func(c *Config) { c.Workers = 99 }, "workers must be"},
		{"extension", func(c *Config) { c.Extensions = []string{"m kv"} }, "invalid extension"},
		{"overwrite", func(c *Config) { c.Overwrite = "merge" }, "overwrite must be"},
		{"crf", func(c *Config) { c.Mode = ModeEncode; c.CRF = 99 }, "crf must be"},
		{"av1 crf", func(c *Config) { c.Mode = ModeEncode; c.VideoCodec = "av1"; c.CRF = 64 }, "crf must be between 0 and 63"},
		{"av1 encoder", func(c *Config) { c.Mode = ModeEncode; c.VideoCodec = "av1"; c.AV1Encoder = "nvenc" }, "av1Encoder must be"},
		{"video codec", func(c *Config) { c.Mode = ModeEncode; c.VideoCodec = "vp9" }, "video codec must be"},
		{"preset", func(c *Config) { c.Mode = ModeEncode; c.Preset = "hyperfast" }, "preset must be"},
		{"audio", func(c *Config) { c.AudioCodec = "mp3" }, "audio codec must be"},
		{"bitrate", func(c *Config) { c.AudioCodec = "aac"; c.AudioRate = "192" }, "audio bitrate"},
		{"audio tracks", func(c *Config) { c.AudioTracks = "stereo" }, "audioTracks must be"},
		{"audio language", func(c *Config) { c.AudioTracks = "preferred"; c.AudioLanguage = "arabic" }, "audioLanguage"},
		{"trim start", func(c *Config) { c.TrimStart = "1:70" }, "trimStart"},
		{"trim end order", func(c *Config) { c.TrimStart = "10"; c.TrimEnd = "5" }, "trimEnd must be later"},
		{"archive and delete", func(c *Config) { c.ArchiveDir = "archive"; c.DeleteSource = true }, "mutually exclusive"},
		{"rename archive", func(c *Config) { c.Mode = ModeRename; c.Format = "ts"; c.ArchiveDir = "archive" }, "not supported in rename"},
		{"aspect", func(c *Config) { c.Aspect = "wide" }, "aspect must"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			test.mutate(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestLoadPartialKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ffbatch.json")
	if err := os.WriteFile(path, []byte(`{"mode":"encode","crf":18,"videoCodec":"h265","extensions":["mkv"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModeEncode || cfg.VideoCodec != "h265" || cfg.CRF != 18 {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
	if cfg.Preset != "fast" {
		t.Fatalf("preset should keep its default, got %q", cfg.Preset)
	}
	if cfg.AV1Encoder != "svt" {
		t.Fatalf("av1Encoder should keep its default, got %q", cfg.AV1Encoder)
	}
	if cfg.Workers < 1 {
		t.Fatalf("workers should keep its default, got %d", cfg.Workers)
	}
	if len(cfg.Extensions) != 1 || cfg.Extensions[0] != "mkv" {
		t.Fatalf("extensions = %v", cfg.Extensions)
	}
	if !cfg.VerifyOutput {
		t.Fatal("verifyOutput should default to true when the file omits it")
	}
	if cfg.HashFiles {
		t.Fatal("hashFiles should default to false")
	}
}

func TestLoadKeepsExplicitFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ffbatch.json")
	if err := os.WriteFile(path, []byte(`{"verifyOutput":false,"hashFiles":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.VerifyOutput {
		t.Fatal("explicit verifyOutput=false was ignored")
	}
	if !cfg.HashFiles {
		t.Fatal("explicit hashFiles=true was ignored")
	}
}

func TestLoadStripsUTF8BOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ffbatch.json")
	body := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"format":"mkv"}`)...)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Format != "mkv" {
		t.Fatalf("format = %q, want mkv", cfg.Format)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "ffbatch.json")
	cfg := Default()
	cfg.Mode = ModeEncode
	cfg.VideoCodec = "h265"
	cfg.CRF = 19
	cfg.LogFile = "run.log"

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	// Saving twice must replace the file on Windows too.
	if err := Save(path, cfg); err != nil {
		t.Fatalf("second save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mode != ModeEncode || loaded.VideoCodec != "h265" || loaded.CRF != 19 || loaded.LogFile != "run.log" {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults should be valid: %v", err)
	}
}

func TestRecipes(t *testing.T) {
	cfg := Default()
	cfg, ok := cfg.ApplyRecipe("ts-seeking-fix")
	if !ok {
		t.Fatal("recipe not found")
	}
	if cfg.Mode != ModeRemux || !cfg.GenPTS || !cfg.Faststart || cfg.Format != "mp4" {
		t.Fatalf("unexpected recipe result: %+v", cfg)
	}

	cfg, ok = cfg.ApplyRecipe("encode-hevc")
	if !ok || cfg.Mode != ModeEncode || cfg.VideoCodec != "h265" {
		t.Fatalf("unexpected hevc recipe: %+v ok=%v", cfg, ok)
	}

	cfg, ok = cfg.ApplyRecipe("encode-av1")
	if !ok || cfg.Mode != ModeEncode || cfg.VideoCodec != "av1" || cfg.CRF != 30 {
		t.Fatalf("unexpected av1 recipe: %+v ok=%v", cfg, ok)
	}

	if _, ok := cfg.ApplyRecipe("nope"); ok {
		t.Fatal("unknown recipe should not apply")
	}
}

func TestNormalizeTools(t *testing.T) {
	cfg := Default()
	cfg.Tools = map[string]string{" MKVMERGE ": " C:\\Tools\\mkvmerge.exe ", "": "x", "mp4box": ""}
	cfg.Normalize()
	if len(cfg.Tools) != 1 || cfg.Tools["mkvmerge"] != `C:\Tools\mkvmerge.exe` {
		t.Fatalf("tools = %+v", cfg.Tools)
	}
}

func TestResolvePathExplicit(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "custom.json")
	got, err := ResolvePath(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ResolvePath = %q, want %q", got, want)
	}
}
