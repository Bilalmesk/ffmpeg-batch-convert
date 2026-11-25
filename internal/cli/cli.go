// Package cli implements the ffbatch command-line interface.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/version"
)

// Run executes one CLI invocation and returns the process exit code.
func Run(args []string) int {
	style := term.New()

	// --no-color is accepted anywhere on the command line.
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--no-color" || arg == "-no-color" {
			style = term.Style{}
			continue
		}
		filtered = append(filtered, arg)
	}
	args = filtered

	if len(args) == 0 {
		fmt.Print(mainUsage)
		return 0
	}

	command, rest := args[0], args[1:]
	switch command {
	case "convert":
		return cmdConvert(rest, style)
	case "watch":
		return cmdWatch(rest, style)
	case "remux":
		return cmdConvert(append([]string{"--mode", "remux"}, rest...), style)
	case "encode":
		return cmdConvert(append([]string{"--mode", "encode"}, rest...), style)
	case "rename":
		return cmdConvert(append([]string{"--mode", "rename", "--to", "ts"}, rest...), style)
	case "scan":
		return cmdScan(rest, style)
	case "probe":
		return cmdProbe(rest, style)
	case "serve", "ui", "web":
		return cmdServe(rest, style)
	case "doctor", "check":
		return cmdDoctor(rest, style)
	case "verify":
		return cmdVerify(rest, style)
	case "config":
		return cmdConfig(rest, style)
	case "version", "--version", "-version":
		fmt.Println(version.Full())
		return 0
	case "help", "-h", "--help", "-help":
		if len(rest) > 0 {
			return printCommandHelp(rest[0], style)
		}
		fmt.Print(mainUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "%s unknown command %q\n\n", style.Red("error:"), command)
		fmt.Fprint(os.Stderr, mainUsage)
		return 2
	}
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string, usage string, style term.Style) (int, bool) {
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(usage)
			return 0, false
		}
		fmt.Fprintf(os.Stderr, "%s %v\n\n", style.Red("error:"), err)
		fmt.Fprint(os.Stderr, usage)
		return 2, false
	}
	return 0, true
}

// reorderArgs lets flags appear after positional paths. The standard flag
// package stops at the first non-flag argument, which is a poor experience for
// commands like `ffbatch convert "D:\TV" --recursive`.
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))
	terminated := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !terminated && arg == "--" {
			terminated = true
			flags = append(flags, "--")
			continue
		}
		if terminated || !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			continue
		}
		if f := fs.Lookup(name); f != nil {
			if boolFlag, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && boolFlag.IsBoolFlag() {
				continue
			}
		}
		// Unknown flags are treated as value-taking so the parser can report
		// them instead of silently swallowing a path.
		if i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return append(flags, positional...)
}

func visited(fs *flag.FlagSet) map[string]bool {
	seen := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	return seen
}

func loadConfig(path string) (config.Config, string, error) {
	configPath, err := config.ResolvePath(path)
	if err != nil {
		return config.Config{}, "", err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return config.Config{}, configPath, err
	}
	return cfg, configPath, nil
}

func splitList(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		field = strings.TrimPrefix(field, ".")
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func hasAny(seen map[string]bool, names ...string) bool {
	for _, name := range names {
		if seen[name] {
			return true
		}
	}
	return false
}

func printCommandHelp(command string, style term.Style) int {
	switch command {
	case "convert", "remux", "encode", "rename":
		fmt.Print(convertUsage)
	case "watch":
		fmt.Print(watchUsage)
	case "scan":
		fmt.Print(scanUsage)
	case "probe":
		fmt.Print(probeUsage)
	case "serve", "ui", "web":
		fmt.Print(serveUsage)
	case "doctor", "check":
		fmt.Print(doctorUsage)
	case "verify":
		fmt.Print(verifyUsage)
	case "config":
		fmt.Print(configUsage)
	case "version":
		fmt.Println("Usage: ffbatch version\n\nPrint the version, commit, build date and Go runtime.")
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", command, mainUsage)
		return 2
	}
	return 0
}
