# HTTP API

The dashboard is a single-page app over a small JSON API. All endpoints are
served from the same origin as the UI (default `http://127.0.0.1:8737`).
Multi-step workflows are currently a CLI feature (`ffbatch workflow`); see
[WORKFLOWS.md](WORKFLOWS.md).

## Security rules

- The server binds to loopback by default. Binding to a non-loopback address
  requires `--token` / `FFBATCH_TOKEN` (16+ characters) unless `--insecure` is
  passed explicitly.
- With a token configured, every route requires it. The browser flow is
  `GET /?token=SECRET`: the server sets an HttpOnly, SameSite=Strict cookie and
  redirects to `/`. Non-browser clients may send `Authorization: Bearer SECRET`
  or `X-FFBatch-Token: SECRET`.
- `POST`/`PUT`/`PATCH`/`DELETE` requests **must** send
  `Content-Type: application/json`.
- If an `Origin` header is present it must match the request host.
- Request bodies are limited to 1 MiB.
- Unknown JSON fields are rejected.
- Responses set `Content-Security-Policy`, `X-Frame-Options: DENY`,
  `X-Content-Type-Options: nosniff` and `Referrer-Policy: no-referrer`.
- Errors use `{"error":"message"}` with a meaningful HTTP status. A missing or
  wrong token returns `401`.

## Status and configuration

### `GET /api/status`

```json
{
  "version": "0.5.0",
  "commit": "abc1234",
  "platform": "windows/amd64",
  "go": "go1.24.0",
  "cpus": 8,
  "startedAt": "2026-01-01T10:00:00Z",
  "uptimeSeconds": 42,
  "address": "127.0.0.1:8737",
  "configPath": "C:\\Users\\you\\AppData\\Roaming\\ffbatch\\ffbatch.json",
  "config": { },
  "tool": {
    "ffmpeg": "C:\\ffmpeg\\bin\\ffmpeg.exe",
    "ffprobe": "C:\\ffmpeg\\bin\\ffprobe.exe",
    "version": "ffmpeg version 7.1 ...",
    "ok": true,
    "error": ""
  },
  "activeRun": null,
  "runs": []
}
```

### `GET /api/config`

Returns `{config, configPath, recipes, formats, presets}`. Security and
chain-of-custody fields include `archiveDir` (mutually exclusive with
`deleteSource`), `verifyOutput` (default `true`) and `hashFiles`. Video fields
include `videoCodec` (`h264|h265|av1`), `av1Encoder` (`svt|aom|rav1e`) and
`preset`. `tools` maps optional tool IDs to explicit executable paths. Audio
fields include `audioTracks` (`all|first|preferred`), `audioLanguage`,
`trimStart` and `trimEnd`.

### `PUT /api/config`

```json
{ "config": { }, "save": true }
```

Validates the configuration. When `save` is true it is written atomically to the
active config path. Returns `{config, configPath, saved}`.

### `GET /api/capabilities`

Lists supported formats, presets, default extensions, modes, overwrite and
codec policies, AV1 encoders, and recipes. Useful for clients other than the
bundled UI.

## Scanning and the file system

### `POST /api/scan`

```json
{ "dir": "D:\\Recordings", "recursive": true, "extensions": ["ts", "mkv"], "mode": "remux" }
```

`mode` is optional. Set it to `rename` to include `.mp4` files in the scan
(target-extension renaming needs them), matching the CLI behaviour.

Returns:

```json
{
  "root": "D:\\Recordings",
  "recursive": true,
  "extensions": ["ts", "mkv"],
  "count": 42,
  "totalBytes": 123456789,
  "truncated": false,
  "files": [
    { "path": "...", "name": "episode.ts", "ext": "ts", "sizeBytes": 123, "modTime": "..." }
  ]
}
```

At most 5000 candidates are returned; `truncated` indicates more were found.

### `GET /api/fs?path=...`

Server-side folder browser. With no `path`, opens the user's home folder.
Returns `{path, parent, roots, places, entries}` where entries are directories
only. `roots` contains drive letters on Windows; `places` contains Home,
Videos, the working folder and the configured output folder when they exist.

### `POST /api/reveal`

```json
{ "path": "D:\\Output" }
```

Opens Explorer/Finder/the default file manager. The path must exist.

## Runs

### `POST /api/runs`

```json
{
  "dir": "D:\\Recordings",
  "files": [],
  "config": { },
  "saveConfig": true
}
```

`dir` is scanned with the supplied config. An explicit `files` list (absolute
paths) takes precedence, which is how the dashboard applies checkbox
selection. Exactly one run may be active.

- `201` → full `RunSnapshot`
- `409` another run is active
- `422` no matching files
- `503` FFmpeg is unavailable

### `GET /api/runs`

Returns `{runs: RunSummary[], active: RunSummary | null}` newest first.

### `GET /api/runs/{id}`

Returns a full `RunSnapshot`:

```json
{
  "id": "run-20260101-101500-ab12cd",
  "status": "running",
  "mode": "remux",
  "root": "D:\\Recordings",
  "format": "mp4",
  "workers": 4,
  "total": 3,
  "pending": 1,
  "running": 1,
  "done": 1,
  "skipped": 0,
  "failed": 0,
  "canceled": 0,
  "planned": 0,
  "percent": 55.5,
  "bytesIn": 123,
  "bytesOut": 100,
  "verified": 1,
  "startedAt": "...",
  "finishedAt": null,
  "config": { },
  "jobs": [ ],
  "log": [ { "at": "...", "level": "info", "message": "..." } ]
}
```

