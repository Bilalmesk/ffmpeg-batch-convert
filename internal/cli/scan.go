package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
)

func cmdScan(args []string, style term.Style) int {
	fs := newFlagSet("scan")
	var (
		configPath string
		extensions string
		mode       string
		recursive  bool
		asJSON     bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&extensions, "ext", "", "")
	fs.StringVar(&mode, "mode", "", "")
	fs.BoolVar(&recursive, "recursive", false, "")
	fs.BoolVar(&asJSON, "json", false, "")
	if code, ok := parseFlags(fs, args, scanUsage, style); !ok {
		return code
	}

	cfg, _, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	seen := visited(fs)
	if seen["recursive"] {
		cfg.Recursive = recursive
	}
	if seen["ext"] {
		cfg.Extensions = splitList(extensions)
	}
	if seen["mode"] {
		cfg.Mode = config.Mode(mode)
	}
	cfg.Normalize()

	root := "."
	if fs.NArg() > 1 {
		fmt.Fprintf(os.Stderr, "%s scan takes at most one folder\n", style.Red("error:"))
		return 2
	}
	if fs.NArg() == 1 {
		root = fs.Arg(0)
	}
	if !filepath.IsAbs(root) {
		abs, err := filepath.Abs(root)
		if err == nil {
			root = abs
		}
	}

	extensionsForScan := engine.ScanExtensions(cfg)
	candidates, err := engine.Scan(root, cfg.Recursive, extensionsForScan)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}

	if asJSON {
		writeJSON(map[string]any{
			"root":       root,
			"recursive":  cfg.Recursive,
			"extensions": extensionsForScan,
			"count":      len(candidates),
			"files":      candidates,
		})
		return 0
	}

	var total int64
	for _, candidate := range candidates {
		total += candidate.SizeBytes
	}
	fmt.Printf("%s\n", style.Bold(root))
	fmt.Printf("%d file(s) · %s · extensions: %s\n\n", len(candidates), units.Bytes(total), joinExtensions(extensionsForScan))
	if len(candidates) == 0 {
		return 0
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, style.Dim("  SIZE\tEXT\tNAME"))
	for _, candidate := range candidates {
		fmt.Fprintf(writer, "  %s\t%s\t%s\n", units.Bytes(candidate.SizeBytes), candidate.Ext, candidate.Name)
	}
	_ = writer.Flush()
	return 0
}

func cmdProbe(args []string, style term.Style) int {
	fs := newFlagSet("probe")
	var (
		configPath string
		asJSON     bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.BoolVar(&asJSON, "json", false, "")
	if code, ok := parseFlags(fs, args, probeUsage, style); !ok {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "%s probe needs exactly one file\n\n%s", style.Red("error:"), probeUsage)
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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	info, err := mediainfo.Probe(ctx, tool.FFprobe, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 1
	}
	if asJSON {
		writeJSON(info)
		return 0
	}

	label := func(text string) string { return style.Dim(fmt.Sprintf("%-10s", text)) }
	fmt.Printf("%s %s\n", label("file"), info.Path)
	fmt.Printf("%s %s\n", label("container"), info.Format)
	fmt.Printf("%s %s\n", label("duration"), units.Clock(info.DurationMs))
	fmt.Printf("%s %s\n", label("size"), units.Bytes(info.SizeBytes))
	if info.Bitrate > 0 {
		fmt.Printf("%s %s\n", label("bitrate"), fmt.Sprintf("%d kb/s", info.Bitrate/1000))
	}
	if info.Video != nil {
		fmt.Printf("%s %s · %dx%d · %.2f fps\n", label("video"), info.Video.Codec, info.Video.Width, info.Video.Height, info.Video.FPS)
	}
	for i, audio := range info.Audios {
		name := "audio"
		if i > 0 {
			name = fmt.Sprintf("audio %d", i+1)
		}
		extra := ""
		if audio.Language != "" {
			extra = " · " + audio.Language
		}
		fmt.Printf("%s %s · %dch%s\n", label(name), audio.Codec, audio.Channels, extra)
	}
	for i, subtitle := range info.Subtitles {
		name := "subtitle"
		if i > 0 {
			name = fmt.Sprintf("subtitle %d", i+1)
		}
		extra := ""
		if subtitle.Language != "" {
			extra = " · " + subtitle.Language
		}
		fmt.Printf("%s %s%s\n", label(name), subtitle.Codec, extra)
	}
	return 0
}

func joinExtensions(extensions []string) string {
	out := ""
	for i, ext := range extensions {
		if i > 0 {
			out += " "
		}
		out += "*." + ext
	}
	return out
}
