package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/audio"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/hashutil"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/muxer"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/packager"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/quality"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/toolchain"
)

func runStep(ctx context.Context, step Step, params map[string]any, state *runState, tool ffmpeg.Tool) (*stepResult, error) {
	switch step.Type {
	case "probe":
		return runProbe(ctx, params, state, tool)
	case "convert":
		return runConvert(ctx, params, state, tool)
	case "quality":
		return runQuality(ctx, params, state, tool)
	case "mux":
		return runMux(ctx, params, state, tool)
	case "package":
		return runPackage(ctx, params, state, tool)
	case "audio":
		return runAudio(ctx, params, state, tool)
	case "copy":
		return runCopy(ctx, params, state)
	case "delete":
		return runDelete(ctx, params, state)
	default:
		return nil, fmt.Errorf("unknown step type %q", step.Type)
	}
}

func runProbe(ctx context.Context, _ map[string]any, state *runState, tool ffmpeg.Tool) (*stepResult, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	info, err := mediainfo.ProbeAny(probeCtx, tool.FFprobe, state.Input)
	if err != nil {
		return nil, fmt.Errorf("probe input: %w", err)
	}
	probe := probeMap(info)
	state.Probe = probe
	return &stepResult{Path: state.Input, Info: info, Values: probe}, nil
}

func runConvert(ctx context.Context, params map[string]any, state *runState, tool ffmpeg.Tool) (*stepResult, error) {
	input := absolutePath(paramString(params, "input", state.LastOutput), state.OutputDir)
	output := absolutePath(paramString(params, "output", ""), state.OutputDir)
	if output == "" {
		return nil, fmt.Errorf("convert: output is required")
	}
	if samePath(input, output) {
		return nil, fmt.Errorf("convert: output would overwrite the input")
	}
	overwrite := paramBool(params, "overwrite", state.Overwrite)
	if fileExists(output) && !overwrite {
		return nil, fmt.Errorf("convert: output already exists: %s (use --overwrite)", output)
	}

	cfg := config.Default()
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("convert params: %w", err)
	}
	cfg.Normalize()
	cfg.DryRun = false
	if cfg.DeleteSource || cfg.ArchiveDir != "" {
		return nil, fmt.Errorf("convert: use a copy/delete step instead of deleteSource/archiveDir")
	}
	if cfg.Mode == config.ModeRename {
		return nil, fmt.Errorf("convert: use the copy step for rename-only workflows")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	source, err := mediainfo.Probe(probeCtx, tool.FFprobe, input)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("probe input: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return nil, fmt.Errorf("create output folder: %w", err)
	}
	ext := filepath.Ext(output)
	base := strings.TrimSuffix(filepath.Base(output), ext)
	temp := filepath.Join(filepath.Dir(output), fmt.Sprintf(".%s.ffbatch-workflow.part%s", base, ext))
	_ = os.Remove(temp)

	var sourceHash string
	if cfg.HashFiles {
		sourceHash, err = hashutil.SHA256(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("hash source: %w", err)
		}
	}

	build := ffmpeg.Build(ffmpeg.BuildRequest{Cfg: cfg, Input: input, Output: temp, Info: source})
	result, err := tool.Run(ctx, build.Args, ffmpeg.RunOptions{TotalMs: source.DurationMs})
	if err != nil {
		_ = os.Remove(temp)
		return nil, err
	}
	verified := false
	if cfg.VerifyOutput {
		if err := engine.VerifyOutput(ctx, tool, cfg, source, temp); err != nil {
			_ = os.Remove(temp)
			return nil, fmt.Errorf("verify output: %w", err)
		}
		verified = true
	}
	var outputHash string
	if cfg.HashFiles {
		outputHash, err = hashutil.SHA256(ctx, temp)
		if err != nil {
			_ = os.Remove(temp)
			return nil, fmt.Errorf("hash output: %w", err)
		}
	}
	if err := commitFile(temp, output, overwrite); err != nil {
		_ = os.Remove(temp)
		return nil, err
	}
	probeCtx, cancel = context.WithTimeout(ctx, 2*time.Minute)
	outInfo, err := mediainfo.Probe(probeCtx, tool.FFprobe, output)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("probe output: %w", err)
	}
	values := map[string]any{
		"command":     build.Command,
		"verified":    verified,
		"durationMs":  outInfo.DurationMs,
		"sourceHash":  sourceHash,
		"outputHash":  outputHash,
		"outputBytes": outInfo.SizeBytes,
		"elapsedMs":   result.Duration.Milliseconds(),
	}
	return &stepResult{Output: output, Path: output, Info: outInfo, Values: values}, nil
}

