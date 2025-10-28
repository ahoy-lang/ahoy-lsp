package main

import (
	"context"
	"encoding/json"
	"strings"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (s *Server) handleDefinition(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DefinitionParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil || doc.SymbolTable == nil {
		return reply(ctx, nil, nil)
	}

	// Get the word at the cursor position
	word := getWordAtPosition(doc.Content, int(params.Position.Line), int(params.Position.Character))
	if word == "" {
		return reply(ctx, nil, nil)
	}

	// Look up the symbol in the symbol table
	symbol := doc.SymbolTable.Lookup(word)
	if symbol == nil {
		return reply(ctx, nil, nil)
	}

	// Return the definition location
	location := protocol.Location{
		URI: params.TextDocument.URI,
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(symbol.Line - 1),
				Character: uint32(symbol.Column),
			},
			End: protocol.Position{
				Line:      uint32(symbol.Line - 1),
				Character: uint32(symbol.Column + len(symbol.Name)),
			},
		},
	}

	return reply(ctx, location, nil)
}

// getWordAtPosition extracts the word at the given position
func getWordAtPosition(content string, line, character int) string {
	lines := strings.Split(content, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}

	currentLine := lines[line]
	if character < 0 || character >= len(currentLine) {
		return ""
	}

	// Find word boundaries
	start := character
	end := character

	// Move start backwards to beginning of word
	for start > 0 && isWordChar(rune(currentLine[start-1])) {
		start--
	}

	// Move end forwards to end of word
	for end < len(currentLine) && isWordChar(rune(currentLine[end])) {
		end++
	}

	if start >= end {
		return ""
	}

	return currentLine[start:end]
}

// isWordChar checks if a character is part of an identifier
func isWordChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
}
