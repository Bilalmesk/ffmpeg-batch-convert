@echo off
setlocal enabledelayedexpansion

:: Force the command prompt to use UTF-8 encoding for Arabic file names
chcp 65001 >nul

:: Loop through all .ts files in the current directory
for /f "delims=" %%f in ('dir /b *.ts') do (
    echo Processing: "%%f"
    
    :: Remux to MP4, inject faststart for instant seeking, and fix structural index
    ffmpeg -i "%%f" -c copy -movflags +faststart "%%~nf.mp4"
)

echo Conversion complete!
pause