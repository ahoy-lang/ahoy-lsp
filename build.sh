#!/usr/bin/env bash

set -e

echo "Building Ahoy Language Server..."

# Note: older versions killed running ahoy-lsp processes here. Zed does not
# always restart a killed language server, which leaves the editor with no
# diagnostics until it is restarted. The installed command is a symlink, so a
# running server can keep the old image harmlessly; restart Zed (or run
# "editor: restart language server") when convenient to pick up this build.
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

    # Symlink, not copy. A copy goes stale the moment the next `go build`
    # replaces this repository's binary; a symlink always resolves to the
    # artifact that was just built. -n keeps ln from descending into a
    # symlink-to-directory at the destination; -f replaces the old file.
    echo "Linking $HOME/.local/bin/ahoy-lsp -> $SCRIPT_DIR/ahoy-lsp..."
    ln -sfn "$SCRIPT_DIR/ahoy-lsp" "$HOME/.local/bin/ahoy-lsp"

    echo ""
    echo "✓ ahoy-lsp installed to ~/.local/bin/ahoy-lsp (symlink)"
    echo "  $(readlink -f "$HOME/.local/bin/ahoy-lsp")"
    echo ""
    echo "Make sure ~/.local/bin is in your PATH:"
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
    echo "Verify installation:"
    echo "  which ahoy-lsp"
    echo "  ahoy-lsp --version"
    echo ""

    # Older manual installs are copies. They are harmless only while
    # ~/.local/bin comes first in PATH; warn instead of deleting user files.
    for stale in "$HOME/bin/ahoy-lsp" "$HOME/go/bin/ahoy-lsp" "/usr/local/bin/ahoy-lsp"; do
        if [ -e "$stale" ] && [ ! -L "$stale" ]; then
            echo "⚠️  Stale copy found: $stale"
            echo "   It may shadow the symlinked build if PATH order changes."
            if [ -w "$stale" ]; then
                echo "   Remove it with: rm $stale"
            else
                echo "   Remove it with: sudo rm $stale"
            fi
            echo ""
        fi
    done

    echo "Restart Zed (or run 'editor: restart language server') to load this build."
    echo ""
else
    echo "Build failed!"
    exit 1
fi
