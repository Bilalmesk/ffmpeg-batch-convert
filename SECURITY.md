# Security policy

FFBatch is a local tool that can read and write files with the permissions of
the user running it. The dashboard is designed to stay on loopback and to reject
cross-origin mutations.

## Supported versions

Security fixes are provided for the latest released minor version.

| Version | Supported |
| --- | --- |
| 0.2.x | ✅ |
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
  to a non-loopback address is an explicit opt-in and your responsibility.
- Media files are untrusted input. Report crashes or unexpected command
  construction, not the media content itself.
