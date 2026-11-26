# Contributing

Thanks for taking a look. This is a small, dependency-free project; keeping it
that way is part of the design.

## Ground rules

- **Go standard library only.** New modules need a very good reason.
- **Optional tools are opt-in.** MKVToolNix, MP4Box, Bento4, yt-dlp and other
  executables are detected at runtime and used only by their command. Core
  conversion must keep working with FFmpeg alone.
- **No UI build step.** The dashboard is HTML, CSS and ES2020 in
  `internal/web/assets/`. Rebuild the binary to refresh the embedded copy.
- **Test the engine, not FFmpeg.** The default test suite must pass on a machine
  without FFmpeg installed. The end-to-end pipeline test skips itself when
  `ffmpeg` is absent.
- **Never silently weaken a safety check.** Verification, hashing and archive
  paths are covered by focused tests; add cases for new failure modes.
- **Update `docs/API.md`** when an endpoint, event or job field changes.

## Local workflow

```powershell
go test ./...
go vet ./...
.\scripts\build.ps1
.\dist\ffbatch.exe serve --open
```

On macOS/Linux use `make build` and `./dist/ffbatch serve --open`.

Watch-folder changes should include watcher tests (`internal/watch`) and a
manual check that files are picked up, queued while busy, and ignored when they
are still growing.

## Continuous integration

Every pull request runs formatting, `go vet`, the unit/HTTP suite, `govulncheck`,
a full build on Windows and Ubuntu, CodeQL analysis, and an end-to-end FFmpeg
conversion smoke test on Ubuntu that also verifies hashes and rejects a tampered
file. Tests cover AV1 argv, VMAF/SSIM/PSNR parsing, MKVToolNix muxing, HLS/DASH
packaging and yt-dlp arguments. Integration tests skip when their optional tool
is not installed. Release tags run the same CI matrix before GoReleaser publishes
archives.

## Pull requests

1. Fork and branch from `main`.
2. Keep commits focused; `docs:`, `test:` and `chore:` prefixes are ignored by
   the changelog.
3. Run `gofmt -w .`, `go vet ./...` and `go test ./...`.
4. For UI changes, include a before/after screenshot and check the console for
   errors.
5. Describe the user-visible behavior in the PR body.

## Reporting bugs

Include:

- `ffbatch version`
- `ffbatch doctor --json`
- the command or a screenshot of the dashboard
- the failing job's error line (Runs → job, or the JSON report)
- the FFmpeg version (`ffmpeg -version`)

Do not paste full media files. A small `ffprobe` excerpt is usually enough.

## Security

The dashboard is a local tool that can read and write files under the user's
account. Please report path traversal, CSRF, command injection or arbitrary
write issues privately to the repository owner before opening a public issue.
