package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"ahoy"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

var debugLog *log.Logger

func init() {
	// Initialize debug logger to stderr
	debugLog = log.New(os.Stderr, "[ahoy-lsp] ", log.LstdFlags)
}

func main() {
	// Check for CLI validate mode
	if len(os.Args) > 1 && os.Args[1] == "--validate" {
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: ahoy-lsp --validate <file.ahoy>")
			os.Exit(1)
		}
		runValidate(os.Args[2])
		return
	}

	debugLog.Println("Starting Ahoy Language Server")

	// Set aggressive garbage collection to prevent memory buildup
	debug.SetGCPercent(20) // Run GC more frequently (default is 100)

	// Set memory limit to prevent system crashes
	// This will cause the runtime to GC more aggressively as we approach the limit
	debug.SetMemoryLimit(500 * 1024 * 1024) // 500MB limit

	// Start memory monitor goroutine
	go monitorMemory()

	ctx := context.Background()

	// Create stdio stream for communication with editor
	stream := jsonrpc2.NewStream(&stdrwc{})
	conn := jsonrpc2.NewConn(stream)

	// Create server
	server := NewServer(conn)
	debugLog.Println("Server created successfully")

	// Start JSON-RPC handler
	handler := jsonrpc2.ReplyHandler(server.Handle)
	conn.Go(ctx, handler)
	debugLog.Println("Handler started, waiting for requests")

	// Wait for connection to close
	<-conn.Done()

	debugLog.Println("Connection closed")

	// Check for errors
	if err := conn.Err(); err != nil {
		debugLog.Printf("LSP connection error: %v\n", err)
		fmt.Fprintf(os.Stderr, "LSP connection error: %v\n", err)
		os.Exit(1)
	}

	debugLog.Println("Shutting down cleanly")
}

