package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/doctor"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/version"
)

func cmdDoctor(args []string, style term.Style) int {
	fs := newFlagSet("doctor")
	var (
		configPath string
		asJSON     bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.BoolVar(&asJSON, "json", false, "")
	if code, ok := parseFlags(fs, args, doctorUsage, style); !ok {
		return code
	}

	cfg, resolvedPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	cfg.Normalize()

	tool, locateErr := ffmpeg.Locate(cfg.FFmpegPath, cfg.FFprobePath)
	toolErr := ""
	if locateErr != nil {
		toolErr = locateErr.Error()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report := doctor.Run(ctx, tool, cfg, resolvedPath, toolErr)

	failed := 0
	for _, check := range report.Checks {
		if check.Status == "fail" {
			failed++
		}
	}

	if asJSON {
		writeJSON(report)
	} else {
		fmt.Printf("%s %s  %s\n", style.Bold("FFBatch"), version.Short(), style.Dim(report.Platform+" · "+report.GoVersion))
		fmt.Printf("%s %s\n", style.Dim("config "), report.ConfigPath)
		if report.Health.OK {
			fmt.Printf("%s %s\n\n", style.Dim("ffmpeg "), report.Health.Version)
		} else {
			fmt.Printf("%s %s\n\n", style.Dim("ffmpeg "), style.Red(report.Health.Error))
		}
		writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		for _, check := range report.Checks {
			tag := style.Green("PASS")
			if check.Status == "warn" {
				tag = style.Yellow("WARN")
			}
			if check.Status == "fail" {
				tag = style.Red("FAIL")
			}
			fmt.Fprintf(writer, "  %s  %s\t%s\n", tag, check.Label, check.Detail)
			if check.Hint != "" {
				fmt.Fprintf(writer, "\t%s\n", style.Dim(check.Hint))
			}
		}
		_ = writer.Flush()
		fmt.Println()
		if failed > 0 {
			fmt.Printf("%s %d check(s) failed\n", style.Red("✗"), failed)
		} else {
			fmt.Printf("%s environment looks healthy\n", style.Green("✓"))
		}
	}

	if locateErr != nil {
		if !asJSON {
			fmt.Fprintf(os.Stderr, "%s winget install Gyan.FFmpeg\n", style.Dim("hint:"))
		}
		return 3
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func cmdConfig(args []string, style term.Style) int {
	if len(args) == 0 {
		fmt.Print(configUsage)
		return 0
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "show":
		return configShow(rest, style)
	case "path":
		return configPath(rest, style)
	case "init":
		return configInit(rest, style)
	case "validate":
		return configValidate(rest, style)
	case "help", "-h", "--help":
		fmt.Print(configUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "%s unknown config command %q\n\n%s", style.Red("error:"), sub, configUsage)
		return 2
	}
}

func configShow(args []string, style term.Style) int {
	fs := newFlagSet("config show")
	var (
		configPath string
		asJSON     bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.BoolVar(&asJSON, "json", false, "")
	if code, ok := parseFlags(fs, args, configUsage, style); !ok {
		return code
	}
	cfg, resolvedPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	if asJSON {
		writeJSON(cfg)
		return 0
	}
	fmt.Printf("%s %s\n\n", style.Dim("config"), resolvedPath)
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	row := func(key string, value any) {
		fmt.Fprintf(writer, "  %s\t%v\n", style.Dim(key), value)
	}
	row("mode", cfg.Mode)
	row("format", cfg.Format)
	row("outputDir", emptyAs(cfg.OutputDir, "(next to source)"))
	row("workers", cfg.Workers)
	row("recursive", cfg.Recursive)
	row("overwrite", cfg.Overwrite)
	row("deleteSource", cfg.DeleteSource)
	row("extensions", joinExtensions(cfg.Extensions))
	row("videoCodec", cfg.VideoCodec)
	row("crf", cfg.CRF)
	row("preset", cfg.Preset)
	row("audio", cfg.AudioCodec+" "+cfg.AudioRate)
	row("subtitles", cfg.Subtitle)
	row("faststart", cfg.Faststart)
	row("aspect", emptyAs(cfg.Aspect, "(keep source)"))
	row("genPts", cfg.GenPTS)
	row("ffmpeg", cfg.FFmpegPath)
	row("ffprobe", cfg.FFprobePath)
	row("logFile", emptyAs(cfg.LogFile, "(disabled)"))
	_ = writer.Flush()
	return 0
}

func configPath(args []string, style term.Style) int {
	fs := newFlagSet("config path")
	var configPath string
	fs.StringVar(&configPath, "config", "", "")
	if code, ok := parseFlags(fs, args, configUsage, style); !ok {
		return code
	}
	path, err := config.ResolvePath(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	fmt.Println(path)
	return 0
}

func configInit(args []string, style term.Style) int {
	fs := newFlagSet("config init")
	var (
		target string
		force  bool
	)
	fs.StringVar(&target, "path", "", "")
	fs.BoolVar(&force, "force", false, "")
	if code, ok := parseFlags(fs, args, configUsage, style); !ok {
		return code
	}
	if target == "" {
		target = config.FileName
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	if _, err := os.Stat(abs); err == nil && !force {
		fmt.Fprintf(os.Stderr, "%s %s already exists (use --force to overwrite)\n", style.Red("error:"), abs)
		return 2
	}
	if err := config.Save(abs, config.Default()); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	fmt.Printf("%s created %s\n", style.Green("✓"), abs)
	fmt.Printf("%s edit it, then run: ffbatch convert <folder>\n", style.Dim("next:"))
	return 0
}

func configValidate(args []string, style term.Style) int {
	fs := newFlagSet("config validate")
	var (
		configPath string
		asJSON     bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.BoolVar(&asJSON, "json", false, "")
	if code, ok := parseFlags(fs, args, configUsage, style); !ok {
		return code
	}
	cfg, resolvedPath, err := loadConfig(configPath)
	if err != nil {
		if asJSON {
			writeJSON(map[string]any{"ok": false, "path": resolvedPath, "error": err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		}
		return 2
	}
	cfg.Normalize()
	validateErr := cfg.Validate()
	if asJSON {
		doc := map[string]any{"ok": validateErr == nil, "path": resolvedPath}
		if validateErr != nil {
			doc["error"] = validateErr.Error()
		}
		writeJSON(doc)
		if validateErr != nil {
			return 2
		}
		return 0
	}
	if validateErr != nil {
		fmt.Printf("%s %s\n%s %v\n", style.Red("✗"), resolvedPath, style.Dim("reason:"), validateErr)
		return 2
	}
	fmt.Printf("%s %s is valid\n", style.Green("✓"), resolvedPath)
	return 0
}

func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
