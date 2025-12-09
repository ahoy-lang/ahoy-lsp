package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ahoy"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type Document struct {
	URI            uri.URI
	Content        string
	Lines          []string // Cached split lines
	Version        int32
	Tokens         []ahoy.Token
	AST            *ahoy.ASTNode
	Errors         []ahoy.ParseError
	SymbolTable    *SymbolTable
	CHeaders       map[string]*ahoy.CHeaderInfo // namespace -> C header info
	CHeaderGlobal  *ahoy.CHeaderInfo            // Global C header imports
	ProgramName    string                       // Program name from declaration (empty if script)
	PackageFiles   map[uri.URI]*PackageFile     // Other files in the same package
	PackageSymbols *SymbolTable                 // Merged symbol table from all package files
	InferredParams map[string][]string          // function name -> inferred parameter types
}

type PackageFile struct {
	URI     uri.URI
	Content string
	AST     *ahoy.ASTNode
	Symbols *SymbolTable
}

type Server struct {
	conn      jsonrpc2.Conn
	documents map[uri.URI]*Document
	mu        sync.RWMutex
}

func NewServer(conn jsonrpc2.Conn) *Server {
	return &Server{
		conn:      conn,
		documents: make(map[uri.URI]*Document),
	}
}

func (s *Server) Handle(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	// Wrap all handlers with panic recovery to prevent server crashes
	defer func() {
		if r := recover(); r != nil {
			// Log the panic but return successful empty response to prevent server shutdown
			debugLog.Printf("PANIC in handler %s: %v", req.Method(), r)
			// Return empty result with nil error to keep server alive
			reply(ctx, nil, nil)
		}
	}()

	debugLog.Printf("Handling request: %s", req.Method())

	switch req.Method() {
	case protocol.MethodInitialize:
		return s.handleInitialize(ctx, reply, req)
	case protocol.MethodInitialized:
		return reply(ctx, nil, nil)
	case protocol.MethodShutdown:
		return reply(ctx, nil, nil)
	case protocol.MethodExit:
		return nil
	case protocol.MethodTextDocumentDidOpen:
		return s.handleDidOpen(ctx, reply, req)
	case protocol.MethodTextDocumentDidChange:
		return s.handleDidChange(ctx, reply, req)
	case protocol.MethodTextDocumentDidSave:
		return reply(ctx, nil, nil)
	case protocol.MethodTextDocumentDidClose:
		return s.handleDidClose(ctx, reply, req)
	case protocol.MethodTextDocumentCompletion:
		return s.handleCompletion(ctx, reply, req)
	case protocol.MethodTextDocumentDefinition:
		return s.handleDefinition(ctx, reply, req)
	case protocol.MethodTextDocumentHover:
		return s.handleHover(ctx, reply, req)
	case protocol.MethodTextDocumentDocumentSymbol:
		return s.handleDocumentSymbol(ctx, reply, req)
	case protocol.MethodTextDocumentCodeAction:
		return s.handleCodeAction(ctx, reply, req)
	case protocol.MethodTextDocumentReferences:
		return s.handleReferences(ctx, reply, req)
	case protocol.MethodTextDocumentPrepareRename:
		return s.handlePrepareRename(ctx, reply, req)
	case protocol.MethodTextDocumentRename:
		return s.handleRename(ctx, reply, req)
	case protocol.MethodTextDocumentTypeDefinition:
		return s.handleTypeDefinition(ctx, reply, req)
	case protocol.MethodTextDocumentDeclaration:
		return s.handleDeclaration(ctx, reply, req)
	case protocol.MethodTextDocumentFormatting:
		return s.handleFormatting(ctx, reply, req)
	case "textDocument/inlayHint":
		return s.handleInlayHint(ctx, reply, req)
	default:
		return reply(ctx, nil, jsonrpc2.ErrMethodNotFound)
	}
}

