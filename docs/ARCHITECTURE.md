# Architecture

FFBatch is deliberately dependency-free: the entire backend is the Go standard
library, and the dashboard is plain HTML/CSS/JavaScript embedded with
`go:embed`. This document explains how the pieces fit together.

## Package graph

```
main.go                  release/build entry point
cmd/ffbatch/main.go      go-install entry point
        │
        └── cli
             ├── config
             ├── doctor
             ├── engine
             │    ├── ffmpeg
             │    ├── mediainfo
             │    └── units
             ├── watch
             │    └── engine
             ├── web
             │    ├── engine
             │    ├── doctor
             │    ├── watch
             │    └── config
             ├── ffmpeg
             ├── term
             └── version
```

No package imports `cli` or `web`; the engine can be embedded in another
program without pulling in HTTP or terminal code.

## Lifecycle of a batch

```
Start(ctx, cfg, paths)
  ├─ validate config
  ├─ collectInputs: expand folders, de-duplicate, sort
  ├─ Plan: choose output paths, apply overwrite policy, mark initial skips
  ├─ create Run + context, register in Manager.history
  ├─ broadcast run.started (with a full job snapshot)
  └─ go execute(runCtx, run)
        ├─ N worker goroutines read from jobsCh
        │    ├─ probe with ffprobe
        │    ├─ build argv
        │    ├─ write to a .part temp file
        │    ├─ parse -progress pipe:1
        │    ├─ commit with rename
        │    └─ optionally delete the source
        └─ producer feeds pending jobs, handles cancellation
```

The Manager allows **one active run** at a time. A second `Start` returns
`ErrBusy`; the dashboard surfaces this as HTTP 409. History keeps the last 30
runs in memory.

## Concurrency and locking

- `Manager.mu` guards all run/job fields and the subscriber map. Every mutation
  and every snapshot happens under it.
- Job mutations are wrapped in `setJob`, which copies the job and the run
  summary **inside** the lock, then broadcasts **outside** it. This prevents
  deadlocks and data races.
- Progress events are emitted at most every 500 ms per job by `ffmpeg.Run` and
  are sent to subscribers with a non-blocking `select`. A slow browser can
  never stall FFmpeg.
- `run.started` and `run.finished` carry the full job list so a client that
  missed progress messages can still converge.
- Work is distributed through an unbuffered channel. When the context is
  cancelled, the producer and workers mark remaining jobs `canceled`, and
  `exec.CommandContext` kills the FFmpeg process.

## Watch folders

`internal/watch` is independent of HTTP and the terminal. It is constructed
with four injected functions: current config, busy check, launcher and event
callback. The dashboard uses the live server config and the shared Manager;
the CLI uses the parsed flags and blocks inside the launcher while its renderer
follows the run.

On every poll it scans each enabled folder and keeps a small state machine per
file:

```
new → settling(size/mtime stable for N seconds) → launched → (unchanged) ignored
                                                        └─ source changes → settling
```

Only one batch is returned per tick, so multiple watched folders cannot race
into `ErrBusy`; the others remain pending. Files under the configured output
folder and files already in the target container are excluded. Overlapping
folders are rejected at add time. A failed launch backs the target off before
retrying. Watched folders and their state live in memory for the life of the
process.

## Trim and audio selection

Trim uses **output-side** `-ss` followed by `-t duration`. Input-side seeking is
tempting (it is faster) but on a transport stream whose first keyframe is after
the seek point, FFmpeg can lose the video stream entirely. Output-side seeking
is reliable for both copy and encode. For MP4 stream copy it also adds
`-copyinkf` so the first packets at the cut are not dropped; the trade-off is a
possible brief non-keyframe at the start, which is why Encode mode is documented
as the frame-accurate option.

`audioTracks` is resolved after `ffprobe`: all streams, the first stream, or
every stream matching an ISO 639 language tag. A missing preferred language
falls back to the first stream with a job warning. The selected indices are
mapped explicitly, so dropped tracks are never decoded.

## Progress parsing

FFmpeg is invoked with:

```
-progress pipe:1 -nostats -stats_period 0.5
```

`stdout` carries key=value blocks terminated by `progress=continue|end`.
`stderr` is captured into a 64 KiB tail buffer so a failure can be summarised
without holding an unbounded log in memory.

`ffmpeg.Parser` turns the keys into a `Progress` sample:

- `out_time_us` / `out_time_ms` (both are microseconds in FFmpeg) → elapsed
- probed duration → percentage
- `speed=1.5x` + remaining media → wall-clock ETA
- `progress=end` → 100 %

The percentage is clamped below 100 % until the process actually exits, so a
job cannot look finished while it is still muxing.

## Output commit

FFmpeg writes to a hidden temporary file in the destination folder:

```
.episode.ffbatch-job-003.part.mp4
```

The extension is preserved so FFmpeg can infer the muxer. On success:

1. If the destination appeared during the run, apply the overwrite policy again.
2. `replaceFile` renames over the destination (removing it first on Windows,
   where `os.Rename` refuses to replace).
3. The output size is recorded, then the source is deleted only if requested.

On failure or cancellation the temporary file is removed. Cross-device renames
(in rename mode) fall back to copy + fsync + replace + delete.

## Overwrite planning

| Policy | At plan time | At commit time |
| --- | --- | --- |
| `skip` | Mark `skipped` with a reason | Clean temp, mark skipped |
| `number` | Find `name (n).ext` that is free on disk | Re-number if needed |
| `overwrite` | Keep the path | Remove and replace |
| `fail` | Mark `failed` | Fail the job |

Two inputs that resolve to the same output are disambiguated with a numbered
suffix and a warning, regardless of policy.

## Configuration

`config.Config` is flat, so partial JSON files and CLI overrides merge
predictably. Loading starts from `Default()`, unmarshals over it, then
normalises and validates. Validation is strict enough to catch typos in
enums, CRF ranges, audio bitrate syntax, aspect ratios and extension lists.

The web server owns the mutable copy (`cfgMu`). A start request may carry its
own config; it is validated, used for that run, and optionally persisted.

## Web layer

- `http.ServeMux` with Go 1.22 method/wildcard patterns.
- Middleware enforces JSON content type and same-origin for mutations, and sets
  CSP, `X-Frame-Options`, `X-Content-Type-Options` and `Referrer-Policy`.
- SSE uses a subscriber channel per client, a 15-second keepalive comment and
  `retry: 3000`.
- `GET /api/fs` is a server-side folder browser because browsers cannot expose
  absolute local paths through a file picker.
- **Watch** endpoints are thin wrappers over `internal/watch`; the UI polls
  `/api/watch` every three seconds while the Watch tab is open, and refreshes
  immediately on run start/finish events.
- **Retry** rebuilds a new run from the failed inputs of an existing run via
  `Manager.Start`, reusing the original configuration.
- The UI holds a single copy of the active run, merges job deltas by id and
  rebuilds rows only when the run changes. Progress events update a row in
  place, keeping thousands of jobs responsive.
- Every dynamic string is written with `textContent`; no user-controlled HTML
  is injected.

## Testing

- Table-driven tests for config validation, argv generation, progress parsing,
  frame-rate parsing, path planning, collision numbering, file commit and
  rename fallbacks.
- HTTP tests use `httptest` against the real router, covering static assets,
  config updates, scanning, traversal-free folder browsing and security
  middleware.
- FFmpeg itself is not required by the test suite.