func runQuality(ctx context.Context, params map[string]any, state *runState, tool ffmpeg.Tool) (*stepResult, error) {
	reference := absolutePath(paramString(params, "reference", state.Input), state.OutputDir)
	candidate := absolutePath(paramString(params, "candidate", state.LastOutput), state.OutputDir)
	metric, ok := quality.ParseMetric(paramString(params, "metric", "vmaf"))
	if !ok {
		return nil, fmt.Errorf("quality: metric must be vmaf, ssim or psnr")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	refInfo, err := mediainfo.Probe(probeCtx, tool.FFprobe, reference)
	candInfo, candErr := mediainfo.Probe(probeCtx, tool.FFprobe, candidate)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("probe reference: %w", err)
	}
	if candErr != nil {
		return nil, fmt.Errorf("probe candidate: %w", candErr)
	}
	if refInfo.Video == nil || candInfo.Video == nil {
		return nil, fmt.Errorf("quality: both files need a video stream")
	}
	duration := refInfo.DurationMs
	if candInfo.DurationMs > 0 && (duration == 0 || candInfo.DurationMs < duration) {
		duration = candInfo.DurationMs
	}
	measureCtx, measureCancel := context.WithTimeout(ctx, quality.Timeout(duration))
	defer measureCancel()
	result, err := quality.Measure(measureCtx, quality.Request{
		Reference:  reference,
		Candidate:  candidate,
		Metric:     metric,
		RefWidth:   refInfo.Video.Width,
		RefHeight:  refInfo.Video.Height,
		DurationMs: duration,
		Threads:    paramInt(params, "threads", 0),
		FFmpeg:     tool.FFmpeg,
	})
	if err != nil {
		return nil, err
	}
	minimum := paramFloat(params, "min", 0)
	if minimum > 0 && !result.Infinite && result.Score < minimum {
		return nil, fmt.Errorf("quality gate failed: %s %.3f is below %.3f", metric, result.Score, minimum)
	}
	values := map[string]any{
		"metric":       string(metric),
		"score":        result.Score,
		"scoreText":    result.ScoreText,
		"infinite":     result.Infinite,
		"min":          result.Min,
		"max":          result.Max,
		"harmonicMean": result.HarmonicMean,
		"frames":       result.Frames,
	}
	return &stepResult{Path: candidate, Score: result.Score, Values: values}, nil
}

