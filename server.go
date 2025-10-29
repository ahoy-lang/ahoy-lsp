package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"ahoy"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type Document struct {
	URI         uri.URI
	Content     string
	Lines       []string // Cached split lines
	Version     int32
	Tokens      []ahoy.Token
	AST         *ahoy.ASTNode
	Errors      []ahoy.ParseError
	SymbolTable *SymbolTable
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
			// Log the panic and return an error response
			debugLog.Printf("PANIC in handler %s: %v", req.Method(), r)
			err := fmt.Errorf("handler panic: %v", r)
			reply(ctx, nil, err)
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
	default:
		return reply(ctx, nil, jsonrpc2.ErrMethodNotFound)
	}
}

func (s *Server) handleInitialize(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.InitializeParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
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
			HoverProvider:          true,
			DocumentSymbolProvider: true,
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

	return reply(ctx, result, nil)
}

func (s *Server) handleDidOpen(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidOpenTextDocumentParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
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
		doc.AST, doc.Errors = ahoy.ParseLint(doc.Tokens)
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
		debugLog.Printf("Building symbol table...")
		doc.SymbolTable = BuildSymbolTable(doc.AST)
		debugLog.Printf("Symbol table built")
	} else {
		doc.SymbolTable = NewSymbolTable()
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
		return reply(ctx, nil, err)
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
			doc.AST, doc.Errors = ahoy.ParseLint(doc.Tokens)
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
			doc.SymbolTable = BuildSymbolTable(doc.AST)
		} else {
			doc.SymbolTable = NewSymbolTable()
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
		return reply(ctx, nil, err)
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