func (s *Server) handleInitialize(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.InitializeParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		debugLog.Printf("Failed to unmarshal params in server.go: %v", err)
		return reply(ctx, nil, nil)
	}

	result := protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.TextDocumentSyncKindFull,
			},
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{".", ":", " "},
			},
			DefinitionProvider:     true,
			TypeDefinitionProvider: true,
			DeclarationProvider:    true,
			HoverProvider:          true,
			DocumentSymbolProvider: true,
			ReferencesProvider:     true,
			RenameProvider: protocol.RenameOptions{
				PrepareProvider: true,
			},
			DocumentFormattingProvider: true,
			CodeActionProvider: protocol.CodeActionOptions{
				CodeActionKinds: []protocol.CodeActionKind{
					protocol.QuickFix,
					protocol.Refactor,
				},
			},
		},
		ServerInfo: &protocol.ServerInfo{
			Name:    "ahoy-lsp",
			Version: "0.1.0",
		},
	}

	// Marshal to JSON to add custom fields
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return reply(ctx, result, nil)
	}
	
	// Add inlayHintProvider capability
	var resultMap map[string]interface{}
	if err := json.Unmarshal(resultJSON, &resultMap); err != nil {
		return reply(ctx, result, nil)
	}
	
	if capabilities, ok := resultMap["capabilities"].(map[string]interface{}); ok {
		capabilities["inlayHintProvider"] = true
	}

	return reply(ctx, resultMap, nil)
}

func (s *Server) handleDidOpen(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidOpenTextDocumentParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		debugLog.Printf("Failed to unmarshal params in server.go: %v", err)
		return reply(ctx, nil, nil)
	}

	debugLog.Printf("DidOpen: %s (size: %d bytes)", params.TextDocument.URI, len(params.TextDocument.Text))

	// Safety check: prevent opening extremely large files
	if len(params.TextDocument.Text) > 5000000 {
		debugLog.Printf("File too large, skipping parsing: %d bytes", len(params.TextDocument.Text))
		return reply(ctx, nil, fmt.Errorf("file too large"))
	}

	doc := &Document{
		URI:     params.TextDocument.URI,
		Content: params.TextDocument.Text,
		Lines:   strings.Split(params.TextDocument.Text, "\n"),
		Version: params.TextDocument.Version,
	}

	// Parse the document - handle panics gracefully with timeout
	parseSuccess := false
	parseDone := make(chan bool, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Parser panicked - create error diagnostic
				debugLog.Printf("Parser panic: %v", r)
				doc.Errors = []ahoy.ParseError{
					{
						Line:    1,
						Column:  1,
						Message: fmt.Sprintf("Parser error: %v", r),
					},
				}
				doc.AST = nil
				doc.Tokens = nil
			}
			parseDone <- true
		}()

		doc.Tokens = ahoy.Tokenize(doc.Content)
		debugLog.Printf("Tokenized: %d tokens", len(doc.Tokens))
		// Convert URI to file path for relative import resolution
		filePath := string(doc.URI)
		if strings.HasPrefix(filePath, "file://") {
			filePath = strings.TrimPrefix(filePath, "file://")
		}
		doc.AST, doc.Errors = ahoy.ParseLintWithPath(doc.Tokens, filePath)
		debugLog.Printf("Parsed: %d errors", len(doc.Errors))
		parseSuccess = true
	}()

	// Wait for parsing with timeout (5 seconds safety)
	select {
	case <-parseDone:
		if !parseSuccess {
			debugLog.Printf("Parsing failed")
		} else {
			debugLog.Printf("Parsing completed successfully")
		}
	case <-time.After(5 * time.Second):
		debugLog.Printf("Parser timeout after 5 seconds!")
		doc.Errors = []ahoy.ParseError{
			{
				Line:    1,
				Column:  1,
				Message: "Parser timeout - file may be too complex",
			},
		}
		doc.AST = nil
		doc.Tokens = nil
	}

	// Build symbol table - only if AST exists
	if doc.AST != nil {
		// Infer parameter types FIRST (before symbol table)
		debugLog.Printf("Inferring parameter types...")
		doc.InferredParams = inferParameterTypes(doc.AST, doc)
		debugLog.Printf("Parameter types inferred for %d functions", len(doc.InferredParams))

		// Infer return types for functions with 'infer' keyword SECOND (before symbol table)
		debugLog.Printf("Resolving infer return types...")
		inferredReturnTypes := resolveAllInferReturnTypes(doc.AST, doc)
		debugLog.Printf("Return types resolved for %d functions", len(inferredReturnTypes))

		debugLog.Printf("Building symbol table...")
		doc.SymbolTable = BuildSymbolTableWithFile(doc.AST, string(doc.URI), doc.InferredParams)
		doc.SymbolTable.Doc = doc // Set document reference
		doc.SymbolTable.InferredReturns = inferredReturnTypes // Store resolved return types
		debugLog.Printf("Symbol table built")

		// Extract C header imports
		debugLog.Printf("Extracting C header imports...")
		doc.CHeaders, doc.CHeaderGlobal = extractCHeaderInfoWithURI(doc.AST, doc.URI)
		debugLog.Printf("C headers extracted: %d namespaced, global has %d functions, %d enums, %d defines",
			len(doc.CHeaders), len(doc.CHeaderGlobal.Functions), len(doc.CHeaderGlobal.Enums), len(doc.CHeaderGlobal.Defines))

		// Debug: List enum names
		if len(doc.CHeaderGlobal.Enums) > 0 {
			debugLog.Printf("Global C enums loaded:")
			for enumName, enum := range doc.CHeaderGlobal.Enums {
				debugLog.Printf("  - %s (%d values)", enumName, len(enum.Values))
			}
		}
	} else {
		doc.SymbolTable = NewSymbolTable()
		doc.CHeaders = make(map[string]*ahoy.CHeaderInfo)
		doc.CHeaderGlobal = &ahoy.CHeaderInfo{
			Functions: make(map[string]*ahoy.CFunction),
			Enums:     make(map[string]*ahoy.CEnum),
			Defines:   make(map[string]*ahoy.CDefine),
			Typedefs:  make(map[string]*ahoy.CTypedef),
		}
	}

	// Extract program name and load package files
	if doc.AST != nil {
		doc.ProgramName = extractProgramName(doc.AST)
		if doc.ProgramName != "" {
			debugLog.Printf("Program name: %s, loading package files...", doc.ProgramName)
			s.loadPackageFiles(doc)
		}
	}

	s.mu.Lock()
	s.documents[doc.URI] = doc
	s.mu.Unlock()

	// Send diagnostics
	s.publishDiagnostics(ctx, doc)

	debugLog.Printf("DidOpen complete")
	return reply(ctx, nil, nil)
}

