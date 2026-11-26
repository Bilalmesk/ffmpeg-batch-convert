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

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/muxer"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/toolchain"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
)

func cmdMux(args []string, style term.Style) int {
	fs := newFlagSet("mux")
	var (
		configPath  string
		output      string
		title       string
		chapters    string
		languages   string
		trackNames  string
		toolName    string
		noAudio     bool
		noVideo     bool
		noSubtitles bool
		noChapters  bool
		overwrite   bool
		dryRun      bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&output, "output", "", "")
	fs.StringVar(&output, "out", "", "")
	fs.StringVar(&title, "title", "", "")
	fs.StringVar(&chapters, "chapters", "", "")
	fs.StringVar(&languages, "languages", "", "")
	fs.StringVar(&trackNames, "track-names", "", "")
	fs.StringVar(&toolName, "tool", "mkvmerge", "")
	fs.BoolVar(&noAudio, "no-audio", false, "")
	fs.BoolVar(&noVideo, "no-video", false, "")
	fs.BoolVar(&noSubtitles, "no-subtitles", false, "")
	fs.BoolVar(&noChapters, "no-chapters", false, "")
	fs.BoolVar(&overwrite, "overwrite", false, "")
	fs.BoolVar(&dryRun, "dry-run", false, "")
	if code, ok := parseFlags(fs, args, muxUsage, style); !ok {
		return code
	}
	if fs.NArg() != 1 || output == "" {
		fmt.Fprintf(os.Stderr, "%s mux needs one input and --output FILE.mkv\n\n%s", style.Red("error:"), muxUsage)
		return 2
	}
	if toolName != "mkvmerge" {
		fmt.Fprintf(os.Stderr, "%s only the mkvmerge backend is available right now\n", style.Red("error:"))
		return 2
	}
	languageOptions, err := muxer.ParseOptions(languages)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s --languages: %v\n", style.Red("error:"), err)
		return 2
	}
	trackNameOptions, err := muxer.ParseOptions(trackNames)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s --track-names: %v\n", style.Red("error:"), err)
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
	mkvTool, _ := toolchain.Resolve(ctx, "mkvmerge", cfg.Tools)
	if !mkvTool.Available {
		fmt.Fprintf(os.Stderr, "%s mkvmerge was not found\n", style.Red("error:"))
		fmt.Fprintf(os.Stderr, "%s install MKVToolNix: winget install MoritzBunkus.MKVToolNix\n", style.Dim("hint:"))
		return 3
	}
	mkvmerge := mkvTool.Path
	probeTool, _ := toolchain.Resolve(ctx, "ffprobe", cfg.Tools)
	ffprobe := cfg.FFprobePath
	if ffprobe == "" || ffprobe == "ffprobe" {
		if probeTool.Available {
			ffprobe = probeTool.Path
		}
	}

	request := muxer.Request{
		Input:       fs.Arg(0),
		Output:      output,
		Title:       title,
		Chapters:    chapters,
		NoAudio:     noAudio,
		NoVideo:     noVideo,
		NoSubtitles: noSubtitles,
		NoChapters:  noChapters,
		Languages:   languageOptions,
		TrackNames:  trackNameOptions,
	}
	if !dryRun && fileExists(output) && !overwrite {
		fmt.Fprintf(os.Stderr, "%s output already exists: %s (use --overwrite)\n", style.Red("error:"), output)
		return 2
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "%s create output folder: %v\n", style.Red("error:"), err)
		return 2
	}
	ext := filepath.Ext(output)
	base := strings.TrimSuffix(filepath.Base(output), ext)
	temp := filepath.Join(filepath.Dir(output), "."+base+".ffbatch-mux.part"+ext)
	_ = os.Remove(temp)
	request.Output = temp
	cmdArgs, command, err := muxer.Build(mkvmerge, request)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	displayCommand := strings.Replace(command, temp, output, 1)
	fmt.Printf("%s %s\n", style.Dim("command"), displayCommand)
	if dryRun {
		return 0
	}
	if err := runExternal(ctx, mkvmerge, cmdArgs, os.Stdout, os.Stderr); err != nil {
		_ = os.Remove(temp)
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 1
	}
	if overwrite && fileExists(output) {
		_ = os.Remove(output)
	}
	if err := os.Rename(temp, output); err != nil {
		_ = os.Remove(temp)
		fmt.Fprintf(os.Stderr, "%s finalise output: %v\n", style.Red("error:"), err)
		return 1
	}

	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	info, err := mediainfo.Probe(probeCtx, ffprobe, output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s muxed file could not be probed: %v\n", style.Yellow("warning:"), err)
		return 0
	}
	fmt.Printf("%s %s · %s · %s\n", style.Green("✓"), output, units.Bytes(info.SizeBytes), describeMedia(info))
	return 0
}
