package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/audio"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
)

type audioResult struct {
	Input  string `json:"input"`
	Output string `json:"output"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
}

func cmdAudio(args []string, style term.Style) int {
	fs := newFlagSet("audio")
	var (
		configPath string
		outputDir  string
		formatName string
		bitrate    string
		track      int
		recursive  bool
		normalize  bool
		overwrite  bool
		asJSON     bool
		dryRun     bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&outputDir, "output", "", "")
	fs.StringVar(&outputDir, "out", "", "")
	fs.StringVar(&formatName, "format", "flac", "")
	fs.StringVar(&bitrate, "bitrate", "", "")
	fs.IntVar(&track, "track", 0, "")
	fs.BoolVar(&recursive, "recursive", false, "")
	fs.BoolVar(&normalize, "normalize", false, "")
	fs.BoolVar(&overwrite, "overwrite", false, "")
	fs.BoolVar(&asJSON, "json", false, "")
	fs.BoolVar(&dryRun, "dry-run", false, "")
	if code, ok := parseFlags(fs, args, audioUsage, style); !ok {
		return code
	}
	if fs.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "%s audio needs at least one file or folder\n\n%s", style.Red("error:"), audioUsage)
		return 2
	}
	format, ok := audio.ParseFormat(formatName)
	if !ok {
		fmt.Fprintf(os.Stderr, "%s unknown audio format %q; use flac, opus, mp3, m4a or wav\n", style.Red("error:"), formatName)
		return 2
	}

	cfg, _, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	cfg.Normalize()
	tool, err := ffmpeg.Locate(cfg.FFmpegPath, cfg.FFprobePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 3
	}

	inputs, err := collectAudioInputs(fs.Args(), cfg, recursive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	if outputDir != "" {
		inputs = excludeUnder(inputs, outputDir, audio.Ext(format))
	}
	if len(inputs) == 0 {
		fmt.Fprintf(os.Stderr, "%s no matching media files found\n", style.Red("error:"))
		return 1
	}
	if outputDir != "" {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "%s create output folder: %v\n", style.Red("error:"), err)
			return 2
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	results := make([]audioResult, 0, len(inputs))
	reserved := make(map[string]bool, len(inputs))
	for i, input := range inputs {
		output := audio.OutputPath(input, outputDir, format)
		for n := 1; reserved[audioPathKey(output)]; n++ {
			output = numberedAudioPath(output, n)
		}
		reserved[audioPathKey(output)] = true
		result := audioResult{Input: input, Output: output}
		if sameAudioPath(input, output) {
			result.Status = "skipped"
			result.Error = "output would overwrite the source"
			results = append(results, result)
			if !asJSON {
				fmt.Printf("  %s %s  %s\n", style.Yellow("–"), filepath.Base(input), result.Error)
			}
			continue
		}
		if fileExists(output) && !overwrite {
			result.Status = "skipped"
			result.Error = "output already exists"
			results = append(results, result)
			if !asJSON {
				fmt.Printf("  %s %s  %s\n", style.Yellow("–"), filepath.Base(input), result.Error)
			}
			continue
		}
		ext := filepath.Ext(output)
		temp := filepath.Join(filepath.Dir(output), "."+strings.TrimSuffix(filepath.Base(output), ext)+".ffbatch-audio.part"+ext)
		cmdArgs, err := audio.Build(tool.FFmpeg, input, temp, audio.Options{
			Format: format, Bitrate: bitrate, Track: track, Normalize: normalize,
		})
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			results = append(results, result)
			continue
		}
		if !asJSON {
			fmt.Printf("  %s %s\n", style.Dim(fmt.Sprintf("[%d/%d]", i+1, len(inputs))), filepath.Base(input))
		}
		if dryRun {
			fmt.Printf("    %s\n", audioCommand(output, temp, cmdArgs, tool.FFmpeg))
			result.Status = "planned"
			results = append(results, result)
			continue
		}
		_ = os.Remove(temp)
		if err := runExternal(ctx, tool.FFmpeg, cmdArgs, nil, os.Stderr); err != nil {
			_ = os.Remove(temp)
			result.Status = "failed"
			result.Error = err.Error()
			results = append(results, result)
			if !asJSON {
				fmt.Printf("    %s %s\n", style.Red("✗"), err)
			}
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		info, err := mediainfo.ProbeAny(probeCtx, tool.FFprobe, temp)
		cancel()
		if err != nil || len(info.Audios) == 0 {
			_ = os.Remove(temp)
			result.Status = "failed"
			if err != nil {
				result.Error = "output could not be probed: " + err.Error()
			} else {
				result.Error = "output has no audio stream"
			}
			results = append(results, result)
			if !asJSON {
				fmt.Printf("    %s %s\n", style.Red("✗"), result.Error)
			}
			continue
		}
		if overwrite && fileExists(output) {
			_ = os.Remove(output)
		}
		if err := os.Rename(temp, output); err != nil {
			_ = os.Remove(temp)
			result.Status = "failed"
			result.Error = "finalise: " + err.Error()
			results = append(results, result)
			if !asJSON {
				fmt.Printf("    %s %s\n", style.Red("✗"), result.Error)
			}
			continue
		}
		result.Status = "done"
		result.Bytes = info.SizeBytes
		results = append(results, result)
		if !asJSON {
			fmt.Printf("    %s %s · %s · %s\n", style.Green("✓"), output, units.Bytes(info.SizeBytes), describeMedia(info))
		}
	}
	if asJSON {
		writeJSON(results)
	}
	failed := 0
	for _, result := range results {
		if result.Status == "failed" {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "%s %d audio job(s) failed\n", style.Red("error:"), failed)
		return 1
	}
	return 0
}

func collectAudioInputs(paths []string, cfg config.Config, recursive bool) ([]string, error) {
	exts := append([]string(nil), cfg.Extensions...)
	for _, format := range audio.Formats {
		exts = append(exts, format)
	}
	exts = append(exts, "aac", "ogg", "wma")
	seenExt := make(map[string]bool, len(exts))
	clean := exts[:0]
	for _, ext := range exts {
		ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
		if ext == "" || seenExt[ext] {
			continue
		}
		seenExt[ext] = true
		clean = append(clean, ext)
	}
	exts = clean

	seen := make(map[string]bool)
	var out []string
	add := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		key := strings.ToLower(filepath.Clean(abs))
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, abs)
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if info.IsDir() {
			candidates, err := engine.Scan(path, recursive, exts)
			if err != nil {
				return nil, err
			}
			for _, candidate := range candidates {
				add(candidate.Path)
			}
			continue
		}
		add(path)
	}
	sortStrings(out)
	return out, nil
}

func sameAudioPath(a, b string) bool {
	left, _ := filepath.Abs(a)
	right, _ := filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func audioPathKey(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return strings.ToLower(filepath.Clean(abs))
}

func numberedAudioPath(path string, n int) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	return fmt.Sprintf("%s (%d)%s", base, n, ext)
}

func excludeUnder(paths []string, dir, ext string) []string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return paths
	}
	out := paths[:0]
	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			out = append(out, path)
			continue
		}
		rel, err := filepath.Rel(absDir, absPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
			strings.EqualFold(filepath.Ext(absPath), ext) {
			continue
		}
		out = append(out, path)
	}
	return out
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && strings.ToLower(values[j]) < strings.ToLower(values[j-1]); j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func audioCommand(output, temp string, args []string, tool string) string {
	display := ffmpeg.CommandString(tool, args)
	return strings.Replace(display, temp, output, 1)
}
