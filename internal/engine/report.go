package engine

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ReportDoc is the JSON export of a finished (or in-flight) run.
type ReportDoc struct {
	GeneratedAt time.Time    `json:"generatedAt"`
	Run         *RunSnapshot `json:"run"`
}

// Report renders a run as JSON or CSV and returns the suggested filename.
func (m *Manager) Report(id, format string) ([]byte, string, error) {
	snapshot, ok := m.Snapshot(id)
	if !ok {
		return nil, "", ErrNotFound
	}
	switch strings.ToLower(format) {
	case "csv":
		data, err := runCSV(snapshot)
		return data, fmt.Sprintf("ffbatch-%s.csv", id), err
	case "json", "":
		data, err := json.MarshalIndent(ReportDoc{GeneratedAt: time.Now(), Run: snapshot}, "", "  ")
		if err != nil {
			return nil, "", err
		}
		return append(data, '\n'), fmt.Sprintf("ffbatch-%s.json", id), nil
	default:
		return nil, "", fmt.Errorf("unsupported report format %q", format)
	}
}

func runCSV(snapshot *RunSnapshot) ([]byte, error) {
	var buf bytes.Buffer
	// A UTF-8 BOM keeps Excel from mangling non-ASCII filenames.
	buf.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(&buf)
	header := []string{
		"id", "status", "name", "input", "output", "size_bytes", "output_bytes",
		"duration_ms", "resolution", "video_codec", "audio_codec", "started_at",
		"finished_at", "error", "reason", "warnings",
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	for _, job := range snapshot.Jobs {
		row := []string{
			job.ID,
			string(job.Status),
			job.Name,
			job.Input,
			job.Output,
			strconv.FormatInt(job.SizeBytes, 10),
			strconv.FormatInt(job.OutputBytes, 10),
			strconv.FormatInt(job.DurationMs, 10),
			job.Resolution,
			job.VideoCodec,
			job.AudioCodec,
			formatTime(job.StartedAt),
			formatTime(job.FinishedAt),
			job.Error,
			job.Reason,
			strings.Join(job.Warnings, " | "),
		}
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}
