@echo off
setlocal
cd /d "%~dp0"
if not exist "teamrise.exe" (
  echo The application has not been built. See README.md.
  pause
  exit /b 1
)
echo Teamrise workspace: http://127.0.0.1:8083
echo Keep this window open. Press Ctrl+C to stop the application.
echo If the browser opens before the server is ready, refresh the page.
start "" "http://127.0.0.1:8083"
"%~dp0teamrise.exe" -addr 127.0.0.1:8083
if errorlevel 1 pause
