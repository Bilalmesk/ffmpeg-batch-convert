# Security policy

FFBatch is a local tool that can read and write files with the permissions of
the user running it. The dashboard is designed to stay on loopback and to reject
cross-origin mutations.

## Supported versions

Security fixes are provided for the latest released minor version.

| Version | Supported |
| --- | --- |
| 0.4.x | ✅ |
| 0.3.x | Security fixes only |
| 0.2.x | Security fixes only |
| < 0.2 | ❌ |

## Reporting a vulnerability

Please **do not open a public issue** for:

- path traversal or arbitrary file write/read
- cross-site request forgery against the local dashboard
- command injection through paths, config values or media metadata
- bypassing the loopback/JSON/same-origin checks
- exposure of the dashboard beyond the intended bind address

Instead, use GitHub's private vulnerability reporting on the repository
(Security → Report a vulnerability) or contact the maintainer directly. Include:

- affected version (`ffbatch version`)
- operating system
- a minimal reproduction (request, config and paths)
- impact and any suggested fix

You can expect an acknowledgement within a few days. Please give a reasonable
window for a fix before public disclosure.

## Scope notes

- FFmpeg/FFprobe vulnerabilities belong to the FFmpeg project; update your
  FFmpeg build.
- The dashboard is not intended to be exposed to the public internet. Binding it
  to a non-loopback address requires a shared-secret token (or the explicit
  `--insecure` opt-out) and remains your responsibility.
- `FFBATCH_TOKEN` and `--token` are secrets. Prefer the environment variable over
  a command line that may be captured in shell history. The dashboard uses HTTP,
  not HTTPS; use a VPN, SSH tunnel or trusted network for sensitive media.
- Media files are untrusted input. Report crashes or unexpected command
  construction, not the media content itself.
- Optional tools (MKVToolNix, MP4Box, Bento4, yt-dlp, aria2c) are invoked with an
  argv array, never through a shell. Report any path or argument that could be
  interpreted as a command.
- `ffbatch fetch` passes user-supplied URLs to yt-dlp. This can make network
  requests; only fetch content you are allowed to download.
