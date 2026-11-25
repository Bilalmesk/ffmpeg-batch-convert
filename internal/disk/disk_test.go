package disk

import "testing"

func TestFreeOnTempDir(t *testing.T) {
	if _, err := Free(t.TempDir()); err != nil {
		t.Fatalf("Free: %v", err)
	}
}

func TestFreeOnMissingPathUsesParent(t *testing.T) {
	if _, err := Free(t.TempDir() + "/nested/missing"); err != nil {
		t.Fatalf("Free on missing path: %v", err)
	}
}
