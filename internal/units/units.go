// Package units formats byte sizes and durations for human consumption.
package units

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Bytes renders an IEC size with one decimal place (1.4 GiB).
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// Duration renders a compact duration (850ms, 42.3s, 1m 02s, 1h 04m).
func Duration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// Seconds renders a whole number of seconds as mm:ss or hh:mm:ss.
func Seconds(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

// Clock renders a media duration from milliseconds as hh:mm:ss.
func Clock(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return Seconds(ms / 1000)
}

// ParseTimecode accepts "90", "1:30" or "01:02:03.5" and returns a duration.
// An empty string parses as zero. Negative values are rejected.
func ParseTimecode(value string) (time.Duration, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, nil
	}
	parts := strings.Split(text, ":")
	if len(parts) > 3 {
		return 0, fmt.Errorf("expected seconds, MM:SS or HH:MM:SS (got %q)", value)
	}
	numbers := make([]float64, len(parts))
	for i, part := range parts {
		number, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
			return 0, fmt.Errorf("invalid time %q", value)
		}
		numbers[i] = number
	}

	var seconds float64
	switch len(numbers) {
	case 1:
		seconds = numbers[0]
	case 2:
		if numbers[1] >= 60 {
			return 0, fmt.Errorf("seconds must be below 60 in %q", value)
		}
		seconds = numbers[0]*60 + numbers[1]
	case 3:
		if numbers[1] >= 60 || numbers[2] >= 60 {
			return 0, fmt.Errorf("minutes and seconds must be below 60 in %q", value)
		}
		seconds = numbers[0]*3600 + numbers[1]*60 + numbers[2]
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// FormatTimecode renders a duration as HH:MM:SS.mmm for FFmpeg.
func FormatTimecode(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := d.Seconds()
	hours := int(total) / 3600
	minutes := (int(total) % 3600) / 60
	seconds := total - float64(hours*3600+minutes*60)
	return fmt.Sprintf("%02d:%02d:%06.3f", hours, minutes, seconds)
}
