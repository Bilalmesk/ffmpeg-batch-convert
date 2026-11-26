package toolchain

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryAvailableAndPath(t *testing.T) {
	registry := Registry{Tools: map[string]Tool{
		"ffmpeg": {ID: "ffmpeg", Available: true, Path: "C:\\ffmpeg.exe"},
		"mp4box": {ID: "mp4box", Available: false},
	}}
	if !registry.Available("ffmpeg") {
		t.Fatal("ffmpeg should be available")
	}
	if registry.Available("ffmpeg", "mp4box") {
		t.Fatal("mp4box should be unavailable")
	}
	if registry.Path("ffmpeg") != "C:\\ffmpeg.exe" {
		t.Fatalf("path = %q", registry.Path("ffmpeg"))
	}
	if registry.Path("missing") != "" {
		t.Fatal("missing path should be empty")
	}
}

func TestListIsSorted(t *testing.T) {
	registry := Registry{Tools: map[string]Tool{
		"b": {ID: "b", Name: "Zeta", Category: "Muxing"},
		"a": {ID: "a", Name: "Alpha", Category: "Core"},
		"c": {ID: "c", Name: "Beta", Category: "Core"},
	}}
	list := registry.List()
	if len(list) != 3 {
		t.Fatalf("list length = %d", len(list))
	}
	if list[0].ID != "a" || list[1].ID != "c" || list[2].ID != "b" {
		t.Fatalf("sort order = %v, %v, %v", list[0].ID, list[1].ID, list[2].ID)
	}
}

func TestDetectOverride(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tool, ok := Resolve(context.Background(), "mkvmerge", map[string]string{"mkvmerge": exe})
	if !ok {
		t.Fatal("mkvmerge should be a known tool")
	}
	if !tool.Available {
		t.Fatal("override should be detected")
	}
	if tool.Path != exe {
		t.Fatalf("path = %q, want %q", tool.Path, exe)
	}
	if tool.Version == "" {
		t.Fatal("version fallback should not be empty")
	}
}

func TestResolveDirectoryOverride(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "mkvmerge"+exeSuffix())
	if err := os.WriteFile(binary, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	item := spec{id: "mkvmerge", binaries: []string{"mkvmerge"}}
	if got := resolve(item, dir); got != binary {
		t.Fatalf("resolve = %q, want %q", got, binary)
	}
}

func TestDedupe(t *testing.T) {
	got := dedupe([]string{"a", "b", "a", "", "c", "b"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("dedupe = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupe = %v, want %v", got, want)
		}
	}
}
