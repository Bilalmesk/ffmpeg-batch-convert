// Package quality measures perceptual and signal fidelity between a reference
// and a candidate encode using FFmpeg's libvmaf, SSIM or PSNR filters.
package quality

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
)

// Metric selects the comparison algorithm.
type Metric string

const (
	MetricVMAF Metric = "vmaf"
	MetricSSIM Metric = "ssim"
	MetricPSNR Metric = "psnr"
)

// ErrUnsupported means the installed FFmpeg lacks the requested filter.
var ErrUnsupported = errors.New("the installed FFmpeg does not provide this quality filter")

// Request describes one quality comparison.
type Request struct {
	Reference   string
	Candidate   string
	Metric      Metric
	RefWidth    int
	RefHeight   int
	DurationMs  int64
	Threads     int
	FFmpeg      string
	ExtraFilter string
}

// Result is a single measurement.
type Result struct {
	Metric       Metric  `json:"metric"`
	Reference    string  `json:"reference"`
	Candidate    string  `json:"candidate"`
	Score        float64 `json:"score"`
	ScoreText    string  `json:"scoreText,omitempty"`
	Infinite     bool    `json:"infinite,omitempty"`
	Min          float64 `json:"min"`
	Max          float64 `json:"max"`
	HarmonicMean float64 `json:"harmonicMean"`
	Frames       int     `json:"frames"`
	Command      string  `json:"command"`
}

// Measure runs the comparison. It returns ErrUnsupported when the filter is
// missing from the FFmpeg build.
func Measure(ctx context.Context, req Request) (Result, error) {
	if req.FFmpeg == "" {
		req.FFmpeg = "ffmpeg"
	}
	if req.Metric == "" {
		req.Metric = MetricVMAF
	}
	if req.Reference == "" || req.Candidate == "" {
		return Result{}, errors.New("both reference and candidate are required")
	}
	for _, path := range []string{req.Reference, req.Candidate} {
		if _, err := os.Stat(path); err != nil {
			return Result{}, fmt.Errorf("stat %s: %w", filepath.Base(path), err)
		}
	}
	reference, err := filepath.Abs(req.Reference)
	if err != nil {
		return Result{}, err
	}
	candidate, err := filepath.Abs(req.Candidate)
	if err != nil {
		return Result{}, err
	}

	tempDir, err := os.MkdirTemp("", "ffbatch-quality-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tempDir)

	filter, logLevel, logName := buildFilter(req)
	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-loglevel", logLevel,
		"-i", candidate,
		"-i", reference,
		"-lavfi", filter,
		"-an", "-sn", "-dn",
	}
	if req.DurationMs > 0 {
		args = append(args, "-t", fmt.Sprintf("%.3f", float64(req.DurationMs)/1000))
	}
	args = append(args, "-f", "null", "-")

	cmd := exec.CommandContext(ctx, req.FFmpeg, args...)
	cmd.Dir = tempDir
	output, runErr := cmd.CombinedOutput()
	result := Result{
		Metric:    req.Metric,
		Reference: reference,
		Candidate: candidate,
		Command:   ffmpeg.CommandString(req.FFmpeg, args),
	}
	text := string(output)
	if runErr != nil {
		if isMissingFilter(text) {
			return result, ErrUnsupported
		}
		message := ffmpeg.SummarizeStderr(text, 3)
		if message == "" {
			message = runErr.Error()
		}
		return result, fmt.Errorf("%s comparison failed: %s", req.Metric, message)
	}

	switch req.Metric {
	case MetricVMAF:
		if err := parseVMAF(filepath.Join(tempDir, logName), &result); err != nil {
			return result, err
		}
	case MetricSSIM:
		score, ok := matchNumber(text, `All:([0-9.]+)`)
		if !ok {
			return result, fmt.Errorf("could not parse SSIM output: %s", ffmpeg.SummarizeStderr(text, 2))
		}
		result.Score = score
	case MetricPSNR:
		if match := matchText(text, `average:([^\s]+)`); match != "" {
			if strings.EqualFold(match, "inf") || strings.EqualFold(match, "+inf") {
				result.Infinite = true
				result.ScoreText = "inf"
			} else if score, err := strconv.ParseFloat(match, 64); err == nil {
				result.Score = score
			}
		}
		if result.Score == 0 && !result.Infinite {
			return result, fmt.Errorf("could not parse PSNR output: %s", ffmpeg.SummarizeStderr(text, 2))
		}
	default:
		return result, fmt.Errorf("unsupported metric %q", req.Metric)
	}
	if result.ScoreText == "" {
		result.ScoreText = fmt.Sprintf("%.3f", result.Score)
	}
	return result, nil
}

