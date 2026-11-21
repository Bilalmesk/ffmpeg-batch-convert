@echo off
setlocal enabledelayedexpansion

:: Define the extensions you want to process (separated by spaces)
set "exts=*.ts *.mkv *.mov *.avi *.wmv *.flv"

:: Loop through the specified extensions
for %%e in (%exts%) do (
    for %%f in ("%%e") do (
        :: Check if the file exists to avoid errors with empty extension matches
        if exist "%%f" (
            echo Processing: %%f
            
            :: Apply the ffmpeg command
            :: %%~nf extracts the filename without the extension
            :: The output is forced to .mp4
            ffmpeg -i "%%f" -c copy -aspect 16:9 "%%~nf.mp4"
        )
    )
)

echo.
echo Conversion complete!
pause