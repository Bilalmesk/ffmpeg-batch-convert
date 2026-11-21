@echo off
setlocal enabledelayedexpansion

:: Loop through all .ts files in the current directory
for %%f in (*.ts) do (
    echo Processing: %%f
    
    :: Apply the ffmpeg command
    :: %%~nf extracts the filename without the extension
    ffmpeg -i "%%f" -c copy -aspect 16:9 "%%~nf.mp4"
)

echo Conversion complete!
pause