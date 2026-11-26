package cli

const mainUsage = `FFBatch - a fast, friendly FFmpeg batch converter

Usage:
  ffbatch <command> [flags] [paths...]

Commands:
  convert    Convert files or folders (stream copy by default)
  watch      Watch folders and convert new files automatically
  remux      Shortcut for: convert --mode remux
  encode     Shortcut for: convert --mode encode
  rename     Change file extensions without re-muxing
  scan       List convertible files in a folder
  probe      Inspect streams, duration and size of one file
  serve      Open the local web dashboard
  doctor     Verify FFmpeg, encoders, config and output folders
  verify     Check outputs against a report manifest (size + SHA-256)
  tools      Detect optional muxing, packaging, ingest and quality tools
  quality    Compare a candidate encode with a reference (VMAF/SSIM/PSNR)
  mux        Mux MKV tracks/chapters/tags with MKVToolNix
  package    Package a file as HLS or DASH
  fetch      Download media with yt-dlp
  audio      Extract or convert audio tracks
  config     Show, create or validate ffbatch.json
  version    Print build information

Examples:
  ffbatch convert "D:\Recordings" --recursive
  ffbatch watch "D:\Recordings" --recursive --existing
  ffbatch encode "D:\Recordings" --crf 20 --preset slow --delete-source
  ffbatch remux movie.ts --to mp4 --genpts
  ffbatch rename "D:\Shows" --to ts
  ffbatch verify --manifest "D:\Reports\batch.json"
  ffbatch quality --reference source.mp4 --candidate encode.mp4
  ffbatch audio "D:\Recordings" --recursive --format flac
  ffbatch tools
  ffbatch serve

Run "ffbatch help <command>" for all flags.
`

const convertUsage = `Convert files or folders with FFmpeg.

Usage:
  ffbatch convert [flags] [paths...]

Paths can be files or folders. With no path, the current folder is used.
Folders are scanned for the configured extensions (see --ext).

Modes:
  remux    Stream-copy audio/video into a new container (fast, lossless)
  encode   Re-encode video with x264 or x265 (smaller, slower)
  rename   Change the extension only, without touching the media data

Flags:
  --mode remux|encode|rename   Workflow (default: from ffbatch.json, else remux)
  --recipe NAME                Apply a built-in workflow:
                               remux-mp4, ts-seeking-fix, encode-mp4,
                               encode-hevc, rename-ts
  --to FORMAT                  Output container: mp4, mkv, mov or ts
  --output DIR                 Destination folder (default: next to the source)
  --recursive                  Include sub-folders
  --ext LIST                   Comma-separated source extensions
  --workers N                  Parallel FFmpeg processes (default: up to 4)
  --overwrite POLICY           skip | overwrite | number | fail (default: skip)
  --delete-source              Delete the original after a verified job
  --archive DIR                Move the original into DIR after a verified job
  --verify-output[=false]      Probe outputs before they replace anything (default: on)
  --hash-files                 Record SHA-256 for sources and outputs
  --dry-run                    Print the planned commands without writing files

Video:
  --codec h264|h265|av1            Encoder for encode mode (default: h264)
  --av1-encoder svt|aom|rav1e     AV1 implementation (default: svt)
  --crf N                      Quality, 0-51 (AV1: 0-63), lower is better (default: 22)
  --preset NAME                ultrafast ... veryslow (default: fast)
  --faststart[=false]          MP4 faststart index (default: on)
  --aspect RATIO               Force display aspect, e.g. 16:9 (empty: keep)
  --genpts[=false]             Regenerate timestamps (default: on)

Audio and subtitles:
  --audio copy|aac|opus|none   Audio handling (default: copy)
  --audio-bitrate RATE         Bitrate for AAC/Opus (default: 192k)
  --audio-tracks MODE          all | first | preferred (default: all)
  --audio-lang CODE            Preferred ISO 639-2 language, e.g. ara, eng
  --subs auto|copy|mov_text|none
                               Subtitle handling (default: auto)
  --trim-start TIME            Start time: seconds, MM:SS or HH:MM:SS
  --trim-end TIME              End time (same formats; output stops there)

Diagnostics:
  --log FILE                   Append a text log of the run
  --report FILE                Write a .json or .csv report when finished
  --json                       Stream machine-readable events (JSON Lines)
  --quiet                      Only print errors and the final summary
  --config FILE                Use a specific ffbatch.json
  --no-color                   Disable ANSI colours

Examples:
  ffbatch convert "D:\TV" --recursive
  ffbatch encode "D:\TV" --codec h265 --crf 24 --preset medium
  ffbatch convert movie.mkv --to mkv --subs copy
  ffbatch rename "D:\Shows" --to ts --output "D:\Renamed"
  ffbatch convert "D:\TV" --dry-run --json
`

