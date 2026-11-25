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

func TestIsLoopbackHost(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "::1", "[::1]", "127.0.0.53"} {
		if !isLoopbackHost(host) {
			t.Fatalf("%q should be loopback", host)
		}
	}
	for _, host := range []string{"0.0.0.0", "192.168.1.10", "example.com", ""} {
		if isLoopbackHost(host) {
			t.Fatalf("%q should not be loopback", host)
		}
	}
}

func TestSplitList(t *testing.T) {
	got := splitList(" .ts, mkv ;mov  mp4 ")
	want := []string{"ts", "mkv", "mov", "mp4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitList = %v, want %v", got, want)
	}
}
