# Workflows

FFBatch workflows are **safe, typed pipelines** for cases that need more than one
tool: repair → remux → encode → quality gate → mux → package → audio → archive.
They are declared as JSON, run sequentially, and never use a shell.

```powershell
ffbatch workflow list
ffbatch workflow run multi-delivery --input "D:\master.mov" --output "D:\Delivery"
ffbatch workflow init multi-delivery          # write editable JSON
ffbatch workflow run multi-delivery.json --input "D:\master.mov" --output "D:\Delivery"
```

## Built-in workflows

| Name | What it chains |
| --- | --- |
| `ts-repair` | probe → FFmpeg remux (+genpts, faststart) → verify → SHA-256 |
| `av1-quality` | probe → SVT-AV1 encode → VMAF gate (≥90) → verified copy |
| `mkv-preserve` | probe → MKVToolNix mux (tracks/chapters/tags/attachments) |
| `hls-delivery` | probe → H.264/AAC mezzanine → HLS package |
| `dash-delivery` | probe → H.264/AAC mezzanine → DASH package |
| `audio-delivery` | probe → FLAC extraction + EBU R128 normalization |
| `multi-delivery` | H.264 MP4 + AV1 + VMAF gate + normalized FLAC in one run |

Run `ffbatch workflow show NAME` to see the exact JSON. `ffbatch workflow init
NAME` writes a copy you can edit.

## Document format

```json
{
  "schema": 1,
  "name": "deliver-archive",
  "description": "MP4 delivery plus AV1 archive with a quality gate",
  "steps": [
    { "id": "probe", "type": "probe" },
    {
      "id": "h264",
      "type": "convert",
      "params": {
        "mode": "encode",
        "format": "mp4",
        "videoCodec": "h264",
        "crf": 20,
        "preset": "medium",
        "audioCodec": "aac",
        "audioBitrate": "192k",
        "faststart": true,
        "hashFiles": true,
        "output": "{{outputDir}}/{{base}}.mp4"
      }
    },
    {
      "id": "av1",
      "type": "convert",
      "when": { "field": "probe.videoCodec", "notEquals": "av1" },
      "params": {
        "mode": "encode",
        "format": "mp4",
        "videoCodec": "av1",
        "av1Encoder": "svt",
        "crf": 30,
        "preset": "medium",
        "audioCodec": "copy",
        "output": "{{work}}/{{base}}.av1.mp4"
      }
    },
    {
      "id": "quality",
      "type": "quality",
      "when": { "field": "steps.av1.path", "exists": true },
      "params": {
        "reference": "{{input}}",
        "candidate": "{{steps.av1.output}}",
        "metric": "vmaf",
        "min": 90
      }
    },
    {
      "id": "archive",
      "type": "copy",
      "when": { "field": "steps.av1.path", "exists": true },
      "params": {
        "from": "{{steps.av1.output}}",
        "to": "{{outputDir}}/{{base}}.av1.mp4"
      }
    }
  ]
}
```

## Step types

### `probe`

Reads media metadata and exposes it as `probe.*`. No parameters.

```json
{ "id": "probe", "type": "probe" }
```

### `convert`

Runs the same FFmpeg argv builder and verification/commit path as the batch
engine. Parameters are `config.Config` fields plus:

| Parameter | Meaning |
| --- | --- |
| `input` | Default: previous step output, or the workflow input |
| `output` | Required output file (templates allowed) |
| `overwrite` | Replace an existing output |
| `verifyOutput` | Probe the temporary output before commit (default true) |
| `hashFiles` | Record SHA-256 hashes |

Useful config fields: `mode` (`remux|encode`), `format`, `videoCodec`
(`h264|h265|av1`), `av1Encoder`, `crf`, `preset`, `audioCodec`, `audioTracks`,
`audioLanguage`, `subtitleCodec`, `trimStart`, `trimEnd`, `aspect`, `faststart`,
`genPts`.

`deleteSource` and `archiveDir` are rejected inside workflows. Use explicit
`copy`/`delete` steps so the pipeline is visible.