func runMux(ctx context.Context, params map[string]any, state *runState, tool ffmpeg.Tool) (*stepResult, error) {
	input := absolutePath(paramString(params, "input", state.LastOutput), state.OutputDir)
	output := absolutePath(paramString(params, "output", ""), state.OutputDir)
	if output == "" {
		return nil, fmt.Errorf("mux: output is required")
	}
	overwrite := paramBool(params, "overwrite", state.Overwrite)
	if fileExists(output) && !overwrite {
		return nil, fmt.Errorf("mux: output already exists: %s (use --overwrite)", output)
	}
	mkv, _ := toolchain.Resolve(ctx, "mkvmerge", state.ToolPaths)
	if !mkv.Available {
		return nil, fmt.Errorf("mux: mkvmerge was not found")
	}
	languages, err := muxer.ParseOptions(paramString(params, "languages", ""))
	if err != nil {
		return nil, err
	}
	trackNames, err := muxer.ParseOptions(paramString(params, "trackNames", ""))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return nil, err
	}
	ext := filepath.Ext(output)
	base := strings.TrimSuffix(filepath.Base(output), ext)
	temp := filepath.Join(filepath.Dir(output), "."+base+".ffbatch-workflow.part"+ext)
	_ = os.Remove(temp)
	request := muxer.Request{
		Input:       input,
		Output:      temp,
		Title:       paramString(params, "title", ""),
		Chapters:    paramString(params, "chapters", ""),
		NoAudio:     paramBool(params, "noAudio", false),
		NoVideo:     paramBool(params, "noVideo", false),
		NoSubtitles: paramBool(params, "noSubtitles", false),
		NoChapters:  paramBool(params, "noChapters", false),
		Languages:   languages,
		TrackNames:  trackNames,
	}
	args, command, err := muxer.Build(mkv.Path, request)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 12*time.Hour)
	defer cancel()
	if err := runCommand(runCtx, mkv.Path, args, filepath.Dir(output)); err != nil {
		_ = os.Remove(temp)
		return nil, err
	}
	if err := commitFile(temp, output, overwrite); err != nil {
		_ = os.Remove(temp)
		return nil, err
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 2*time.Minute)
	info, err := mediainfo.ProbeAny(probeCtx, tool.FFprobe, output)
	probeCancel()
	if err != nil {
		return nil, fmt.Errorf("probe muxed output: %w", err)
	}
	return &stepResult{
		Output: output,
		Path:   output,
		Info:   info,
		Values: map[string]any{"command": command, "videoTracks": boolInt(info.Video != nil), "audioTracks": len(info.Audios)},
	}, nil
}

func runPackage(ctx context.Context, params map[string]any, state *runState, tool ffmpeg.Tool) (*stepResult, error) {
	input := absolutePath(paramString(params, "input", state.LastOutput), state.OutputDir)
	out := absolutePath(paramString(params, "out", ""), state.OutputDir)
	if out == "" {
		return nil, fmt.Errorf("package: out is required")
	}
	format := packager.Format(strings.ToLower(paramString(params, "format", "hls")))
	segment := paramInt(params, "segment", 6)
	toolName := packager.Tool(strings.ToLower(paramString(params, "tool", "ffmpeg")))
	request := packager.Request{
		Input: input, OutputDir: out, Format: format,
		SegmentSeconds: segment, Tool: toolName, FFmpeg: tool.FFmpeg,
	}
	switch toolName {
	case packager.ToolMP4Box:
		mp4box, _ := toolchain.Resolve(ctx, "mp4box", state.ToolPaths)
		if !mp4box.Available {
			return nil, fmt.Errorf("package: MP4Box is not installed")
		}
		request.MP4Box = mp4box.Path
	case packager.ToolBento4:
		fragment, _ := toolchain.Resolve(ctx, "mp4fragment", state.ToolPaths)
		dash, _ := toolchain.Resolve(ctx, "mp4dash", state.ToolPaths)
		if !fragment.Available || !dash.Available {
			return nil, fmt.Errorf("package: Bento4 is not installed")
		}
		request.MP4Fragment = fragment.Path
		request.MP4Dash = dash.Path
	}
	plan, err := packager.Build(request)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 12*time.Hour)
	defer cancel()
	commands := make([]string, 0, len(plan.Commands))
	for _, command := range plan.Commands {
		commands = append(commands, packager.CommandString(command))
		if err := runCommand(runCtx, command.Binary, command.Args, command.Dir); err != nil {
			return nil, err
		}
	}
	manifest, err := findManifest(plan.Manifest, out, string(format))
	if err != nil {
		return nil, err
	}
	return &stepResult{
		Path:     out,
		Manifest: manifest,
		Values:   map[string]any{"manifest": manifest, "commands": commands},
	}, nil
}

