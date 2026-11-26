package engine

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
)

// VerifyOutput validates a temporary output before it is committed. Workflow
// steps use this to get the same protection as the batch engine.
func VerifyOutput(ctx context.Context, tool ffmpeg.Tool, cfg config.Config, source *mediainfo.Info, temp string) error {
	return verifyOutput(ctx, tool, cfg, source, temp)
}

// verifyOutput probes the temporary output and rejects truncated or
// structurally incompatible files before they can replace a real output.
func verifyOutput(ctx context.Context, tool ffmpeg.Tool, cfg config.Config, source *mediainfo.Info, temp string) error {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	output, err := mediainfo.Probe(probeCtx, tool.FFprobe, temp)
	if err != nil {
		return fmt.Errorf("probe output: %w", err)
	}
	return validateOutput(cfg, source, output)
}

func validateOutput(cfg config.Config, source, output *mediainfo.Info) error {
	if output == nil || output.Video == nil {
		return fmt.Errorf("output has no video stream")
	}
	if source != nil && source.Video != nil {
		if output.Video.Width != source.Video.Width || output.Video.Height != source.Video.Height {
			return fmt.Errorf("resolution changed from %dx%d to %dx%d",
				source.Video.Width, source.Video.Height, output.Video.Width, output.Video.Height)
		}
		switch cfg.Mode {
		case config.ModeRemux:
			if !sameCodec(output.Video.Codec, source.Video.Codec) {
				return fmt.Errorf("video codec changed from %s to %s", source.Video.Codec, output.Video.Codec)
			}
		case config.ModeEncode:
			expected := cfg.VideoCodec
			switch expected {
			case "h265":
				expected = "hevc"
			case "av1":
				expected = "av1"
			}
			if !sameCodec(output.Video.Codec, expected) {
				return fmt.Errorf("expected %s video, got %s", expected, output.Video.Codec)
			}
		}
	}

	if expected := expectedDurationMs(cfg, source); expected > 0 {
		if output.DurationMs <= 0 {
			return fmt.Errorf("output duration is unknown")
		}
		// Allow a small container/encoder rounding window, but cap it so a
		// badly truncated long recording is still rejected.
		tolerance := int64(math.Max(750, math.Min(10_000, float64(expected)*0.02)))
		if output.DurationMs+tolerance < expected {
			return fmt.Errorf("output is truncated: %s vs expected %s",
				units.Clock(output.DurationMs), units.Clock(expected))
		}
		// Untrimmed conversions should stay close to the source duration.
		if cfg.TrimStart == "" && cfg.TrimEnd == "" {
			if delta := output.DurationMs - expected; delta > tolerance || delta < -tolerance {
				return fmt.Errorf("duration mismatch: %s vs expected %s",
					units.Clock(output.DurationMs), units.Clock(expected))
			}
		}
	}

	expectedAudio := 0
	if cfg.AudioCodec != "none" {
		selected, _ := ffmpeg.SelectedAudioStreams(cfg, source)
		expectedAudio = len(selected)
	}
	if len(output.Audios) != expectedAudio {
		return fmt.Errorf("audio track count is %d, expected %d", len(output.Audios), expectedAudio)
	}
	return nil
}

func expectedDurationMs(cfg config.Config, source *mediainfo.Info) int64 {
	if source == nil || source.DurationMs <= 0 {
		return 0
	}
	start, _ := units.ParseTimecode(cfg.TrimStart)
	end, _ := units.ParseTimecode(cfg.TrimEnd)
	if cfg.TrimEnd != "" {
		if end > start {
			return (end - start).Milliseconds()
		}
		return 0
	}
	if start > 0 {
		expected := source.DurationMs - start.Milliseconds()
		if expected > 0 {
			return expected
		}
		return 0
	}
	return source.DurationMs
}

func sameCodec(a, b string) bool {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if a == "h265" {
		a = "hevc"
	}
	if b == "h265" {
		b = "hevc"
	}
	return a == b
}
