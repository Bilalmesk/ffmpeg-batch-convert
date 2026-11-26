package audio

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuild(t *testing.T) {
	args, err := Build("ffmpeg", "in.mp4", "out.flac", Options{Format: FormatFLAC})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(args, "-c:a") || !contains(args, "flac") || !contains(args, "0:a:0?") {
		t.Fatalf("args = %v", args)
	}
}

func TestBuildOpusNormalize(t *testing.T) {
	args, err := Build("ffmpeg", "in.mkv", "out.opus", Options{Format: FormatOpus, Bitrate: "96k", Normalize: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"libopus", "96k"} {
		if !contains(args, want) {
			t.Fatalf("args %v missing %q", args, want)
		}
	}
	if !containsSubstring(args, "loudnorm") {
		t.Fatalf("args %v missing loudnorm", args)
	}
}

func TestOutputPath(t *testing.T) {
	got := OutputPath(filepath.Join("D:", "Shows", "episode.mkv"), filepath.Join("D:", "Audio"), FormatFLAC)
	want := filepath.Join("D:", "Audio", "episode.flac")
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if got := OutputPath("clip.mp4", "", FormatMP3); got != "clip.mp3" {
		t.Fatalf("same-folder output = %q", got)
	}
}

func TestParseFormat(t *testing.T) {
	if got, ok := ParseFormat("AAC"); !ok || got != FormatM4A {
		t.Fatalf("ParseFormat(aac) = %q, %v", got, ok)
	}
	if _, ok := ParseFormat("ogg"); ok {
		t.Fatal("ogg should not be accepted")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
