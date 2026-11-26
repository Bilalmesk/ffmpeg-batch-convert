package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/version"
)

type convertOptions struct {
	configPath  string
	mode        string
	recipe      string
	format      string
	output      string
	extensions  string
	workers     int
	crf         int
	preset      string
	codec       string
	av1Encoder  string
	audio       string
	audioRate   string
	audioTracks string
	audioLang   string
	subtitles   string
	trimStart   string
	trimEnd     string
	overwrite   string
	aspect      string
	archiveDir  string
	logPath     string
	reportPath  string
	ffmpegPath  string
	ffprobePath string

	recursive    bool
	deleteSource bool
	verifyOutput bool
	hashFiles    bool
	dryRun       bool
	faststart    bool
	genpts       bool
	json         bool
	quiet        bool
}

func registerConvertFlags(fs *flag.FlagSet, opts *convertOptions) {
	fs.StringVar(&opts.configPath, "config", "", "")
	fs.StringVar(&opts.mode, "mode", "", "")
	fs.StringVar(&opts.recipe, "recipe", "", "")
	fs.StringVar(&opts.format, "to", "", "")
	fs.StringVar(&opts.format, "format", "", "")
	fs.StringVar(&opts.output, "output", "", "")
	fs.StringVar(&opts.output, "out", "", "")
	fs.StringVar(&opts.extensions, "ext", "", "")
	fs.IntVar(&opts.workers, "workers", 0, "")
	fs.IntVar(&opts.crf, "crf", 0, "")
	fs.StringVar(&opts.preset, "preset", "", "")
	fs.StringVar(&opts.codec, "codec", "", "")
	fs.StringVar(&opts.av1Encoder, "av1-encoder", "", "")
	fs.StringVar(&opts.audio, "audio", "", "")
	fs.StringVar(&opts.audioRate, "audio-bitrate", "", "")
	fs.StringVar(&opts.audioTracks, "audio-tracks", "", "")
	fs.StringVar(&opts.audioLang, "audio-lang", "", "")
	fs.StringVar(&opts.subtitles, "subs", "", "")
	fs.StringVar(&opts.trimStart, "trim-start", "", "")
	fs.StringVar(&opts.trimEnd, "trim-end", "", "")
	fs.StringVar(&opts.overwrite, "overwrite", "", "")
	fs.StringVar(&opts.aspect, "aspect", "", "")
	fs.StringVar(&opts.archiveDir, "archive", "", "")
	fs.StringVar(&opts.logPath, "log", "", "")
	fs.StringVar(&opts.reportPath, "report", "", "")
	fs.StringVar(&opts.ffmpegPath, "ffmpeg", "", "")
	fs.StringVar(&opts.ffprobePath, "ffprobe", "", "")
	fs.BoolVar(&opts.recursive, "recursive", false, "")
	fs.BoolVar(&opts.deleteSource, "delete-source", false, "")
	fs.BoolVar(&opts.verifyOutput, "verify-output", false, "")
	fs.BoolVar(&opts.hashFiles, "hash-files", false, "")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "")
	fs.BoolVar(&opts.faststart, "faststart", false, "")
	fs.BoolVar(&opts.genpts, "genpts", false, "")
	fs.BoolVar(&opts.json, "json", false, "")
	fs.BoolVar(&opts.quiet, "quiet", false, "")
}

