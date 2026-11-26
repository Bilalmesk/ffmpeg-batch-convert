package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/packager"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/toolchain"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
)

func cmdPackage(args []string, style term.Style) int {
	fs := newFlagSet("package")
	var (
		configPath string
		outputDir  string
		formatName string
		toolName   string
		segment    int
		dryRun     bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&outputDir, "out", "", "")
	fs.StringVar(&outputDir, "output", "", "")
	fs.StringVar(&formatName, "format", "hls", "")
	fs.StringVar(&toolName, "tool", "auto", "")
	fs.IntVar(&segment, "segment", 6, "")
	fs.BoolVar(&dryRun, "dry-run", false, "")
	if code, ok := parseFlags(fs, args, packageUsage, style); !ok {
		return code
	}
	if fs.NArg() != 1 || outputDir == "" {
		fmt.Fprintf(os.Stderr, "%s package needs one input and --out DIR\n\n%s", style.Red("error:"), packageUsage)
		return 2
	}

	cfg, _, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	cfg.Normalize()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	request := packager.Request{
		Input:          fs.Arg(0),
		OutputDir:      outputDir,
		Format:         packager.Format(formatName),
		SegmentSeconds: segment,
		Tool:           packager.Tool(toolName),
	}
	var selected packager.Tool
	if request.Tool == "" || request.Tool == "auto" {
		selected = packager.ToolFFmpeg
	} else {
		selected = request.Tool
	}
	switch selected {
	case packager.ToolFFmpeg:
		tool, err := ffmpeg.Locate(cfg.FFmpegPath, cfg.FFprobePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
			return 3
		}
		request.FFmpeg = tool.FFmpeg
	case packager.ToolMP4Box:
		mp4box, _ := toolchain.Resolve(ctx, "mp4box", cfg.Tools)
		if !mp4box.Available {
			fmt.Fprintf(os.Stderr, "%s MP4Box is not installed\n", style.Red("error:"))
			fmt.Fprintf(os.Stderr, "%s install GPAC, or use --tool ffmpeg / --tool bento4\n", style.Dim("hint:"))
			return 3
		}
		request.MP4Box = mp4box.Path
	case packager.ToolBento4:
		fragment, _ := toolchain.Resolve(ctx, "mp4fragment", cfg.Tools)
		dash, _ := toolchain.Resolve(ctx, "mp4dash", cfg.Tools)
		if !fragment.Available || !dash.Available {
			fmt.Fprintf(os.Stderr, "%s Bento4 (mp4fragment and mp4dash) is not installed\n", style.Red("error:"))
			fmt.Fprintf(os.Stderr, "%s download Bento4, or use --tool ffmpeg\n", style.Dim("hint:"))
			return 3
		}
		request.MP4Fragment = fragment.Path
		request.MP4Dash = dash.Path
	}

	plan, err := packager.Build(request)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	fmt.Printf("%s %s · %s · %ds segments\n", style.Bold("Package"), formatName, selected, segment)
	fmt.Printf("  %s %s\n", style.Dim("input  "), request.Input)
	fmt.Printf("  %s %s\n", style.Dim("output "), request.OutputDir)
	for _, note := range plan.Notes {
		fmt.Printf("  %s %s\n", style.Yellow("note   "), note)
	}
	for _, command := range plan.Commands {
		fmt.Printf("  %s %s\n", style.Dim("command"), packager.CommandString(command))
	}
	if dryRun {
		return 0
	}

	for _, command := range plan.Commands {
		if err := runExternalIn(ctx, command.Binary, command.Args, command.Dir, os.Stdout, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
			return 1
		}
	}
	manifest, err := findManifest(plan.Manifest, request.OutputDir, formatName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s command finished, but no manifest was found: %v\n", style.Red("error:"), err)
		fmt.Fprintf(os.Stderr, "%s check the output folder %s\n", style.Dim("hint:"), request.OutputDir)
		return 1
	}
	info, err := os.Stat(manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s manifest: %v\n", style.Red("error:"), err)
		return 1
	}
	fmt.Printf("%s %s · %s\n", style.Green("✓"), manifest, units.Bytes(info.Size()))
	return 0
}

// findManifest tolerates backend-specific manifest names while still verifying
// that packaging produced something playable.
func findManifest(expected, dir, formatName string) (string, error) {
	if fileExists(expected) {
		return expected, nil
	}
	ext := ".mpd"
	if formatName == "hls" {
		ext = ".m3u8"
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*"+ext))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("expected %s or another *%s file", expected, ext)
	}
	return matches[0], nil
}
