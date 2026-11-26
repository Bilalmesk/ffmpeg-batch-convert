# Toolchain and integrations

FFBatch has one hard dependency: **FFmpeg + FFprobe**. Everything else is
optional, detected at runtime, and invoked directly without a shell. This keeps
the core binary small while letting it sit in a larger professional pipeline.

Run `ffbatch tools` to see what is installed. Paths can be overridden per tool in
`ffbatch.json`:

```json
{
  "tools": {
    "mkvmerge": "C:\\Program Files\\MKVToolNix\\mkvmerge.exe",
    "yt-dlp": "C:\\Tools\\yt-dlp.exe"
  }
}
```

## Integrated now

| Tool | Used by | What it adds |
| --- | --- | --- |
| FFmpeg / FFprobe | core | Remux, encode, probe, verification, VMAF/SSIM/PSNR |
| MKVToolNix (`mkvmerge`) | `ffbatch mux` | Track/chapter/tag/attachment-preserving MKV muxing without re-encoding |
| GPAC (`MP4Box`) | `ffbatch package --tool mp4box` | DASH/HLS/fragmented MP4 packaging |
| Bento4 (`mp4fragment`, `mp4dash`) | `ffbatch package --tool bento4` | Fragmented MP4, DASH and HLS packaging |
| yt-dlp | `ffbatch fetch` | Download from thousands of sites, playlists and authenticated sources |
| aria2c | `ffbatch fetch --downloader aria2c` | Multi-connection downloads for large files |
| libvmaf (FFmpeg filter) | `ffbatch quality --metric vmaf` | Perceptual quality measurement for encodes |
| SSIM/PSNR (FFmpeg filters) | `ffbatch quality --metric ssim/psnr` | Signal-fidelity checks that work on every FFmpeg build |
| libsvtav1 / libaom-av1 / librav1e (FFmpeg encoders) | `ffbatch encode --codec av1` | AV1 encoding through the normal verified pipeline |

All of these capabilities are also available as typed steps in
`ffbatch workflow` (`probe`, `convert`, `quality`, `mux`, `package`, `audio`,
`copy`, `delete`). See [WORKFLOWS.md](WORKFLOWS.md) for multi-tool recipes such
as `multi-delivery`.

| `ffbatch audio` | FFmpeg | Extract one audio track to FLAC, Opus, MP3, M4A or WAV with optional loudness normalization |

### Examples

```powershell
# MKV muxing with chapter and language preservation
ffbatch mux "D:\recording.ts" --output "D:\recording.mkv" `
  --title "Interview" --chapters chapters.xml --languages 0:ara,1:eng

# HLS/DASH packaging
ffbatch package "D:\movie.mp4" --out "D:\Web\movie" --format hls --segment 6
ffbatch package "D:\movie.mp4" --out "D:\Web\movie" --format dash --tool mp4box

# Quality gate
ffbatch quality --reference "D:\source.mp4" --candidate "D:\encode.mp4" --min 93

# Ingest and convert
ffbatch fetch "https://example.com/playlist" --output "D:\Incoming" --playlist
ffbatch watch "D:\Incoming" --recursive

# Audio extraction
ffbatch audio "D:\Recordings" --recursive --format flac --output "D:\Audio"
```

## Detected companions

These tools are detected by `ffbatch tools` and are useful in a wider workflow,
but FFBatch does not drive them yet. Using them alongside FFBatch is fully
supported (for example, run them on FFBatch outputs in your own scripts).

| Tool | Best use | Why it is not wired in yet |
| --- | --- | --- |
| MediaInfo | Deep metadata, bit rates, HDR details | FFprobe already covers the fields the engine needs; a second analyzer adds parsing surface |
| HandBrakeCLI | Preset-based transcoding | Its preset/quality model overlaps the FFmpeg encoder path; adding it would duplicate verification and progress logic |
| Streamlink | Live streams / Twitch-style sources | Live ingestion needs continuous process supervision, not a one-shot batch job |
| N_m3u8DL-RE | Fast HLS/DASH downloading | yt-dlp already covers most HLS/DASH sites; this is a specialist alternative |
| SoX | Audio effects, resampling, normalization | Audio-only effects are a separate pipeline; FFmpeg's `loudnorm` covers simple normalization |
| opusenc / flac / lame | Dedicated audio encoders | FFmpeg exposes libopus, FLAC and MP3 encoders; standalone encoders are useful only for exotic options |
| ab-av1 | Automatic CRF search with VMAF | Great next step for a `ffbatch tune` command; `ffbatch quality` already provides the measurement half |
| Av1an | Chunked parallel AV1 encoding | Requires scene splitting, chunk manifests and reassembly; a separate encoding backend |
| VapourSynth / AviSynth+ | Scripted restoration and frame-level filtering | FFmpeg can read `.vpy`/`.avs` when built with support; native integration would need script execution and sandboxing |
| x264 / x265 / rav1e / SVT-AV1 standalone | Finer encoder control | FFmpeg's wrappers are sufficient for batch work; standalone binaries lack the same progress protocol |
| GStreamer | Live capture, WebRTC, complex pipelines | A different execution model (long-running graphs), not a batch transcoder |
| MLT / melt | Timeline editing and compositing | Editing belongs in Kdenlive/Shotcut; FFBatch is deliberately not an NLE |

## Deliberately out of scope

| Project | Why |
| --- | --- |
| **libav** | The historical FFmpeg fork. It is not the maintained codebase; integrating it would add a second, divergent media stack. FFmpeg is the supported engine. |
| **GStreamer** | Excellent for real-time capture and graph pipelines, but it would duplicate FFmpeg and add a large runtime. Use it upstream of FFBatch when you need live capture. |
| **MLT / melt** | A timeline/compositing engine. FFBatch converts files; it does not edit timelines. |
| **PyAV** | Python bindings to FFmpeg libraries. It is for frame-level Python code, not a Go binary. |
| **ffmpeg-python / python-ffmpeg** | Python command builders. FFBatch's Go argv builder already does this, with planning, progress and verification. |
| **MoviePy** | High-level Python editing. Different layer and much slower for batch transcoding. |
| **OpenCV** | Computer vision and frame processing, not a media delivery pipeline. |
| **fluent-ffmpeg** | Node.js wrapper. FFBatch exposes a CLI and JSON API instead. |
| **ffmpeg.wasm** | In-browser transcoding with significant size and performance limits. |
| **imageio-ffmpeg** | Bundles an FFmpeg binary for Python. FFBatch already resolves FFmpeg on the host. |

If you need these in a larger system, treat FFBatch as the batch/verification
stage and call it from Python, Node or a workflow engine. The CLI and JSON
reports are the integration surface.

## Safe external-tool rules- Optional tools are only used by the command that needs them. `ffbatch convert`
  never depends on mkvmerge, MP4Box, Bento4 or yt-dlp.
- Every process is started with an argv array, never through a shell.
- Output paths are created and validated before a command runs.
- `--dry-run` prints the exact command for mux, package and fetch.
- FFmpeg outputs still go through the normal temporary-file, verification,
  SHA-256 and archive pipeline.
- `ffbatch quality` is read-only and does not modify either input.

## Roadmap

1. **`ffbatch tune`** using ab-av1 or a native VMAF search to find the best CRF.
2. **SoX effect presets** on top of the existing audio extraction mode.
3. **MediaInfo analyzer** for richer reports when installed.
4. **HandBrake backend** for preset-driven encodes.
5. **Av1an backend** for chunked parallel AV1.
6. **VapourSynth input presets** with explicit script validation.
7. **Streamlink live capture** as a separate long-running command.