func (s *Server) handleDidChange(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidChangeTextDocumentParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		debugLog.Printf("Failed to unmarshal params in server.go: %v", err)
		return reply(ctx, nil, nil)
	}

	debugLog.Printf("DidChange: %s", params.TextDocument.URI)

	s.mu.Lock()
	doc := s.documents[params.TextDocument.URI]
	if doc == nil {
		s.mu.Unlock()
		return reply(ctx, nil, fmt.Errorf("document not found"))
	}

	// Full sync - replace entire content
	if len(params.ContentChanges) > 0 {
		debugLog.Printf("Content size: %d bytes", len(params.ContentChanges[0].Text))

		// Safety check: prevent extremely large files from causing issues
		if len(params.ContentChanges[0].Text) > 5000000 {
			debugLog.Printf("File too large after change, skipping reparse: %d bytes", len(params.ContentChanges[0].Text))
			s.mu.Unlock()
			return reply(ctx, nil, nil)
		}
		// Explicitly clear old data to help GC and prevent memory leaks
		if doc.SymbolTable != nil {
			doc.SymbolTable.Clear()
			doc.SymbolTable = nil
		}
		if doc.Tokens != nil {
			doc.Tokens = nil
		}
		if doc.AST != nil {
			doc.AST = nil
		}
		if doc.Errors != nil {
			doc.Errors = nil
		}

		doc.Content = params.ContentChanges[0].Text
		doc.Lines = strings.Split(doc.Content, "\n")
		doc.Version = params.TextDocument.Version

		// Reparse - handle panics gracefully with timeout
		parseSuccess := false
		parseDone := make(chan bool, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					// Parser panicked - create error diagnostic
					debugLog.Printf("Parser panic on change: %v", r)
					doc.Errors = []ahoy.ParseError{
						{
							Line:    1,
							Column:  1,
							Message: fmt.Sprintf("Parser error: %v", r),
						},
					}
					doc.AST = nil
					doc.Tokens = nil
				}
				parseDone <- true
			}()

			// Tokenize and parse
			doc.Tokens = ahoy.Tokenize(doc.Content)
			debugLog.Printf("Tokenized on change: %d tokens", len(doc.Tokens))
			// Convert URI to file path for relative import resolution
			filePath := string(doc.URI)
			if strings.HasPrefix(filePath, "file://") {
				filePath = strings.TrimPrefix(filePath, "file://")
			}
			doc.AST, doc.Errors = ahoy.ParseLintWithPath(doc.Tokens, filePath)
			debugLog.Printf("Parsed on change: %d errors", len(doc.Errors))
			parseSuccess = true
		}()

		// Wait for parsing with timeout
		select {
		case <-parseDone:
			if !parseSuccess {
				debugLog.Printf("Parsing failed on change")
			} else {
				debugLog.Printf("Parsing completed on change")
			}
		case <-time.After(5 * time.Second):
			debugLog.Printf("Parser timeout on change after 5 seconds!")
			doc.Errors = []ahoy.ParseError{
				{
					Line:    1,
					Column:  1,
					Message: "Parser timeout - file may be too complex",
				},
			}
			doc.AST = nil
			doc.Tokens = nil
		}

		// Rebuild symbol table - only if AST exists
		if doc.AST != nil {
			// Infer parameter types first
			doc.InferredParams = inferParameterTypes(doc.AST, doc)
			doc.SymbolTable = BuildSymbolTableWithFile(doc.AST, string(doc.URI), doc.InferredParams)
			doc.SymbolTable.Doc = doc // Set document reference
			// Extract C header imports
			doc.CHeaders, doc.CHeaderGlobal = extractCHeaderInfoWithURI(doc.AST, doc.URI)
			
			// Check if program name changed
			newProgramName := extractProgramName(doc.AST)
			if newProgramName != doc.ProgramName {
				// Program name changed - clear old package data
				if doc.ProgramName != "" && newProgramName == "" {
					// Program declaration removed - clear package files
					debugLog.Printf("Program declaration removed, clearing package files")
					doc.PackageFiles = nil
					doc.PackageSymbols = nil
				}
				doc.ProgramName = newProgramName
			}
			
			// Reload package files if there's a program name
			if doc.ProgramName != "" {
				s.loadPackageFiles(doc)
			}
		} else {
			doc.SymbolTable = NewSymbolTable()
			doc.CHeaders = make(map[string]*ahoy.CHeaderInfo)
			doc.CHeaderGlobal = &ahoy.CHeaderInfo{
				Functions: make(map[string]*ahoy.CFunction),
				Enums:     make(map[string]*ahoy.CEnum),
				Defines:   make(map[string]*ahoy.CDefine),
				Typedefs:  make(map[string]*ahoy.CTypedef),
			}
		}
	}
	s.mu.Unlock()

	// Send diagnostics
	s.publishDiagnostics(ctx, doc)

	return reply(ctx, nil, nil)
}

