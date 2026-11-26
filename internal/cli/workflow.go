package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/workflow"
)

func cmdWorkflow(args []string, style term.Style) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, workflowUsage)
		return 2
	}
	switch args[0] {
	case "list", "ls":
		return workflowList(args[1:], style)
	case "show":
		return workflowShow(args[1:], style)
	case "init":
		return workflowInit(args[1:], style)
	case "run":
		return workflowRun(args[1:], style)
	case "help", "-h", "--help":
		fmt.Print(workflowUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "%s unknown workflow command %q\n\n%s", style.Red("error:"), args[0], workflowUsage)
		return 2
	}
}

func workflowList(args []string, style term.Style) int {
	fs := newFlagSet("workflow list")
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "")
	if code, ok := parseFlags(fs, args, workflowUsage, style); !ok {
		return code
	}
	builtins := workflow.Builtins()
	if asJSON {
		writeJSON(builtins)
		return 0
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 5, 2, ' ', 0)
	fmt.Fprintln(writer, style.Dim("WORKFLOW\tSTEPS\tDESCRIPTION"))
	for _, wf := range builtins {
		fmt.Fprintf(writer, "%s\t%d\t%s\n", wf.Name, len(wf.Steps), wf.Description)
	}
	_ = writer.Flush()
	fmt.Printf("\n%s\n", style.Dim("Run one with: ffbatch workflow run NAME --input FILE --output DIR"))
	fmt.Printf("%s\n", style.Dim("Inspect or edit one with: ffbatch workflow show NAME | ffbatch workflow init NAME"))
	return 0
}

func workflowShow(args []string, style term.Style) int {
	fs := newFlagSet("workflow show")
	if code, ok := parseFlags(fs, args, workflowUsage, style); !ok {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "%s workflow show needs a name or file\n", style.Red("error:"))
		return 2
	}
	wf, err := loadWorkflow(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	data, err := wf.Marshal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	fmt.Print(string(data))
	return 0
}

func workflowInit(args []string, style term.Style) int {
	fs := newFlagSet("workflow init")
	var output string
	var force bool
	fs.StringVar(&output, "output", "", "")
	fs.StringVar(&output, "out", "", "")
	fs.BoolVar(&force, "force", false, "")
	if code, ok := parseFlags(fs, args, workflowUsage, style); !ok {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "%s workflow init needs a built-in name\n", style.Red("error:"))
		return 2
	}
	wf, ok := workflow.Builtin(fs.Arg(0))
	if !ok {
		fmt.Fprintf(os.Stderr, "%s unknown built-in workflow %q\n", style.Red("error:"), fs.Arg(0))
		return 2
	}
	if output == "" {
		output = wf.Name + ".json"
	}
	if fileExists(output) && !force {
		fmt.Fprintf(os.Stderr, "%s %s already exists (use --force)\n", style.Red("error:"), output)
		return 2
	}
	if err := wf.Save(output); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	fmt.Printf("%s %s\n", style.Green("✓"), output)
	fmt.Printf("%s edit the JSON, then: ffbatch workflow run %q --input FILE --output DIR\n", style.Dim("next:"), output)
	return 0
}

func workflowRun(args []string, style term.Style) int {
	fs := newFlagSet("workflow run")
	var (
		configPath string
		input      string
		outputDir  string
		workDir    string
		variables  string
		overwrite  bool
		keepWork   bool
		asJSON     bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&input, "input", "", "")
	fs.StringVar(&input, "in", "", "")
	fs.StringVar(&outputDir, "output", "", "")
	fs.StringVar(&outputDir, "out", "", "")
	fs.StringVar(&workDir, "work", "", "")
	fs.StringVar(&variables, "var", "", "")
	fs.BoolVar(&overwrite, "overwrite", false, "")
	fs.BoolVar(&keepWork, "keep-work", false, "")
	fs.BoolVar(&asJSON, "json", false, "")
	if code, ok := parseFlags(fs, args, workflowUsage, style); !ok {
		return code
	}
	if fs.NArg() != 1 || input == "" {
		fmt.Fprintf(os.Stderr, "%s workflow run needs a name/file and --input FILE\n\n%s", style.Red("error:"), workflowUsage)
		return 2
	}
	wf, err := loadWorkflow(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
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
	vars := parseVariables(variables)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logf := func(level, message string) {
		if asJSON {
			return
		}
		switch level {
		case "step":
			fmt.Printf("  %s %s\n", style.Cyan("▸"), message)
		case "done":
			fmt.Printf("    %s %s\n", style.Green("✓"), message)
		case "warn":
			fmt.Printf("  %s %s\n", style.Yellow("!"), message)
		case "info":
			fmt.Printf("  %s %s\n", style.Dim("·"), message)
		}
	}
	if !asJSON {
		fmt.Printf("%s %s\n", style.Bold("Workflow"), wf.Name)
	}
	report, err := workflow.Run(ctx, wf, workflow.Options{
		Input:     input,
		OutputDir: outputDir,
		WorkDir:   workDir,
		Overwrite: overwrite,
		KeepWork:  keepWork,
		Vars:      vars,
		ToolPaths: cfg.Tools,
		FFmpeg:    tool,
		Log:       logf,
	})
	if report != nil && asJSON {
		writeJSON(report)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 1
	}
	if !asJSON {
		fmt.Printf("\n%s %s · %d steps · output %s\n", style.Green("✓"), report.Status, len(report.Steps), report.OutputDir)
		if report.WorkDir != "" && keepWork {
			fmt.Printf("  %s %s\n", style.Dim("work   "), report.WorkDir)
		}
	}
	if report.Status == "partial" {
		return 1
	}
	return 0
}

func loadWorkflow(nameOrFile string) (workflow.Workflow, error) {
	if info, err := os.Stat(nameOrFile); err == nil && !info.IsDir() {
		return workflow.Load(nameOrFile)
	}
	if wf, ok := workflow.Builtin(nameOrFile); ok {
		return wf, nil
	}
	return workflow.Workflow{}, fmt.Errorf("workflow %q not found (use a built-in name or a .json file)", nameOrFile)
}

func parseVariables(value string) map[string]any {
	out := map[string]any{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return out
}
