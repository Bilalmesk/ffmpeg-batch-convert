// Package term provides minimal ANSI styling that degrades gracefully when
// output is redirected or colours are disabled.
package term

import (
	"os"
	"strings"
	"sync"
)

// Style writes ANSI sequences only when the destination is a terminal.
type Style struct {
	enabled bool
}

var detectOnce sync.Once
var vtReady bool
var vtOnce sync.Once

// New detects colour support for stdout.
func New() Style {
	detectOnce.Do(func() { vtReady = stdoutIsTerminal() })
	if !vtReady {
		return Style{}
	}
	// Enable ANSI + UTF-8 even when colours are disabled, so Unicode filenames
	// and status glyphs render in classic Windows consoles.
	vtOnce.Do(enableVirtualTerminal)
	if os.Getenv("NO_COLOR") != "" || strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return Style{}
	}
	return Style{enabled: true}
}

// Enabled reports whether styling is active.
func (s Style) Enabled() bool { return s.enabled }

func (s Style) paint(code, text string) string {
	if !s.enabled || text == "" {
		return text
	}
	return code + text + "\x1b[0m"
}

func (s Style) Bold(text string) string    { return s.paint("\x1b[1m", text) }
func (s Style) Dim(text string) string     { return s.paint("\x1b[2m", text) }
func (s Style) Red(text string) string     { return s.paint("\x1b[31m", text) }
func (s Style) Green(text string) string   { return s.paint("\x1b[32m", text) }
func (s Style) Yellow(text string) string  { return s.paint("\x1b[33m", text) }
func (s Style) Blue(text string) string    { return s.paint("\x1b[34m", text) }
func (s Style) Magenta(text string) string { return s.paint("\x1b[35m", text) }
func (s Style) Cyan(text string) string    { return s.paint("\x1b[36m", text) }
func (s Style) Gray(text string) string    { return s.paint("\x1b[90m", text) }

// Status renders a coloured job status token.
func (s Style) Status(status string, label string) string {
	switch status {
	case "done", "pass":
		return s.Green(label)
	case "skipped", "warn", "planned", "partial":
		return s.Yellow(label)
	case "failed", "fail":
		return s.Red(label)
	case "running":
		return s.Cyan(label)
	default:
		return s.Gray(label)
	}
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