func (s *Server) handleDidClose(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidCloseTextDocumentParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		debugLog.Printf("Failed to unmarshal params in server.go: %v", err)
		return reply(ctx, nil, nil)
	}

	s.mu.Lock()
	// Clear document data before removing to help GC
	if doc := s.documents[params.TextDocument.URI]; doc != nil {
		debugLog.Printf("Closing document, cleaning up: %s", params.TextDocument.URI)
		if doc.SymbolTable != nil {
			doc.SymbolTable.Clear()
			doc.SymbolTable = nil
		}
		if doc.Tokens != nil {
			doc.Tokens = nil
		}
		if doc.AST != nil {
			doc.AST = nil
		}
		if doc.Errors != nil {
			doc.Errors = nil
		}
		doc.Content = ""
		doc.Lines = nil
	}
	delete(s.documents, params.TextDocument.URI)
	s.mu.Unlock()

	// Send empty diagnostics to clear them in the editor
	s.conn.Notify(ctx, protocol.MethodTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	})

	return reply(ctx, nil, nil)
}

func (s *Server) getDocument(docURI uri.URI) *Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.documents[docURI]
}

// extractCHeaderInfo walks the AST and extracts C header import information
func extractCHeaderInfo(ast *ahoy.ASTNode) (map[string]*ahoy.CHeaderInfo, *ahoy.CHeaderInfo) {
	return extractCHeaderInfoWithURI(ast, "")
}

