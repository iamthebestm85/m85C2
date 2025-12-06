#!/bin/bash

set -e  # Exit on error

echo "=== m85 C2 Setup Script ==="

# Check and install Go if not present
if ! command -v go &> /dev/null; then
    echo "Go not found. Installing Go (requires sudo)..."
    sudo apt update
    sudo apt install -y golang-go
else
    echo "Go is already installed."
fi

# Create branding directory (for custom files)
mkdir -p branding

# Initialize Go module (optional, since no deps)
go mod init m85-c2
echo "Initialized Go module."

echo "=== Setup Complete! ==="
echo "Provide your config.json, login.txt, and branding/ files manually."
echo "Run with: go run main.go"
echo "Connect: telnet localhost 1111"
