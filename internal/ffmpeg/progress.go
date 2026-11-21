package ffmpeg

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// Progress is one sample from FFmpeg's `-progress` stream.
type Progress struct {
	Frame      int64   `json:"frame"`
	FPS        float64 `json:"fps"`
	Bitrate    string  `json:"bitrate"`
	TotalSize  int64   `json:"totalSize"`
	OutTimeMs  int64   `json:"outTimeMs"`
	Speed      float64 `json:"speed"`
	Percent    float64 `json:"percent"`
	ETASeconds int64   `json:"etaSeconds"`
	Done       bool    `json:"done"`
}

// Parser turns key=value progress lines into Progress samples and computes
// percentages / ETA against the probed duration.
type Parser struct {
	totalMs int64
	current Progress
}

// NewParser creates a parser for a job whose media duration is totalMs.
func NewParser(totalMs int64) *Parser {
	return &Parser{totalMs: totalMs}
}

// Feed consumes one line. It reports whether the line produced a complete
// sample (i.e. a `progress=` marker was seen).
func (p *Parser) Feed(line string) (Progress, bool) {
	line = strings.TrimSpace(line)
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return p.current, false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	switch key {
	case "frame":
		p.current.Frame = parseInt64(value)
	case "fps":
		p.current.FPS = parseFloat(value)
	case "bitrate":
		if value != "N/A" {
			p.current.Bitrate = value
		}
	case "total_size":
		p.current.TotalSize = parseInt64(value)
	case "out_time_us", "out_time_ms":
		// ffmpeg historically writes microseconds into out_time_ms too.
		p.current.OutTimeMs = parseInt64(value) / 1000
	case "out_time":
		if ms := parseTimecode(value); ms > 0 {
			p.current.OutTimeMs = ms
		}
	case "speed":
		p.current.Speed = parseSpeed(value)
	case "progress":
		p.current.Done = value == "end"
		p.compute()
		sample := p.current
		if !sample.Done {
			p.current.Done = false
		}
		return sample, true
	}
	return p.current, false
}

func (p *Parser) compute() {
	p.current.Percent = 0
	p.current.ETASeconds = 0
	if p.totalMs <= 0 {
		return
	}
	ratio := float64(p.current.OutTimeMs) / float64(p.totalMs)
	if p.current.Done {
		ratio = 1
	}
	ratio = math.Max(0, math.Min(ratio, 1))
	if !p.current.Done && ratio > 0.995 {
		ratio = 0.995
	}
	p.current.Percent = math.Round(ratio*1000) / 10 // one decimal place
	if p.current.Speed > 0.01 && !p.current.Done {
		remainingMedia := float64(p.totalMs-p.current.OutTimeMs) / 1000
		eta := remainingMedia / p.current.Speed
		if eta > 0 && eta < 60*60*24 {
			p.current.ETASeconds = int64(math.Round(eta))
		}
	}
}

// ETA returns the remaining wall-clock duration, when known.
func (p Progress) ETA() time.Duration {
	return time.Duration(p.ETASeconds) * time.Second
}

func parseInt64(value string) int64 {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func parseFloat(value string) float64 {
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseSpeed(value string) float64 {
	return parseFloat(strings.TrimSuffix(strings.TrimSpace(value), "x"))
}

// parseTimecode understands both HH:MM:SS.micro and plain seconds.
func parseTimecode(value string) int64 {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ":")
	if len(parts) == 3 {
		hours, _ := strconv.ParseFloat(parts[0], 64)
		minutes, _ := strconv.ParseFloat(parts[1], 64)
		seconds, _ := strconv.ParseFloat(parts[2], 64)
		return int64(((hours*60+minutes)*60 + seconds) * 1000)
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		return int64(seconds * 1000)
	}
	return 0
}
