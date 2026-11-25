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
  config     Show, create or validate ffbatch.json
  version    Print build information

Examples:
  ffbatch convert "D:\Recordings" --recursive
  ffbatch watch "D:\Recordings" --recursive --existing
  ffbatch encode "D:\Recordings" --crf 20 --preset slow --delete-source
  ffbatch remux movie.ts --to mp4 --genpts
  ffbatch rename "D:\Shows" --to ts
  ffbatch verify --manifest "D:\Reports\batch.json"
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
  --codec h264|h265            Encoder for encode mode (default: h264)
  --crf N                      Quality, 0-51, lower is better (default: 22)
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
