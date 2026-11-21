# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [0.2.0] - 2026-11-21

### Added

- **Watch folders** in the dashboard and `ffbatch watch` on the CLI. Files are
  converted after their size and modification time stop changing, with
  add/pause/remove controls, an activity feed and overlapping-folder
  protection. The dashboard watcher always uses the current Convert settings.
- **Retry failed** on the Runs screen and `POST /api/runs/{id}/retry`, which
  rebuilds a run from only the failed inputs and the original configuration.
- **Trim in/out** (`--trim-start` / `--trim-end` and Advanced settings), using
  output-side seeking so transport streams keep their video. MP4 stream copy
  adds `-copyinkf` to retain initial non-keyframes.
- **Audio track selection**: all tracks, first track, or every track matching a
  preferred language (`--audio-tracks preferred --audio-lang ara`), with a
  warning and first-track fallback when the language is absent.
- Dashboard watcher timing controls (`serve --watch-interval`, `--watch-settle`)
  and live config synchronisation so Watch sees unsaved Convert changes.
- Watch activity feed, nav badges, and a new Watch screenshot in the docs.
- `workflow_dispatch` CI, PR/issue templates, Dependabot for Actions, security
  policy and code of conduct.

### Fixed

- Trim on TS sources no longer loses the video stream; input-side seeking was
  replaced with output-side seeking plus the keyframe-aware copy flag.
- Watch launches one batch at a time instead of immediately hitting `ErrBusy`
  when several folders have ready files.
- Cancelled jobs no longer show a duplicated "cancelled" error line.

## [0.1.0] - 2026-11-21

### Added

- Single `ffbatch` binary with subcommands: `convert`, `remux`, `encode`,
  `rename`, `scan`, `probe`, `serve`, `doctor`, `config`, `version`.
- Embedded web dashboard with live Server-Sent Events, per-file progress,
  speed, ETA, filters, activity log, cancel and report downloads.
- Server-side folder picker, scan preview, per-file selection and recent
  folders.
- Conversion modes: lossless remux, x264/x265 encode, extension-only rename.
- Built-in recipes reproducing the legacy `.bat` workflows.
- Parallel workers, `skip`/`number`/`overwrite`/`fail` collision policies,
  dry runs, cancellation and graceful shutdown.
- Safe temporary-file commit, cross-volume rename fallback, optional source
  deletion after success.
- Stream-aware planning: real video stream selection, MP4 text subtitle
  conversion, bitmap subtitle warnings and AAC fallback for incompatible audio.
- JSON and CSV run reports, optional text logs, and an in-memory activity feed.
- Environment Doctor: FFmpeg/ffprobe checks, encoder detection, config
  validation and write-access checks.
- Layered JSON configuration, `ffbatch config` helpers and example config.
- CLI flags accepted before or after paths, JSON Lines event output and ANSI
  styling with Windows virtual-terminal support.
- Tests for config, FFmpeg argv, progress parsing, planning, file commits, the
  CLI argument rewriter and the HTTP API.
- GoReleaser configuration and GitHub Actions for CI and tagged releases.
- The original batch scripts preserved under `legacy/`.

### Changed

- Existing outputs are skipped by default instead of silently overwritten.
- Conversion happens through temporary files, never directly into the final
  path.
- Arabic and other Unicode filenames are handled end-to-end as UTF-8.

### Notes

- Requires FFmpeg and FFprobe on `PATH` (or explicit paths in `ffbatch.json`).
- The dashboard binds to `127.0.0.1` unless `--host` is changed.
