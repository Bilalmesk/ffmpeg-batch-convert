package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/toolchain"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ytdlp"
)

func cmdFetch(args []string, style term.Style) int {
	fs := newFlagSet("fetch")
	var (
		configPath  string
		outputDir   string
		format      string
		audioFormat string
		cookies     string
		downloader  string
		archive     string
		audioOnly   bool
		playlist    bool
		writeInfo   bool
		noArchive   bool
		restrict    bool
		dryRun      bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&outputDir, "output", "", "")
	fs.StringVar(&outputDir, "out", "", "")
	fs.StringVar(&format, "format", "", "")
	fs.StringVar(&audioFormat, "audio-format", "flac", "")
	fs.StringVar(&cookies, "cookies", "", "")
	fs.StringVar(&downloader, "downloader", "", "")
	fs.StringVar(&archive, "archive", "", "")
	fs.BoolVar(&audioOnly, "audio-only", false, "")
	fs.BoolVar(&playlist, "playlist", false, "")
	fs.BoolVar(&writeInfo, "write-info-json", false, "")
	fs.BoolVar(&noArchive, "no-archive", false, "")
	fs.BoolVar(&restrict, "restrict-filenames", false, "")
	fs.BoolVar(&dryRun, "dry-run", false, "")
	if code, ok := parseFlags(fs, args, fetchUsage, style); !ok {
		return code
	}
	if fs.NArg() == 0 || outputDir == "" {
		fmt.Fprintf(os.Stderr, "%s fetch needs at least one URL and --output DIR\n\n%s", style.Red("error:"), fetchUsage)
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
	ytTool, _ := toolchain.Resolve(ctx, "yt-dlp", cfg.Tools)
	if !ytTool.Available {
		fmt.Fprintf(os.Stderr, "%s yt-dlp was not found\n", style.Red("error:"))
		fmt.Fprintf(os.Stderr, "%s install it with: winget install yt-dlp.yt-dlp\n", style.Dim("hint:"))
		return 3
	}
	ytdlpPath := ytTool.Path
	if downloader == "aria2c" {
		ariaTool, _ := toolchain.Resolve(ctx, "aria2c", cfg.Tools)
		if !ariaTool.Available {
			fmt.Fprintf(os.Stderr, "%s aria2c was requested but is not installed\n", style.Red("error:"))
			return 3
		}
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "%s create output folder: %v\n", style.Red("error:"), err)
		return 2
	}
	if archive == "" && !noArchive {
		archive = ytdlp.DefaultArchive(outputDir)
	}
	if archive != "" {
		if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "%s create archive folder: %v\n", style.Red("error:"), err)
			return 2
		}
	}
	request := ytdlp.Request{
		URLs:         fs.Args(),
		OutputDir:    outputDir,
		Format:       format,
		AudioOnly:    audioOnly,
		AudioFormat:  audioFormat,
		Cookies:      cookies,
		Downloader:   downloader,
		Playlist:     playlist,
		WriteInfo:    writeInfo,
		Archive:      archive,
		RestrictName: restrict,
	}
	cmdArgs, err := ytdlp.Args(request)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	fmt.Printf("%s %s\n", style.Dim("command"), ffmpeg.CommandString("yt-dlp", cmdArgs))
	fmt.Printf("  %s %s\n", style.Dim("output "), outputDir)
	if dryRun {
		return 0
	}
	if err := runExternal(ctx, ytdlpPath, cmdArgs, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 1
	}
	fmt.Printf("\n%s downloaded to %s\n", style.Green("✓"), outputDir)
	fmt.Printf("%s convert the result with: ffbatch convert %q --recursive\n", style.Dim("next:"), outputDir)
	return 0
}