func resolveConvertOptions(fs *flag.FlagSet, opts *convertOptions) (config.Config, string, error) {
	cfg, configPath, err := loadConfig(opts.configPath)
	if err != nil {
		return config.Config{}, configPath, err
	}
	if opts.recipe != "" {
		var ok bool
		cfg, ok = cfg.ApplyRecipe(opts.recipe)
		if !ok {
			return cfg, configPath, fmt.Errorf("unknown recipe %q; available: %s", opts.recipe, recipeIDs())
		}
	}

	seen := visited(fs)
	if hasAny(seen, "mode") {
		cfg.Mode = config.Mode(opts.mode)
	}
	if hasAny(seen, "to", "format") {
		cfg.Format = opts.format
	}
	if hasAny(seen, "output", "out") {
		cfg.OutputDir = opts.output
	}
	if seen["recursive"] {
		cfg.Recursive = opts.recursive
	}
	if seen["workers"] {
		cfg.Workers = opts.workers
	}
	if seen["crf"] {
		cfg.CRF = opts.crf
	}
	if seen["preset"] {
		cfg.Preset = opts.preset
	}
	if seen["codec"] {
		cfg.VideoCodec = opts.codec
	}
	if seen["av1-encoder"] {
		cfg.AV1Encoder = opts.av1Encoder
	}
	if seen["audio"] {
		cfg.AudioCodec = opts.audio
	}
	if seen["audio-bitrate"] {
		cfg.AudioRate = opts.audioRate
	}
	if seen["audio-tracks"] {
		cfg.AudioTracks = opts.audioTracks
	}
	if seen["audio-lang"] {
		cfg.AudioLanguage = opts.audioLang
	}
	if seen["subs"] {
		cfg.Subtitle = opts.subtitles
	}
	if seen["trim-start"] {
		cfg.TrimStart = opts.trimStart
	}
	if seen["trim-end"] {
		cfg.TrimEnd = opts.trimEnd
	}
	if seen["overwrite"] {
		cfg.Overwrite = config.OverwritePolicy(opts.overwrite)
	}
	if seen["aspect"] {
		cfg.Aspect = opts.aspect
	}
	if seen["archive"] {
		cfg.ArchiveDir = opts.archiveDir
	}
	if seen["verify-output"] {
		cfg.VerifyOutput = opts.verifyOutput
	}
	if seen["hash-files"] {
		cfg.HashFiles = opts.hashFiles
	}
	if seen["delete-source"] {
		cfg.DeleteSource = opts.deleteSource
	}
	if seen["dry-run"] {
		cfg.DryRun = opts.dryRun
	}
	if seen["faststart"] {
		cfg.Faststart = opts.faststart
	}
	if seen["genpts"] {
		cfg.GenPTS = opts.genpts
	}
	if seen["ext"] {
		cfg.Extensions = splitList(opts.extensions)
	}
	if seen["log"] {
		cfg.LogFile = opts.logPath
	}
	if seen["ffmpeg"] {
		cfg.FFmpegPath = opts.ffmpegPath
	}
	if seen["ffprobe"] {
		cfg.FFprobePath = opts.ffprobePath
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return cfg, configPath, err
	}
	return cfg, configPath, nil
}

