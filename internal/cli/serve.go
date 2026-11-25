package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/version"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/web"
)

func cmdServe(args []string, style term.Style) int {
	fs := newFlagSet("serve")
	var (
		configPath string
		host       string
		open       bool
		insecure   bool
		token      string
		port       int
		watchPoll  int
		watchWait  int
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&host, "host", "127.0.0.1", "")
	fs.StringVar(&token, "token", "", "")
	fs.BoolVar(&insecure, "insecure", false, "")
	fs.IntVar(&port, "port", 8737, "")
	fs.IntVar(&watchPoll, "watch-interval", 5, "")
	fs.IntVar(&watchWait, "watch-settle", 10, "")
	fs.BoolVar(&open, "open", false, "")
	if code, ok := parseFlags(fs, args, serveUsage, style); !ok {
		return code
	}
	if port < 0 || port > 65535 {
		fmt.Fprintf(os.Stderr, "%s port must be between 0 and 65535\n", style.Red("error:"))
		return 2
	}
	if watchPoll < 1 || watchPoll > 3600 {
		fmt.Fprintf(os.Stderr, "%s watch-interval must be between 1 and 3600 seconds\n", style.Red("error:"))
		return 2
	}
	if watchWait < 0 || watchWait > 86400 {
		fmt.Fprintf(os.Stderr, "%s watch-settle must be between 0 and 86400 seconds\n", style.Red("error:"))
		return 2
	}
	if token == "" {
		token = os.Getenv("FFBATCH_TOKEN")
	}
	if token != "" && len(token) < 16 {
		fmt.Fprintf(os.Stderr, "%s the dashboard token must be at least 16 characters\n", style.Red("error:"))
		return 2
	}
	if !isLoopbackHost(host) && token == "" && !insecure {
		fmt.Fprintf(os.Stderr, "%s refusing to expose the dashboard on %s without a token\n", style.Red("error:"), host)
		fmt.Fprintf(os.Stderr, "%s set FFBATCH_TOKEN or --token, or pass --insecure on a trusted network\n", style.Dim("hint:"))
		return 2
	}
	cfg, configPath, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}

	tool, locateErr := ffmpeg.Locate(cfg.FFmpegPath, cfg.FFprobePath)
	if locateErr == nil {
		versionCtx, versionCancel := context.WithTimeout(context.Background(), 10*time.Second)
		tool.DetectVersion(versionCtx)
		versionCancel()
	}
	toolError := ""
	if locateErr != nil {
		toolError = locateErr.Error()
	}
	manager := engine.NewManager(tool)

	address := net.JoinHostPort(host, strconv.Itoa(port))
	server, err := web.New(web.Options{
		Address:       address,
		Config:        cfg,
		ConfigPath:    configPath,
		Manager:       manager,
		Tool:          tool,
		ToolError:     toolError,
		WatchInterval: time.Duration(watchPoll) * time.Second,
		WatchSettle:   time.Duration(watchWait) * time.Second,
		Version:       version.Version,
		Commit:        version.Commit,
		Token:         token,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 2
	}
	listener, err := server.Listen()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s cannot listen on %s: %v\n", style.Red("error:"), address, err)
		fmt.Fprintf(os.Stderr, "%s try --port 0 to pick a free port\n", style.Dim("hint:"))
		return 1
	}
	url := web.URL(listener)

	fmt.Printf("%s %s  %s\n\n", style.Bold("FFBatch"), version.Short(), style.Dim("local dashboard"))
	fmt.Printf("  %s %s\n", style.Dim("dashboard"), style.Cyan(url))
	fmt.Printf("  %s %s\n", style.Dim("config   "), configPath)
	if locateErr != nil {
		fmt.Printf("  %s %s\n", style.Dim("ffmpeg   "), style.Red("not found — install it from the Doctor tab"))
	} else {
		versionText := tool.Version
		if versionText == "" {
			versionText = tool.FFmpeg
		}
		fmt.Printf("  %s %s\n", style.Dim("ffmpeg   "), versionText)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		fmt.Printf("\n  %s the dashboard is exposed beyond loopback (%s)\n", style.Yellow("warning:"), host)
	}
	if token != "" {
		fmt.Printf("  %s token required (cookie is set after the first ?token= visit)\n", style.Dim("security "))
	}
	fmt.Printf("\n  %s\n\n", style.Dim("Press Ctrl+C to stop."))

	if open {
		if err := openBrowser(web.TokenizedURL(url, token)); err != nil {
			fmt.Fprintf(os.Stderr, "%s could not open the browser: %v\n", style.Yellow("warning:"), err)
		}
	}

	server.StartWatcher()

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
			return 1
		}
	}

	fmt.Println(style.Dim("\nShutting down…"))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Yellow("warning:"), err)
	}
	manager.Shutdown(shutdownCtx)
	return 0
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}