const watchUsage = `Watch folders and convert new files automatically.

Usage:
  ffbatch watch [flags] [folders...]

Every convert flag is accepted, plus the watcher flags below. Settings come from
ffbatch.json unless a flag overrides them. A file is processed after its size
and modification time have been stable for --settle seconds. New files are
queued while a batch is running and picked up when it finishes.

Watcher flags:
  --interval SECONDS   How often to scan (default: 5)
  --settle SECONDS     Stability window before converting (default: 10)
  --existing           Also convert files that are already in the folder

All convert flags are available, including:
  --recursive, --output, --archive, --mode, --to, --workers, --overwrite,
  --delete-source, --verify-output, --hash-files, --codec, --crf, --preset,
  --audio, --audio-tracks, --audio-lang, --subs, --trim-start, --trim-end,
  --dry-run, --report, --log

Watchers run until Ctrl+C. Overlapping folders are rejected, output folders are
ignored, and files already in the target format are skipped.

Examples:
  ffbatch watch "D:\Recordings" --recursive
  ffbatch watch "D:\Recordings" "D:\Anime" --settle 20 --output "D:\Converted"
  ffbatch watch "D:\TV" --existing --delete-source --audio-tracks preferred --audio-lang ara
`

const scanUsage = `List files that a batch would pick up.
Usage:
  ffbatch scan [flags] [folder]

Flags:
  --recursive          Include sub-folders
  --ext LIST           Comma-separated extensions to match
  --mode MODE          remux | encode | rename (rename also scans .mp4)
  --json               Print JSON instead of a table
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

Example:
  ffbatch scan "D:\Recordings" --recursive
  ffbatch scan "D:\Shows" --mode rename
`

const probeUsage = `Inspect one media file with ffprobe.

Usage:
  ffbatch probe [flags] <file>

Flags:
  --json               Print the raw JSON structure
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

Example:
  ffbatch probe episode.ts
`

const serveUsage = `Start the local FFmpeg dashboard.

Usage:
  ffbatch serve [flags]

Flags:
  --host ADDRESS       Bind address (default: 127.0.0.1)
  --port PORT          HTTP port (default: 8737, 0 picks a free port)
  --token SECRET       Require this shared secret (or set FFBATCH_TOKEN)
  --insecure           Allow a non-loopback bind without a token
  --watch-interval N   Folder scan interval in seconds (default: 5)
  --watch-settle N     File stability window in seconds (default: 10)
  --open               Open the dashboard in the default browser
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

The server listens on loopback only unless you change --host. A non-loopback
bind requires --token (at least 16 characters) unless you pass --insecure.
Open the printed URL with ?token=SECRET once; the server sets an HttpOnly
cookie and redirects to /. API clients may instead send
"Authorization: Bearer SECRET" or "X-FFBatch-Token: SECRET".

Press Ctrl+C to stop it.

Example:
  ffbatch serve --open
  ffbatch serve --host 0.0.0.0 --token "a-long-random-secret"
`

const doctorUsage = `Check the local FFmpeg setup and configuration.

Usage:
  ffbatch doctor [flags]

Flags:
  --json               Print the full report as JSON
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

Example:
  ffbatch doctor
`

const verifyUsage = `Check outputs against a run report.

Usage:
  ffbatch verify --manifest REPORT.json [--json]
  ffbatch verify FILE [FILE...] [--json]

Manifest mode checks every completed job in a JSON report: the file must exist,
its size must match, and its SHA-256 must match the recorded output hash. If the
report contains a source hash, the original or archived source is checked too.

File mode probes each file with ffprobe to confirm it is readable media.

Flags:
  --manifest FILE      JSON report written by --report
  --json               Print results as JSON
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

Exit code is 0 when everything passes or is skipped, 1 on any failure.

Examples:
  ffbatch verify --manifest ffbatch-20260101-120000.json
  ffbatch verify episode.mp4
`

const toolsUsage = `Detect the optional tools that FFBatch can use.

Usage:
  ffbatch tools [flags]

Flags:
  --json               Print the registry as JSON
  --missing            Only show tools that were not found
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

Core FFmpeg/FFprobe are required. Everything else is optional and is only
invoked by the commands that need it: mkvmerge (mux), MP4Box/Bento4 (package),
yt-dlp (fetch), libvmaf (quality), and companion tools such as MediaInfo, SoX,
HandBrakeCLI, Streamlink, Av1an and VapourSynth.

Examples:
  ffbatch tools
  ffbatch tools --missing
  ffbatch tools --json
`

const qualityUsage = `Compare a candidate encode with a reference using FFmpeg filters.

Usage:
  ffbatch quality --reference REF --candidate FILE [flags]

Flags:
  --reference FILE     Original/reference video
  --candidate FILE     Encoded/output video
  --metric vmaf|ssim|psnr
                       Quality metric (default: vmaf)
  --min SCORE          Exit 1 when the score is below this value
  --threads N          VMAF worker threads (default: up to 8)
  --json               Print the result as JSON
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

VMAF requires an FFmpeg build with libvmaf. SSIM and PSNR are built in.
Candidates at a different resolution are scaled to the reference resolution.
Only the shorter duration is compared.

Examples:
  ffbatch quality --reference source.mp4 --candidate encode.mp4
  ffbatch quality --reference source.mp4 --candidate encode.mp4 --min 93
  ffbatch quality --reference source.mp4 --candidate encode.mp4 --metric ssim
`

