package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ahoy-lang/ahoy"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type Document struct {
	URI         uri.URI
	Content     string
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
			err := fmt.Errorf("handler panic: %v", r)
			reply(ctx, nil, err)
		}
	}()

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

	doc := &Document{
		URI:     params.TextDocument.URI,
		Content: params.TextDocument.Text,
		Version: params.TextDocument.Version,
	}

	// Parse the document - handle panics gracefully
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Parser panicked - create error diagnostic
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
		}()

		doc.Tokens = ahoy.Tokenize(doc.Content)
		doc.AST, doc.Errors = ahoy.ParseLint(doc.Tokens)
	}()

	// Build symbol table - only if AST exists
	if doc.AST != nil {
		doc.SymbolTable = BuildSymbolTable(doc.AST)
	} else {
		doc.SymbolTable = NewSymbolTable()
	}

	s.mu.Lock()
	s.documents[doc.URI] = doc
	s.mu.Unlock()

	// Send diagnostics
	s.publishDiagnostics(ctx, doc)

	return reply(ctx, nil, nil)
}

func (s *Server) handleDidChange(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidChangeTextDocumentParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	s.mu.Lock()
	doc := s.documents[params.TextDocument.URI]
	if doc == nil {
		s.mu.Unlock()
		return reply(ctx, nil, fmt.Errorf("document not found"))
	}

	// Full sync - replace entire content
	if len(params.ContentChanges) > 0 {
		doc.Content = params.ContentChanges[0].Text
		doc.Version = params.TextDocument.Version

		// Reparse - handle panics gracefully
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Parser panicked - create error diagnostic
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
			}()

			doc.Tokens = ahoy.Tokenize(doc.Content)
			doc.AST, doc.Errors = ahoy.ParseLint(doc.Tokens)
		}()

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
	delete(s.documents, params.TextDocument.URI)
	s.mu.Unlock()

	return reply(ctx, nil, nil)
}

func (s *Server) getDocument(docURI uri.URI) *Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.documents[docURI]
}
