package ffmpeg

import (
	"strconv"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
)

// videoEncodeArgs builds the codec-specific part of an encode command. AV1 is
// mapped onto the three encoders FFmpeg commonly exposes, with sane speed
// equivalents for the existing preset names.
func videoEncodeArgs(cfg config.Config) ([]string, []string) {
	args := make([]string, 0, 10)
	var warnings []string

	switch cfg.VideoCodec {
	case "h265", "hevc":
		args = append(args, "-c:v", "libx265", "-preset", cfg.Preset,
			"-crf", strconv.Itoa(cfg.CRF), "-pix_fmt", "yuv420p")
		if isMP4(cfg.Format) {
			args = append(args, "-tag:v", "hvc1")
		}
		return args, warnings
	case "av1":
		switch cfg.AV1Encoder {
		case "aom":
			args = append(args, "-c:v", "libaom-av1",
				"-crf", strconv.Itoa(cfg.CRF),
				"-cpu-used", strconv.Itoa(aomCPUUsed(cfg.Preset)),
				"-row-mt", "1", "-pix_fmt", "yuv420p")
		case "rav1e":
			args = append(args, "-c:v", "librav1e",
				"-qp", strconv.Itoa(cfg.CRF),
				"-speed", strconv.Itoa(rav1eSpeed(cfg.Preset)),
				"-pix_fmt", "yuv420p")
		default:
			args = append(args, "-c:v", "libsvtav1",
				"-preset", strconv.Itoa(svtPreset(cfg.Preset)),
				"-crf", strconv.Itoa(cfg.CRF),
				"-pix_fmt", "yuv420p")
		}
		if isMP4(cfg.Format) {
			args = append(args, "-tag:v", "av01")
		}
		if cfg.Format == "ts" {
			warnings = append(warnings, "AV1 in MPEG-TS has very limited player support; MP4 or MKV is recommended")
		}
		return args, warnings
	default:
		args = append(args, "-c:v", "libx264", "-preset", cfg.Preset,
			"-crf", strconv.Itoa(cfg.CRF), "-pix_fmt", "yuv420p")
		return args, warnings
	}
}

func svtPreset(name string) int {
	switch name {
	case "ultrafast":
		return 13
	case "superfast":
		return 12
	case "veryfast":
		return 11
	case "faster":
		return 10
	case "fast":
		return 9
	case "slow":
		return 6
	case "slower":
		return 4
	case "veryslow":
		return 2
	default:
		return 8 // medium
	}
}

func aomCPUUsed(name string) int {
	switch name {
	case "ultrafast", "superfast":
		return 8
	case "veryfast":
		return 7
	case "faster":
		return 6
	case "fast":
		return 5
	case "slow":
		return 3
	case "slower":
		return 2
	case "veryslow":
		return 0
	default:
		return 4 // medium
	}
}

func rav1eSpeed(name string) int {
	switch name {
	case "ultrafast", "superfast":
		return 10
	case "veryfast":
		return 9
	case "faster":
		return 8
	case "fast":
		return 7
	case "slow":
		return 4
	case "slower":
		return 2
	case "veryslow":
		return 1
	default:
		return 6 // medium
	}
}
