package workflow

// Builtins returns the workflows shipped with FFBatch.
func Builtins() []Workflow {
	return []Workflow{
		{
			Schema:      Schema,
			Name:        "ts-repair",
			Description: "Repair a transport stream: regenerate timestamps, remux to MP4, verify the output and record SHA-256 hashes.",
			Steps: []Step{
				{ID: "probe", Type: "probe"},
				{
					ID:   "repair",
					Type: "convert",
					When: &Condition{Field: "ext", In: []any{"ts", "m2ts", "mpg", "mpeg", "m4v", "mkv", "mov", "avi", "wmv", "flv", "webm"}},
					Params: map[string]any{
						"mode":         "remux",
						"format":       "mp4",
						"genPts":       true,
						"faststart":    true,
						"verifyOutput": true,
						"hashFiles":    true,
						"output":       "{{outputDir}}/{{base}}.mp4",
					},
				},
			},
		},
		{
			Schema:      Schema,
			Name:        "av1-quality",
			Description: "Encode AV1 with SVT-AV1, require a VMAF score of at least 90, then copy the verified result to the output folder.",
			Steps: []Step{
				{ID: "probe", Type: "probe"},
				{
					ID:   "encode",
					Type: "convert",
					Params: map[string]any{
						"mode":         "encode",
						"format":       "mp4",
						"videoCodec":   "av1",
						"av1Encoder":   "svt",
						"crf":          30,
						"preset":       "medium",
						"audioCodec":   "copy",
						"faststart":    true,
						"verifyOutput": true,
						"hashFiles":    true,
						"output":       "{{work}}/{{base}}.av1.mp4",
					},
				},
				{
					ID:   "quality",
					Type: "quality",
					Params: map[string]any{
						"reference": "{{input}}",
						"candidate": "{{steps.encode.output}}",
						"metric":    "vmaf",
						"min":       90,
					},
				},
				{
					ID:   "deliver",
					Type: "copy",
					Params: map[string]any{
						"from": "{{steps.encode.output}}",
						"to":   "{{outputDir}}/{{base}}.av1.mp4",
					},
				},
			},
		},
		{
			Schema:      Schema,
			Name:        "mkv-preserve",
			Description: "Mux into MKV with MKVToolNix, preserving tracks, chapters, tags and attachments without re-encoding.",
			Steps: []Step{
				{ID: "probe", Type: "probe"},
				{
					ID:   "mux",
					Type: "mux",
					Params: map[string]any{
						"input":  "{{input}}",
						"output": "{{outputDir}}/{{base}}.mkv",
						"title":  "{{base}}",
					},
				},
			},
		},
		{
			Schema:      Schema,
			Name:        "hls-delivery",
			Description: "Build a conservative H.264/AAC mezzanine, then package it as HLS with 6-second segments.",
			Steps: []Step{
				{ID: "probe", Type: "probe"},
				{
					ID:   "encode",
					Type: "convert",
					Params: map[string]any{
						"mode":         "encode",
						"format":       "mp4",
						"videoCodec":   "h264",
						"crf":          20,
						"preset":       "medium",
						"audioCodec":   "aac",
						"audioBitrate": "192k",
						"faststart":    true,
						"verifyOutput": true,
						"output":       "{{work}}/{{base}}.mezz.mp4",
					},
				},
				{
					ID:   "package",
					Type: "package",
					Params: map[string]any{
						"input":   "{{steps.encode.output}}",
						"out":     "{{outputDir}}/{{base}}.hls",
						"format":  "hls",
						"segment": 6,
					},
				},
			},
		},
		{
			Schema:      Schema,
			Name:        "dash-delivery",
			Description: "Build a conservative H.264/AAC mezzanine, then package it as DASH with 6-second segments.",
			Steps: []Step{
				{ID: "probe", Type: "probe"},
				{
					ID:   "encode",
					Type: "convert",
					Params: map[string]any{
						"mode":         "encode",
						"format":       "mp4",
						"videoCodec":   "h264",
						"crf":          20,
						"preset":       "medium",
						"audioCodec":   "aac",
						"audioBitrate": "192k",
						"faststart":    true,
						"verifyOutput": true,
						"output":       "{{work}}/{{base}}.mezz.mp4",
					},
				},
				{
					ID:   "package",
					Type: "package",
					Params: map[string]any{
						"input":   "{{steps.encode.output}}",
						"out":     "{{outputDir}}/{{base}}.dash",
						"format":  "dash",
						"segment": 6,
					},
				},
			},
		},
		{
			Schema:      Schema,
			Name:        "audio-delivery",
			Description: "Extract the first audio track to FLAC with EBU R128 loudness normalization and probe the result.",
			Steps: []Step{
				{ID: "probe", Type: "probe"},
				{
					ID:   "audio",
					Type: "audio",
					Params: map[string]any{
						"input":     "{{input}}",
						"output":    "{{outputDir}}/{{base}}.flac",
						"format":    "flac",
						"normalize": true,
					},
				},
			},
		},
		{
			Schema:      Schema,
			Name:        "multi-delivery",
			Description: "The full mashup: compatibility MP4, verified AV1 copy, VMAF gate, and normalized FLAC audio from one input.",
			Steps: []Step{
				{ID: "probe", Type: "probe"},
				{
					ID:   "h264",
					Type: "convert",
					Params: map[string]any{
						"mode":         "encode",
						"format":       "mp4",
						"videoCodec":   "h264",
						"crf":          20,
						"preset":       "medium",
						"audioCodec":   "aac",
						"audioBitrate": "192k",
						"faststart":    true,
						"verifyOutput": true,
						"hashFiles":    true,
						"output":       "{{outputDir}}/{{base}}.h264.mp4",
					},
				},
				{
					ID:   "av1",
					Type: "convert",
					Params: map[string]any{
						"mode":         "encode",
						"format":       "mp4",
						"videoCodec":   "av1",
						"av1Encoder":   "svt",
						"crf":          30,
						"preset":       "medium",
						"audioCodec":   "copy",
						"faststart":    true,
						"verifyOutput": true,
						"hashFiles":    true,
						"output":       "{{work}}/{{base}}.av1.mp4",
					},
				},
				{
					ID:   "quality",
					Type: "quality",
					Params: map[string]any{
						"reference": "{{input}}",
						"candidate": "{{steps.av1.output}}",
						"metric":    "vmaf",
						"min":       85,
					},
				},
				{
					ID:   "audio",
					Type: "audio",
					Params: map[string]any{
						"input":     "{{input}}",
						"output":    "{{outputDir}}/{{base}}.flac",
						"format":    "flac",
						"normalize": true,
					},
				},
				{
					ID:   "deliver-av1",
					Type: "copy",
					Params: map[string]any{
						"from": "{{steps.av1.output}}",
						"to":   "{{outputDir}}/{{base}}.av1.mp4",
					},
				},
			},
		},
	}
}

// Builtin returns a shipped workflow by name.
func Builtin(name string) (Workflow, bool) {
	for _, wf := range Builtins() {
		if wf.Name == name {
			return wf, true
		}
	}
	return Workflow{}, false
}
