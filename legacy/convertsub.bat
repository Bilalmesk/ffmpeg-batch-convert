@echo off
title Video Converter Batch Tool
chcp 65001 >nul
setlocal EnableDelayedExpansion

:: ================= CONFIG =================
set "FFMPEG=ffmpeg"
set "LOGFILE=convert_log.txt"

:: Video extensions to scan
set "EXTS=*.ts *.mkv *.mov *.avi *.wmv *.flv *.webm"

:: Encoding quality (lower = better quality, bigger file)
set CRF=22

:: Preset: ultrafast / fast / medium / slow / veryslow
set PRESET=fast
:: ==========================================

echo =======================================
echo  Video Batch Converter Started
echo =======================================
echo.

:: ---------- Check ffmpeg ----------
where %FFMPEG% >nul 2>&1
if errorlevel 1 (
    echo [ERROR] FFmpeg not found!
    echo Please install FFmpeg or add it to PATH.
    pause
    exit /b
)

echo FFmpeg detected successfully.
echo Log file: %LOGFILE%
echo.

echo ================================
echo Conversion Log > "%LOGFILE%"
echo ================================ >> "%LOGFILE%"

:: ---------- Scan files ----------
for %%E in (%EXTS%) do (
    for /f "delims=" %%F in ('dir /b /s "%%E" 2^>nul') do (

        set "INPUT=%%F"
        set "OUTPUT=%%~dpnF.mp4"
        set "TEMP=%%~dpnF_tmp.mp4"

        :: Skip if mp4 already exists
        if exist "!OUTPUT!" (
            echo [SKIPPED] Already converted:
            echo %%F
            echo.
            echo SKIPPED: %%F >> "%LOGFILE%"
        ) else (

            echo ---------------------------------------
            echo Converting:
            echo %%F
            echo.

            "%FFMPEG%" -y ^
            -hide_banner ^
            -loglevel error ^
            -stats ^
            -i "%%F" ^
            -map 0 ^
            -c:v libx264 ^
            -preset %PRESET% ^
            -crf %CRF% ^
            -c:a copy ^
            -c:s copy ^
            -movflags +faststart ^
            "!TEMP!"

            if exist "!TEMP!" (

                move /y "!TEMP!" "!OUTPUT!" >nul

                if exist "!OUTPUT!" (
                    del "%%F"
                    echo [OK] Converted and deleted original.
                    echo SUCCESS: %%F >> "%LOGFILE%"
                ) else (
                    echo [ERROR] Output rename failed.
                    echo FAILED: %%F >> "%LOGFILE%"
                )

            ) else (
                echo [ERROR] Conversion failed.
                echo FAILED: %%F >> "%LOGFILE%"
            )

            echo ---------------------------------------
            echo.
        )
    )
)

echo.
echo =======================================
echo All conversions finished!
echo Check convert_log.txt for details.
echo =======================================
pause
