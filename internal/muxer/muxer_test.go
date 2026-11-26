package muxer

import (
	"strings"
	"testing"
)

func TestBuild(t *testing.T) {
	args, command, err := Build("mkvmerge", Request{
		Input:     `C:\in.ts`,
		Output:    `C:\out.mkv`,
		Title:     "Episode 1",
		Chapters:  `C:\chapters.txt`,
		NoAudio:   false,
		Languages: []Option{{ID: "0", Value: "ara"}, {ID: "1", Value: "eng"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-o", `C:\out.mkv`, "--title", "Episode 1", "--chapters", "--language 0:ara", "--language 1:eng", `C:\in.ts`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	if !strings.Contains(command, "mkvmerge") {
		t.Fatalf("command = %q", command)
	}
}

func TestBuildRejectsNonMKV(t *testing.T) {
	if _, _, err := Build("mkvmerge", Request{Input: "in.ts", Output: "out.mp4"}); err == nil {
		t.Fatal("expected an error for a non-MKV output")
	}
}

func TestParseOptions(t *testing.T) {
	options, err := ParseOptions("0:ara, 1:eng")
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 2 || options[0].ID != "0" || options[0].Value != "ara" || options[1].Value != "eng" {
		t.Fatalf("options = %+v", options)
	}
	if _, err := ParseOptions("0"); err == nil {
		t.Fatal("expected an error for a missing value")
	}
}
