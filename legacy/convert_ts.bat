@echo off
setlocal enabledelayedexpansion

for %%f in (*.ts) do (
    echo Processing: %%f

    ffmpeg -fflags +genpts -i "%%f" -c copy -movflags +faststart "%%~nf.mp4"
)

echo Conversion complete!
pause
