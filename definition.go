package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"ahoy"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// resolveImportPath resolves a relative import path relative to the document URI
func resolveImportPath(importPath string, docURI protocol.URI) string {
	if filepath.IsAbs(importPath) {
		return importPath
	}
	
	docPath := string(docURI)
	if strings.HasPrefix(docPath, "file://") {
		docPath = docPath[7:]
	}
	docDir := filepath.Dir(docPath)
	resolved := filepath.Join(docDir, importPath)
	return filepath.Clean(resolved)
}

func (s *Server) handleDefinition(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DefinitionParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		debugLog.Printf("Failed to unmarshal params in definition.go: %v", err)
		return reply(ctx, nil, nil)
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
				// Use the File field directly from the function
				if cFunc.File != "" {
					location := protocol.Location{
						URI: protocol.URI("file://" + cFunc.File),
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(cFunc.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(cFunc.Line - 1), Character: 100},
						},
					}
					debugLog.Printf("Go to definition: C function %s -> %s:%d", word, cFunc.File, cFunc.Line)
					return reply(ctx, location, nil)
				}
			}
		}
		
		// Check if it's a C enum VALUE (not enum name)
		for _, cEnum := range doc.CHeaderGlobal.Enums {
			if _, ok := cEnum.Values[word]; ok {
				// Try to get the specific line for this enum value
				enumLineNum := cEnum.Line
				if cEnum.ValueLines != nil {
					if valueLine, exists := cEnum.ValueLines[word]; exists {
						enumLineNum = valueLine
					}
				}
				// Use the File field directly from the enum
				if cEnum.File != "" {
					location := protocol.Location{
						URI: protocol.URI("file://" + cEnum.File),
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(enumLineNum - 1), Character: 0},
							End:   protocol.Position{Line: uint32(enumLineNum - 1), Character: 100},
						},
					}
					debugLog.Printf("Go to definition: C enum value %s -> %s:%d", word, cEnum.File, enumLineNum)
					return reply(ctx, location, nil)
				}
			}
		}
		
		if cDefine, ok := doc.CHeaderGlobal.Defines[word]; ok {
			// Use the File field directly from the define
			if cDefine.File != "" {
				location := protocol.Location{
					URI: protocol.URI("file://" + cDefine.File),
					Range: protocol.Range{
						Start: protocol.Position{Line: uint32(cDefine.Line - 1), Character: 0},
						End:   protocol.Position{Line: uint32(cDefine.Line - 1), Character: 100},
					},
				}
				debugLog.Printf("Go to definition: C define %s -> %s:%d", word, cDefine.File, cDefine.Line)
				return reply(ctx, location, nil)
			}
		}
		
		// Check C structs (case-insensitive)
		for structName, cStruct := range doc.CHeaderGlobal.Structs {
			if ahoy.ToLowerFirst(structName) == word || structName == word {
				// Use the File field directly from the struct
				if cStruct.File != "" {
					location := protocol.Location{
						URI: protocol.URI("file://" + cStruct.File),
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(cStruct.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(cStruct.Line - 1), Character: 100},
						},
					}
					debugLog.Printf("Go to definition: C struct %s -> %s:%d", word, cStruct.File, cStruct.Line)
					return reply(ctx, location, nil)
				}
			}
		}
	}
	
	// Check namespaced C headers
	for namespace, headerInfo := range doc.CHeaders {
		// Check functions
		for cFuncName, cFunc := range headerInfo.Functions {
			if ahoy.PascalToSnake(cFuncName) == word {
				for _, child := range doc.AST.Children {
					if child.Type == ahoy.NODE_IMPORT_STATEMENT && child.DataType == namespace && strings.HasSuffix(child.Value, ".h") {
						resolvedPath := resolveImportPath(child.Value, params.TextDocument.URI)
						location := protocol.Location{
							URI: protocol.URI("file://" + resolvedPath),
							Range: protocol.Range{
								Start: protocol.Position{Line: uint32(cFunc.Line - 1), Character: 0},
								End:   protocol.Position{Line: uint32(cFunc.Line - 1), Character: 100},
							},
						}
						debugLog.Printf("Go to definition: namespaced C function %s -> %s:%d", word, resolvedPath, cFunc.Line)
						return reply(ctx, location, nil)
					}
				}
			}
		}
		
		// Check enum values
		for _, cEnum := range headerInfo.Enums {
			if _, ok := cEnum.Values[word]; ok {
				for _, child := range doc.AST.Children {
					if child.Type == ahoy.NODE_IMPORT_STATEMENT && child.DataType == namespace {
						resolvedPath := resolveImportPath(child.Value, params.TextDocument.URI)
						lineNum := cEnum.Line
						if cEnum.ValueLines != nil {
							if valueLine, exists := cEnum.ValueLines[word]; exists {
								lineNum = valueLine
							}
						}
						location := protocol.Location{
							URI: protocol.URI("file://" + resolvedPath),
							Range: protocol.Range{
								Start: protocol.Position{Line: uint32(lineNum - 1), Character: 0},
								End:   protocol.Position{Line: uint32(lineNum - 1), Character: 100},
							},
						}
						debugLog.Printf("Go to definition: namespaced C enum value %s -> %s:%d", word, resolvedPath, lineNum)
						return reply(ctx, location, nil)
					}
				}
			}
		}
		
		// Check defines
		if cDefine, ok := headerInfo.Defines[word]; ok {
			for _, child := range doc.AST.Children {
				if child.Type == ahoy.NODE_IMPORT_STATEMENT && child.DataType == namespace {
					resolvedPath := resolveImportPath(child.Value, params.TextDocument.URI)
					location := protocol.Location{
						URI: protocol.URI("file://" + resolvedPath),
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(cDefine.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(cDefine.Line - 1), Character: 100},
						},
					}
					debugLog.Printf("Go to definition: namespaced C define %s -> %s:%d", word, resolvedPath, cDefine.Line)
					return reply(ctx, location, nil)
				}
			}
		}
		
		// Check structs
		for structName, cStruct := range headerInfo.Structs {
			if ahoy.ToLowerFirst(structName) == word || structName == word {
				for _, child := range doc.AST.Children {
					if child.Type == ahoy.NODE_IMPORT_STATEMENT && child.DataType == namespace {
						resolvedPath := resolveImportPath(child.Value, params.TextDocument.URI)
						location := protocol.Location{
							URI: protocol.URI("file://" + resolvedPath),
							Range: protocol.Range{
								Start: protocol.Position{Line: uint32(cStruct.Line - 1), Character: 0},
								End:   protocol.Position{Line: uint32(cStruct.Line - 1), Character: 100},
							},
						}
						debugLog.Printf("Go to definition: namespaced C struct %s -> %s:%d", word, resolvedPath, cStruct.Line)
						return reply(ctx, location, nil)
					}
				}
			}
		}
	}

	// Look up the symbol in the symbol table (for Ahoy symbols)
	// Use PackageSymbols if available (for multi-file packages), otherwise use regular SymbolTable
	var symbolTable *SymbolTable
	var symbol *Symbol
	
	if doc.PackageSymbols != nil {
		symbolTable = doc.PackageSymbols
		symbol = symbolTable.LookupAtPosition(word, int(params.Position.Line)+1, int(params.Position.Character))
		debugLog.Printf("Go to definition: Looking for '%s' in PackageSymbols at line %d - found: %v", word, int(params.Position.Line)+1, symbol != nil)
	} else if doc.SymbolTable != nil {
		symbolTable = doc.SymbolTable
		symbol = symbolTable.LookupAtPosition(word, int(params.Position.Line)+1, int(params.Position.Character))
		debugLog.Printf("Go to definition: Looking for '%s' in SymbolTable at line %d - found: %v", word, int(params.Position.Line)+1, symbol != nil)
	}
	
	if symbol != nil {
		// Find which file contains this symbol (check package files)
		targetURI := params.TextDocument.URI
		bestSymbol := symbol
		
		// If we found the symbol in package symbols, check which file it's actually from
		if doc.PackageSymbols != nil && doc.PackageFiles != nil {
			// Collect all matching symbols from all files
			allSymbols := []struct {
				sym *Symbol
				uri protocol.URI
			}{}
			
			// Add current file symbol
			if doc.SymbolTable != nil {
				if currentSym := doc.SymbolTable.LookupAtPosition(word, int(params.Position.Line)+1, int(params.Position.Character)); currentSym != nil {
					allSymbols = append(allSymbols, struct {
						sym *Symbol
						uri protocol.URI
					}{currentSym, params.TextDocument.URI})
				}
			}
			
			// Add package file symbols
			for pkgURI, pkgFile := range doc.PackageFiles {
				if pkgFile.Symbols != nil {
					if pkgSym := pkgFile.Symbols.Lookup(word); pkgSym != nil {
						allSymbols = append(allSymbols, struct {
							sym *Symbol
							uri protocol.URI
						}{pkgSym, pkgURI})
					}
				}
			}
			
			// Find the closest symbol before or at the cursor position
			cursorLine := int(params.Position.Line) + 1
			for _, candidate := range allSymbols {
				// Prefer symbols from current file if cursor is in same file
				if candidate.uri == params.TextDocument.URI {
					// For loop variables and local vars, find the closest declaration before cursor
					if candidate.sym.Kind == SymbolKindVariable || candidate.sym.Kind == SymbolKindParameter {
						// Only consider if it's before the cursor or on same line
						if candidate.sym.Line <= cursorLine {
							// If we don't have a best symbol yet, or this one is closer
							if bestSymbol == nil || candidate.sym.Line > bestSymbol.Line {
								bestSymbol = candidate.sym
								targetURI = candidate.uri
							}
						}
					} else {
						// For functions, structs, etc, just use this one
						bestSymbol = candidate.sym
						targetURI = candidate.uri
					}
				} else if targetURI == params.TextDocument.URI {
					// Only use package file symbol if we haven't found one in current file
					bestSymbol = candidate.sym
					targetURI = candidate.uri
				}
			}
		}
		
		// Return the definition location
		location := protocol.Location{
			URI: targetURI,
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(bestSymbol.Line - 1),
					Character: uint32(bestSymbol.Column),
				},
				End: protocol.Position{
					Line:      uint32(bestSymbol.Line - 1),
					Character: uint32(bestSymbol.Column + len(bestSymbol.Name)),
				},
			},
		}

		debugLog.Printf("Go to definition: %s -> %s:%d", word, targetURI, bestSymbol.Line)
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
