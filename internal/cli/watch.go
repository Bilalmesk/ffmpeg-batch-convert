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

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/version"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/watch"
)

func cmdWatch(args []string, style term.Style) int {
	opts := &convertOptions{}
	fs := newFlagSet("watch")
	registerConvertFlags(fs, opts)
	var interval, settle int
	var existing bool
	fs.IntVar(&interval, "interval", 5, "")
	fs.IntVar(&settle, "settle", 10, "")
	fs.BoolVar(&existing, "existing", false, "")
	if code, ok := parseFlags(fs, args, watchUsage, style); !ok {
		return code
	}
	if interval < 1 || interval > 3600 {
		fmt.Fprintf(os.Stderr, "%s interval must be between 1 and 3600 seconds\n", style.Red("error:"))
		return 2
	}
	if settle < 0 || settle > 86400 {
		fmt.Fprintf(os.Stderr, "%s settle must be between 0 and 86400 seconds\n", style.Red("error:"))
		return 2
	}

	cfg, configPath, err := resolveConvertOptions(fs, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}

	tool, err := ffmpeg.Locate(cfg.FFmpegPath, cfg.FFprobePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		fmt.Fprintf(os.Stderr, "%s Install FFmpeg with: winget install Gyan.FFmpeg\n", style.Dim("hint:"))
		return 3
	}

	manager := engine.NewManager(tool)
	onEvent := func(event watch.Event) {
		if opts.json {
			writeJSON(map[string]any{
				"type":     "watch.event",
				"at":       event.At,
				"level":    event.Level,
				"targetId": event.TargetID,
				"message":  event.Message,
			})
			return
		}
		if opts.quiet && event.Level == "info" {
			return
		}
		marker := style.Gray("·")
		switch event.Level {
		case "warn":
			marker = style.Yellow("!")
		case "error":
			marker = style.Red("✗")
		}
		fmt.Printf("  %s %s\n", marker, event.Message)
	}

	watcher := watch.New(watch.Options{
		Interval: time.Duration(interval) * time.Second,
		Settle:   time.Duration(settle) * time.Second,
		Config:   func() config.Config { return cfg },
		Busy:     func() bool { return manager.Active() != nil },
		OnEvent:  onEvent,
		Launch: func(launchCtx context.Context, runCfg config.Config, files []string) error {
			return runWatchedBatch(launchCtx, manager, runCfg, files, opts, style)
		},
	})

	if !opts.json && !opts.quiet {
		fmt.Printf("%s %s  %s\n", style.Bold("FFBatch"), version.Short(), style.Dim("· watch mode"))
		fmt.Printf("  %s %s · %s\n", style.Dim("config"), configPath, title(string(cfg.Mode))+" → "+strings.ToUpper(cfg.Format))
		fmt.Printf("  %s poll %ds · settle %ds · overwrite %s\n\n",
			style.Dim("engine"), interval, settle, cfg.Overwrite)
	}

	dirs := fs.Args()
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	added := 0
	for _, dir := range dirs {
		if _, err := watcher.Add(dir, cfg.Recursive, existing); err != nil {
			fmt.Fprintf(os.Stderr, "%s %s: %v\n", style.Yellow("warning:"), dir, err)
			continue
		}
		added++
	}
	if added == 0 {
		fmt.Fprintf(os.Stderr, "%s no folders could be watched\n", style.Red("error:"))
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !opts.json && !opts.quiet {
		fmt.Printf("  %s\n\n", style.Dim("Press Ctrl+C to stop watching."))
	}
	watcher.Start(ctx)
	<-ctx.Done()
	watcher.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = manager.Shutdown(shutdownCtx)

	if !opts.json && !opts.quiet {
		fmt.Printf("\n%s stopped watching %d %s\n", style.Green("✓"), added, countWord(added, "folder", "folders"))
	}
	return 0
}

func runWatchedBatch(ctx context.Context, manager *engine.Manager, cfg config.Config, files []string, opts *convertOptions, style term.Style) error {
	subID, events, _ := manager.Subscribe()
	defer manager.Unsubscribe(subID)

	snapshot, err := manager.Start(ctx, cfg, files)
	if err != nil {
		return err
	}
	if !opts.json && !opts.quiet {
		printRunHeader(snapshot, style)
	}
	renderer := &renderer{
		style:   style,
		json:    opts.json,
		quiet:   opts.quiet,
		total:   snapshot.Total,
		workers: cfg.Workers,
	}
	renderer.consume(manager, snapshot.ID, events)

	if opts.reportPath != "" {
		path := perRunReportPath(opts.reportPath, snapshot.ID)
		if err := writeReport(manager, snapshot.ID, path); err != nil {
			fmt.Fprintf(os.Stderr, "%s report: %v\n", style.Yellow("warning:"), err)
		} else if !opts.json && !opts.quiet {
			fmt.Printf("  %s %s\n", style.Dim("report"), path)
		}
	}
	return nil
}

func perRunReportPath(base, runID string) string {
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext) + "-" + runID + ext
}