func buildFilter(req Request) (filter, logLevel, logName string) {
	var chain strings.Builder
	chain.WriteString("[0:v]setpts=PTS-STARTPTS")
	if req.RefWidth > 0 && req.RefHeight > 0 {
		chain.WriteString(fmt.Sprintf(",scale=%d:%d:flags=bicubic", req.RefWidth, req.RefHeight))
	}
	chain.WriteString("[dist];[1:v]setpts=PTS-STARTPTS[ref];[dist][ref]")

	switch req.Metric {
	case MetricSSIM:
		chain.WriteString("ssim")
		logLevel = "info"
	case MetricPSNR:
		chain.WriteString("psnr")
		logLevel = "info"
	default:
		logName = "vmaf.json"
		chain.WriteString("libvmaf=log_fmt=json:log_path=")
		chain.WriteString(logName)
		if req.Threads > 1 {
			chain.WriteString(fmt.Sprintf(":n_threads=%d", req.Threads))
		}
		if req.ExtraFilter != "" {
			chain.WriteString(":" + req.ExtraFilter)
		}
		logLevel = "error"
	}
	return chain.String(), logLevel, logName
}

type vmafDoc struct {
	Frames []struct {
		Metrics map[string]float64 `json:"metrics"`
	} `json:"frames"`
	PooledMetrics map[string]struct {
		Min          float64 `json:"min"`
		Max          float64 `json:"max"`
		Mean         float64 `json:"mean"`
		HarmonicMean float64 `json:"harmonic_mean"`
	} `json:"pooled_metrics"`
}

func parseVMAF(path string, result *Result) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read VMAF log: %w", err)
	}
	var doc vmafDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse VMAF log: %w", err)
	}
	result.Frames = len(doc.Frames)
	if pooled, ok := doc.PooledMetrics["vmaf"]; ok {
		result.Score = pooled.Mean
		result.Min = pooled.Min
		result.Max = pooled.Max
		result.HarmonicMean = pooled.HarmonicMean
	}
	if result.Score == 0 && len(doc.Frames) > 0 {
		var sum, min, max, harmonic float64
		min = math.Inf(1)
		for _, frame := range doc.Frames {
			value := frame.Metrics["vmaf"]
			sum += value
			if value < min {
				min = value
			}
			if value > max {
				max = value
			}
			if value > 0 {
				harmonic += 1 / value
			}
		}
		result.Score = sum / float64(len(doc.Frames))
		result.Min = min
		result.Max = max
		if harmonic > 0 {
			result.HarmonicMean = float64(len(doc.Frames)) / harmonic
		}
	}
	if result.Frames == 0 {
		return errors.New("VMAF produced no frames; are the files readable and the same length?")
	}
	return nil
}

func isMissingFilter(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "no such filter") || strings.Contains(lower, "unknown filter") ||
		strings.Contains(lower, "filter not found")
}

func matchNumber(text, pattern string) (float64, bool) {
	match := matchText(text, pattern)
	if match == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(match, 64)
	return value, err == nil
}

func matchText(text, pattern string) string {
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

// ParseMetric validates a CLI metric name.
func ParseMetric(value string) (Metric, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "vmaf", "":
		return MetricVMAF, true
	case "ssim":
		return MetricSSIM, true
	case "psnr":
		return MetricPSNR, true
	default:
		return "", false
	}
}

// Timeout returns a sensible upper bound for a comparison of this duration.
func Timeout(durationMs int64) time.Duration {
	seconds := durationMs / 1000
	if seconds < 60 {
		seconds = 60
	}
	return time.Duration(seconds*10+300) * time.Second
}