func runValidate(filename string) {
	// Read file
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Import ahoy package
	tokens := ahoy.Tokenize(string(content))
	// Use ParseLintWithPath to support relative imports
	ast, parseErrors := ahoy.ParseLintWithPath(tokens, filename)

	// Check syntax errors
	if len(parseErrors) > 0 {
		fmt.Printf("Found %d syntax error(s) in %s:\n", len(parseErrors), filename)
		for _, err := range parseErrors {
			fmt.Printf("  Line %d, Column %d: %s\n", err.Line, err.Column, err.Message)
		}
		os.Exit(1)
	}

	if ast == nil {
		fmt.Printf("✓ No errors found in %s\n", filename)
		return
	}

	// Build symbol table
	doc := &Document{
		URI:     uri.URI("file://" + filename),
		Content: string(content),
		Lines:   splitLines(string(content)),
		AST:     ast,
	}
	doc.SymbolTable = BuildSymbolTableWithFile(ast, string(doc.URI))

	// Parse C headers from imports - use file URI for path resolution
	doc.CHeaders, doc.CHeaderGlobal = extractCHeaderInfoWithURI(ast, doc.URI)

	// Extract program name and load package files for multi-file validation
	if doc.AST != nil {
		doc.ProgramName = extractProgramName(doc.AST)
		if doc.ProgramName != "" {
			// Create a minimal server just for loading package files
			dummyServer := &Server{
				documents: make(map[uri.URI]*Document),
			}
			dummyServer.loadPackageFiles(doc)
		}
	}

	// Run all diagnostic checks (same as publishDiagnostics)
	allDiagnostics := []protocol.Diagnostic{}

	// Check const reassignment
	constDiags := checkConstReassignment(doc)
	allDiagnostics = append(allDiagnostics, constDiags...)

	// Check const method calls
	constMethodDiags := checkConstMethodCalls(doc)
	allDiagnostics = append(allDiagnostics, constMethodDiags...)

	// Check invalid method calls
	invalidMethodDiags := checkInvalidMethodCalls(doc)
	allDiagnostics = append(allDiagnostics, invalidMethodDiags...)

	// Check return type violations
	returnDiags := checkReturnTypeViolations(doc)
	allDiagnostics = append(allDiagnostics, returnDiags...)

	// Check enum duplicates
	enumDiags := checkEnumDuplicates(doc)
	allDiagnostics = append(allDiagnostics, enumDiags...)

	// Check enum name duplicates
	enumNameDiags := checkEnumNameDuplicates(doc)
	allDiagnostics = append(allDiagnostics, enumNameDiags...)

	// Check undefined functions
	undefinedFuncDiags := checkUndefinedFunctions(doc)
	allDiagnostics = append(allDiagnostics, undefinedFuncDiags...)

	// Check undeclared identifiers
	undeclaredDiags := checkUndeclaredIdentifiers(doc)
	allDiagnostics = append(allDiagnostics, undeclaredDiags...)

	// Check function call argument counts
	argCountDiags := checkFunctionCallArgumentCounts(doc)
	allDiagnostics = append(allDiagnostics, argCountDiags...)

	// Check function call argument types
	argTypeDiags := checkFunctionCallArgumentTypes(doc)
	allDiagnostics = append(allDiagnostics, argTypeDiags...)

	// Check variable/constant type mismatches
	typeMismatchDiags := checkTypeMismatches(doc)
	allDiagnostics = append(allDiagnostics, typeMismatchDiags...)

	// Check struct member access
	memberAccessDiags := checkStructMemberAccess(doc)
	allDiagnostics = append(allDiagnostics, memberAccessDiags...)

	// Check object literal property assignment
	objectPropDiags := checkObjectPropertyAssignment(doc)
	allDiagnostics = append(allDiagnostics, objectPropDiags...)

	// Check type typos
	typeTypoDiags := checkTypeTypos(doc)
	allDiagnostics = append(allDiagnostics, typeTypoDiags...)

	// Check access syntax
	accessSyntaxDiags := checkAccessSyntax(doc)
	allDiagnostics = append(allDiagnostics, accessSyntaxDiags...)

	// Check binary operation types (string + int, etc.)
	binaryOpDiags := checkBinaryOperationTypes(doc)
	allDiagnostics = append(allDiagnostics, binaryOpDiags...)

	// Check for unhandled multi-return function calls
	unhandledReturnDiags := checkUnhandledMultiReturns(doc)
	allDiagnostics = append(allDiagnostics, unhandledReturnDiags...)

	// Check for duplicate function definitions across package files
	duplicateFuncDiags := checkDuplicateFunctionDefinitions(doc)
	allDiagnostics = append(allDiagnostics, duplicateFuncDiags...)

	// Filter for errors only
	errorCount := 0
	for _, diag := range allDiagnostics {
		if diag.Severity == protocol.DiagnosticSeverityError {
			if errorCount == 0 {
				fmt.Printf("Found validation error(s) in %s:\n", filename)
			}
			fmt.Printf("  Line %d: %s\n", diag.Range.Start.Line+1, diag.Message)
			errorCount++
		}
	}

	if errorCount > 0 {
		os.Exit(1)
	}

	fmt.Printf("✓ No errors found in %s\n", filename)
}

func splitLines(content string) []string {
	lines := []string{}
	current := ""
	for _, ch := range content {
		if ch == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// monitorMemory periodically logs memory usage and forces GC if needed
func monitorMemory() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		allocMB := m.Alloc / 1024 / 1024
		sysMB := m.Sys / 1024 / 1024

		debugLog.Printf("Memory: Alloc=%dMB Sys=%dMB NumGC=%d", allocMB, sysMB, m.NumGC)

		// Force GC if memory usage is high
		if allocMB > 300 {
			debugLog.Printf("High memory usage detected, forcing GC")
			runtime.GC()
			debug.FreeOSMemory()
		}
	}
}

// stdrwc implements io.ReadWriteCloser for stdio communication
type stdrwc struct{}

func (stdrwc) Read(p []byte) (int, error) {
	return os.Stdin.Read(p)
}

func (stdrwc) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

func (stdrwc) Close() error {
	if err := os.Stdin.Close(); err != nil {
		return err
	}
	return os.Stdout.Close()
}