// extractCHeaderInfoWithURI walks the AST and extracts C header import information
// with path resolution relative to the document URI
func extractCHeaderInfoWithURI(ast *ahoy.ASTNode, docURI uri.URI) (map[string]*ahoy.CHeaderInfo, *ahoy.CHeaderInfo) {
	cHeaders := make(map[string]*ahoy.CHeaderInfo)
	cHeaderGlobal := &ahoy.CHeaderInfo{
		Functions: make(map[string]*ahoy.CFunction),
		Enums:     make(map[string]*ahoy.CEnum),
		Defines:   make(map[string]*ahoy.CDefine),
		Structs:   make(map[string]*ahoy.CStruct),
		Typedefs:  make(map[string]*ahoy.CTypedef),
	}

	if ast == nil {
		return cHeaders, cHeaderGlobal
	}

	// Get document directory for relative path resolution
	var docDir string
	if docURI != "" {
		docPath := string(docURI)
		if strings.HasPrefix(docPath, "file://") {
			docPath = docPath[7:]
		}
		docDir = filepath.Dir(docPath)
	}

	// Walk through top-level nodes looking for imports
	for _, child := range ast.Children {
		if child.Type == ahoy.NODE_IMPORT_STATEMENT {
			path := child.Value
			namespace := child.DataType // namespace stored in DataType field

			// Only process .h files
			if !strings.HasSuffix(path, ".h") {
				continue
			}

			// Resolve relative paths
			resolvedPath := path
			if docDir != "" && !filepath.IsAbs(path) {
				resolvedPath = filepath.Join(docDir, path)
				resolvedPath = filepath.Clean(resolvedPath)
			}

			// Parse the C header
			headerInfo, err := ahoy.ParseCHeader(resolvedPath)
			if err != nil {
				debugLog.Printf("Failed to parse C header %s (resolved: %s): %v", path, resolvedPath, err)
				continue
			}

			if namespace != "" {
				// Store with namespace
				cHeaders[namespace] = headerInfo
				debugLog.Printf("Loaded C header %s with namespace '%s': %d functions, %d enums, %d structs",
					resolvedPath, namespace, len(headerInfo.Functions), len(headerInfo.Enums), len(headerInfo.Structs))
			} else {
				// Merge into global
				for name, fn := range headerInfo.Functions {
					cHeaderGlobal.Functions[name] = fn
				}
				for name, enum := range headerInfo.Enums {
					cHeaderGlobal.Enums[name] = enum
				}
				for name, def := range headerInfo.Defines {
					cHeaderGlobal.Defines[name] = def
				}
				for name, str := range headerInfo.Structs {
					cHeaderGlobal.Structs[name] = str
				}
				debugLog.Printf("Loaded C header %s globally: %d functions, %d enums, %d structs",
					resolvedPath, len(headerInfo.Functions), len(headerInfo.Enums), len(headerInfo.Structs))
			}
		}
	}

	return cHeaders, cHeaderGlobal
}