func cmdConvert(args []string, style term.Style) int {
	opts := &convertOptions{}
	fs := newFlagSet("convert")
	registerConvertFlags(fs, opts)
	if code, ok := parseFlags(fs, args, convertUsage, style); !ok {
		return code
	}

	cfg, configPath, err := resolveConvertOptions(fs, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	return runBatch(cfg, configPath, fs.Args(), opts, style)
}

func runBatch(cfg config.Config, configPath string, paths []string, opts *convertOptions, style term.Style) int {
	tool, err := ffmpeg.Locate(cfg.FFmpegPath, cfg.FFprobePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		fmt.Fprintf(os.Stderr, "%s Install FFmpeg with: winget install Gyan.FFmpeg\n", style.Dim("hint:"))
		return 3
	}

	manager := engine.NewManager(tool)
	subID, events, _ := manager.Subscribe()
	defer manager.Unsubscribe(subID)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !opts.json && !opts.quiet {
		fmt.Printf("%s %s  %s\n", style.Bold("FFBatch"), version.Short(),
			style.Dim("· "+title(string(cfg.Mode))+" · "+strings.ToUpper(cfg.Format)))
		fmt.Printf("  %s %s\n", style.Dim("source "), displayPaths(paths))
		fmt.Printf("  %s %s\n\n", style.Dim("config "), configPath)
	}

	snapshot, err := manager.Start(ctx, cfg, paths)
	if err != nil {
		if opts.json {
			writeJSON(map[string]any{"type": "error", "message": err.Error()})
		}
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}

	renderer := &renderer{
		style:   style,
		json:    opts.json,
		quiet:   opts.quiet,
		total:   snapshot.Total,
		workers: cfg.Workers,
	}
	if !opts.json && !opts.quiet {
		printRunHeader(snapshot, style)
	}

	code := renderer.consume(manager, snapshot.ID, events)

	if opts.reportPath != "" {
		if err := writeReport(manager, snapshot.ID, opts.reportPath); err != nil {
			fmt.Fprintf(os.Stderr, "%s report: %v\n", style.Yellow("warning:"), err)
		} else if !opts.json && !opts.quiet {
			fmt.Printf("  %s %s\n\n", style.Dim("report"), opts.reportPath)
		}
	}
	return code
}

func rendererExitCode(snapshot *engine.RunSnapshot) int {
	if snapshot == nil {
		return 1
	}
	if snapshot.Canceled > 0 || snapshot.Status == engine.StatusCanceled {
		return 130
	}
	if snapshot.Failed > 0 || snapshot.Status == engine.StatusFailed || snapshot.Status == engine.StatusPartial {
		return 1
	}
	return 0
}

func printRunHeader(snapshot *engine.RunSnapshot, style term.Style) {
	overwrite := snapshot.Config.Overwrite
	mode := title(string(snapshot.Mode))
	fmt.Printf("  %s %d %s · %s · %s\n",
		style.Dim("plan  "), snapshot.Total, countWord(snapshot.Total, "file", "files"), units.Bytes(snapshot.BytesIn), mode+" → "+strings.ToUpper(snapshot.Format))
	fmt.Printf("  %s %d %s · overwrite %s\n\n",
		style.Dim("engine"), snapshot.Workers, countWord(snapshot.Workers, "worker", "workers"), overwrite)
}

type renderer struct {
	style   term.Style
	json    bool
	quiet   bool
	total   int
	workers int
	inline  bool
}

func (r *renderer) consume(manager *engine.Manager, runID string, events <-chan engine.Event) int {
	var final *engine.RunSnapshot
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
loop:
	for {
		select {
		case event := <-events:
			if event.Run != nil && event.Run.ID != runID {
				continue
			}
			if r.json {
				writeJSON(event)
			} else {
				r.printEvent(event)
			}
			if event.Type == "run.finished" {
				final = r.snapshotOr(manager, runID, event)
				break loop
			}
		case <-ticker.C:
			// Guard against a dropped terminal event on a very busy run.
			if manager.Active() == nil {
				if snapshot, ok := manager.Snapshot(runID); ok && snapshot.FinishedAt != nil {
					final = snapshot
					break loop
				}
			}
		}
	}
	if final == nil {
		final, _ = manager.Snapshot(runID)
	}
	if final == nil {
		return 1
	}
	if !r.json {
		r.clearInline()
		r.printSummary(final)
	}
	return rendererExitCode(final)
}

func (r *renderer) snapshotOr(manager *engine.Manager, runID string, event engine.Event) *engine.RunSnapshot {
	if snap, ok := manager.Snapshot(runID); ok {
		return snap
	}
	return &engine.RunSnapshot{RunSummary: *event.Run}
}

func (r *renderer) printEvent(event engine.Event) {
	if event.Run != nil && event.Type == "run.canceling" {
		r.clearInline()
		fmt.Println(r.style.Yellow("  cancelling…"))
		return
	}
	switch event.Type {
	case "log":
		if event.Level == "warn" || event.Level == "error" {
			if isJobLog(event.Message) {
				return
			}
			r.clearInline()
			fmt.Printf("  %s %s\n", r.style.Yellow("!"), event.Message)
		}
	case "job.started":
		if r.quiet {
			return
		}
		r.clearInline()
		index := jobNumber(event.Job.ID)
		fmt.Printf("  %s %s\n", r.style.Dim(fmt.Sprintf("[%d/%d]", index, r.total)), event.Job.Name)
	case "job.progress":
		if r.quiet || r.workers != 1 || !r.style.Enabled() || event.Job == nil {
			return
		}
		r.printInline(event.Job)
	case "job.done":
		r.clearInline()
		if r.quiet || event.Job == nil {
			return
		}
		speed := ""
		if event.Job.Speed > 0.01 {
			speed = fmt.Sprintf(" · %.1f×", event.Job.Speed)
		}
		elapsed := ""
		if event.Job.StartedAt != nil && event.Job.FinishedAt != nil {
			elapsed = " · " + units.Duration(event.Job.FinishedAt.Sub(*event.Job.StartedAt))
		}
		fmt.Printf("  %s %s  %s%s%s\n", r.style.Green("✓"), event.Job.Name,
			units.Bytes(event.Job.OutputBytes), elapsed, speed)
	case "job.skipped":
		r.clearInline()
		if r.quiet {
			return
		}
		fmt.Printf("  %s %s  %s\n", r.style.Yellow("–"), event.Job.Name, r.style.Dim(event.Job.Reason))
	case "job.failed":
		r.clearInline()
		fmt.Printf("  %s %s\n", r.style.Red("✗"), r.style.Bold(event.Job.Name))
		fmt.Printf("    %s\n", r.style.Red(event.Job.Error))
	case "job.canceled":
		r.clearInline()
		fmt.Printf("  %s %s\n", r.style.Gray("·"), r.style.Dim(event.Job.Name+" cancelled"))
	case "job.planned":
		r.clearInline()
		if r.quiet {
			return
		}
		fmt.Printf("  %s %s  %s\n", r.style.Cyan("◇"), event.Job.Name, r.style.Dim("planned"))
	}
}

func (r *renderer) printInline(job *engine.Job) {
	width := 22
	filled := int(job.Progress / 100 * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	details := ""
	if job.Speed > 0.01 {
		details += fmt.Sprintf("  %.1f×", job.Speed)
	}
	if job.ETASeconds > 0 {
		details += "  ETA " + units.Seconds(job.ETASeconds)
	}
	fmt.Printf("\r  [%s] %5.1f%%%s\x1b[K", bar, job.Progress, details)
	r.inline = true
}

func (r *renderer) clearInline() {
	if r.inline {
		fmt.Print("\r\x1b[2K")
		r.inline = false
	}
}

func (r *renderer) printSummary(snapshot *engine.RunSnapshot) {
	if !r.quiet {
		fmt.Println()
	}
	if snapshot.DryRun {
		fmt.Printf("  %s %d %s planned · %s · nothing was written (dry run)\n",
			r.style.Cyan("◇"), snapshot.Planned, countWord(snapshot.Planned, "file", "files"), units.Bytes(snapshot.BytesIn))
		if snapshot.Failed > 0 {
			fmt.Printf("  %s %d could not be planned\n", r.style.Red("✗"), snapshot.Failed)
		}
		return
	}
	parts := []string{
		r.style.Green(fmt.Sprintf("%d converted", snapshot.Done)),
		r.style.Yellow(fmt.Sprintf("%d skipped", snapshot.Skipped)),
	}
	if snapshot.Failed > 0 {
		parts = append(parts, r.style.Red(fmt.Sprintf("%d failed", snapshot.Failed)))
	}
	if snapshot.Canceled > 0 {
		parts = append(parts, r.style.Gray(fmt.Sprintf("%d cancelled", snapshot.Canceled)))
	}
	if snapshot.Verified > 0 {
		parts = append(parts, r.style.Cyan(fmt.Sprintf("%d verified", snapshot.Verified)))
	}
	elapsed := time.Since(snapshot.StartedAt)
	if snapshot.FinishedAt != nil {
		elapsed = snapshot.FinishedAt.Sub(snapshot.StartedAt)
	}
	fmt.Printf("  %s\n", strings.Join(parts, r.style.Dim(" · ")))
	fmt.Printf("  %s %s → %s   %s\n",
		r.style.Dim("data"), units.Bytes(snapshot.BytesIn), units.Bytes(snapshot.BytesOut), units.Duration(elapsed))
	if snapshot.Status == engine.StatusCanceled {
		fmt.Printf("  %s\n", r.style.Yellow("Cancelled by user"))
	}
	if snapshot.Done == 0 && snapshot.Planned == 0 && snapshot.Skipped == snapshot.Total && snapshot.Total > 0 {
		fmt.Printf("  %s\n", r.style.Dim("Everything was skipped. Use --overwrite overwrite or --overwrite number to re-create outputs."))
	}
	fmt.Println()
}

func isJobLog(message string) bool {
	prefixes := []string{"✓", "✗", "Skipped ", "Cancelled ", "Dry run:", "Run finished", "Run cancelled", "Cancellation requested"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}

func jobNumber(id string) int {
	if i := strings.LastIndexByte(id, '-'); i >= 0 {
		if n, err := strconv.Atoi(id[i+1:]); err == nil {
			return n
		}
	}
	return 0
}

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func writeReport(manager *engine.Manager, runID, path string) error {
	format := "json"
	if strings.EqualFold(filepath.Ext(path), ".csv") {
		format = "csv"
	}
	data, _, err := manager.Report(runID, format)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func recipeIDs() string {
	recipes := config.Recipes()
	ids := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		ids = append(ids, recipe.ID)
	}
	return strings.Join(ids, ", ")
}

func displayPaths(paths []string) string {
	if len(paths) == 0 {
		wd, _ := os.Getwd()
		return wd
	}
	if len(paths) == 1 {
		return paths[0]
	}
	return fmt.Sprintf("%s (+%d more)", paths[0], len(paths)-1)
}

func title(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func countWord(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
