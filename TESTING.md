# Ahoy LSP Testing Guide

This guide helps you test all the implemented LSP features.

## Prerequisites

1. **Build the LSP server:**
   ```bash
   cd /path/to/ahoy/lsp
   go build -o ahoy-lsp .
   ```

2. **Install the binary:**
   ```bash
   # System-wide
   sudo cp ahoy-lsp /usr/local/bin/

   # Or user installation
   mkdir -p ~/.local/bin
   cp ahoy-lsp ~/.local/bin/
   export PATH="$HOME/.local/bin:$PATH"
   ```

3. **Verify installation:**
   ```bash
   which ahoy-lsp
   # Should output: /usr/local/bin/ahoy-lsp
   ```

## Implemented Features

### ✅ 1. Real-time Diagnostics (Syntax Errors)

**Test File:** Create `test_diagnostics.ahoy`

```ahoy
? Test syntax errors

? Missing 'do' keyword (should show error)
func broken_func
    ahoy "This is missing 'do'"
end

? Missing 'end' keyword (should show error)
func another_broken do
    ahoy "This is missing 'end'"

? Missing 'then' keyword (should show error)
if x is 5
    ahoy "Missing then"
end

? Invalid assignment (should show error)
bad_variable "no colon"

? Valid function (should have NO errors)
func working_func do
    ahoy "This works!"
end
```

**Expected Results:**
- Red squiggly lines under syntax errors
- Hover over errors to see error messages
- Errors appear as you type

**How to Test:**
1. Open the file in Zed/VS Code
2. You should immediately see red underlines on lines with errors
3. Hover over the errors to see messages like "Expected 'do'"
4. Fix an error and watch it disappear

---

### ✅ 2. Autocomplete (Keywords & Operators)

**Test File:** Create `test_autocomplete.ahoy`

```ahoy
? Type these prefixes and trigger autocomplete (Ctrl+Space):

? Type "fu" and press Ctrl+Space
? You should see: func, for suggestions


? Type "lo" and press Ctrl+Space  
? You should see: loop


? Type "el" and press Ctrl+Space
? You should see: else, elseif


? Type "pl" and press Ctrl+Space
? You should see: plus


? Type "ti" and press Ctrl+Space
? You should see: times, true
```

**Expected Results:**
- Autocomplete popup appears with suggestions
- Keywords: if, else, elseif, loop, func, return, etc.
- Word operators: plus, minus, times, div, mod, etc.
- Types: int, float, string, bool, dict, etc.