const muxUsage = `Mux media into MKV with MKVToolNix (no re-encoding).

Usage:
  ffbatch mux <input> --output <output.mkv> [flags]

Flags:
  --output FILE        Output .mkv file (required)
  --title TITLE        Set the segment title
  --chapters FILE      Chapters file (XML/OGM/Matroska)
  --languages LIST     Track languages as ID:lang, e.g. 0:ara,1:eng
  --track-names LIST   Track names as ID:name, e.g. 0:Arabic
  --no-audio           Drop audio tracks
  --no-video           Drop video tracks
  --no-subtitles       Drop subtitle tracks
  --no-chapters        Drop chapters
  --overwrite          Replace an existing output
  --tool mkvmerge      Backend (currently mkvmerge)
  --dry-run            Print the command without running it
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

mkvmerge preserves tracks, chapters, tags and attachments better than a
generic FFmpeg remux, and never re-encodes.

Example:
  ffbatch mux "D:\recording.ts" --output "D:\recording.mkv" --languages 0:ara,1:eng --title "Interview"
`

const packageUsage = `Package a finished file as HLS or DASH.

Usage:
  ffbatch package <input> --out DIR [--format hls|dash] [flags]

Flags:
  --out DIR            Output directory (required)
  --format hls|dash    Streaming format (default: hls)
  --segment SECONDS    Segment length, 1-60 (default: 6)
  --tool auto|ffmpeg|mp4box|bento4
                       Packaging backend (default: ffmpeg)
  --dry-run            Print the commands without running them
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

The FFmpeg backend stream-copies the first video and all audio tracks, so the
source codecs must already be HLS/DASH-friendly. MP4Box and Bento4 are optional
advanced backends; install them to use --tool mp4box or --tool bento4.
Use a fresh output directory: existing segments are not deleted.

Examples:
  ffbatch package movie.mp4 --out D:\Web\movie --format hls
  ffbatch package movie.mp4 --out D:\Web\movie --format dash --segment 4
  ffbatch package movie.mp4 --out D:\Web\movie --tool mp4box --dry-run
`

const fetchUsage = `Download media with yt-dlp for later conversion.

Usage:
  ffbatch fetch <url> [url...] --output DIR [flags]

Flags:
  --output DIR         Download folder (required)
  --format SELECTOR    yt-dlp format selector, e.g. "bv*+ba/b"
  --audio-only         Extract audio instead of video
  --audio-format FMT   flac, opus, mp3, m4a or wav (default: flac)
  --cookies FILE       Netscape cookies file for authenticated sites
  --downloader NAME    Use an external downloader, e.g. aria2c
  --playlist           Download the whole playlist (default: single item)
  --write-info-json    Save metadata next to each download
  --archive FILE       Download archive (default: DIR/.ffbatch-download-archive.txt)
  --no-archive         Do not use a download archive
  --restrict-filenames ASCII-only filenames
  --dry-run            Print the command without downloading
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

A download archive makes repeated runs skip items that were already fetched.
The downloaded folder can be passed straight to convert or watch.

Examples:
  ffbatch fetch "https://example.com/video" --output "D:\Incoming"
  ffbatch fetch "https://example.com/playlist" --output "D:\Incoming" --playlist
  ffbatch fetch "https://example.com/audio" --output "D:\Music" --audio-only --audio-format opus
`

const audioUsage = `Extract or convert the audio from media files.

Usage:
  ffbatch audio <file-or-folder...> [flags]

Flags:
  --output DIR         Write files here (default: next to each source)
  --format FMT         flac | opus | mp3 | m4a | wav (default: flac)
  --bitrate RATE       Bitrate for Opus/MP3/M4A (defaults: 128k/192k/192k)
  --track N            Audio track index to extract (default: 0)
  --recursive          Scan sub-folders
  --normalize          Apply EBU R128 loudness normalization
  --overwrite          Replace existing outputs
  --json               Print a JSON result array
  --dry-run            Print commands without writing files
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

FLAC and WAV are lossless. Opus is efficient for speech; MP3/M4A are broadly
compatible. The source video is discarded, and every output is probed before it
is moved into place.

Examples:
  ffbatch audio "D:\Recordings" --recursive --format flac --output "D:\Audio"
  ffbatch audio interview.mkv --format opus --bitrate 96k --normalize
  ffbatch audio "D:\Music" --format mp3 --overwrite
`

const configUsage = `Inspect or create ffbatch.json.

Usage:
  ffbatch config <show|path|init|validate> [flags]

Commands:
  show        Print the effective configuration (merged with defaults)
  path        Print the configuration file that would be used
  init        Create a commented default configuration in the current folder
  validate    Check the configuration for errors

Flags:
  --json               JSON output for show and validate
  --path FILE          Target file for init (default: ./ffbatch.json)
  --force              Overwrite an existing file with init
  --config FILE        Use a specific ffbatch.json
  --no-color           Disable ANSI colours

Examples:
  ffbatch config init
  ffbatch config show --json
  ffbatch config validate
`