func runAudio(ctx context.Context, params map[string]any, state *runState, tool ffmpeg.Tool) (*stepResult, error) {
	input := absolutePath(paramString(params, "input", state.LastOutput), state.OutputDir)
	format, ok := audio.ParseFormat(paramString(params, "format", "flac"))
	if !ok {
		return nil, fmt.Errorf("audio: format must be flac, opus, mp3, m4a or wav")
	}
	output := paramString(params, "output", "")
	if output == "" {
		output = filepath.Join(state.OutputDir, state.Base+audio.Ext(format))
	}
	output = absolutePath(output, state.OutputDir)
	overwrite := paramBool(params, "overwrite", state.Overwrite)
	if fileExists(output) && !overwrite {
		return nil, fmt.Errorf("audio: output already exists: %s (use --overwrite)", output)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return nil, err
	}
	args, err := audio.Build(tool.FFmpeg, input, output, audio.Options{
		Format:    format,
		Bitrate:   paramString(params, "bitrate", ""),
		Track:     paramInt(params, "track", 0),
		Normalize: paramBool(params, "normalize", false),
	})
	if err != nil {
		return nil, err
	}
	ext := filepath.Ext(output)
	base := strings.TrimSuffix(filepath.Base(output), ext)
	temp := filepath.Join(filepath.Dir(output), "."+base+".ffbatch-workflow.part"+ext)
	_ = os.Remove(temp)
	// Replace the final output with the temporary path for the actual run.
	for i, arg := range args {
		if arg == output {
			args[i] = temp
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, 12*time.Hour)
	defer cancel()
	if err := runCommand(runCtx, tool.FFmpeg, args, ""); err != nil {
		_ = os.Remove(temp)
		return nil, err
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 2*time.Minute)
	info, err := mediainfo.ProbeAny(probeCtx, tool.FFprobe, temp)
	probeCancel()
	if err != nil || len(info.Audios) == 0 {
		_ = os.Remove(temp)
		if err != nil {
			return nil, fmt.Errorf("probe audio output: %w", err)
		}
		return nil, fmt.Errorf("audio output has no audio stream")
	}
	if err := commitFile(temp, output, overwrite); err != nil {
		_ = os.Remove(temp)
		return nil, err
	}
	return &stepResult{
		Output: output,
		Path:   output,
		Info:   info,
		Values: map[string]any{"format": string(format), "audioTracks": len(info.Audios)},
	}, nil
}

func runCopy(ctx context.Context, params map[string]any, state *runState) (*stepResult, error) {
	from := absolutePath(paramString(params, "from", state.LastOutput), state.OutputDir)
	to := absolutePath(paramString(params, "to", ""), state.OutputDir)
	if to == "" {
		return nil, fmt.Errorf("copy: to is required")
	}
	if samePath(from, to) {
		return nil, fmt.Errorf("copy: source and destination are the same")
	}
	overwrite := paramBool(params, "overwrite", state.Overwrite)
	if fileExists(to) && !overwrite {
		return nil, fmt.Errorf("copy: destination already exists: %s (use --overwrite)", to)
	}
	if _, err := os.Stat(from); err != nil {
		return nil, fmt.Errorf("copy: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return nil, err
	}
	move := paramBool(params, "move", false)
	if err := copyOrMove(ctx, from, to, move, overwrite); err != nil {
		return nil, err
	}
	return &stepResult{Output: to, Path: to}, nil
}

func runDelete(ctx context.Context, params map[string]any, state *runState) (*stepResult, error) {
	path := absolutePath(paramString(params, "path", ""), state.OutputDir)
	if path == "" {
		return nil, fmt.Errorf("delete: path is required")
	}
	if samePath(path, state.Input) && !paramBool(params, "allowInput", false) {
		return nil, fmt.Errorf("delete: refusing to delete the workflow input without allowInput=true")
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) && paramBool(params, "missingOk", false) {
			return &stepResult{Path: path}, nil
		}
		return nil, err
	}
	return &stepResult{Path: path}, nil
}

func probeMap(info *mediainfo.Info) map[string]any {
	audioLanguages := make([]string, 0, len(info.Audios))
	audioCodecs := make([]string, 0, len(info.Audios))
	for _, stream := range info.Audios {
		audioLanguages = append(audioLanguages, stream.Language)
		audioCodecs = append(audioCodecs, stream.Codec)
	}
	probe := map[string]any{
		"format":         info.Format,
		"durationMs":     info.DurationMs,
		"sizeBytes":      info.SizeBytes,
		"bitrate":        info.Bitrate,
		"videoTracks":    0,
		"audioTracks":    len(info.Audios),
		"subtitleTracks": len(info.Subtitles),
		"hasVideo":       info.Video != nil,
		"hasAudio":       len(info.Audios) > 0,
		"audioLanguages": audioLanguages,
		"audioCodecs":    audioCodecs,
	}
	if info.Video != nil {
		probe["videoTracks"] = 1
		probe["videoCodec"] = info.Video.Codec
		probe["width"] = info.Video.Width
		probe["height"] = info.Video.Height
		probe["fps"] = info.Video.FPS
	}
	return probe
}

func paramString(params map[string]any, key, fallback string) string {
	value, ok := params[key]
	if !ok || value == nil {
		return fallback
	}
	return stringify(value)
}

func paramBool(params map[string]any, key string, fallback bool) bool {
	value, ok := params[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func paramInt(params map[string]any, key string, fallback int) int {
	value, ok := params[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		number, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return number
		}
	}
	return fallback
}

func paramFloat(params map[string]any, key string, fallback float64) float64 {
	value, ok := params[key]
	if !ok || value == nil {
		return fallback
	}
	if number, ok := numberValue(value); ok {
		return number
	}
	return fallback
}

func absolutePath(value, baseDir string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(baseDir, value)
}

func samePath(a, b string) bool {
	left, _ := filepath.Abs(a)
	right, _ := filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func runCommand(ctx context.Context, binary string, args []string, dir string) error {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	stderr := &limitedBuffer{max: 64 << 10}
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	err := cmd.Run()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var exitErr *exec.ExitError
	if exitErr != nil {
		return fmt.Errorf("%s exited with code %d: %s", filepath.Base(binary), exitErr.ExitCode(), summarize(stderr.String()))
	}
	return fmt.Errorf("run %s: %w", filepath.Base(binary), err)
}

type limitedBuffer struct {
	buf []byte
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-b.max:]...)
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return string(b.buf)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func summarize(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	message := strings.Join(lines, " | ")
	if len(message) > 800 {
		message = message[:800] + "..."
	}
	return message
}

func commitFile(src, dst string, overwrite bool) error {
	if fileExists(dst) {
		if !overwrite {
			return fmt.Errorf("destination already exists: %s", dst)
		}
		if err := os.Remove(dst); err != nil {
			return err
		}
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	return copyFileAtomic(src, dst)
}

func copyOrMove(ctx context.Context, src, dst string, move, overwrite bool) error {
	if err := copyFileAtomic(src, dst); err != nil {
		return err
	}
	if move {
		if err := os.Remove(src); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return ctx.Err()
}

func copyFileAtomic(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	temp := dst + ".ffbatch-copy.part"
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		_ = in.Close()
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = in.Close()
		_ = os.Remove(temp)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = in.Close()
		_ = os.Remove(temp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = in.Close()
		_ = os.Remove(temp)
		return err
	}
	if err := in.Close(); err != nil {
		_ = os.Remove(temp)
		return err
	}
	if copied, err := os.Stat(temp); err != nil || copied.Size() != info.Size() {
		_ = os.Remove(temp)
		return fmt.Errorf("copy verification failed for %s", filepath.Base(src))
	}
	if fileExists(dst) {
		if err := os.Remove(dst); err != nil {
			_ = os.Remove(temp)
			return err
		}
	}
	if err := os.Rename(temp, dst); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

func findManifest(expected, dir, format string) (string, error) {
	if fileExists(expected) {
		return expected, nil
	}
	ext := ".mpd"
	if format == "hls" {
		ext = ".m3u8"
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*"+ext))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no manifest found in %s", dir)
	}
	return matches[0], nil
}
