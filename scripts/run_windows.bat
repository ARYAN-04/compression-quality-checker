@echo off
setlocal enabledelayedexpansion

echo =========================================================
echo    can-i-compress-it: Lab Benchmark Runner (Windows)
echo =========================================================
echo.

rem Default configuration for local LM Studio testing
set DEFAULT_MODEL=qwen2.5-7b-instruct
set DEFAULT_ENDPOINT=http://localhost:1234/v1

if "%~1"=="" (
    set MODEL=%DEFAULT_MODEL%
) else (
    set MODEL=%~1
)

if "%~2"=="" (
    set ENDPOINT=%DEFAULT_ENDPOINT%
) else (
    set ENDPOINT=%~2
)

echo [CONFIG] Target Model:    %MODEL%
echo [CONFIG] Server Endpoint: %ENDPOINT%
echo.

rem Step 1: Ensure executable exists
if not exist "can-i-compress-it.exe" (
    echo [INFO] can-i-compress-it.exe not found in current folder.
    echo [INFO] Attempting to build binary using Go...
    where go >nul 2>nul
    if %errorlevel% neq 0 (
        echo [ERROR] Go compiler not detected and can-i-compress-it.exe missing.
        echo Please copy can-i-compress-it.exe into this directory or install Go.
        echo.
        pause
        exit /b 1
    )
    go build -o can-i-compress-it.exe .\cmd\can-i-compress-it
    if %errorlevel% neq 0 (
        echo [ERROR] Build failed. Please inspect compiler errors above.
        pause
        exit /b 1
    )
    echo [OK] Binary built successfully.
    echo.
)

rem Step 2: Ensure results output directory exists
if not exist "results" mkdir results

rem Step 3: Run quick smoke test to verify server is alive
echo [1/2] Verifying model endpoint connectivity (smoke test)...
.\can-i-compress-it.exe test ^
    --endpoint "%ENDPOINT%" ^
    --model "%MODEL%" ^
    --prompt "Get issue 42 in repo torvalds/linux" ^
    --schema github ^
    --level none

if %errorlevel% neq 0 (
    echo.
    echo [ERROR] Smoke test failed!
    echo Checklist for lab assistants:
    echo   1. Is LM Studio open?
    echo   2. Did you click the green "Start Server" button on port 1234?
    echo   3. Is model "%MODEL%" loaded in LM Studio?
    echo.
    pause
    exit /b 1
)

echo [OK] Smoke test passed! Model is responsive.
echo.

rem Step 4: Run full factorial benchmark matrix (150 cells)
echo [2/2] Running full factorial benchmark matrix across all 5 compression tiers...
echo (This may take several minutes depending on your GPU/model speed)
echo.

rem Sanitize model name for filename safety (strip slashes, colons, spaces)
set SAFE_MODEL=%MODEL:/=_%
set SAFE_MODEL=%SAFE_MODEL:\=_%
set SAFE_MODEL=%SAFE_MODEL::=_%
set SAFE_MODEL=%SAFE_MODEL: =_%

rem Generate ISO timestamp (YYYYMMDD_HHMMSS) so subsequent runs never overwrite
for /f %%a in ('powershell -NoProfile -Command "Get-Date -Format yyyyMMdd_HHmmss"') do set TIMESTAMP=%%a
if "%TIMESTAMP%"=="" set TIMESTAMP=%RANDOM%

set REPORT_FILE=results\%SAFE_MODEL%_%TIMESTAMP%_report.html
set JSON_FILE=results\%SAFE_MODEL%_%TIMESTAMP%_results.json
set TABLE_FILE=results\%SAFE_MODEL%_%TIMESTAMP%_table.md

.\can-i-compress-it.exe run ^
    --endpoint "%ENDPOINT%" ^
    --model "%MODEL%" ^
    --levels none,low,medium,high,max ^
    --output "%JSON_FILE%" ^
    --html-report "%REPORT_FILE%" ^
    --md-table "%TABLE_FILE%"

if %errorlevel% neq 0 (
    echo.
    echo [ERROR] Benchmark execution encountered an error.
    pause
    exit /b 1
)

echo.
echo =========================================================
echo [SUCCESS] Benchmark completed successfully!
copy /y "%REPORT_FILE%" "results\%SAFE_MODEL%_latest_report.html" >nul
echo Reports generated:
echo   - Interactive HTML: %REPORT_FILE%
echo   - Latest HTML Copy: results\%SAFE_MODEL%_latest_report.html
echo   - JSON Data Trace:  %JSON_FILE%
echo   - Markdown Table:   %TABLE_FILE%
echo.
echo Launching HTML report in your default browser...
echo =========================================================
start "" "%REPORT_FILE%"

pause
