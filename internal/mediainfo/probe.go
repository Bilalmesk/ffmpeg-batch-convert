// Package mediainfo wraps ffprobe and exposes the handful of stream facts the
// conversion planner needs.
package mediainfo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Stream is a normalised view of one ffprobe stream.
type Stream struct {
	Index       int     `json:"index"`
	Type        string  `json:"type"`
	Codec       string  `json:"codec"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	FPS         float64 `json:"fps,omitempty"`
	Channels    int     `json:"channels,omitempty"`
	Language    string  `json:"language,omitempty"`
	Title       string  `json:"title,omitempty"`
	Bitrate     int64   `json:"bitrate,omitempty"`
	AttachedPic bool    `json:"attachedPic,omitempty"`
}

// Info is the subset of ffprobe metadata used by the app.
type Info struct {
	Path       string   `json:"path"`
	Format     string   `json:"format"`
	DurationMs int64    `json:"durationMs"`
	SizeBytes  int64    `json:"sizeBytes"`
	Bitrate    int64    `json:"bitrate"`
	Video      *Stream  `json:"video,omitempty"`
	Audios     []Stream `json:"audios,omitempty"`
	Subtitles  []Stream `json:"subtitles,omitempty"`
}

type probeDoc struct {
	Streams []struct {
		Index        int               `json:"index"`
		CodecName    string            `json:"codec_name"`
		CodecType    string            `json:"codec_type"`
		Width        int               `json:"width"`
		Height       int               `json:"height"`
		AvgFrameRate string            `json:"avg_frame_rate"`
		RFrameRate   string            `json:"r_frame_rate"`
		Channels     int               `json:"channels"`
		BitRate      string            `json:"bit_rate"`
		Duration     string            `json:"duration"`
		Disposition  map[string]int    `json:"disposition"`
		Tags         map[string]string `json:"tags"`
		SideDataList []json.RawMessage `json:"side_data_list"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
}

// Probe runs ffprobe and converts its JSON output into an Info value. It
// requires a video stream because the conversion planner is video-oriented.
func Probe(ctx context.Context, ffprobe, path string) (*Info, error) {
	return probe(ctx, ffprobe, path, true)
}

// ProbeAny is Probe without the video requirement, for audio-only outputs.
func ProbeAny(ctx context.Context, ffprobe, path string) (*Info, error) {
	return probe(ctx, ffprobe, path, false)
}

func probe(ctx context.Context, ffprobe, path string, requireVideo bool) (*Info, error) {
	args := []string{
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-show_entries", "stream=index,codec_name,codec_type,width,height,avg_frame_rate,r_frame_rate,channels,bit_rate,duration:stream_tags=language,title:stream_disposition=attached_pic:format=format_name,duration,size,bit_rate",
		path,
	}
	cmd := exec.CommandContext(ctx, ffprobe, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("ffprobe: %s", firstLine(msg))
	}

	var doc probeDoc
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		return nil, fmt.Errorf("ffprobe returned invalid JSON: %w", err)
	}
	if len(doc.Streams) == 0 {
		return nil, errors.New("no audio or video streams found")
	}

	info := &Info{
		Path:       path,
		Format:     doc.Format.FormatName,
		DurationMs: parseSecondsToMs(doc.Format.Duration),
		SizeBytes:  parseInt(doc.Format.Size),
		Bitrate:    parseInt(doc.Format.BitRate),
	}
	for _, raw := range doc.Streams {
		stream := Stream{
			Index:       raw.Index,
			Type:        raw.CodecType,
			Codec:       raw.CodecName,
			Width:       raw.Width,
			Height:      raw.Height,
			FPS:         parseFrameRate(raw.AvgFrameRate),
			Channels:    raw.Channels,
			Language:    raw.Tags["language"],
			Title:       raw.Tags["title"],
			Bitrate:     parseInt(raw.BitRate),
			AttachedPic: raw.Disposition["attached_pic"] == 1,
		}
		if stream.FPS == 0 {
			stream.FPS = parseFrameRate(raw.RFrameRate)
		}
		switch raw.CodecType {
		case "video":
			if info.Video == nil || (info.Video.AttachedPic && !stream.AttachedPic) {
				copy := stream
				info.Video = &copy
			}
		case "audio":
			info.Audios = append(info.Audios, stream)
		case "subtitle":
			info.Subtitles = append(info.Subtitles, stream)
		}
		if info.DurationMs == 0 {
			info.DurationMs = parseSecondsToMs(raw.Duration)
		}
	}
	if requireVideo && info.Video == nil {
		return nil, errors.New("file has no video stream")
	}
	return info, nil
}

// IsTextSubtitle reports whether a subtitle codec can be stored as mov_text
// without rasterisation.
func IsTextSubtitle(codec string) bool {
	switch strings.ToLower(codec) {
	case "subrip", "srt", "ass", "ssa", "mov_text", "webvtt", "text", "utf8", "subviewer", "microdvd", "jacosub", "sami", "realtext", "stl", "vplayer":
		return true
	default:
		return false
	}
}

// TextSubtitles returns only the subtitles that can be converted to mov_text.
func (i *Info) TextSubtitles() []Stream {
	out := make([]Stream, 0, len(i.Subtitles))
	for _, s := range i.Subtitles {
		if IsTextSubtitle(s.Codec) {
			out = append(out, s)
		}
	}
	return out
}

// BitmapSubtitles returns subtitle streams that require a bitmap-capable
// container (MKV, MP4 with dvb/pgs support is unreliable).
func (i *Info) BitmapSubtitles() []Stream {
	out := make([]Stream, 0, len(i.Subtitles))
	for _, s := range i.Subtitles {
		if !IsTextSubtitle(s.Codec) {
			out = append(out, s)
		}
	}
	return out
}

func parseSecondsToMs(value string) int64 {
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return int64(seconds*1000 + 0.5)
}

func parseInt(value string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func parseFrameRate(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "0/0" {
		return 0
	}
	if num, den, ok := strings.Cut(value, "/"); ok {
		n, err1 := strconv.ParseFloat(num, 64)
		d, err2 := strconv.ParseFloat(den, 64)
		if err1 == nil && err2 == nil && d != 0 {
			return n / d
		}
		return 0
	}
	f, _ := strconv.ParseFloat(value, 64)
	return f
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
