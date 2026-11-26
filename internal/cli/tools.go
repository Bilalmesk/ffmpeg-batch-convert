package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/toolchain"
)

func cmdTools(args []string, style term.Style) int {
	fs := newFlagSet("tools")
	var (
		configPath string
		asJSON     bool
		missing    bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.BoolVar(&asJSON, "json", false, "")
	fs.BoolVar(&missing, "missing", false, "")
	if code, ok := parseFlags(fs, args, toolsUsage, style); !ok {
		return code
	}

	cfg, _, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	cfg.Normalize()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	registry := toolchain.Detect(ctx, cfg.Tools)

	list := registry.List()
	if missing {
		filtered := list[:0]
		for _, tool := range list {
			if !tool.Available {
				filtered = append(filtered, tool)
			}
		}
		list = filtered
	}
	if asJSON {
		writeJSON(list)
		return 0
	}
	if len(list) == 0 {
		fmt.Println(style.Dim("No matching tools."))
		return 0
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, style.Dim("TOOL\tCATEGORY\tSTATUS\tVERSION / HINT"))
	for _, tool := range list {
		status := style.Green("ready")
		detail := tool.Version
		if !tool.Available {
			status = style.Yellow("missing")
			detail = tool.Hint
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", tool.Name, tool.Category, status, detail)
	}
	_ = writer.Flush()

	core := registry.Available("ffmpeg", "ffprobe")
	if !core {
		fmt.Printf("\n%s FFmpeg/FFprobe are required. Run %s for details.\n", style.Red("!"), style.Bold("ffbatch doctor"))
	}
	return 0
}
