package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFileOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "temp.mp4")
	dst := filepath.Join(dir, "final.mp4")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(src, dst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("destination = %q, want new", data)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source should be gone")
	}
}

func TestMoveFileAcrossFolders(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.ts")
	dst := filepath.Join(dir, "nested", "a.ts")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(src, dst, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source should be moved")
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != "payload" {
		t.Fatalf("destination content = %q, err=%v", data, err)
	}
}

func TestMoveFileRefusesExistingWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.ts")
	dst := filepath.Join(dir, "b.ts")
	if err := os.WriteFile(src, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(src, dst, false); err == nil {
		t.Fatal("expected an error when the destination exists")
	}
}
