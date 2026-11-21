package engine

import (
	"strings"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
)

func upperOr(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return strings.ToUpper(value)
}

func titleCase(value string) string {
	value = strings.ToLower(value)
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func verbFor(mode config.Mode) string {
	switch mode {
	case config.ModeEncode:
		return "re-encode"
	case config.ModeRename:
		return "rename"
	default:
		return "stream copy"
	}
}

// humanDuration is kept as a thin alias so call sites read clearly.
func humanDuration(d time.Duration) string { return units.Duration(d) }

// humanBytes is kept as a thin alias so call sites read clearly.
func humanBytes(n int64) string { return units.Bytes(n) }