Job fields include `id`, `name`, `input`, `output`, `status`, `stage`,
`progress`, `speed`, `fps`, `bitrate`, `etaSeconds`, `outTimeMs`,
`durationMs`, `resolution`, `videoCodec`, `audioCodec`, `command`, `warnings`,
`error`, `reason`, `outputBytes`, `sourceRemoved`, `verified`, `verification`
(`passed|failed|skipped`), `sourceSha256`, `outputSha256`,
`sourceArchivedTo`, `startedAt` and `finishedAt`.

### `POST /api/runs/{id}/cancel`

Requests cancellation. Returns `{ok:true}` or `409` if the run is not active.
FFmpeg is killed through its process context; the run transitions to
`canceled` and partial outputs are removed.

### `POST /api/runs/{id}/retry`

Starts a new run containing only the failed inputs from the original run, with
the original configuration. Successful and skipped jobs are not repeated.

- `201` → new `RunSnapshot`
- `409` another run is active
- `400` the run has no failed jobs
- `404` unknown run

### `GET /api/runs/{id}/report?format=json|csv`

Downloads the run. JSON includes the schema number, generator version, config and
all jobs, including verification status and SHA-256 hashes. This document is also
the input for `ffbatch verify --manifest`, which re-hashes outputs and sources
(including archived copies). CSV has one row per job with size, duration,
codecs, timestamps, errors, reasons, warnings, verification and hashes.

## Live events

### `GET /api/events`

`Content-Type: text/event-stream`. On connect the server sends:

```
retry: 3000

event: snapshot
data: { ...RunSnapshot... }
```

The snapshot is the most recent run, or omitted when there is no history. It is
followed by named events whose `data` is the JSON of an `Event`:

| Event | Extra fields |
| --- | --- |
| `run.started` | `run` summary + `jobs` array |
| `run.canceling` | `run` summary |
| `run.finished` | `run` summary + final `jobs` array |
| `job.started` | `run` summary + `job` |
| `job.updated` | `run` summary + `job` |
| `job.progress` | `run` summary + `job` |
| `job.done` | `run` summary + `job` |
| `job.skipped` | `run` summary + `job` |
| `job.failed` | `run` summary + `job` |
| `job.canceled` | `run` summary + `job` |
| `job.planned` | `run` summary + `job` |
| `log` | `run` summary + `message` + `level` |

A heartbeat comment (`: keepalive`) is sent every 15 seconds. Clients should
merge job objects by `id` and use `run.started`/`run.finished`/`snapshot` to
resynchronise.

## Watch folders

Watched folders are held in server memory and polled on an interval. A file is
queued after its size and modification time have been stable for the settle
window (default 10 s). Only one batch runs at a time; ready files stay queued
while the engine is busy. Output folders, archive folders, and files whose
extension matches the target format are ignored.

### `GET /api/watch`

```json
{
  "running": false,
  "intervalSeconds": 5,
  "settleSeconds": 10,
  "targets": [
    {
      "id": "watch-001-18e9ebe4c6d80ed4",
      "dir": "D:\\Recordings",
      "recursive": true,
      "enabled": true,
      "addedAt": "2026-01-01T10:00:00Z",
      "lastScanAt": "2026-01-01T10:01:00Z",
      "lastStartAt": null,
      "pending": 2,
      "tracked": 40,
      "lastError": ""
    }
  ],
  "events": [
    { "at": "...", "level": "info", "targetId": "watch-...", "message": "Watching D:\\Recordings" }
  ]
}
```

`running` means the conversion engine is busy, not that the watcher is enabled.
Events are newest-first (last 150). `pending` counts known files that have not
been launched; `tracked` is the number of files the watcher remembers.

### `POST /api/watch`

```json
{ "dir": "D:\\Recordings", "recursive": true, "convertExisting": false, "config": { } }
```

`config` is optional; when present it updates the server's current settings
(without saving to disk) before the folder is added. `convertExisting` queues
files already in the folder after the settle window; otherwise they are
ignored. Existing files are always ignored by default.

- `201` → watched `Target`
- `409` the folder is already watched or overlaps another watched folder
- `400` invalid or unreadable folder

### `PATCH /api/watch/{id}`

```json
{ "enabled": false }
```

Pauses or resumes scanning. A batch already running is not affected.

### `DELETE /api/watch/{id}`

Stops watching the folder. Files and runs are untouched. `404` if unknown.

## Doctor

### `GET /api/doctor`

Runs the same checks as `ffbatch doctor`:

```json
{
  "generatedAt": "...",
  "version": "0.5.0",
  "platform": "windows/amd64",
  "goVersion": "go1.24.0",
  "cpus": 8,
  "configPath": "...",
  "configOk": true,
  "health": { "ok": true, "ffmpeg": "...", "ffprobe": "...", "version": "..." },
  "checks": [ { "id": "ffmpeg", "label": "...", "status": "pass", "detail": "...", "hint": "" } ],
  "encoders": { "libx264": true, "libx265": true, "aac": true }
}
```

`status` is `pass`, `warn` or `fail`.

## Static dashboard

`GET /` serves the embedded `index.html`; `/app.css`, `/app.js` and
`/favicon.svg` are served from the same embedded bundle. The UI uses hash
routing (`#/convert`, `#/runs`, `#/doctor`), so no server rewrite is needed.
When a dashboard token is configured, these routes require the same cookie or
bearer token as the API.
