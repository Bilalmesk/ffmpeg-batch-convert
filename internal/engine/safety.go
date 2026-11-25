package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/disk"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
)

func verificationRequired(cfg config.Config) bool {
	return cfg.VerifyOutput || cfg.DeleteSource || cfg.ArchiveDir != ""
}

func hashingRequired(cfg config.Config) bool {
	return cfg.HashFiles || cfg.DeleteSource || cfg.ArchiveDir != ""
}

func estimatedBytes(cfg config.Config, sourceSize int64) uint64 {
	if sourceSize <= 0 {
		return 0
	}
	size := uint64(sourceSize)
	switch cfg.Mode {
	case config.ModeEncode:
		// Encoding usually shrinks; half the source plus a large fixed margin
		// catches the common "nearly full disk" case without blocking jobs.
		return size/2 + 512<<20
	case config.ModeRemux:
		return size + size/50 + 64<<20
	default:
		return size + 64<<20
	}
}

func ensureOutputSpace(cfg config.Config, job *Job) error {
	need := estimatedBytes(cfg, job.SizeBytes)
	if need == 0 {
		return nil
	}
	free, err := disk.Free(filepath.Dir(job.Output))
	if err != nil {
		return nil // unknown volume: let FFmpeg report the real problem
	}
	if free < need {
		return fmt.Errorf("not enough free space in the output folder: need %s, have %s",
			units.Bytes(int64(need)), units.Bytes(int64(free)))
	}
	return nil
}

// archiveSource moves the original into ArchiveDir, preserving its path
// relative to the run root where possible. It returns the final destination.
func archiveSource(run *Run, job *Job, cfg config.Config) (string, error) {
	root := cfg.ArchiveDir
	if root == "" {
		return "", fmt.Errorf("archive folder is empty")
	}
	if !filepath.IsAbs(root) {
		abs, err := filepath.Abs(root)
		if err != nil {
			return "", err
		}
		root = abs
	}

	name := filepath.Base(job.Input)
	if run.Root != "" {
		if rel, err := filepath.Rel(run.Root, job.Input); err == nil && rel != "." && filepath.IsLocal(rel) {
			name = rel
		}
	}
	if !filepath.IsLocal(name) {
		return "", fmt.Errorf("refusing a non-local archive path: %q", name)
	}
	dest := filepath.Join(root, name)
	if samePath(dest, job.Input) {
		return "", fmt.Errorf("archive folder would overwrite the source")
	}
	if fileExists(dest) {
		dest = uniquePath(dest, map[string]bool{}, true)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("create archive folder: %w", err)
	}
	if err := moveFile(job.Input, dest, false); err != nil {
		return "", err
	}
	return dest, nil
}

func sourceStillMatches(path string, size int64, modTime time.Time) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Size() == size && info.ModTime().Equal(modTime)
}
