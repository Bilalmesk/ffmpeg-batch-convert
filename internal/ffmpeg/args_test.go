package ffmpeg

import (
	"strings"
	"testing"

	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/config"
	"github.com/Bilalmesk/ffmpeg-batch-convert/internal/mediainfo"
)

func baseConfig() config.Config {
	cfg := config.Default()
	cfg.Normalize()
	return cfg
}

func testInfo() *mediainfo.Info {
	return &mediainfo.Info{
		DurationMs: 60_000,
		Video:      &mediainfo.Stream{Index: 0, Type: "video", Codec: "h264", Width: 1920, Height: 1080},
		Audios:     []mediainfo.Stream{{Index: 1, Type: "audio", Codec: "aac", Channels: 2}},
		Subtitles: []mediainfo.Stream{
			{Index: 2, Type: "subtitle", Codec: "subrip"},
			{Index: 3, Type: "subtitle", Codec: "hdmv_pgs_subtitle"},
		},
	}
}

func containsSequence(args []string, sequence ...string) bool {
	if len(sequence) == 0 {
		return true
	}
	for i := 0; i+len(sequence) <= len(args); i++ {
		match := true
		for j, item := range sequence {
			if args[i+j] != item {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func indexOf(args []string, value string) int {
	for i, arg := range args {
		if arg == value {
			return i
		}
	}
	return -1
}

func TestBuildAV1Encoders(t *testing.T) {
	tests := []struct {
		encoder string
		want    []string
		tag     bool
	}{
		{"svt", []string{"-c:v", "libsvtav1", "-preset", "9", "-crf", "30"}, true},
		{"aom", []string{"-c:v", "libaom-av1", "-crf", "30", "-cpu-used", "5"}, true},
		{"rav1e", []string{"-c:v", "librav1e", "-qp", "30", "-speed", "7"}, true},
	}
	for _, test := range tests {
		t.Run(test.encoder, func(t *testing.T) {
			cfg := baseConfig()
			cfg.Mode = config.ModeEncode
			cfg.Format = "mp4"
			cfg.VideoCodec = "av1"
			cfg.AV1Encoder = test.encoder
			cfg.CRF = 30
			cfg.Preset = "fast"
			result := Build(BuildRequest{Cfg: cfg, Input: "in.ts", Output: "out.mp4", Info: testInfo()})
			if !containsSequence(result.Args, test.want...) {
				t.Fatalf("args %v missing %v", result.Args, test.want)
			}
			if test.tag && !containsSequence(result.Args, "-tag:v", "av01") {
				t.Fatalf("missing AV1 tag: %v", result.Args)
			}
		})
	}
}

func TestBuildAV1WarnsForTS(t *testing.T) {
	cfg := baseConfig()
	cfg.Mode = config.ModeEncode
	cfg.Format = "ts"
	cfg.VideoCodec = "av1"
	result := Build(BuildRequest{Cfg: cfg, Input: "in.ts", Output: "out.ts", Info: testInfo()})
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "MPEG-TS") {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestBuildRemuxMP4SkipsBitmapSubtitles(t *testing.T) {
	cfg := baseConfig()
	cfg.Mode = config.ModeRemux
	cfg.Format = "mp4"
	cfg.Aspect = "16:9"
	cfg.GenPTS = true
	cfg.Faststart = true

	result := Build(BuildRequest{Cfg: cfg, Input: "in.ts", Output: "out.mp4", Info: testInfo()})
	args := result.Args

	if !containsSequence(args, "-map", "0:0") || !containsSequence(args, "-map", "0:1") {
		t.Fatalf("missing video/audio maps: %v", args)
	}
	if !containsSequence(args, "-map", "0:2") {
		t.Fatalf("text subtitle should be mapped: %v", args)
	}
	if containsSequence(args, "-map", "0:3") {
		t.Fatalf("bitmap subtitle should be skipped for MP4: %v", args)
	}
	if !containsSequence(args, "-c:v", "copy") || !containsSequence(args, "-c:a", "copy") {
		t.Fatalf("remux should copy streams: %v", args)
	}
	if !containsSequence(args, "-c:s", "mov_text") {
		t.Fatalf("subtitles should convert to mov_text: %v", args)
	}
	if !containsSequence(args, "-movflags", "+faststart") {
		t.Fatalf("faststart missing: %v", args)
	}
	if !containsSequence(args, "-aspect", "16:9") {
		t.Fatalf("aspect missing: %v", args)
	}
	if indexOf(args, "-i") < indexOf(args, "-fflags") {
		t.Fatalf("genpts must come before the input: %v", args)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "bitmap") {
		t.Fatalf("expected bitmap warning, got %v", result.Warnings)
	}
}

func TestBuildEncodeHEVCAvoidsIncompatibleAudio(t *testing.T) {
	cfg := baseConfig()
	cfg.Mode = config.ModeEncode
	cfg.Format = "mp4"
	cfg.VideoCodec = "h265"
	cfg.CRF = 20
	cfg.Preset = "slow"
	cfg.AudioCodec = "copy"
	info := testInfo()
	info.Audios[0].Codec = "opus"

	result := Build(BuildRequest{Cfg: cfg, Input: "in.mkv", Output: "out.mp4", Info: info})
	args := result.Args
	if !containsSequence(args, "-c:v", "libx265") {
		t.Fatalf("expected libx265: %v", args)
	}
	if !containsSequence(args, "-preset", "slow") || !containsSequence(args, "-crf", "20") {
		t.Fatalf("encoding options missing: %v", args)
	}
	if !containsSequence(args, "-tag:v", "hvc1") {
		t.Fatalf("HEVC MP4 should use the hvc1 tag: %v", args)
	}
	if !containsSequence(args, "-c:a", "aac") {
		t.Fatalf("Opus audio should fall back to AAC: %v", args)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected a compatibility warning")
	}
}

func TestBuildMKVKeepsAllSubtitles(t *testing.T) {
	cfg := baseConfig()
	cfg.Mode = config.ModeRemux
	cfg.Format = "mkv"
	cfg.Faststart = true
	cfg.Subtitle = "auto"

	result := Build(BuildRequest{Cfg: cfg, Input: "in.ts", Output: "out.mkv", Info: testInfo()})
	if !containsSequence(result.Args, "-map", "0:3") {
		t.Fatalf("MKV should keep bitmap subtitles: %v", result.Args)
	}
	if !containsSequence(result.Args, "-c:s", "copy") {
		t.Fatalf("MKV subtitles should be copied: %v", result.Args)
	}
	if containsSequence(result.Args, "-movflags", "+faststart") {
		t.Fatalf("faststart is MP4-only: %v", result.Args)
	}
}

func TestBuildSubtitleAndAudioPolicies(t *testing.T) {
	cfg := baseConfig()
	cfg.Mode = config.ModeRemux
	cfg.Format = "mp4"
	cfg.Subtitle = "none"
	cfg.AudioCodec = "none"

	result := Build(BuildRequest{Cfg: cfg, Input: "in.ts", Output: "out.mp4", Info: testInfo()})
	if containsSequence(result.Args, "-map", "0:1") {
		t.Fatalf("audio should be dropped: %v", result.Args)
	}
	if containsSequence(result.Args, "-map", "0:2") || containsSequence(result.Args, "-c:s") {
		t.Fatalf("subtitles should be dropped: %v", result.Args)
	}
}

func TestBuildTSOutputHasNoFaststart(t *testing.T) {
	cfg := baseConfig()
	cfg.Mode = config.ModeRemux
	cfg.Format = "ts"
	cfg.Faststart = true

	result := Build(BuildRequest{Cfg: cfg, Input: "in.mkv", Output: "out.ts", Info: testInfo()})
	if containsSequence(result.Args, "-movflags", "+faststart") {
		t.Fatalf("TS must not receive movflags: %v", result.Args)
	}
	if result.Args[len(result.Args)-1] != "out.ts" {
		t.Fatalf("output should be the last argument: %v", result.Args)
	}
}

func TestBuildSelectsPreferredAudioLanguage(t *testing.T) {
	cfg := baseConfig()
	cfg.Mode = config.ModeRemux
	cfg.Format = "mkv"
	cfg.AudioTracks = "preferred"
	cfg.AudioLanguage = "ara"

	info := testInfo()
	info.Audios = []mediainfo.Stream{
		{Index: 1, Type: "audio", Codec: "aac", Language: "ara"},
		{Index: 4, Type: "audio", Codec: "ac3", Language: "eng"},
		{Index: 5, Type: "audio", Codec: "aac", Language: "ara"},
	}
	result := Build(BuildRequest{Cfg: cfg, Input: "in.ts", Output: "out.mkv", Info: info})
	if !containsSequence(result.Args, "-map", "0:1") || !containsSequence(result.Args, "-map", "0:5") {
		t.Fatalf("Arabic tracks should be mapped: %v", result.Args)
	}
	if containsSequence(result.Args, "-map", "0:4") {
		t.Fatalf("English track should be skipped: %v", result.Args)
	}
}

func TestBuildFallsBackToFirstAudio(t *testing.T) {
	cfg := baseConfig()
	cfg.Mode = config.ModeRemux
	cfg.Format = "mkv"
	cfg.AudioTracks = "preferred"
	cfg.AudioLanguage = "ara"

	info := testInfo()
	info.Audios[0].Language = "eng"
	result := Build(BuildRequest{Cfg: cfg, Input: "in.ts", Output: "out.mkv", Info: info})
	if !containsSequence(result.Args, "-map", "0:1") {
		t.Fatalf("fallback should map the first audio track: %v", result.Args)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "not found") {
		t.Fatalf("expected a preferred-language warning: %v", result.Warnings)
	}
}

func TestBuildFirstAudioOnly(t *testing.T) {
	cfg := baseConfig()
	cfg.Mode = config.ModeRemux
	cfg.Format = "mp4"
	cfg.AudioTracks = "first"
	info := testInfo()
	info.Audios = append(info.Audios, mediainfo.Stream{Index: 4, Type: "audio", Codec: "aac", Language: "eng"})

	result := Build(BuildRequest{Cfg: cfg, Input: "in.ts", Output: "out.mp4", Info: info})
	if !containsSequence(result.Args, "-map", "0:1") || containsSequence(result.Args, "-map", "0:4") {
		t.Fatalf("only the first audio track should be mapped: %v", result.Args)
	}
}

func TestBuildTrim(t *testing.T) {
	cfg := baseConfig()
	cfg.Mode = config.ModeRemux
	cfg.Format = "mp4"
	cfg.TrimStart = "00:00:05"
	cfg.TrimEnd = "00:00:15"

	result := Build(BuildRequest{Cfg: cfg, Input: "in.ts", Output: "out.mp4", Info: testInfo()})
	ss := indexOf(result.Args, "-ss")
	input := indexOf(result.Args, "-i")
	if ss < 0 || input < 0 || ss < input {
		t.Fatalf("trim must be an output option (after -i): %v", result.Args)
	}
	if !containsSequence(result.Args, "-ss", "00:00:05") {
		t.Fatalf("missing trim start: %v", result.Args)
	}
	if !containsSequence(result.Args, "-t", "00:00:10.000") {
		t.Fatalf("missing trim duration: %v", result.Args)
	}
	if indexOf(result.Args, "-copyinkf") < 0 {
		t.Fatalf("MP4 remux trim should copy initial non-keyframes: %v", result.Args)
	}
}

func TestBuildNoTrimByDefault(t *testing.T) {
	cfg := baseConfig()
	cfg.Mode = config.ModeRemux
	cfg.Format = "mp4"

	result := Build(BuildRequest{Cfg: cfg, Input: "in.ts", Output: "out.mp4", Info: testInfo()})
	if indexOf(result.Args, "-ss") >= 0 || indexOf(result.Args, "-t") >= 0 {
		t.Fatalf("unexpected trim options: %v", result.Args)
	}
}

func TestCommandStringQuotesPaths(t *testing.T) {
	got := CommandString("ffmpeg", []string{"-i", `C:\My Videos\a.ts`, "out.mp4"})
	if !strings.Contains(got, `"C:\My Videos\a.ts"`) {
		t.Fatalf("command string did not quote the path: %s", got)
	}
}
