// Package disk reports free space for output-folder preflight checks.
package disk

import (
	"os"
	"path/filepath"
)

// Free returns the bytes available to the current user on the volume that
// contains path. The nearest existing parent directory is used when path does
// not exist yet.
func Free(path string) (uint64, error) {
	dir, err := nearestDir(path)
	if err != nil {
		return 0, err
	}
	return free(dir)
}

func nearestDir(path string) (string, error) {
	path = filepath.Clean(path)
	if path == "" {
		path = "."
	}
	for {
		info, err := os.Stat(path)
		if err == nil {
			if info.IsDir() {
				return path, nil
			}
			path = filepath.Dir(path)
			continue
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		path = parent
	}
}

// ErrUnavailable is returned by callers that decide a platform cannot report
// free space; it is not returned by Free on supported platforms.
