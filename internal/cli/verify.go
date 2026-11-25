package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/engine"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/ffmpeg"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/hashutil"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/term"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
)

type verifyResult struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Status   string `json:"status"` // pass | fail | skip
	Detail   string `json:"detail,omitempty"`
	Expected string `json:"expectedSha256,omitempty"`
	Actual   string `json:"actualSha256,omitempty"`
}

func cmdVerify(args []string, style term.Style) int {
	fs := newFlagSet("verify")
	var (
		configPath string
		manifest   string
		asJSON     bool
	)
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&manifest, "manifest", "", "")
	fs.BoolVar(&asJSON, "json", false, "")
	if code, ok := parseFlags(fs, args, verifyUsage, style); !ok {
		return code
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if manifest != "" {
		if fs.NArg() > 0 {
			fmt.Fprintf(os.Stderr, "%s use either --manifest or file paths, not both\n", style.Red("error:"))
			return 2
		}
		results, err := verifyManifestFile(ctx, manifest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", style.Red("error:"), err)
			return 2
		}
		return printVerifyResults(results, asJSON, style)
	}

	if fs.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "%s verify needs a manifest or at least one file\n\n%s", style.Red("error:"), verifyUsage)
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

	results := make([]verifyResult, 0, fs.NArg())
	for _, path := range fs.Args() {
		probeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		info, err := mediainfo.Probe(probeCtx, tool.FFprobe, path)
		cancel()
		result := verifyResult{Name: filepath.Base(path), Path: path}
		if err != nil {
			result.Status = "fail"
			result.Detail = err.Error()
		} else {
			result.Status = "pass"
			result.Detail = fmt.Sprintf("%s · %s · %s", units.Clock(info.DurationMs), units.Bytes(info.SizeBytes), describeMedia(info))
		}
		results = append(results, result)
	}
	return printVerifyResults(results, asJSON, style)
}

func verifyManifestFile(ctx context.Context, path string) ([]verifyResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc engine.ReportDoc
	var snapshot *engine.RunSnapshot
	if err := json.Unmarshal(data, &doc); err == nil && doc.Run != nil {
		if doc.Schema > engine.ReportSchema {
			return nil, fmt.Errorf("manifest schema %d is newer than this build supports (%d)", doc.Schema, engine.ReportSchema)
		}
		snapshot = doc.Run
	} else {
		var direct engine.RunSnapshot
		if err := json.Unmarshal(data, &direct); err != nil {
			return nil, fmt.Errorf("parse manifest: %w", err)
		}
		snapshot = &direct
	}
	if snapshot == nil || len(snapshot.Jobs) == 0 {
		return nil, errors.New("manifest contains no jobs")
	}

	results := make([]verifyResult, 0, len(snapshot.Jobs))
	for _, job := range snapshot.Jobs {
		result := verifyResult{Name: job.Name, Path: job.Output}
		if job.Status != engine.StatusDone {
			result.Status = "skip"
			result.Detail = "job is " + string(job.Status)
			results = append(results, result)
			continue
		}
		if job.Output == "" {
			result.Status = "skip"
			result.Detail = "no output path"
			results = append(results, result)
			continue
		}

		info, err := os.Stat(job.Output)
		if err != nil {
			result.Status = "fail"
			result.Detail = "output missing: " + err.Error()
			results = append(results, result)
			continue
		}
		if job.OutputBytes > 0 && info.Size() != job.OutputBytes {
			result.Status = "fail"
			result.Detail = fmt.Sprintf("size is %d, expected %d", info.Size(), job.OutputBytes)
			results = append(results, result)
			continue
		}
		if job.OutputSHA256 == "" {
			result.Status = "skip"
			result.Detail = "no SHA-256 recorded in the manifest"
			results = append(results, result)
			continue
		}
		actual, err := hashutil.SHA256(ctx, job.Output)
		if err != nil {
			result.Status = "fail"
			result.Detail = err.Error()
			results = append(results, result)
			continue
		}
		result.Expected = job.OutputSHA256
		result.Actual = actual
		if !strings.EqualFold(actual, job.OutputSHA256) {
			result.Status = "fail"
			result.Detail = "SHA-256 mismatch"
			results = append(results, result)
			continue
		}

		note := fmt.Sprintf("%s · %s", units.Bytes(info.Size()), shortSHA(actual))
		if job.SourceSHA256 != "" {
			sourcePath := job.Input
			noteSuffix := " · source intact"
			if _, statErr := os.Stat(sourcePath); statErr != nil {
				switch {
				case job.SourceArchivedTo != "":
					sourcePath = job.SourceArchivedTo
					noteSuffix = " · source archived"
				case job.SourceRemoved:
					sourcePath = ""
					noteSuffix = " · source removed"
				default:
					result.Status = "fail"
					result.Detail = "source missing: " + job.Input
					results = append(results, result)
					continue
				}
			}
			if sourcePath != "" {
				sourceHash, err := hashutil.SHA256(ctx, sourcePath)
				switch {
				case err != nil:
					result.Status = "fail"
					result.Detail = "source: " + err.Error()
					results = append(results, result)
					continue
				case !strings.EqualFold(sourceHash, job.SourceSHA256):
					result.Status = "fail"
					result.Detail = "source SHA-256 mismatch"
					results = append(results, result)
					continue
				}
			}
			note += noteSuffix
		}
		result.Status = "pass"
		result.Detail = note
		results = append(results, result)
	}
	return results, nil
}

func printVerifyResults(results []verifyResult, asJSON bool, style term.Style) int {
	if asJSON {
		writeJSON(results)
	} else {
		writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, style.Dim("STATUS\tFILE\tDETAIL"))
		for _, result := range results {
			tag := style.Green("PASS")
			switch result.Status {
			case "fail":
				tag = style.Red("FAIL")
			case "skip":
				tag = style.Yellow("SKIP")
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\n", tag, result.Name, result.Detail)
		}
		_ = writer.Flush()
	}
	for _, result := range results {
		if result.Status == "fail" {
			return 1
		}
	}
	return 0
}

func describeMedia(info *mediainfo.Info) string {
	parts := make([]string, 0, 4)
	if info.Video != nil {
		parts = append(parts, fmt.Sprintf("%dx%d %s", info.Video.Width, info.Video.Height, info.Video.Codec))
	}
	if len(info.Audios) > 0 {
		parts = append(parts, fmt.Sprintf("%d audio", len(info.Audios)))
	}
	if len(info.Subtitles) > 0 {
		parts = append(parts, fmt.Sprintf("%d subtitles", len(info.Subtitles)))
	}
	return strings.Join(parts, " · ")
}

func shortSHA(hash string) string {
	if len(hash) > 12 {
		return hash[:12] + "…"
	}
	return hash
}
