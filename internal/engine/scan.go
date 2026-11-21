package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
)

// ErrNoFiles means the scan produced no usable inputs.
var ErrNoFiles = errors.New("no matching video files found")

// Candidate is one file discovered by a scan.
type Candidate struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	Ext       string    `json:"ext"`
	SizeBytes int64     `json:"sizeBytes"`
	ModTime   time.Time `json:"modTime"`
}

var skippedDirs = map[string]bool{
	".git": true, ".svn": true, ".hg": true, "node_modules": true,
	"$RECYCLE.BIN": true, "System Volume Information": true, ".ffbatch": true,
}

// Scan walks root (one level unless recursive) and returns matching videos.
func Scan(root string, recursive bool, extensions []string) ([]Candidate, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a folder", root)
	}

	allowed := make(map[string]bool, len(extensions))
	for _, ext := range extensions {
		allowed[strings.TrimPrefix(strings.ToLower(ext), ".")] = true
	}

	var out []Candidate
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root {
				return walkErr
			}
			return nil // unreadable sub-folder: keep going
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			if !recursive || skippedDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		name := entry.Name()
		if isTempName(name) {
			return nil
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
		if !allowed[ext] {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		out = append(out, Candidate{
			Path:      path,
			Name:      name,
			Ext:       ext,
			SizeBytes: info.Size(),
			ModTime:   info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path)
	})
	return out, nil
}

func isTempName(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, ".ffbatch-") || strings.Contains(lower, ".ffbatch-tmp") || strings.HasSuffix(lower, ".part")
}

// ScanExtensions returns the extension set used for folder scans. Rename mode
// must also discover MP4 files, because the default conversion input list does
// not include MP4 (they are normally already converted).
func ScanExtensions(cfg config.Config) []string {
	exts := append([]string(nil), cfg.Extensions...)
	if cfg.Mode == config.ModeRename {
		seen := make(map[string]bool, len(exts))
		for _, ext := range exts {
			seen[ext] = true
		}
		if !seen["mp4"] {
			exts = append(exts, "mp4")
		}
	}
	return exts
}

// collectInputs expands folders, de-duplicates paths and returns a stable,
// sorted list plus a display root.
func collectInputs(cfg config.Config, paths []string) (files []string, root string, warnings []string, err error) {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	seen := make(map[string]bool)
	add := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		key := strings.ToLower(filepath.Clean(abs))
		if seen[key] {
			return
		}
		seen[key] = true
		files = append(files, abs)
	}

	for _, input := range paths {
		abs, err := filepath.Abs(input)
		if err != nil {
			return nil, "", warnings, fmt.Errorf("resolve %s: %w", input, err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, "", warnings, fmt.Errorf("open %s: %w", input, err)
		}
		if info.IsDir() {
			if root == "" {
				root = abs
			}
			candidates, err := Scan(abs, cfg.Recursive, ScanExtensions(cfg))
			if err != nil {
				return nil, "", warnings, err
			}
			if len(candidates) == 0 {
				warnings = append(warnings, fmt.Sprintf("no matching files in %s", abs))
			}
			for _, candidate := range candidates {
				add(candidate.Path)
			}
			continue
		}
		if isTempName(filepath.Base(abs)) {
			continue
		}
		add(abs)
	}

	sort.SliceStable(files, func(i, j int) bool {
		return strings.ToLower(files[i]) < strings.ToLower(files[j])
	})
	if root == "" && len(files) > 0 {
		root = filepath.Dir(files[0])
	}
	if len(files) == 0 {
		exts := make([]string, len(cfg.Extensions))
		for i, ext := range cfg.Extensions {
			exts[i] = "*." + ext
		}
		return nil, root, warnings, fmt.Errorf("%w in %s (%s)", ErrNoFiles, firstOr(root, "the selected folders"), strings.Join(exts, " "))
	}
	return files, root, warnings, nil
}

// Plan decides the output path and initial status for every input.
func Plan(cfg config.Config, files []string) ([]*Job, []string, error) {
	var warnings []string
	jobs := make([]*Job, 0, len(files))
	reserved := make(map[string]bool, len(files))
	defaultDir := strings.TrimSpace(cfg.OutputDir)
	if defaultDir != "" && !filepath.IsAbs(defaultDir) {
		abs, err := filepath.Abs(defaultDir)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve output folder: %w", err)
		}
		defaultDir = abs
	}

	for index, input := range files {
		info, err := os.Stat(input)
		if err != nil {
			return nil, warnings, fmt.Errorf("stat %s: %w", input, err)
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(input)), ".")
		base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))

		outDir := defaultDir
		if outDir == "" {
			outDir = filepath.Dir(input)
		}
		output := filepath.Join(outDir, base+cfg.ExtFor())

		job := &Job{
			ID:        fmt.Sprintf("job-%03d", index+1),
			Name:      filepath.Base(input),
			Input:     input,
			Output:    output,
			Ext:       ext,
			Mode:      cfg.Mode,
			SizeBytes: info.Size(),
			Status:    StatusPending,
		}

		switch {
		case cfg.Mode == config.ModeRename && ext == cfg.Format:
			job.Status = StatusSkipped
			job.Reason = "already " + cfg.ExtFor()
			jobs = append(jobs, job)
			continue
		case cfg.Mode == config.ModeRemux && ext == cfg.Format:
			job.Status = StatusSkipped
			job.Reason = "already " + cfg.ExtFor()
			jobs = append(jobs, job)
			continue
		}

		if samePath(input, output) {
			job.Status = StatusFailed
			job.Error = "output would overwrite the source; choose another output folder or format"
			jobs = append(jobs, job)
			continue
		}

		// Resolve collisions with other planned jobs first.
		if reserved[pathKey(output)] {
			unique := uniquePath(output, reserved, true)
			warnings = append(warnings, fmt.Sprintf("%s and another input map to the same output; using %s", job.Name, filepath.Base(unique)))
			output = unique
			job.Output = output
		}

		exists := fileExists(output)
		switch cfg.Overwrite {
		case config.OverwriteSkip:
			if exists {
				job.Status = StatusSkipped
				job.Reason = "output already exists"
			}
		case config.OverwriteFail:
			if exists {
				job.Status = StatusFailed
				job.Error = "output already exists"
			}
		case config.OverwriteNumbering:
			if exists || reserved[pathKey(output)] {
				job.Output = uniquePath(output, reserved, true)
			}
		}
		reserved[pathKey(job.Output)] = true
		jobs = append(jobs, job)
	}
	return jobs, warnings, nil
}

func uniquePath(path string, reserved map[string]bool, checkDisk bool) string {
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	for n := 1; n < 10000; n++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, n, ext))
		if reserved[pathKey(candidate)] {
			continue
		}
		if checkDisk && fileExists(candidate) {
			continue
		}
		return candidate
	}
	return path
}

func pathKey(path string) string {
	return strings.ToLower(filepath.Clean(path))
}

func samePath(a, b string) bool {
	return pathKey(a) == pathKey(b)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func firstOr(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
