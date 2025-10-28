#!/usr/bin/env bash

set -e

echo "Building Ahoy Language Server..."

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Clean any previous builds
echo "Cleaning previous builds..."
rm -f ahoy-lsp

# Download dependencies
echo "Downloading dependencies..."
go mod tidy

# Build the language server
echo "Building ahoy-lsp..."
go build -o ahoy-lsp .

# Install to ~/.local/bin if it was successful
if [ -f "ahoy-lsp" ]; then
    echo "Build successful!"

    # Create ~/.local/bin if it doesn't exist
    mkdir -p "$HOME/.local/bin"

    # Copy to ~/.local/bin
    echo "Installing to ~/.local/bin/ahoy-lsp..."
    cp ahoy-lsp "$HOME/.local/bin/ahoy-lsp"
    chmod +x "$HOME/.local/bin/ahoy-lsp"

    echo ""
    echo "✓ ahoy-lsp installed to ~/.local/bin/ahoy-lsp"
    echo ""
    echo "Make sure ~/.local/bin is in your PATH:"
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
    echo "Verify installation:"
    echo "  which ahoy-lsp"
    echo ""
else
    echo "Build failed!"
    exit 1
fi
