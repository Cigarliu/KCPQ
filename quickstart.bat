@echo off
echo KCP-NATS Quick Start
echo ===================
echo.

REM Check if Go is installed
where go >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo Error: Go is not installed or not in PATH
    echo Please install Go from https://golang.org/dl/
    exit /b 1
)

REM Download dependencies
echo Step 1: Downloading dependencies...
cd /d "%~dp0"
go mod tidy
if %ERRORLEVEL% NEQ 0 (
    echo Error: Failed to download dependencies
    exit /b 1
)
echo Dependencies downloaded
echo.

REM Build examples
echo Step 2: Building examples...
if not exist bin mkdir bin

echo   - Building server...
go build -o bin/server.exe examples/server/main.go
if %ERRORLEVEL% NEQ 0 (
    echo Error: Failed to build server
    exit /b 1
)

echo   - Building subscriber...
go build -o bin/subscriber.exe examples/subscriber/main.go
if %ERRORLEVEL% NEQ 0 (
    echo Error: Failed to build subscriber
    exit /b 1
)

echo   - Building publisher...
go build -o bin/publisher.exe examples/publisher/main.go
if %ERRORLEVEL% NEQ 0 (
    echo Error: Failed to build publisher
    exit /b 1
)

echo All examples built
echo.

echo ===================
echo Build successful!
echo.
echo To run the examples, open 3 terminals:
echo.
echo Terminal 1 - Start server:
echo   bin\server.exe
echo.
echo Terminal 2 - Start subscriber:
echo   bin\subscriber.exe
echo.
echo Terminal 3 - Start publisher:
echo   bin\publisher.exe
echo.
echo ===================
pause
