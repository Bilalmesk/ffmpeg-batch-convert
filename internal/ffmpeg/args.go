package ffmpeg

import (
	"fmt"
	"strings"
	"time"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/units"
)

// BuildRequest describes one conversion command.
type BuildRequest struct {
	Cfg    config.Config
	Input  string
	Output string
	Info   *mediainfo.Info
}

// BuildResult carries the argv plus non-fatal compatibility notes.
type BuildResult struct {
	Args     []string
	Warnings []string
	Command  string
}

// Build assembles a deterministic FFmpeg argv for the configured mode.
func Build(req BuildRequest) BuildResult {
	cfg := req.Cfg
	res := BuildResult{}
	warn := func(format string, args ...any) {
		res.Warnings = append(res.Warnings, fmt.Sprintf(format, args...))
	}

	args := []string{
		"-hide_banner", "-nostdin", "-y",
		"-loglevel", "error",
		"-stats_period", "0.5",
	}
	if cfg.GenPTS {
		args = append(args, "-fflags", "+genpts")
	}
	args = append(args, "-i", req.Input)
	// Trim with output-side seeking. Input seeking drops video on transport
	// streams whose first keyframe is after the seek target.
	if cfg.TrimStart != "" {
		args = append(args, "-ss", cfg.TrimStart)
		if cfg.Mode == config.ModeRemux && isMP4(cfg.Format) {
			args = append(args, "-copyinkf")
		}
	}
	args = append(args, "-map_metadata", "0", "-map_chapters", "0")

	info := req.Info
	if info != nil && info.Video != nil {
		args = append(args, "-map", fmt.Sprintf("0:%d", info.Video.Index))
	} else {
		args = append(args, "-map", "0:v:0?")
	}

	// ---- Video ---------------------------------------------------------
	switch cfg.Mode {
	case config.ModeRemux:
		args = append(args, "-c:v", "copy")
	case config.ModeEncode, config.ModeRename:
		encodeArgs, encodeWarnings := videoEncodeArgs(cfg)
		args = append(args, encodeArgs...)
		res.Warnings = append(res.Warnings, encodeWarnings...)
	}

	// ---- Audio ---------------------------------------------------------
	audioStreams, audioWarnings := selectAudioStreams(cfg, info)
	res.Warnings = append(res.Warnings, audioWarnings...)
	audioCodec := strings.ToLower(cfg.AudioCodec)
	if audioCodec == "none" {
		// No -map for audio means all audio is dropped.
	} else {
		if info != nil {
			for _, stream := range audioStreams {
				args = append(args, "-map", fmt.Sprintf("0:%d", stream.Index))
			}
		} else {
			args = append(args, "-map", "0:a?")
		}

		switch {
		case cfg.Mode == config.ModeRemux:
			args = append(args, "-c:a", "copy")
			if info != nil && isMP4(cfg.Format) {
				for _, stream := range audioStreams {
					if !mp4AudioCompatible(stream.Codec) {
						warn("audio track %d uses %s, which many MP4 players cannot decode; consider Encode mode", stream.Index, stream.Codec)
					}
				}
			}
		case audioCodec == "copy":
			if info != nil && isMP4(cfg.Format) && !allAudioCompatible(audioStreams) {
				reasons := incompatibleAudioCodecs(audioStreams)
				args = append(args, "-c:a", "aac", "-b:a", cfg.AudioRate)
				warn("audio codec %s is not MP4-friendly; re-encoding audio to AAC %s", strings.Join(reasons, ", "), cfg.AudioRate)
			} else {
				args = append(args, "-c:a", "copy")
			}
		case audioCodec == "aac":
			args = append(args, "-c:a", "aac", "-b:a", cfg.AudioRate)
		case audioCodec == "opus":
			args = append(args, "-c:a", "libopus", "-b:a", cfg.AudioRate)
			if isMP4(cfg.Format) {
				warn("Opus in MP4 is valid but has limited hardware-player support")
			}
		default:
			args = append(args, "-c:a", "copy")
		}
	}

	// ---- Subtitles -----------------------------------------------------
	subArgs, subWarnings := subtitleArgs(cfg, info)
	res.Warnings = append(res.Warnings, subWarnings...)
	args = append(args, subArgs...)

	// ---- Container -----------------------------------------------------
	if duration := trimDuration(cfg); duration > 0 {
		args = append(args, "-t", units.FormatTimecode(duration))
	}
	if cfg.Aspect != "" {
		args = append(args, "-aspect", cfg.Aspect)
	}
	if cfg.Faststart && isFaststartContainer(cfg.Format) {
		args = append(args, "-movflags", "+faststart")
	}
	args = append(args, "-max_muxing_queue_size", "4096")
	args = append(args, "-progress", "pipe:1", "-nostats")
	args = append(args, req.Output)

	res.Args = args
	res.Command = CommandString("ffmpeg", args)
	return res
}

