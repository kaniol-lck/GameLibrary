@echo off
rem GameLibrary launches as a GUI application, so when it fails before the window
rem is created nothing is visible. This wrapper keeps a console open and shows the
rem error text and exit code, which is the fastest way to diagnose a start-up
rem failure. It is shipped as a release asset next to the executable.
setlocal
cd /d "%~dp0"

echo Starting GameLibrary...
echo.
GameLibrary-windows-amd64.exe
set EXITCODE=%ERRORLEVEL%

echo.
echo ---------------------------------------------------------------
echo Exit code: %EXITCODE%
if not "%EXITCODE%"=="0" (
  echo.
  echo The text above explains why the window did not appear.
  echo A log is also written to:
  echo   %%LOCALAPPDATA%%\GameLibrary\logs
)
echo ---------------------------------------------------------------
echo.
pause
