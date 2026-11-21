package cli

import (
	"flag"
	"reflect"
	"testing"
)

func TestReorderArgsMovesFlagsBeforePaths(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var recursive, faststart bool
	var output, to string
	fs.BoolVar(&recursive, "recursive", false, "")
	fs.BoolVar(&faststart, "faststart", true, "")
	fs.StringVar(&output, "output", "", "")
	fs.StringVar(&to, "to", "", "")

	args := []string{"D:\\TV", "--recursive", "extra.ts", "--output", "D:\\Out", "--to=mp4", "--faststart=false"}
	reordered := reorderArgs(fs, args)
	if err := fs.Parse(reordered); err != nil {
		t.Fatal(err)
	}
	if !recursive {
		t.Fatal("recursive flag was not set")
	}
	if faststart {
		t.Fatal("faststart=false was not honoured")
	}
	if output != "D:\\Out" || to != "mp4" {
		t.Fatalf("string flags: output=%q to=%q", output, to)
	}
	if want := []string{"D:\\TV", "extra.ts"}; !reflect.DeepEqual(fs.Args(), want) {
		t.Fatalf("positional args = %v, want %v", fs.Args(), want)
	}
}

func TestReorderArgsHonoursTerminator(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var recursive bool
	fs.BoolVar(&recursive, "recursive", false, "")
	args := []string{"--recursive", "--", "--not-a-flag.ts"}
	reordered := reorderArgs(fs, args)
	if err := fs.Parse(reordered); err != nil {
		t.Fatal(err)
	}
	if !recursive {
		t.Fatal("flag before -- should be parsed")
	}
	if want := []string{"--not-a-flag.ts"}; !reflect.DeepEqual(fs.Args(), want) {
		t.Fatalf("args = %v, want %v", fs.Args(), want)
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" .ts, mkv ;mov  mp4 ")
	want := []string{"ts", "mkv", "mov", "mp4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitList = %v, want %v", got, want)
	}
}
