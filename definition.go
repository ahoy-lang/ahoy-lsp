package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"ahoy"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// StdlibMethod represents a method definition from the stdlib
type StdlibMethod struct {
	Name       string // Method name (e.g., "length", "push")
	Category   string // "array", "dict", "string", or "builtin"
	Line       int    // Line number in stdlib file
	ReturnType string
	Params     string
	Doc        string
}

var (
	stdlibMethods     map[string]StdlibMethod
	stdlibPath        string
	stdlibLoadOnce    sync.Once
	stdlibLoadErr     error
)

// getStdlibPath returns the path to the stdlib file in the ahoy cache directory
func getStdlibPath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		cacheDir = filepath.Join(homeDir, ".cache")
	}
	
	// Check in ahoy cache directory (created by ahoy compiler) for .c file first
	cachedPath := filepath.Join(cacheDir, "ahoy", "ahoy_stdlib.c")
	if _, err := os.Stat(cachedPath); err == nil {
		return cachedPath
	}
	
	// Fallback to old .ahoy file for backwards compatibility
	oldPath := filepath.Join(cacheDir, "ahoy", "ahoy_stdlib.ahoy")
	if _, err := os.Stat(oldPath); err == nil {
		return oldPath
	}
	
	return ""
}

// loadStdlib loads and parses the stdlib file
func loadStdlib() {
	stdlibLoadOnce.Do(func() {
		stdlibMethods = make(map[string]StdlibMethod)
		stdlibPath = getStdlibPath()
		
		if stdlibPath == "" {
			debugLog.Printf("Stdlib not found")
			return
		}
		
		content, err := os.ReadFile(stdlibPath)
		if err != nil {
			stdlibLoadErr = err
			debugLog.Printf("Failed to read stdlib: %v", err)
			return
		}
		
		lines := strings.Split(string(content), "\n")
		var currentCategory string
		var currentDoc strings.Builder
		
		// Check if this is a .c file (new format) or .ahoy file (old format)
		isCFormat := strings.HasSuffix(stdlibPath, ".c")
		
		for i, line := range lines {
			lineNum := i + 1
			trimmed := strings.TrimSpace(line)
			
			// Track category sections - handle both formats
			if strings.Contains(trimmed, "ARRAY METHODS") {
				currentCategory = "array"
			} else if strings.Contains(trimmed, "DICTIONARY METHODS") {
				currentCategory = "dict"
			} else if strings.Contains(trimmed, "STRING METHODS") {
				currentCategory = "string"
			} else if strings.Contains(trimmed, "BUILT-IN FUNCTIONS") {
				currentCategory = "builtin"
			}
			
			if isCFormat {
				// Parse C format: * @ function_name |params| return_type:
				// Collect documentation from * ? comments
				if strings.HasPrefix(trimmed, "* ?") {
					docLine := strings.TrimPrefix(trimmed, "* ? ")
					currentDoc.WriteString(docLine)
					currentDoc.WriteString("\n")
					continue
				}
				
				// Parse function definition line in C format
				if strings.HasPrefix(trimmed, "* @ ") {
					// Format: * @ method_name |params| return_type:
					defLine := strings.TrimPrefix(trimmed, "* @ ")
					parts := strings.SplitN(defLine, "|", 3)
					if len(parts) >= 2 {
						funcName := strings.TrimSpace(parts[0])
						
						// Extract the actual method name from prefixed names like "array_length"
						methodName := funcName
						if strings.HasPrefix(funcName, "array_") {
							methodName = strings.TrimPrefix(funcName, "array_")
						} else if strings.HasPrefix(funcName, "dict_") {
							methodName = strings.TrimPrefix(funcName, "dict_")
						} else if strings.HasPrefix(funcName, "string_") {
							methodName = strings.TrimPrefix(funcName, "string_")
						}
						
						params := ""
						returnType := ""
						if len(parts) >= 3 {
							params = parts[1]
							retPart := parts[2]
							if idx := strings.Index(retPart, ":"); idx != -1 {
								returnType = strings.TrimSpace(retPart[:idx])
							}
						}
						
						// Store with both full name and method name
						method := StdlibMethod{
							Name:       methodName,
							Category:   currentCategory,
							Line:       lineNum,
							ReturnType: returnType,
							Params:     params,
							Doc:        currentDoc.String(),
						}
						stdlibMethods[funcName] = method
						// Also store by just method name for easier lookup
						if methodName != funcName {
							key := currentCategory + "." + methodName
							stdlibMethods[key] = method
						}
						
						currentDoc.Reset()
					}
				}
			} else {
				// Old .ahoy format parsing
				// Collect documentation comments
				if strings.HasPrefix(trimmed, "?") {
					currentDoc.WriteString(strings.TrimPrefix(trimmed, "? "))
					currentDoc.WriteString("\n")
					continue
				}
				
				// Parse function definitions
				if strings.HasPrefix(trimmed, "@ ") {
					// Format: @ method_name |params| return_type:
					parts := strings.SplitN(trimmed, "|", 3)
					if len(parts) >= 2 {
						funcName := strings.TrimSpace(strings.TrimPrefix(parts[0], "@ "))
						
						// Extract the actual method name from prefixed names like "array_length"
						methodName := funcName
						if strings.HasPrefix(funcName, "array_") {
							methodName = strings.TrimPrefix(funcName, "array_")
						} else if strings.HasPrefix(funcName, "dict_") {
							methodName = strings.TrimPrefix(funcName, "dict_")
						} else if strings.HasPrefix(funcName, "string_") {
							methodName = strings.TrimPrefix(funcName, "string_")
						}
						
						params := ""
						returnType := ""
						if len(parts) >= 3 {
							params = parts[1]
							retPart := parts[2]
							if idx := strings.Index(retPart, ":"); idx != -1 {
								returnType = strings.TrimSpace(retPart[:idx])
							}
						}
						
						// Store with both full name and method name
						method := StdlibMethod{
							Name:       methodName,
							Category:   currentCategory,
							Line:       lineNum,
							ReturnType: returnType,
							Params:     params,
							Doc:        currentDoc.String(),
						}
						stdlibMethods[funcName] = method
						// Also store by just method name for easier lookup
						if methodName != funcName {
							key := currentCategory + "." + methodName
							stdlibMethods[key] = method
						}
						
						currentDoc.Reset()
					}
				}
			}
		}
		
		debugLog.Printf("Loaded %d stdlib methods from %s", len(stdlibMethods), stdlibPath)
	})
}

