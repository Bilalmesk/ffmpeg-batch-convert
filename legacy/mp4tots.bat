@echo off
setlocal enabledelayedexpansion

:: Check if a folder was dragged onto the script
if "%~1"=="" (
    set "target_dir=%cd%"
) else (
    set "target_dir=%~1"
)

echo Target Directory: "!target_dir!"
echo.

:: Change to the target directory
pushd "!target_dir!"

:: Check for .mp4 files
if not exist *.mp4 (
    echo No .mp4 files found in this directory.
    pause
    exit /b
)

:: Perform the rename
echo Renaming files...
ren *.mp4 *.ts

if %errorlevel% equ 0 (
    echo.
    echo Success! All .mp4 files have been renamed to .ts.
) else (
    echo.
    echo An error occurred during renaming.
)

popd
pause