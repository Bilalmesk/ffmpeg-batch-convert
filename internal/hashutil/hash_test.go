package hashutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(path, []byte("ffbatch"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := SHA256(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("ffbatch"))
	if want := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("hash = %q, want %q", got, want)
	}
}

func TestSHA256MissingFile(t *testing.T) {
	if _, err := SHA256(context.Background(), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected an error")
	}
}
