# Legacy batch scripts

These are the original Windows `.bat` files, kept for reference. The Go app
(`ffbatch`) replaces all of them with one tool, a live dashboard and safer
defaults.

| Legacy script | New command |
| --- | --- |
| `convert.bat` | `ffbatch convert <folder> --aspect 16:9` |
| `convert_all_extensions.bat` | `ffbatch convert <folder> --aspect 16:9` (defaults already include all the old extensions) |
| `convert_ts.bat` | `ffbatch convert <folder> --genpts --faststart` |
| `convertsub.bat` | `ffbatch encode <folder> --crf 22 --preset fast --delete-source --log convert_log.txt` |
| `mp4tots.bat` | `ffbatch rename <folder> --to ts` |
| `seeking_fix.bat` | `ffbatch convert <folder> --genpts` |

Or open the dashboard with `ffbatch serve --open` and pick a recipe:

- **Remux to MP4**
- **TS seeking fix**
- **Re-encode**
- **HEVC**
- **MP4 → TS**

The old scripts converted the current folder only and overwrote outputs
silently. `ffbatch` scans subfolders on request, writes through a temporary
file, skips existing outputs by default and can report or undo nothing
destructively unless you ask for `--delete-source`.
