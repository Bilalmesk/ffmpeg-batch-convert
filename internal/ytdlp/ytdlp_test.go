package ytdlp

import (
	"strings"
	"testing"
)

func TestArgs(t *testing.T) {
	args, err := Args(Request{
		URLs:       []string{"https://example.com/video"},
		OutputDir:  `C:\Downloads`,
		Format:     "bv*+ba/b",
		Cookies:    "cookies.txt",
		Downloader: "aria2c",
		Archive:    `C:\Downloads\archive.txt`,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"--no-playlist", "--paths C:\\Downloads", "-f bv*+ba/b", "--cookies cookies.txt", "--downloader aria2c", "--download-archive", "--no-overwrites", "https://example.com/video"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}

func TestArgsAudioOnly(t *testing.T) {
	args, err := Args(Request{URLs: []string{"https://example.com/a"}, OutputDir: "out", AudioOnly: true, AudioFormat: "opus", Playlist: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--extract-audio") || !strings.Contains(joined, "--audio-format opus") {
		t.Fatalf("args = %v", args)
	}
	if strings.Contains(joined, "--no-playlist") {
		t.Fatal("playlist flag should be omitted")
	}
}

func TestArgsRequiresURL(t *testing.T) {
	if _, err := Args(Request{OutputDir: "out"}); err == nil {
		t.Fatal("expected an error without URLs")
	}
}