### `quality`

| Parameter | Meaning |
| --- | --- |
| `reference` | Default: workflow input |
| `candidate` | Default: previous output |
| `metric` | `vmaf`, `ssim`, `psnr` |
| `min` | Fail the step below this score |
| `threads` | VMAF worker threads |

VMAF needs an FFmpeg build with libvmaf. SSIM/PSNR work on normal builds.

### `mux`

MKVToolNix parameters: `input`, `output`, `title`, `chapters`, `languages`
(`0:ara,1:eng`), `trackNames`, `noAudio`, `noVideo`, `noSubtitles`,
`noChapters`, `overwrite`.

### `package`

| Parameter | Meaning |
| --- | --- |
| `input` | Default: previous output |
| `out` | Output directory |
| `format` | `hls` or `dash` |
| `segment` | Segment seconds (1–60) |
| `tool` | `ffmpeg` (default), `mp4box`, `bento4` |

### `audio`

`input`, `output`, `format` (`flac|opus|mp3|m4a|wav`), `bitrate`, `track`,
`normalize`, `overwrite`.

### `copy`

`from`, `to`, `move` (default false), `overwrite`. Uses a temporary file, size
check and atomic rename, including cross-volume copies.

### `delete`

`path`, `allowInput` (default false), `missingOk`. Deleting the workflow input
requires `allowInput: true`.

## Templates

`{{...}}` references are resolved before each step:

| Template | Value |
| --- | --- |
| `{{input}}` | Absolute input path |
| `{{name}}` | Input filename |
| `{{base}}` | Input filename without extension |
| `{{ext}}` | Lower-case extension |
| `{{work}}` | Per-run work directory |
| `{{outputDir}}` | Final output directory |
| `{{overwrite}}` | Workflow overwrite flag |
| `{{probe.videoCodec}}` | `h264`, `hevc`, `av1`, … |
| `{{probe.audioTracks}}` | Audio track count |
| `{{probe.audioCodecs}}` | Audio codec list |
| `{{probe.audioLanguages}}` | Audio language list |
| `{{steps.ID.output}}` | A step's output path |
| `{{steps.ID.score}}` | A quality score |
| `{{steps.ID.manifest}}` | A package manifest |
| `{{anyCustomVar}}` | `--var anyCustomVar=value` |

## Conditions

`when` can use `field`, `equals`, `notEquals`, `in`, `contains`, `exists`,
`greaterThan`, `lessThan`, and combine with `all`, `any` and `not`.

```json
{ "field": "ext", "in": ["ts", "m2ts"] }
{ "field": "probe.videoCodec", "equals": "hevc" }
{ "field": "probe.audioTracks", "greaterThan": 1 }
{ "field": "probe.audioCodecs", "contains": "opus" }
{ "all": [
  { "field": "probe.videoCodec", "equals": "h264" },
  { "field": "probe.audioTracks", "greaterThan": 0 }
] }
```

## Error handling

- `onError`: `fail` (default) or `continue`.
- `retries`: 0–10, for transient tool failures.
- A failed quality gate is a normal step error; calling `ffbatch workflow run`
  returns exit code 1.

## Safety

- Every `convert`, `mux` and `audio` output is written to a temporary `.part`
  file, verified where applicable, then committed.
- `copy` verifies the byte count before renaming.
- Cross-volume moves fall back to copy + fsync + size check + delete.
- Work directories are removed unless `--keep-work` is set.
- Nothing is executed through a shell; every tool receives an argv array.
- The input is never deleted unless a `delete` step explicitly allows it.
- Existing outputs are refused unless `--overwrite` or `overwrite: true`.

## Limitations

- Workflows are CLI-first. Dashboard integration is planned.
- There is no parallel branch execution yet; steps run sequentially.
- A `fetch` step (yt-dlp) and an adaptive CRF search step are on the roadmap.
- Workflow JSON is trusted input: only run workflows you understand, because
  `copy` and `delete` can touch any path you give them.