**How to Test:**
1. Create a new line
2. Start typing a keyword prefix (e.g., "fu")
3. Press Ctrl+Space (or your editor's autocomplete key)
4. Select from suggestions

---

### ✅ 3. Go-to-Definition

**Test File:** Create `test_goto.ahoy`

```ahoy
? Define some symbols
my_variable: 42

func calculate x int y int do
    result: x plus y
    return result
end

status enum:
    PENDING
    ACTIVE
    COMPLETE
end

Point struct:
    x int
    y int
end

? Use the symbols (Ctrl+Click or F12 to jump to definition)
value: my_variable

answer: calculate 5 10

current_status: status.ACTIVE

point: Point
```

**Expected Results:**
- Ctrl+Click (or F12) on `my_variable` jumps to line 2
- Ctrl+Click on `calculate` jumps to line 4
- Ctrl+Click on `status` jumps to line 10
- Ctrl+Click on `Point` jumps to line 16

**How to Test:**
1. Ctrl+Click (or press F12) on a variable/function name
2. Editor should jump to the definition
3. Works for: variables, functions, enums, structs

---

### ✅ 4. Hover Information

**Test File:** Create `test_hover.ahoy`

```ahoy
? Hover over symbols to see type information

? Variable with inferred type
age: 25

? Variable with explicit type
name: "Alice"

? Function with return type
func add x int y int -> int do
    return x plus y
end

? Enum
Color enum:
    RED
    GREEN
    BLUE
end

? Struct
Person struct:
    name string
    age int
end

? Constant
PI :: 3.14159

? Hover over keywords
if age greater 18 then
    ahoy "Adult"
end

? Hover over operators
result: 5 plus 3
```

**Expected Results:**

Hovering over `age` shows:
```
age: int

Variable age
Type: int
Defined at line 4
```

Hovering over `add` shows:
```
func add -> int

Function add
Returns: int
Defined at line 10
```

Hovering over `if` shows:
```
if - Conditional statement
Syntax: if condition then ... end
```

Hovering over `plus` shows:
```
plus - Addition operator (+)
Syntax: result: a plus b
```

**How to Test:**
1. Hover your mouse over any symbol
2. A popup should appear with type info
3. Works for: variables, functions, parameters, keywords, operators

---

### ✅ 5. Document Symbols (Outline View)

**Test File:** Create `test_outline.ahoy`

```ahoy
? Global variables
version: "1.0.0"
debug_mode: true

? Constants
MAX_SIZE :: 100
DEFAULT_NAME :: "Unknown"

? Functions
func initialize do
    ahoy "Starting..."
end

func process data string do
    ahoy "Processing: " data
end

func calculate x int y int -> int do
    return x plus y
end

? Enums
Status enum:
    PENDING
    ACTIVE
    DONE
end

Priority enum:
    LOW
    MEDIUM
    HIGH
end

? Structs
User struct:
    name string
    email string
    age int
end

Config struct:
    host string
    port int
    ssl bool
end
```

**Expected Results:**

Outline/Symbols view shows:
```
📋 Document Symbols
├── 📦 version
├── 📦 debug_mode
├── 🔒 MAX_SIZE
├── 🔒 DEFAULT_NAME
├── ƒ initialize
├── ƒ process
│   └── data: string
├── ƒ calculate -> int
│   ├── x: int
│   └── y: int
├── 🔢 Status
│   ├── PENDING
│   ├── ACTIVE
│   └── DONE
├── 🔢 Priority
│   ├── LOW
│   ├── MEDIUM
│   └── HIGH
├── 📐 User
│   ├── name: string
│   ├── email: string
│   └── age: int
└── 📐 Config
    ├── host: string
    ├── port: int
    └── ssl: bool
```

**How to Test:**
1. Open the Outline/Symbols view in your editor:
   - **Zed**: Cmd+Shift+O (Mac) or Ctrl+Shift+O (Linux)
   - **VS Code**: Ctrl+Shift+O
   - **Neovim**: `:LSOutlineToggle` (with plugin)
2. You should see a hierarchical tree of all symbols
3. Click on a symbol to jump to it

---

### ✅ 6. Code Actions (Quick Fixes)

**Test File:** Create `test_quickfix.ahoy`

```ahoy
? Trigger quick fixes with Ctrl+. or Cmd+.

? Missing 'do' - should offer "Add 'do' keyword"
func broken
    ahoy "test"
end

? Missing 'then' - should offer "Add 'then' keyword"
if x is 5
    ahoy "test"
end

? Word operators - should offer conversion to symbols
result: 5 plus 3        ? Offers: "Convert 'plus' to '+'"
result2: 10 minus 2     ? Offers: "Convert 'minus' to '-'"
result3: 4 times 5      ? Offers: "Convert 'times' to '*'"
result4: 8 div 2        ? Offers: "Convert 'div' to '/'"

? Word comparison - should offer conversion
if x is 5 then          ? Offers: "Convert 'is' to '=='"
    ahoy "equal"
end

? Function without docs - should offer "Add function documentation"
func calculate x int y int do
    return x plus y
end
```

**Expected Results:**

1. **Quick Fix for Missing 'do':**
   - Place cursor on error
   - Press Ctrl+. (or Cmd+.)
   - See: "Add 'do' keyword"
   - Select it → `do` is inserted

2. **Refactor word operators:**
   - Place cursor on `plus`
   - Press Ctrl+.
   - See: "Convert 'plus' to '+'"
   - Select it → line becomes: `result: 5 + 3`

3. **Add documentation:**
   - Place cursor on function line
   - Press Ctrl+.
   - See: "Add function documentation"
   - Select it → comment added above function

**How to Test:**
1. Place cursor on an error or highlighted code
2. Press Ctrl+. (or Cmd+. on Mac)
3. Select a code action from the menu
4. The fix is applied automatically

---

## Testing in Different Editors

### Zed Editor

1. **Install Extension:**
   ```bash
   cd /path/to/zed-ahoy
   cargo build --release
   # Restart Zed
   ```

2. **Verify LSP is Running:**
   - Open any `.ahoy` file
   - Check bottom status bar: should show "ahoy-lsp"
   - Check logs: `~/.config/zed/logs/`

3. **Test All Features:**
   - Diagnostics: Open `test_diagnostics.ahoy`
   - Autocomplete: Type "fu" + Ctrl+Space
   - Go-to-Def: Ctrl+Click on symbol
   - Hover: Hover over any symbol
   - Outline: Cmd+Shift+O (Mac) / Ctrl+Shift+O (Linux)
   - Quick Fix: Ctrl+.

### VS Code

1. **Install Extension** (being developed by another Claude)

2. **Enable LSP Logging:**
   ```json
   {
     "ahoy.trace.server": "verbose"
   }
   ```

3. **Check Output:**
   - View → Output → Select "Ahoy Language Server"

### Neovim

1. **Configure LSP:**
   ```lua
   require('lspconfig').ahoy_lsp.setup{}
   ```

2. **Test Commands:**
   ```vim
   :LspInfo                    " Check LSP status
   :lua vim.lsp.buf.hover()    " Test hover
   gd                          " Go-to-definition
   <C-x><C-o>                  " Autocomplete
   ```

## Troubleshooting

### LSP Not Starting

```bash
# Check if binary exists
which ahoy-lsp

# Test LSP manually
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ahoy-lsp

# Check editor logs
# Zed: ~/.config/zed/logs/
# VS Code: Help → Toggle Developer Tools → Console
# Neovim: :lua print(vim.lsp.get_log_path())
```

### No Diagnostics

1. Make sure file extension is `.ahoy`
2. Try saving the file
3. Check LSP is connected (`:LspInfo` in Neovim, status bar in Zed/VS Code)
4. Check for parser errors in LSP logs

### Autocomplete Not Working

1. Make sure you trigger it (Ctrl+Space)
2. Type at least 1-2 characters first
3. Check LSP is connected
4. Try on a known keyword like "fu" → should suggest "func"

### Go-to-Definition Not Working

1. Make sure you're on an identifier (variable/function name)
2. Try Ctrl+Click or F12 or right-click → "Go to Definition"
3. Check symbol table is built (no parse errors)

### Hover Not Working

1. Make sure you're hovering over a symbol or keyword
2. Wait 1-2 seconds for hover to appear
3. Check HoverProvider is enabled in server capabilities

## Performance Testing

### Large File Test

Create a large file to test performance:

```bash
cat > large_test.ahoy << 'EOF'
? Generate 1000 functions
EOF

for i in {1..1000}; do
  echo "func test_$i x int do" >> large_test.ahoy
  echo "    return x plus $i" >> large_test.ahoy
  echo "end" >> large_test.ahoy
  echo "" >> large_test.ahoy
done
```

**Expected:**
- LSP should handle this without lag
- Diagnostics should appear within 1-2 seconds
- Go-to-definition should be instant
- Outline should show all 1000 functions

## Feature Comparison

| Feature              | Status | Editor Support           |
|---------------------|--------|--------------------------|
| Diagnostics         | ✅     | All editors              |
| Autocomplete        | ✅     | All editors              |
| Go-to-Definition    | ✅     | All editors              |
| Hover Info          | ✅     | All editors              |
| Document Symbols    | ✅     | All editors              |
| Code Actions        | ✅     | All editors              |
| Semantic Tokens     | 🚧     | Disabled (protocol issue)|
| Find References     | 🚧     | Coming soon              |
| Rename              | 🚧     | Coming soon              |
| Signature Help      | 🚧     | Coming soon              |

## Next Steps

If all tests pass:
1. ✅ LSP is working correctly
2. ✅ All major features are functional
3. ✅ Ready for daily use

To contribute:
1. See `lsp/README.md` for architecture
2. Check `symbol_table.go` for symbol tracking
3. Look at `code_actions.go` for adding new quick fixes
4. Study `hover.go` for adding more documentation

## Reporting Issues

If something doesn't work:
1. Check LSP logs (see Troubleshooting above)
2. Verify binary is in PATH: `which ahoy-lsp`
3. Test with minimal file (just one function)
4. Check Go version: `go version` (need 1.20+)
5. Rebuild LSP: `cd lsp && go build -o ahoy-lsp .`

Happy coding with Ahoy! 🚢