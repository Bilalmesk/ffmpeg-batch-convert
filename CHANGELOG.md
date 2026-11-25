# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.3.0] - 2026-11-25

### Added

- **Verified commits** (`verifyOutput`, on by default): before a temporary output
  is renamed into place, FFBatch probes it and checks video codec, resolution,
  duration and the expected audio-track count. A mismatch deletes the temporary
  file and fails the job, leaving the source untouched.
- **Chain-of-custody hashes** (`--hash-files`, `hashFiles`): streamed SHA-256 for
  every source and output, embedded in JSON and CSV reports. Hashing is forced
  whenever a source will be archived or deleted.
- **Safe source archiving** (`--archive DIR`, `archiveDir`): move verified
  originals into a mirror tree instead of deleting them. The source is re-checked
  for size/mtime changes first, archive/delete are mutually exclusive, and the
  watcher ignores the archive folder.
- **`ffbatch verify`**: re-check a JSON report manifest by existence, size,
  SHA-256 and source integrity, or probe arbitrary files with ffprobe. Exits 1 on
  any failure, so it can gate backups or publishing.
- **Disk-space preflight** before each job, using platform free-space probes on
  Windows and Unix.
- **Dashboard authentication**: `serve --token` / `FFBATCH_TOKEN` with a
  constant-time comparison, HttpOnly SameSite=Strict cookie after a `?token=`
  login, and `Authorization: Bearer` / `X-FFBatch-Token` for API clients.
- **Supply-chain hardening**: release archives now receive signed build
  provenance and SBOM attestations plus an SPDX SBOM asset; CI runs
  `govulncheck` and CodeQL.

### Changed

- The web dashboard exposes archive folder, output verification and SHA-256
  hashing controls. Job rows show `verified` and `source archived`.
- CLI summaries report the number of verified outputs.
- A non-loopback `serve --host` now requires a token unless `--insecure` is
  passed explicitly.
- `ffbatch.example.json`, README, API and architecture docs describe the trusted
  pipeline and authentication.

### Fixed

- Archive folders are excluded from Watch scans, preventing archived originals
  from being converted again.
- The duration check uses a bounded tolerance so short trimmed outputs cannot
  pass with a badly truncated result.

### Upgrade notes

- Output verification is enabled by default. If an existing workflow produces
  structurally different files on purpose (for example a deliberate resolution
  change), pass `--verify-output=false` or set `"verifyOutput": false`.
- A dashboard bound beyond loopback now refuses to start without `--token` or
  `FFBATCH_TOKEN` unless `--insecure` is supplied.

## [0.2.2] - 2026-11-21

### Fixed

- `go install` now uses the `cmd/ffbatch` entry point, so it installs a binary
  named `ffbatch` (not `ffmpeg-batch-convert`) and reports the release version.
  CI verifies the install and version.
- README install instructions point at `.../cmd/ffbatch@latest`; the development
  tree and architecture docs include the new entry point.

## [0.2.1] - 2026-11-21

### Fixed

- A finished run is now removed from the active slot before its final log
  entry and `run.finished` event. Previously a client that saw a terminal run
  could get a false `409` when immediately starting another batch or retrying.
- Added a regression test that asserts the manager reports no active run while
  emitting the "Run finished" log line.
- Removed stray UTF-8 BOMs from the Makefile, shell/PowerShell build scripts and
  embedded web assets; `scripts/build.sh` now has an executable bit and LF line
  endings.
- Improved dark/light contrast to pass WCAG AA checks, fixed navigation
  accessible names when badge counts are visible, and made the watched-folder
  list semantically valid. Axe is clean on the populated dashboard in both
  themes.

### Changed

- CI now runs an end-to-end FFmpeg smoke conversion on Ubuntu in addition to
  formatting, vet, unit/HTTP tests and builds on Windows and Ubuntu.
- The release workflow calls the CI workflow first, so a tagged release cannot
  be published unless the full matrix and FFmpeg smoke test pass.
- Singular/plural labels ("1 file", "1 worker") in the dashboard, watcher
  activity and CLI summaries.

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
