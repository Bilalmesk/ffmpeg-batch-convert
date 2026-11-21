package units

import (
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
	tests := map[int64]string{
		0:                      "0 B",
		1023:                   "1023 B",
		1024:                   "1.0 KiB",
		1536:                   "1.5 KiB",
		1024 * 1024:            "1.0 MiB",
		3 * 1024 * 1024 * 1024: "3.0 GiB",
	}
	for input, want := range tests {
		if got := Bytes(input); got != want {
			t.Fatalf("Bytes(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestClock(t *testing.T) {
	tests := map[int64]string{
		0:         "00:00",
		1500:      "00:01",
		61_000:    "01:01",
		3_723_000: "1:02:03",
	}
	for input, want := range tests {
		if got := Clock(input); got != want {
			t.Fatalf("Clock(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestSeconds(t *testing.T) {
	if got := Seconds(3661); got != "1:01:01" {
		t.Fatalf("Seconds = %q", got)
	}
	if got := Seconds(-5); got != "00:00" {
		t.Fatalf("negative seconds = %q", got)
	}
}

func TestParseTimecode(t *testing.T) {
	tests := map[string]time.Duration{
		"":           0,
		"90":         90 * time.Second,
		"1:30":       90 * time.Second,
		"01:02:03.5": time.Hour + 2*time.Minute + 3500*time.Millisecond,
		"0:00:01.25": 1250 * time.Millisecond,
	}
	for input, want := range tests {
		got, err := ParseTimecode(input)
		if err != nil {
			t.Fatalf("ParseTimecode(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseTimecode(%q) = %v, want %v", input, got, want)
		}
	}

	for _, input := range []string{"-5", "1:70", "1:2:70", "a:b", "1:2:3:4", "NaN"} {
		if _, err := ParseTimecode(input); err == nil {
			t.Fatalf("ParseTimecode(%q) should fail", input)
		}
	}
}

func TestFormatTimecode(t *testing.T) {
	if got := FormatTimecode(90*time.Second + 250*time.Millisecond); got != "00:01:30.250" {
		t.Fatalf("FormatTimecode = %q", got)
	}
	if got := FormatTimecode(-time.Second); got != "00:00:00.000" {
		t.Fatalf("negative FormatTimecode = %q", got)
	}
}
