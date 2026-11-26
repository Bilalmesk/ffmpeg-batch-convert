package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/quality"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
)

func cmdQuality(args []string, style term.Style) int {
	fs := newFlagSet("quality")
	var (
		configPath string
		reference  string
		candidate  string
		metricName string
		minScore   float64
		threads    int
		asJSON     bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&reference, "reference", "", "")
	fs.StringVar(&candidate, "candidate", "", "")
	fs.StringVar(&metricName, "metric", "vmaf", "")
	fs.Float64Var(&minScore, "min", 0, "")
	fs.IntVar(&threads, "threads", 0, "")
	fs.BoolVar(&asJSON, "json", false, "")
	if code, ok := parseFlags(fs, args, qualityUsage, style); !ok {
		return code
	}
	if reference == "" || candidate == "" {
		fmt.Fprintf(os.Stderr, "%s both --reference and --candidate are required\n\n%s", style.Red("error:"), qualityUsage)
		return 2
	}
	metric, ok := quality.ParseMetric(metricName)
	if !ok {
		fmt.Fprintf(os.Stderr, "%s unknown metric %q; use vmaf, ssim or psnr\n", style.Red("error:"), metricName)
		return 2
	}
	if threads <= 0 {
		threads = runtime.NumCPU()
		if threads > 8 {
			threads = 8
		}
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	refInfo, refErr := mediainfo.Probe(probeCtx, tool.FFprobe, reference)
	candInfo, candErr := mediainfo.Probe(probeCtx, tool.FFprobe, candidate)
	cancel()
	if refErr != nil {
		fmt.Fprintf(os.Stderr, "%s reference: %v\n", style.Red("error:"), refErr)
		return 2
	}
	if candErr != nil {
		fmt.Fprintf(os.Stderr, "%s candidate: %v\n", style.Red("error:"), candErr)
		return 2
	}
	if refInfo.Video == nil || candInfo.Video == nil {
		fmt.Fprintf(os.Stderr, "%s both files need a video stream\n", style.Red("error:"))
		return 2
	}

	duration := refInfo.DurationMs
	if candInfo.DurationMs > 0 && (duration == 0 || candInfo.DurationMs < duration) {
		duration = candInfo.DurationMs
	}
	measureCtx, measureCancel := context.WithTimeout(ctx, quality.Timeout(duration))
	defer measureCancel()
	result, err := quality.Measure(measureCtx, quality.Request{
		Reference:  reference,
		Candidate:  candidate,
		Metric:     metric,
		RefWidth:   refInfo.Video.Width,
		RefHeight:  refInfo.Video.Height,
		DurationMs: duration,
		Threads:    threads,
		FFmpeg:     tool.FFmpeg,
	})
	if err != nil {
		if errors.Is(err, quality.ErrUnsupported) {
			fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
			fmt.Fprintf(os.Stderr, "%s try --metric ssim or --metric psnr, or use an FFmpeg build with libvmaf\n", style.Dim("hint:"))
			return 3
		}
		fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
		return 1
	}

	if asJSON {
		writeJSON(result)
	} else {
		fmt.Printf("%s %s\n", style.Bold(strings.ToUpper(string(metric))), style.Dim("quality comparison"))
		fmt.Printf("  reference  %s\n", reference)
		fmt.Printf("  candidate  %s\n", candidate)
		if result.Infinite {
			fmt.Printf("  score      %s\n", style.Green("infinite (identical)"))
		} else {
			fmt.Printf("  score      %s\n", style.Green(result.ScoreText))
		}
		if metric == quality.MetricVMAF {
			fmt.Printf("  range      %.3f – %.3f · harmonic %.3f · %d frames\n", result.Min, result.Max, result.HarmonicMean, result.Frames)
		} else {
			fmt.Printf("  duration   %s\n", units.Clock(duration))
		}
	}
	if minScore > 0 && !result.Infinite && result.Score < minScore {
		fmt.Fprintf(os.Stderr, "%s score %.3f is below the requested minimum %.3f\n", style.Red("fail:"), result.Score, minScore)
		return 1
	}
	return 0
}
