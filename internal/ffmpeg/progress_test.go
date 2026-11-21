package ffmpeg

import (
	"math"
	"testing"
)

func TestParserComputesPercentAndETA(t *testing.T) {
	parser := NewParser(10_000) // 10 seconds
	lines := []string{
		"frame=100",
		"fps=25.0",
		"bitrate=1200.0kbits/s",
		"out_time_us=2000000",
		"speed=1.5x",
		"progress=continue",
	}
	var sample Progress
	var ok bool
	for _, line := range lines {
		sample, ok = parser.Feed(line)
	}
	if !ok {
		t.Fatal("expected a completed sample")
	}
	if math.Abs(sample.Percent-20) > 0.01 {
		t.Fatalf("percent = %v, want 20", sample.Percent)
	}
	if sample.FPS != 25 || sample.Speed != 1.5 {
		t.Fatalf("unexpected sample: %+v", sample)
	}
	if sample.ETASeconds != 5 {
		t.Fatalf("eta = %d, want 5", sample.ETASeconds)
	}
	if sample.Done {
		t.Fatal("sample should not be done yet")
	}
}

func TestParserMarksDone(t *testing.T) {
	parser := NewParser(5000)
	parser.Feed("out_time_us=4000000")
	parser.Feed("speed=2.0x")
	parser.Feed("progress=continue")
	sample, _ := parser.Feed("progress=end")
	if !sample.Done || sample.Percent != 100 {
		t.Fatalf("expected done at 100%%, got %+v", sample)
	}
}

func TestParserHandlesOutTimeMsAsMicroseconds(t *testing.T) {
	parser := NewParser(2000)
	parser.Feed("out_time_ms=1000000") // ffmpeg writes microseconds here
	sample, _ := parser.Feed("progress=continue")
	if sample.OutTimeMs != 1000 {
		t.Fatalf("out time = %d, want 1000", sample.OutTimeMs)
	}
	if math.Abs(sample.Percent-50) > 0.01 {
		t.Fatalf("percent = %v, want 50", sample.Percent)
	}
}

func TestParserWithoutDuration(t *testing.T) {
	parser := NewParser(0)
	sample, _ := parser.Feed("progress=continue")
	if sample.Percent != 0 || sample.ETASeconds != 0 {
		t.Fatalf("unexpected sample for unknown duration: %+v", sample)
	}
}

func TestParseTimecode(t *testing.T) {
	tests := map[string]int64{
		"00:00:01.500000": 1500,
		"01:02:03.000000": 3723000,
		"2.5":             2500,
		"nope":            0,
	}
	for input, want := range tests {
		if got := parseTimecode(input); got != want {
			t.Fatalf("parseTimecode(%q) = %d, want %d", input, got, want)
		}
	}
}