func subtitleArgs(cfg config.Config, info *mediainfo.Info) ([]string, []string) {
	var warnings []string
	if cfg.Subtitle == "none" {
		return nil, nil
	}
	if info == nil {
		return []string{"-map", "0:s?", "-c:s", "copy"}, nil
	}
	if len(info.Subtitles) == 0 {
		return nil, nil
	}

	policy := cfg.Subtitle
	if policy == "auto" {
		if isMP4(cfg.Format) {
			policy = "mov_text"
		} else {
			policy = "copy"
		}
	}

	switch policy {
	case "mov_text":
		if !isMP4(cfg.Format) {
			warnings = append(warnings, "mov_text subtitles are designed for MP4; use Copy for "+strings.ToUpper(cfg.Format))
		}
		var args []string
		if isMP4(cfg.Format) {
			// Bitmap subtitles (PGS/VobSub) cannot be stored as mov_text.
			for _, s := range info.TextSubtitles() {
				args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
			}
			if skipped := len(info.BitmapSubtitles()); skipped > 0 {
				warnings = append(warnings,
					fmt.Sprintf("%d bitmap subtitle track(s) skipped because MP4 cannot store them as mov_text; use MKV to keep them", skipped))
			}
		} else {
			for _, s := range info.Subtitles {
				args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
			}
		}
		args = append(args, "-c:s", "mov_text")
		return args, warnings

	case "copy":
		var args []string
		for _, s := range info.Subtitles {
			args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
		}
		args = append(args, "-c:s", "copy")
		if isMP4(cfg.Format) && len(info.BitmapSubtitles()) > 0 {
			warnings = append(warnings,
				"MP4 cannot reliably store bitmap subtitles; the conversion may fail. Use MKV or Subtitle = auto")
		}
		return args, warnings
	}
	return nil, warnings
}

func isMP4(format string) bool {
	switch strings.ToLower(format) {
	case "mp4", "m4v", "mov":
		return true
	default:
		return false
	}
}

func isFaststartContainer(format string) bool {
	return isMP4(format)
}

// SelectedAudioStreams applies the configured audio policy and returns the
// streams that will be mapped, plus any compatibility warnings.
func SelectedAudioStreams(cfg config.Config, info *mediainfo.Info) ([]mediainfo.Stream, []string) {
	return selectAudioStreams(cfg, info)
}

// selectAudioStreams applies the all/first/preferred-language policy.
func selectAudioStreams(cfg config.Config, info *mediainfo.Info) ([]mediainfo.Stream, []string) {
	if info == nil || len(info.Audios) == 0 {
		return nil, nil
	}
	switch cfg.AudioTracks {
	case "first":
		return info.Audios[:1], nil
	case "preferred":
		wanted := strings.TrimSpace(cfg.AudioLanguage)
		var matched []mediainfo.Stream
		for _, stream := range info.Audios {
			if languageMatches(stream.Language, wanted) {
				matched = append(matched, stream)
			}
		}
		if len(matched) > 0 {
			return matched, nil
		}
		first := info.Audios[0]
		label := first.Language
		if label == "" {
			label = "untagged"
		}
		return info.Audios[:1], []string{
			fmt.Sprintf("preferred audio language %q not found; using the first track (%s, %s)", wanted, label, first.Codec),
		}
	default:
		return info.Audios, nil
	}
}

func languageMatches(tag, wanted string) bool {
	tag = strings.ToLower(strings.TrimSpace(tag))
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	if tag == "" || wanted == "" {
		return false
	}
	tag = strings.Split(tag, "-")[0]
	wanted = strings.Split(wanted, "-")[0]
	return tag == wanted || strings.HasPrefix(tag, wanted) || strings.HasPrefix(wanted, tag)
}

// trimDuration returns the output duration implied by trimStart/trimEnd.
func trimDuration(cfg config.Config) time.Duration {
	if cfg.TrimEnd == "" {
		return 0
	}
	start, err := units.ParseTimecode(cfg.TrimStart)
	if err != nil {
		return 0
	}
	end, err := units.ParseTimecode(cfg.TrimEnd)
	if err != nil {
		return 0
	}
	if end <= start {
		return 0
	}
	return end - start
}

func mp4AudioCompatible(codec string) bool {
	switch strings.ToLower(codec) {
	case "aac", "mp3", "ac3", "eac3", "alac", "mp2":
		return true
	default:
		return false
	}
}

func allAudioCompatible(streams []mediainfo.Stream) bool {
	for _, s := range streams {
		if !mp4AudioCompatible(s.Codec) {
			return false
		}
	}
	return true
}

func incompatibleAudioCodecs(streams []mediainfo.Stream) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range streams {
		if !mp4AudioCompatible(s.Codec) && !seen[s.Codec] {
			seen[s.Codec] = true
			out = append(out, s.Codec)
		}
	}
	return out
}

// CommandString renders an argv for display and logs.
func CommandString(binary string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteArg(binary))
	for _, arg := range args {
		parts = append(parts, quoteArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if strings.ContainsAny(arg, " \t\"") {
		return `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
	}
	return arg
}
