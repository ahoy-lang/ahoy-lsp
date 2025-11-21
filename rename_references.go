package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ahoy"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// handlePrepareRename checks if renaming is possible at the cursor position
func (s *Server) handlePrepareRename(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.PrepareRenameParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil || doc.AST == nil {
		return reply(ctx, nil, nil)
	}

	// Find the symbol at the cursor position
	symbol := s.findSymbolAtPosition(doc, params.Position)
	if symbol == nil {
		return reply(ctx, nil, nil)
	}

	// Get the exact word boundaries from the line
	lineText := doc.Lines[params.Position.Line]
	start := int(params.Position.Character)
	end := int(params.Position.Character)
	
	// Move start backward to find word start (including underscores)
	for start > 0 && isIdentifierCharRename(rune(lineText[start-1])) {
		start--
	}
	
	// Move end forward to find word end (including underscores)
	for end < len(lineText) && isIdentifierCharRename(rune(lineText[end])) {
		end++
	}

	// Return the range of the complete symbol
	result := protocol.Range{
		Start: protocol.Position{
			Line:      params.Position.Line,
			Character: uint32(start),
		},
		End: protocol.Position{
			Line:      params.Position.Line,
			Character: uint32(end),
		},
	}

	return reply(ctx, result, nil)
}

// handleRename performs the rename operation
func (s *Server) handleRename(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.RenameParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil || doc.AST == nil {
		return reply(ctx, nil, nil)
	}

	// Find the symbol at the cursor position
	symbol := s.findSymbolAtPosition(doc, params.Position)
	if symbol == nil {
		return reply(ctx, nil, nil)
	}

	// Find all references to this symbol (includes declaration)
	locations := s.findAllReferences(doc, symbol.Name, symbol.Kind)
	
	// Add the declaration location if we have it
	if symbol.Line > 0 {
		// Column is 1-based in AST, LSP expects 0-based
		defCol := uint32(symbol.Column - 1)
		if symbol.Column == 0 {
			defCol = 0
		}
		declLocation := protocol.Location{
			URI: params.TextDocument.URI,
			Range: protocol.Range{
				Start: protocol.Position{Line: uint32(symbol.Line - 1), Character: defCol},
				End:   protocol.Position{Line: uint32(symbol.Line - 1), Character: defCol + uint32(len(symbol.Name))},
			},
		}
		locations = append(locations, declLocation)
	}
	
	// Deduplicate locations
	seen := make(map[string]bool)
	uniqueLocations := []protocol.Location{}
	for _, loc := range locations {
		key := fmt.Sprintf("%s:%d:%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character)
		if !seen[key] {
			seen[key] = true
			uniqueLocations = append(uniqueLocations, loc)
		}
	}

	// Build workspace edit with sorted edits (reverse order to avoid offset issues)
	changes := make(map[protocol.DocumentURI][]protocol.TextEdit)
	for _, loc := range uniqueLocations {
		changes[loc.URI] = append(changes[loc.URI], protocol.TextEdit{
			Range:   loc.Range,
			NewText: params.NewName,
		})
	}
	
	// Sort edits from end to start for each URI
	for uri := range changes {
		edits := changes[uri]
		// Sort in reverse order (from end of file to start)
		for i := 0; i < len(edits)-1; i++ {
			for j := i + 1; j < len(edits); j++ {
				// If edit j comes after edit i in the file, swap them
				if edits[j].Range.Start.Line > edits[i].Range.Start.Line ||
					(edits[j].Range.Start.Line == edits[i].Range.Start.Line &&
						edits[j].Range.Start.Character > edits[i].Range.Start.Character) {
					edits[i], edits[j] = edits[j], edits[i]
				}
			}
		}
		changes[uri] = edits
	}

	result := protocol.WorkspaceEdit{
		Changes: changes,
	}

	return reply(ctx, result, nil)
}

// handleReferences finds all references to a symbol
func (s *Server) handleReferences(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.ReferenceParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil || doc.AST == nil {
		return reply(ctx, []protocol.Location{}, nil)
	}

	// Find the symbol at the cursor position
	symbol := s.findSymbolAtPosition(doc, params.Position)
	if symbol == nil {
		return reply(ctx, []protocol.Location{}, nil)
	}

	// Find all references
	locations := s.findAllReferences(doc, symbol.Name, symbol.Kind)

	// Include declaration if requested
	if params.Context.IncludeDeclaration && symbol.Line > 0 {
		// Column is 1-based in AST, LSP expects 0-based
		defCol := uint32(symbol.Column - 1)
		if symbol.Column == 0 {
			defCol = 0
		}
		declLocation := protocol.Location{
			URI: params.TextDocument.URI,
			Range: protocol.Range{
				Start: protocol.Position{Line: uint32(symbol.Line - 1), Character: defCol},
				End:   protocol.Position{Line: uint32(symbol.Line - 1), Character: defCol + uint32(len(symbol.Name))},
			},
		}
		locations = append([]protocol.Location{declLocation}, locations...)
	}

	return reply(ctx, locations, nil)
}

// findSymbolAtPosition finds the symbol at a given cursor position
func (s *Server) findSymbolAtPosition(doc *Document, pos protocol.Position) *Symbol {
	if doc.SymbolTable == nil {
		return nil
	}

	line := int(pos.Line) + 1
	col := int(pos.Character) + 1

	// Get the word at the cursor position
	if int(pos.Line) >= len(doc.Lines) {
		return nil
	}

	lineText := doc.Lines[pos.Line]
	if int(pos.Character) >= len(lineText) {
		return nil
	}

	// Find word boundaries (including underscores)
	start := int(pos.Character)
	end := int(pos.Character)

	// Move start backward to find word start
	for start > 0 && isIdentifierCharRename(rune(lineText[start-1])) {
		start--
	}

	// Move end forward to find word end
	for end < len(lineText) && isIdentifierCharRename(rune(lineText[end])) {
		end++
	}

	if start == end {
		return nil
	}

	word := lineText[start:end]

	// Search in symbol table
	return doc.SymbolTable.LookupAtPosition(word, line, col)
}

// findAllReferences finds all references to a symbol in the document
func (s *Server) findAllReferences(doc *Document, name string, kind SymbolKind) []protocol.Location {
	var locations []protocol.Location

	// Track which positions we've already added to avoid duplicates
	addedPositions := make(map[string]bool)
	// Track which lines we've fully searched (when no column info)
	searchedLines := make(map[int]bool)

	// Walk the AST to find all uses of the symbol
	var walk func(*ahoy.ASTNode)
	walk = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		// Check various node types that can reference the symbol
		shouldAdd := false
		
		switch node.Type {
		case ahoy.NODE_IDENTIFIER:
			// Variable/function references
			if node.Value == name {
				shouldAdd = true
			}
			
		case ahoy.NODE_MEMBER_ACCESS:
			// Struct member access (obj.field)
			if node.Value == name {
				shouldAdd = true
			}
			
		case ahoy.NODE_FUNCTION:
			// Function declarations
			if node.Value == name {
				shouldAdd = true
			}
			
		case ahoy.NODE_STRUCT_DECLARATION:
			// Struct declarations
			if node.Value == name {
				shouldAdd = true
			}
			
		case ahoy.NODE_ENUM_DECLARATION:
			// Enum declarations
			if node.Value == name {
				shouldAdd = true
			}
			
		case ahoy.NODE_CONSTANT_DECLARATION:
			// Constant declarations
			if node.Value == name {
				shouldAdd = true
			}
			
		case ahoy.NODE_ASSIGNMENT:
			// Variable assignments - check the left side
			if node.Value == name {
				shouldAdd = true
			}
			
		case ahoy.NODE_VARIABLE_DECLARATION:
			// Variable declarations
			if node.Value == name {
				shouldAdd = true
			}
		}
		
		if shouldAdd && node.Line > 0 && node.Line <= len(doc.Lines) {
			// Find the actual position of the name in the source line
			lineIndex := node.Line - 1
			line := doc.Lines[lineIndex]
			
			// Use the node's column if available, otherwise search for all occurrences
			if node.Column > 0 {
				// Node has column information, use it directly
				// Column is 1-based, LSP expects 0-based
				charPos := node.Column - 1
				posKey := fmt.Sprintf("%d:%d", lineIndex, charPos)
				
				if !addedPositions[posKey] {
					addedPositions[posKey] = true
					locations = append(locations, protocol.Location{
						URI: doc.URI,
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(lineIndex), Character: uint32(charPos)},
							End:   protocol.Position{Line: uint32(lineIndex), Character: uint32(charPos + len(name))},
						},
					})
				}
			} else if !searchedLines[lineIndex] {
				// No column info, search for ALL occurrences on this line
				// Only do this once per line to avoid duplicates
				searchedLines[lineIndex] = true
				
				for i := 0; i <= len(line)-len(name); i++ {
					if strings.HasPrefix(line[i:], name) {
						// Verify it's a complete word (check boundaries including underscores)
						isWordStart := i == 0 || !isIdentifierCharRename(rune(line[i-1]))
						isWordEnd := i+len(name) >= len(line) || !isIdentifierCharRename(rune(line[i+len(name)]))
						
						if isWordStart && isWordEnd {
							// Create unique key for this position
							posKey := fmt.Sprintf("%d:%d", lineIndex, i)
							
							// Only add if we haven't added this position yet
							if !addedPositions[posKey] {
								addedPositions[posKey] = true
								locations = append(locations, protocol.Location{
									URI: doc.URI,
									Range: protocol.Range{
										Start: protocol.Position{Line: uint32(lineIndex), Character: uint32(i)},
										End:   protocol.Position{Line: uint32(lineIndex), Character: uint32(i + len(name))},
									},
								})
							}
						}
					}
				}
			}
		}

		// Recurse into children
		for _, child := range node.Children {
			walk(child)
		}
	}

	walk(doc.AST)
	return locations
}

