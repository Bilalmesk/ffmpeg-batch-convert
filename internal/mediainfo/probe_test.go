package mediainfo

import (
	"math"
	"testing"
)

func TestParseFrameRate(t *testing.T) {
	tests := map[string]float64{
		"30000/1001": 29.97002997002997,
		"25/1":       25,
		"50":         50,
		"0/0":        0,
		"":           0,
		"nonsense":   0,
	}
	for input, want := range tests {
		got := parseFrameRate(input)
		if math.Abs(got-want) > 0.0001 {
			t.Fatalf("parseFrameRate(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestParseSecondsToMs(t *testing.T) {
	if got := parseSecondsToMs("12.345"); got != 12345 {
		t.Fatalf("got %d", got)
	}
	if got := parseSecondsToMs("N/A"); got != 0 {
		t.Fatalf("invalid duration should be zero, got %d", got)
	}
}

func TestIsTextSubtitle(t *testing.T) {
	for _, codec := range []string{"subrip", "ass", "mov_text", "webvtt"} {
		if !IsTextSubtitle(codec) {
			t.Fatalf("%s should be text", codec)
		}
	}
	for _, codec := range []string{"hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle"} {
		if IsTextSubtitle(codec) {
			t.Fatalf("%s should be bitmap", codec)
		}
	}
}

func TestTextSubtitleSelection(t *testing.T) {
	info := &Info{Subtitles: []Stream{
		{Index: 2, Codec: "subrip"},
		{Index: 3, Codec: "hdmv_pgs_subtitle"},
	}}
	if got := len(info.TextSubtitles()); got != 1 {
		t.Fatalf("text subtitles = %d", got)
	}
	if got := len(info.BitmapSubtitles()); got != 1 {
		t.Fatalf("bitmap subtitles = %d", got)
	}
}
