# Performance and Stability Fixes Needed

## Critical Issues Found

### 1. Parser - Infinite Loop Risk in Multi-Line Switch Cases
**File:** `../ahoy/parser.go` line ~1097
**Issue:** `for` loop parsing multi-line case blocks has NO safety limit
**Risk:** If parseStatement() returns nil without advancing position → infinite loop → 100% CPU

**Fix Required:**
```go
// Parse multi-line case block
caseBody = &ASTNode{Type: NODE_BLOCK}
caseBlockIterations := 0
maxCaseBlockIterations := 1000
lastPos := p.pos

for {
    caseBlockIterations++
    
    // Safety check: prevent infinite loops
    if caseBlockIterations > maxCaseBlockIterations {
        errMsg := fmt.Sprintf("Safety limit reached parsing case block at line %d", node.Line)
        if p.LintMode {
            p.recordError(errMsg)
        }
        break
    }
    
    // Check for end of case block
    if p.current().Type == TOKEN_DEDENT || p.current().Type == TOKEN_EOF {
        break
    }
    
    // ... existing code ...
    
    // Parse statement in case block
    stmt := p.parseStatement()
    if stmt != nil {
        caseBody.Children = append(caseBody.Children, stmt)
    }
    
    // Safety: ensure position advanced
    if p.pos == lastPos {
        // Stuck! Break to avoid infinite loop
        break
    }
    lastPos = p.pos
}
```

### 2. LSP Diagnostics - No Depth Limits on Recursive Checkers
**Files:** `diagnostics.go` - multiple functions
**Issue:** Recursive `checkNode` functions have NO depth limits
**Risk:** Deep/malformed AST → stack overflow → crash or 100% CPU

**Functions Affected:**
- `checkConstReassignment` (line ~280)
- `checkConstMethodCalls` (line ~360)
- `checkInvalidMethodCalls` (line ~440)
- `checkReturnTypeViolations` (line ~590)
- `checkEnumDuplicates` (line ~750)
- `checkEnumNameDuplicates` (line ~820)
- `checkUndefinedFunctions` (line ~1040)
- `checkUndeclaredIdentifiers` (line ~1100)

**Fix Required:** Add depth tracking to ALL checkNode functions:

```go
func checkConstReassignment(doc *Document) []protocol.Diagnostic {
    diagnostics := []protocol.Diagnostic{}
    // ... existing setup ...
    
    var checkNode func(*ahoy.ASTNode, int)
    checkNode = func(node *ahoy.ASTNode, depth int) {
        if node == nil {
            return
        }
        
        // Prevent excessive recursion
        if depth > 500 {
            return
        }
        
        // ... existing logic ...
        
        // Recursively check children WITH depth limit
        for _, child := range node.Children {
            checkNode(child, depth+1)
        }
    }
    
    checkNode(doc.AST, 0)
    return diagnostics
}
```

### 3. Switch Statement Range Case Bug
**File:** `../ahoy/parser.go` line ~1000-1200
**Issue:** Second consecutive multi-line range case fails with "Unexpected token 'to'"

**Example that fails:**
```ahoy
switch x on
    1 to 5:
        print|"Small"|
    6 to 10:        ← Fails here
        print|"Medium"|
$
```

**Root Cause Analysis:**
After parsing first case block and breaking, the parser is positioned at `6`.
The outer switch loop should parse `6 to 10:` as a complete case.
But the range handling code (TOKEN_TO check) isn't matching.

**Possible causes:**
1. Position mismatch after breaking from case block
2. Token lookahead issue
3. Case value parsing loop exits early

**Debug Steps:**
1. Add debug logging at key positions
2. Verify token stream after first case block
3. Check if `p.current().Type` is actually TOKEN_TO when expected

**Potential Fix:**
After breaking from multi-line case block, ensure we're at the right position:
```go
// Consume dedent at end of case block
if p.current().Type == TOKEN_DEDENT {
    p.advance()
}

// IMPORTANT: Skip any trailing newlines before next case
for p.current().Type == TOKEN_NEWLINE || p.current().Type == TOKEN_SEMICOLON {
    p.advance()
}
```

Then in the outer switch loop, the case value parsing should work correctly.

## Priority

1. **CRITICAL** - Fix parser infinite loop (could cause LSP hang)
2. **HIGH** - Add depth limits to all diagnostic checkers
3. **MEDIUM** - Fix multi-line range case parsing

## Testing

After fixes:
1. Test deep AST (100+ nested blocks) - should not crash/hang
2. Test multi-line switch with multiple range cases
3. Monitor CPU usage with LSP open on large file
4. Run all existing tests to ensure no regression

