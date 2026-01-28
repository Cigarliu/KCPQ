#!/bin/bash

echo "KCP-NATS Quick Start"
echo "==================="
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed or not in PATH"
    echo "Please install Go from https://golang.org/dl/"
    exit 1
fi

# Download dependencies
echo "Step 1: Downloading dependencies..."
cd "$(dirname "$0")"
go mod tidy
if [ $? -ne 0 ]; then
    echo "Error: Failed to download dependencies"
    exit 1
fi
echo "✓ Dependencies downloaded"
echo ""

# Build examples
echo "Step 2: Building examples..."
mkdir -p bin

echo "  - Building server..."
go build -o bin/server examples/server/main.go
if [ $? -ne 0 ]; then
    echo "Error: Failed to build server"
    exit 1
fi

echo "  - Building subscriber..."
go build -o bin/subscriber examples/subscriber/main.go
if [ $? -ne 0 ]; then
    echo "Error: Failed to build subscriber"
    exit 1
fi

echo "  - Building publisher..."
go build -o bin/publisher examples/publisher/main.go
if [ $? -ne 0 ]; then
    echo "Error: Failed to build publisher"
    exit 1
fi

echo "✓ All examples built"
echo ""

echo "==================="
echo "Build successful!"
echo ""
echo "To run the examples:"
echo ""
echo "1. Start server (terminal 1):"
echo "   ./bin/server"
echo ""
echo "2. Start subscriber (terminal 2):"
echo "   ./bin/subscriber"
echo ""
echo "3. Start publisher (terminal 3):"
echo "   ./bin/publisher"
echo ""
echo "==================="
