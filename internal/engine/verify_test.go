package engine

import (
	"strings"
	"testing"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
)

func verifyFixture() (*mediainfo.Info, *mediainfo.Info) {
	source := &mediainfo.Info{
		DurationMs: 10_000,
		Video:      &mediainfo.Stream{Type: "video", Codec: "h264", Width: 1920, Height: 1080},
		Audios: []mediainfo.Stream{
			{Type: "audio", Codec: "aac", Language: "ara"},
			{Type: "audio", Codec: "aac", Language: "eng"},
		},
	}
	output := &mediainfo.Info{
		DurationMs: 10_000,
		Video:      &mediainfo.Stream{Type: "video", Codec: "h264", Width: 1920, Height: 1080},
		Audios: []mediainfo.Stream{
			{Type: "audio", Codec: "aac", Language: "ara"},
			{Type: "audio", Codec: "aac", Language: "eng"},
		},
	}
	return source, output
}

func TestValidateOutputPasses(t *testing.T) {
	source, output := verifyFixture()
	cfg := config.Default()
	if err := validateOutput(cfg, source, output); err != nil {
		t.Fatal(err)
	}
}

func TestValidateOutputRejectsTruncation(t *testing.T) {
	source, output := verifyFixture()
	output.DurationMs = 3_000
	if err := validateOutput(config.Default(), source, output); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("error = %v, want truncated", err)
	}
}

func TestValidateOutputRejectsResolutionChange(t *testing.T) {
	source, output := verifyFixture()
	output.Video.Width = 1280
	output.Video.Height = 720
	if err := validateOutput(config.Default(), source, output); err == nil || !strings.Contains(err.Error(), "resolution") {
		t.Fatalf("error = %v, want resolution", err)
	}
}

func TestValidateOutputRejectsAudioLoss(t *testing.T) {
	source, output := verifyFixture()
	output.Audios = output.Audios[:1]
	cfg := config.Default()
	if err := validateOutput(cfg, source, output); err == nil || !strings.Contains(err.Error(), "audio track") {
		t.Fatalf("error = %v, want audio track", err)
	}
}

func TestValidateOutputHonoursAudioPolicy(t *testing.T) {
	source, output := verifyFixture()
	output.Audios = output.Audios[:1]
	cfg := config.Default()
	cfg.AudioTracks = "first"
	if err := validateOutput(cfg, source, output); err != nil {
		t.Fatal(err)
	}
	cfg.AudioCodec = "none"
	output.Audios = nil
	if err := validateOutput(cfg, source, output); err != nil {
		t.Fatal(err)
	}
}

func TestValidateOutputEncodeCodec(t *testing.T) {
	source, output := verifyFixture()
	cfg := config.Default()
	cfg.Mode = config.ModeEncode
	cfg.VideoCodec = "h265"
	output.Video.Codec = "hevc"
	if err := validateOutput(cfg, source, output); err != nil {
		t.Fatal(err)
	}
	output.Video.Codec = "h264"
	if err := validateOutput(cfg, source, output); err == nil {
		t.Fatal("expected a codec mismatch")
	}
}

func TestValidateOutputTrimDuration(t *testing.T) {
	source, output := verifyFixture()
	cfg := config.Default()
	cfg.TrimStart = "00:00:01"
	cfg.TrimEnd = "00:00:03"
	output.DurationMs = 2_000
	if err := validateOutput(cfg, source, output); err != nil {
		t.Fatal(err)
	}
	output.DurationMs = 500
	if err := validateOutput(cfg, source, output); err == nil {
		t.Fatal("expected a truncated trim")
	}
}

func TestValidateOutputNoVideo(t *testing.T) {
	source, output := verifyFixture()
	output.Video = nil
	if err := validateOutput(config.Default(), source, output); err == nil {
		t.Fatal("expected no-video error")
	}
}