// getStdlibMethodLocation returns the location of a stdlib method
func getStdlibMethodLocation(methodName string, objectType string) *protocol.Location {
	loadStdlib()
	
	if stdlibPath == "" || len(stdlibMethods) == 0 {
		return nil
	}
	
	// Determine category based on object type
	category := ""
	switch {
	case objectType == "array" || strings.HasPrefix(objectType, "array[") || objectType == "AhoyArray*":
		category = "array"
	case objectType == "dict" || strings.HasPrefix(objectType, "dict<") || objectType == "HashMap*":
		category = "dict"
	case objectType == "string" || objectType == "char*":
		category = "string"
	}
	
	// Try category-specific lookup first
	if category != "" {
		key := category + "." + methodName
		if method, ok := stdlibMethods[key]; ok {
			return &protocol.Location{
				URI: protocol.URI("file://" + stdlibPath),
				Range: protocol.Range{
					Start: protocol.Position{Line: uint32(method.Line - 1), Character: 0},
					End:   protocol.Position{Line: uint32(method.Line - 1), Character: 100},
				},
			}
		}
		// Also try with category prefix
		prefixedName := category + "_" + methodName
		if method, ok := stdlibMethods[prefixedName]; ok {
			return &protocol.Location{
				URI: protocol.URI("file://" + stdlibPath),
				Range: protocol.Range{
					Start: protocol.Position{Line: uint32(method.Line - 1), Character: 0},
					End:   protocol.Position{Line: uint32(method.Line - 1), Character: 100},
				},
			}
		}
	}
	
	// Try all categories
	for _, cat := range []string{"array", "dict", "string", "builtin"} {
		key := cat + "." + methodName
		if method, ok := stdlibMethods[key]; ok {
			return &protocol.Location{
				URI: protocol.URI("file://" + stdlibPath),
				Range: protocol.Range{
					Start: protocol.Position{Line: uint32(method.Line - 1), Character: 0},
					End:   protocol.Position{Line: uint32(method.Line - 1), Character: 100},
				},
			}
		}
		prefixedName := cat + "_" + methodName
		if method, ok := stdlibMethods[prefixedName]; ok {
			return &protocol.Location{
				URI: protocol.URI("file://" + stdlibPath),
				Range: protocol.Range{
					Start: protocol.Position{Line: uint32(method.Line - 1), Character: 0},
					End:   protocol.Position{Line: uint32(method.Line - 1), Character: 100},
				},
			}
		}
	}
	
	return nil
}

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
		
		// Check C typedef aliases - go to the typedef line itself
		if doc.CHeaderGlobal.Typedefs != nil {
			if typedef, isAlias := doc.CHeaderGlobal.Typedefs[word]; isAlias {
				if typedef.File != "" && typedef.Line > 0 {
					location := protocol.Location{
						URI: protocol.URI("file://" + typedef.File),
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(typedef.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(typedef.Line - 1), Character: 100},
						},
					}
					debugLog.Printf("Go to definition: C typedef alias %s at %s:%d", word, typedef.File, typedef.Line)
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
		
		// Check typedef aliases - go to the typedef line itself
		if headerInfo.Typedefs != nil {
			if typedef, isAlias := headerInfo.Typedefs[word]; isAlias {
				for _, child := range doc.AST.Children {
					if child.Type == ahoy.NODE_IMPORT_STATEMENT && child.DataType == namespace {
						resolvedPath := resolveImportPath(child.Value, params.TextDocument.URI)
						if typedef.Line > 0 {
							location := protocol.Location{
								URI: protocol.URI("file://" + resolvedPath),
								Range: protocol.Range{
									Start: protocol.Position{Line: uint32(typedef.Line - 1), Character: 0},
									End:   protocol.Position{Line: uint32(typedef.Line - 1), Character: 100},
								},
							}
							debugLog.Printf("Go to definition: namespaced typedef alias %s at %s:%d", word, resolvedPath, typedef.Line)
							return reply(ctx, location, nil)
						}
					}
				}
			}
		}
	}

	// Check for stdlib method calls (array.push, dict.keys, string.upper, etc.)
	// Look for method call context by checking if cursor is after a dot
	line := ""
	if int(params.Position.Line) < len(doc.Lines) {
		line = doc.Lines[int(params.Position.Line)]
	}
	
	// Find if we're in a method call context
	charPos := int(params.Position.Character)
	if charPos > 0 && charPos <= len(line) {
		// Look backward for a dot
		beforeCursor := line[:charPos]
		if dotIdx := strings.LastIndex(beforeCursor, "."); dotIdx >= 0 {
			// Get the object before the dot
			objectPart := strings.TrimSpace(beforeCursor[:dotIdx])
			// Extract the last identifier before the dot
			objectName := ""
			for i := len(objectPart) - 1; i >= 0; i-- {
				ch := objectPart[i]
				if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' {
					continue
				}
				objectName = objectPart[i+1:]
				break
			}
			if objectName == "" && len(objectPart) > 0 {
				objectName = objectPart
			}
			
			// Try to infer object type
			objectType := ""
			if doc.SymbolTable != nil {
				if sym := doc.SymbolTable.Lookup(objectName); sym != nil {
					objectType = sym.Type
				}
			}
			
			// Check if word is a stdlib method
			if location := getStdlibMethodLocation(word, objectType); location != nil {
				debugLog.Printf("Go to definition: stdlib method %s -> %s:%d", word, stdlibPath, location.Range.Start.Line+1)
				return reply(ctx, *location, nil)
			}
		}
	}
	
	// Also check if the word itself is a builtin function like print, log, panic
	builtinFuncs := []string{"print", "log", "panic", "assert", "free"}
	for _, builtin := range builtinFuncs {
		if word == builtin {
			if location := getStdlibMethodLocation(word, "builtin"); location != nil {
				debugLog.Printf("Go to definition: builtin function %s -> %s:%d", word, stdlibPath, location.Range.Start.Line+1)
				return reply(ctx, *location, nil)
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