// isIdentifierCharRename checks if a rune is valid in an identifier (including underscores)
func isIdentifierCharRename(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// handleTypeDefinition navigates to the type definition
func (s *Server) handleTypeDefinition(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.TypeDefinitionParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil || doc.AST == nil {
		return reply(ctx, nil, nil)
	}

	// Find the symbol at the cursor position
	symbol := s.findSymbolAtPosition(doc, params.Position)
	if symbol == nil {
		return reply(ctx, nil, nil)
	}

	// If the symbol has a type, try to find that type's definition
	if symbol.Type != "" {
		typeSymbol := doc.SymbolTable.Lookup(symbol.Type)
		if typeSymbol != nil && typeSymbol.Line > 0 {
			// Column is 1-based in AST, LSP expects 0-based
			defCol := uint32(typeSymbol.Column - 1)
			if typeSymbol.Column == 0 {
				defCol = 0
			}
			location := protocol.Location{
				URI: params.TextDocument.URI,
				Range: protocol.Range{
					Start: protocol.Position{Line: uint32(typeSymbol.Line - 1), Character: defCol},
					End:   protocol.Position{Line: uint32(typeSymbol.Line - 1), Character: defCol + uint32(len(typeSymbol.Name))},
				},
			}
			return reply(ctx, location, nil)
		}
	}

	return reply(ctx, nil, nil)
}

// handleDeclaration navigates to the declaration (same as definition for now)
func (s *Server) handleDeclaration(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	// For Ahoy, declaration and definition are the same
	// Just delegate to handleDefinition
	return s.handleDefinition(ctx, reply, req)
}

// handleFormatting formats the document
func (s *Server) handleFormatting(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DocumentFormattingParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, nil, nil)
	}

	// Format the document using the Ahoy formatter
	// For now, return code as-is (formatter not yet implemented)
	debugLog.Printf("Format requested but formatter not yet implemented")
	return reply(ctx, []protocol.TextEdit{}, nil)
}
