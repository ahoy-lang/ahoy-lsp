# LSP Error Handling Fix

## Problem
The Ahoy LSP server was shutting down unexpectedly when encountering errors, causing cascading failures with error messages like:
- "Get code actions via ahoy-lsp failed: server shut down"
- "Get hover via ahoy-lsp failed: server shut down"  
- "Error handling response for request GetCodeActions/GetHover: server shut down"

## Root Cause
When LSP request handlers encountered errors (particularly during JSON unmarshaling of parameters), they were returning the error to the JSON-RPC layer via `reply(ctx, nil, err)`. The JSON-RPC connection interpreted these errors as fatal and shut down the entire server connection, causing all subsequent requests to fail.

## Solution Applied

### 1. Enhanced Panic Recovery (server.go)
**Changed**: The panic recovery handler in `Handle()` now returns a successful empty response instead of propagating the error:

```go
defer func() {
    if r := recover(); r != nil {
        debugLog.Printf("PANIC in handler %s: %v", req.Method(), r)
        // Return empty result with nil error to keep server alive
        reply(ctx, nil, nil)  // Changed from reply(ctx, nil, err)
    }
}()
```

### 2. Fixed JSON Unmarshal Error Handling
**Changed**: All handlers that unmarshal JSON parameters now log the error and return an empty response instead of propagating the error:

**Before:**
```go
if err := json.Unmarshal(req.Params(), &params); err != nil {
    return reply(ctx, nil, err)  // Fatal - shuts down server
}
```

**After:**
```go
if err := json.Unmarshal(req.Params(), &params); err != nil {
    debugLog.Printf("Failed to unmarshal params: %v", err)
    return reply(ctx, nil, nil)  // Non-fatal - returns empty result
}
```

**Files Fixed:**
- completion.go
- definition.go
- hover.go
- rename_references.go (5 handlers)
- server.go (multiple handlers)
- symbols.go

### 3. Graceful Degradation Principle
The fix implements graceful degradation:
- Errors are logged for debugging
- Empty/null results are returned to the client
- Server stays alive and continues processing subsequent requests
- Client sees missing functionality rather than a dead server

## Testing
After the fix:
1. LSP server no longer shuts down on parameter unmarshal errors
2. Panic recovery prevents crashes from unexpected errors
3. Clients receive empty responses instead of connection failures
4. Debugging logs still capture all errors for troubleshooting

## Impact
- **Stability**: LSP server stays running even when individual requests fail
- **User Experience**: Editor features may temporarily not work, but don't break the entire LSP connection
- **Debugging**: All errors are still logged to stderr for diagnosis
- **Reliability**: Prevents cascading failures where one bad request kills the server

## Build Instructions
```bash
cd /home/lee/Documents/ahoy-lang/ahoy-lsp
go build -o ahoy-lsp .
```

The fixed binary should be installed to your PATH (e.g., `/home/lee/bin/ahoy-lsp`) to be used by your editor.
