package main

import (
	"context"
	"encoding/json"
	"strings"

	"ahoy"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (s *Server) handleDefinition(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DefinitionParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, nil, nil)
	}

	// Get the word at the cursor position
	word := getWordAtPosition(doc, int(params.Position.Line), int(params.Position.Character))
	if word == "" {
		return reply(ctx, nil, nil)
	}

	// First check if it's a C function/enum/define
	if doc.CHeaderGlobal != nil {
		// Check if it's a C function (snake_case name)
		for cFuncName, cFunc := range doc.CHeaderGlobal.Functions {
			if ahoy.PascalToSnake(cFuncName) == word {
				// Find which import has this function
				for _, child := range doc.AST.Children {
					if child.Type == ahoy.NODE_IMPORT_STATEMENT && strings.HasSuffix(child.Value, ".h") {
						location := protocol.Location{
							URI: protocol.URI("file://" + child.Value),
							Range: protocol.Range{
								Start: protocol.Position{Line: uint32(cFunc.Line - 1), Character: 0},
								End:   protocol.Position{Line: uint32(cFunc.Line - 1), Character: 100},
							},
						}
						debugLog.Printf("Go to definition: C function %s -> %s:%d", word, child.Value, cFunc.Line)
						return reply(ctx, location, nil)
					}
				}
			}
		}
		
		// Check if it's a C enum or define
		if cEnum, ok := doc.CHeaderGlobal.Enums[word]; ok {
			for _, child := range doc.AST.Children {
				if child.Type == ahoy.NODE_IMPORT_STATEMENT && strings.HasSuffix(child.Value, ".h") {
					location := protocol.Location{
						URI: protocol.URI("file://" + child.Value),
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(cEnum.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(cEnum.Line - 1), Character: 100},
						},
					}
					debugLog.Printf("Go to definition: C enum %s -> %s:%d", word, child.Value, cEnum.Line)
					return reply(ctx, location, nil)
				}
			}
		}
		
		if cDefine, ok := doc.CHeaderGlobal.Defines[word]; ok {
			for _, child := range doc.AST.Children {
				if child.Type == ahoy.NODE_IMPORT_STATEMENT && strings.HasSuffix(child.Value, ".h") {
					location := protocol.Location{
						URI: protocol.URI("file://" + child.Value),
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(cDefine.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(cDefine.Line - 1), Character: 100},
						},
					}
					debugLog.Printf("Go to definition: C define %s -> %s:%d", word, child.Value, cDefine.Line)
					return reply(ctx, location, nil)
				}
			}
		}
		
		// Check C structs (case-insensitive)
		for structName, cStruct := range doc.CHeaderGlobal.Structs {
			if ahoy.ToLowerFirst(structName) == word || structName == word {
				for _, child := range doc.AST.Children {
					if child.Type == ahoy.NODE_IMPORT_STATEMENT && strings.HasSuffix(child.Value, ".h") {
						location := protocol.Location{
							URI: protocol.URI("file://" + child.Value),
							Range: protocol.Range{
								Start: protocol.Position{Line: uint32(cStruct.Line - 1), Character: 0},
								End:   protocol.Position{Line: uint32(cStruct.Line - 1), Character: 100},
							},
						}
						debugLog.Printf("Go to definition: C struct %s -> %s:%d", word, child.Value, cStruct.Line)
						return reply(ctx, location, nil)
					}
				}
			}
		}
	}
	
	// Check namespaced C headers
	for namespace, headerInfo := range doc.CHeaders {
		// Check functions
		for cFuncName := range headerInfo.Functions {
			if ahoy.PascalToSnake(cFuncName) == word {
				for _, child := range doc.AST.Children {
					if child.Type == ahoy.NODE_IMPORT_STATEMENT && child.DataType == namespace && strings.HasSuffix(child.Value, ".h") {
						location := protocol.Location{
							URI: protocol.URI("file://" + child.Value),
							Range: protocol.Range{
								Start: protocol.Position{Line: 0, Character: 0},
								End:   protocol.Position{Line: 0, Character: 0},
							},
						}
						return reply(ctx, location, nil)
					}
				}
			}
		}
		
		// Check enums/defines
		if _, ok := headerInfo.Enums[word]; ok {
			for _, child := range doc.AST.Children {
				if child.Type == ahoy.NODE_IMPORT_STATEMENT && child.DataType == namespace {
					location := protocol.Location{
						URI: protocol.URI("file://" + child.Value),
						Range: protocol.Range{
							Start: protocol.Position{Line: 0, Character: 0},
							End:   protocol.Position{Line: 0, Character: 0},
						},
					}
					return reply(ctx, location, nil)
				}
			}
		}
		
		// Check defines
		if cDefine, ok := headerInfo.Defines[word]; ok {
			for _, child := range doc.AST.Children {
				if child.Type == ahoy.NODE_IMPORT_STATEMENT && child.DataType == namespace {
					location := protocol.Location{
						URI: protocol.URI("file://" + child.Value),
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(cDefine.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(cDefine.Line - 1), Character: 100},
						},
					}
					return reply(ctx, location, nil)
				}
			}
		}
		
		// Check structs
		for structName, cStruct := range headerInfo.Structs {
			if ahoy.ToLowerFirst(structName) == word || structName == word {
				for _, child := range doc.AST.Children {
					if child.Type == ahoy.NODE_IMPORT_STATEMENT && child.DataType == namespace {
						location := protocol.Location{
							URI: protocol.URI("file://" + child.Value),
							Range: protocol.Range{
								Start: protocol.Position{Line: uint32(cStruct.Line - 1), Character: 0},
								End:   protocol.Position{Line: uint32(cStruct.Line - 1), Character: 100},
							},
						}
						debugLog.Printf("Go to definition: C struct %s -> %s:%d", word, child.Value, cStruct.Line)
						return reply(ctx, location, nil)
					}
				}
			}
		}
	}

	// Look up the symbol in the symbol table (for Ahoy symbols)
	if doc.SymbolTable != nil {
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

	return reply(ctx, nil, nil)
}

// getWordAtPosition extracts the word at the given position
// Uses cached document.Lines to avoid repeated string splitting
func getWordAtPosition(doc *Document, line, character int) string {
	if doc == nil || doc.Lines == nil {
		return ""
	}
	
	if line < 0 || line >= len(doc.Lines) {
		return ""
	}

	currentLine := doc.Lines[line]
	if character < 0 || character >= len(currentLine) {
		return ""
	}

	// Safety check on line length
	if len(currentLine) > 10000 {
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
