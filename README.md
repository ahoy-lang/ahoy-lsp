# Ahoy Language Server Protocol (LSP)

A Language Server Protocol implementation for the Ahoy programming language, providing IDE features like diagnostics, auto-completion, hover information, and more.

## Architecture

This LSP is structured as a separate module that imports the Ahoy parser and tokenizer from the main Ahoy package:

```
ahoy-lang/
├── ahoy/              # Main Ahoy compiler and toolchain
│   ├── parser.go      # Parser (imported by LSP)
│   ├── tokenizer.go   # Tokenizer (imported by LSP)
│   └── ...
└── ahoy-lsp/          # Language Server (this directory)
    ├── main.go        # LSP entry point
    ├── server.go      # Core LSP server
    ├── diagnostics.go # Syntax error reporting
    ├── completion.go  # Auto-completion
    ├── hover.go       # Hover information
    ├── definition.go  # Go-to-definition
    ├── symbols.go     # Document symbols
    └── ...
```

## Features

- ✅ **Diagnostics** - Real-time syntax error detection
- ✅ **Hover Information** - Documentation on hover
- ✅ **Auto-completion** - Context-aware code completion
- ✅ **Go to Definition** - Navigate to symbol definitions
- ✅ **Document Symbols** - Outline view of code structure
- ✅ **Code Actions** - Quick fixes for common issues
- 🚧 **Semantic Tokens** - Semantic syntax highlighting (disabled, needs column tracking)
- 🚧 **Cross-file Features** - Workspace-wide symbols, references, rename (future)

## Building

### Prerequisites

- Go 1.25 or later
- Access to the parent `ahoy` directory (for parser/tokenizer imports)

### Build Commands

```bash
# From the ahoy-lsp directory
go mod tidy           # Download dependencies
go build              # Build ahoy-lsp binary
./build.sh            # Alternative: use build script

# Install to ~/.local/bin (recommended for editor integration)
go build -o ~/.local/bin/ahoy-lsp
```

### Build Script

The included `build.sh` script builds and installs the LSP:

```bash
chmod +x build.sh
./build.sh
```

This will:
1. Build the `ahoy-lsp` binary
2. Install it to `~/.local/bin/ahoy-lsp`
3. Make it executable

## Installation

### For VS Code

The LSP is automatically used by the `vscode-ahoy` extension. Just install the extension:

```bash
cd ../vscode-ahoy
npm install
npm run package        # Creates .vsix file
code --install-extension vscode-ahoy-0.0.1.vsix
```

### For Zed

The LSP is used by the Zed extension. Build and install:

```bash
cd ../zed-ahoy
cargo build --release
mkdir -p ~/.local/share/zed/extensions/installed/ahoy
cp -r extension.toml languages grammars target/wasm32-wasi/release/zed_ahoy.wasm \
    ~/.local/share/zed/extensions/installed/ahoy/
```

### Manual Installation (Any Editor)

```bash
# Build and install
cd ahoy-lsp
go build -o ~/.local/bin/ahoy-lsp
chmod +x ~/.local/bin/ahoy-lsp

# Ensure ~/.local/bin is in your PATH
export PATH="$HOME/.local/bin:$PATH"

# Verify installation
ahoy-lsp --version  # (if version flag is implemented)
which ahoy-lsp      # Should show ~/.local/bin/ahoy-lsp
```

## Usage

### Running the Server

The LSP communicates via JSON-RPC over stdio:

```bash
ahoy-lsp
```

The server will:
1. Read LSP protocol messages from stdin
2. Parse Ahoy code using the imported parser
3. Send responses and notifications to stdout
4. Log debug information to stderr

### Testing

Test files are included:

```bash
# test.ahoy contains valid Ahoy syntax examples
cat test.ahoy

# Manual protocol test
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"capabilities":{}}}' | ahoy-lsp
```

## Development

### Module Structure

The `go.mod` uses a `replace` directive to import the local Ahoy package:

```go
module github.com/ahoy-lang/ahoy-lsp

require (
    github.com/ahoy-lang/ahoy v0.0.0  // Parser & tokenizer
    go.lsp.dev/jsonrpc2 v0.10.0
    go.lsp.dev/protocol v0.12.0
    // ...
)

// Use local ahoy package during development
replace github.com/ahoy-lang/ahoy => ../ahoy
```

This means:
- **No code duplication** - Parser and tokenizer are always in sync
- **Easy development** - Changes to the parser are immediately available
- **Clean separation** - LSP and compiler are separate concerns
- **Future-ready** - Can switch to GitHub imports when stable

### Importing Ahoy Code

In LSP files, import the Ahoy package:

```go
import "github.com/ahoy-lang/ahoy"

// Use parser
tokens := ahoy.Tokenize(code)
ast, errors := ahoy.Parse(tokens)

// Access types
var token ahoy.Token
var node *ahoy.ASTNode
var err ahoy.ParseError
```

### Adding New Features

1. **Diagnostics** - Add error detection in `diagnostics.go`
2. **Completion** - Add completion items in `completion.go`
3. **Hover** - Add hover information in `hover.go`
4. **Definition** - Enhance symbol tracking in `definition.go`
5. **Symbols** - Update symbol extraction in `symbols.go`

### Debugging

Enable debug output by checking stderr when running the LSP:

```bash
# The server logs to stderr
ahoy-lsp 2>debug.log

# Or in your editor, check the language server output channel
```

Common log messages:
- `Starting Ahoy Language Server...` - Server initialized
- `Initialized with capabilities...` - Handshake complete
- `Error parsing file:` - Parser errors (check syntax)
- Handler panic: - Critical error (report as bug)

## Troubleshooting

### Server Not Starting

**Problem:** Editor can't find `ahoy-lsp`

**Solution:**
```bash
# Verify the binary exists
which ahoy-lsp

# If not found, rebuild and install
cd ahoy-lsp
go build -o ~/.local/bin/ahoy-lsp

# Ensure PATH includes ~/.local/bin
export PATH="$HOME/.local/bin:$PATH"
```

### No Diagnostics Appearing

**Problem:** Syntax errors not highlighted

**Solution:**
- Verify your Ahoy syntax is actually incorrect
- Check that the file is saved (LSP updates on save)
- Check editor's LSP output/logs for parsing errors
- Test with known-bad syntax: `xyz 123 invalid syntax here`

### Hover/Completion Not Working

**Problem:** No information on hover or no completions

**Solution:**
- Ensure you're hovering over a valid symbol
- Check that the document has been successfully parsed
- Verify the LSP server is running (check editor logs)
- Try restarting the LSP server (command palette: "Restart Language Server")

### Import Errors When Building

**Problem:** `cannot find package "github.com/ahoy-lang/ahoy"`

**Solution:**
```bash
# The replace directive should handle this, but if not:
cd ahoy-lsp
go mod tidy

# Verify the replace directive exists in go.mod
grep "replace" go.mod

# Should show: replace github.com/ahoy-lang/ahoy => ../ahoy
```

## Future Enhancements

### Short Term
- [ ] Add column tracking to parser for precise ranges
- [ ] Re-enable semantic tokens once column tracking is added
- [ ] Implement workspace symbols
- [ ] Add find references support
- [ ] Implement rename support

### Long Term
- [ ] Cross-file type inference
- [ ] Workspace-wide diagnostics
- [ ] Code formatting
- [ ] Refactoring actions
- [ ] Debugger protocol support
- [ ] Publish to GitHub and remove replace directive

## Contributing

When making changes:

1. **Parser changes** - Update `../ahoy/parser.go` and rebuild
2. **LSP changes** - Update files in this directory
3. **Test** - Ensure both VS Code and Zed extensions still work
4. **Document** - Update this README with new features

## License

Part of the Ahoy programming language project.

## Links

- [Ahoy Repository](https://github.com/ahoy-lang/ahoy)
- [LSP Specification](https://microsoft.github.io/language-server-protocol/)
- [go.lsp.dev Documentation](https://pkg.go.dev/go.lsp.dev)