// extractProgramName gets the program name from a program declaration
func extractProgramName(ast *ahoy.ASTNode) string {
	if ast == nil {
		return ""
	}

	for _, child := range ast.Children {
		if child.Type == ahoy.NODE_PROGRAM_DECLARATION {
			return child.Value
		}
	}

	return ""
}

// loadPackageFiles loads other .ahoy files in the same directory with the same program name
func (s *Server) loadPackageFiles(doc *Document) {
	// Get directory from URI
	filePath := string(doc.URI)[7:] // Remove "file://" prefix
	dir := filePath[:strings.LastIndex(filePath, "/")]

	// Read directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		debugLog.Printf("Error reading directory %s: %v", dir, err)
		return
	}

	doc.PackageFiles = make(map[uri.URI]*PackageFile)

	// Load all .ahoy files with matching program name
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ahoy") {
			continue
		}

		fileURI := uri.URI("file://" + dir + "/" + entry.Name())

		// Skip the current document
		if fileURI == doc.URI {
			continue
		}

		// Read file content
		content, err := os.ReadFile(dir + "/" + entry.Name())
		if err != nil {
			debugLog.Printf("Error reading file %s: %v", entry.Name(), err)
			continue
		}

		// Parse file
		tokens := ahoy.Tokenize(string(content))
		filePath := dir + "/" + entry.Name()
		ast, _ := ahoy.ParseLintWithPath(tokens, filePath)

		// Check if it has the same program name
		progName := extractProgramName(ast)
		if progName != doc.ProgramName {
			continue
		}

		// Build symbol table for this file with file path tracking
		symbols := BuildSymbolTableWithFile(ast, string(fileURI))

		// Add to package files
		doc.PackageFiles[fileURI] = &PackageFile{
			URI:     fileURI,
			Content: string(content),
			AST:     ast,
			Symbols: symbols,
		}

		debugLog.Printf("Loaded package file: %s", entry.Name())
	}

	// Merge symbol tables from all package files
	doc.PackageSymbols = NewSymbolTable()

	// Add symbols from current file
	if doc.SymbolTable != nil {
		mergeSymbolTable(doc.PackageSymbols.GlobalScope, doc.SymbolTable.GlobalScope)
	}

	// Add symbols from other package files
	for _, pkgFile := range doc.PackageFiles {
		if pkgFile.Symbols != nil {
			mergeSymbolTable(doc.PackageSymbols.GlobalScope, pkgFile.Symbols.GlobalScope)
		}
	}

	debugLog.Printf("Merged package symbols from %d files", len(doc.PackageFiles)+1)
}

// mergeSymbolTable merges symbols from src into dst
func mergeSymbolTable(dst *Scope, src *Scope) {
	if src == nil || dst == nil {
		return
	}

	// Copy all symbols from src to dst
	for name, symbol := range src.Symbols {
		existingSymbol, exists := dst.Symbols[name]
		
		// For functions, we want to detect duplicates, so we keep the FIRST one
		// but log when we encounter duplicates for debugging
		if exists {
			// If both are functions and from different files/lines, this is a duplicate
			if symbol.Kind == SymbolKindFunction && existingSymbol.Kind == SymbolKindFunction {
				if symbol.File != existingSymbol.File || symbol.Line != existingSymbol.Line {
					debugLog.Printf("[MergeSymbols] Duplicate function %s found: existing=%s:%d, new=%s:%d", 
						name, existingSymbol.File, existingSymbol.Line, symbol.File, symbol.Line)
					// Keep the existing one (from current file), but we've logged the duplicate
				}
			}
			debugLog.Printf("[MergeSymbols] Skipped duplicate symbol %s (already exists)", name)
		} else {
			dst.Symbols[name] = symbol
			debugLog.Printf("[MergeSymbols] Added symbol %s (kind=%d, file=%s, line=%d)", name, symbol.Kind, symbol.File, symbol.Line)
		}
	}

	// Recursively merge child scopes
	for _, srcChild := range src.Children {
		mergeSymbolTable(dst, srcChild)
	}
}
