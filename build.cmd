@echo off
REM ===========================================================================
REM  build.cmd - Build script for hjson-go (Windows)
REM
REM  Compiles the Hjson Go library and the hjson-cli command line tool.
REM  Run this file from anywhere; it always builds the project located in the
REM  same folder as this script.
REM
REM  Usage:
REM    build.cmd          Build the library and the hjson-cli executable.
REM    build.cmd test     Build, then run "go vet" and the test suite.
REM    build.cmd clean    Remove the bin output directory.
REM ===========================================================================

setlocal EnableExtensions

REM Always operate from the directory that contains this script.
cd /d "%~dp0"

REM --- Make sure the Go toolchain is available. -------------------------------
where go >nul 2>nul
if errorlevel 1 (
    echo [build] ERROR: 'go' was not found in PATH.
    echo [build]        Install Go from https://go.dev/dl/ and try again.
    endlocal
    exit /b 1
)

REM --- Handle the optional "clean" argument. ---------------------------------
if /i "%~1"=="clean" (
    if exist "bin" (
        echo [build] Removing bin directory...
        rmdir /s /q "bin"
    )
    echo [build] Clean complete.
    endlocal
    exit /b 0
)

echo [build] Using:
go version

REM --- Derive a version string from git (optional, never fatal). --------------
set "VERSION="
for /f "delims=" %%i in ('git describe --tags 2^>nul') do set "VERSION=%%i"
if defined VERSION (
    echo [build] Version: %VERSION%
) else (
    echo [build] Version: unknown ^(no git tag found^)
)

REM --- Compile every package to catch any compilation errors. -----------------
echo [build] Compiling all packages ^(go build ./...^)...
go build ./...
if errorlevel 1 goto :failed

REM --- Build the hjson-cli executable into the bin directory. -----------------
if not exist "bin" mkdir "bin"

echo [build] Building hjson-cli -^> bin\hjson.exe ...
if defined VERSION (
    go build -ldflags "-X main.Version=%VERSION%" -o "bin\hjson.exe" ".\hjson-cli"
) else (
    go build -o "bin\hjson.exe" ".\hjson-cli"
)
if errorlevel 1 goto :failed

REM --- Optional: run vet and tests when "test" is passed. ---------------------
if /i "%~1"=="test" (
    echo [build] Running go vet...
    go vet ./...
    echo [build] Running tests...
    go test ./...
    if errorlevel 1 goto :failed
)

echo.
echo [build] SUCCESS. Executable: %CD%\bin\hjson.exe
endlocal
exit /b 0

:failed
echo.
echo [build] BUILD FAILED.
endlocal
exit /b 1
