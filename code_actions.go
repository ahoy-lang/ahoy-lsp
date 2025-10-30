package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ahoy"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (s *Server) handleCodeAction(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	// Add timeout to prevent hanging
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Use channel to handle timeout
	done := make(chan struct{})
	var result []protocol.CodeAction
	var handleErr error

	go func() {
		defer func() {
			if r := recover(); r != nil {
				debugLog.Printf("PANIC in handleCodeAction: %v", r)
				handleErr = fmt.Errorf("code action panic: %v", r)
			}
			close(done)
		}()

		var params protocol.CodeActionParams
		if err := json.Unmarshal(req.Params(), &params); err != nil {
			handleErr = err
			return
		}

		doc := s.getDocument(params.TextDocument.URI)
		if doc == nil {
			result = []protocol.CodeAction{}
			return
		}

		// Quick validation
		if doc.SymbolTable == nil || doc.Content == "" {
			result = []protocol.CodeAction{}
			return
		}

		actions := []protocol.CodeAction{}

		// Limit number of diagnostics processed to prevent timeouts
		maxDiagnostics := 5
		diagCount := 0

		// Get diagnostics in the range
		for _, diagnostic := range params.Context.Diagnostics {
			if diagCount >= maxDiagnostics {
				break
			}
			diagCount++

			// Generate fixes based on the error message (with limits)
			fixes := generateQuickFixes(doc, diagnostic)
			actions = append(actions, fixes...)

			// Limit total actions to prevent memory issues
			if len(actions) >= 10 {
				break
			}
		}

		// Add general code actions based on context (only if not too many already)
		if len(actions) < 8 {
			contextActions := generateContextActions(doc, params.Range)
			actions = append(actions, contextActions...)
		}

		// Final limit on actions
		if len(actions) > 15 {
			actions = actions[:15]
		}

		result = actions
	}()

	select {
	case <-timeoutCtx.Done():
		debugLog.Printf("Code action request timed out")
		return reply(ctx, []protocol.CodeAction{}, nil)
	case <-done:
		if handleErr != nil {
			return reply(ctx, []protocol.CodeAction{}, nil)
		}
		return reply(ctx, result, nil)
	}
}

func generateQuickFixes(doc *Document, diagnostic protocol.Diagnostic) []protocol.CodeAction {
	actions := []protocol.CodeAction{}

	// Safety check
	if doc == nil || doc.SymbolTable == nil {
		return actions
	}

	message := diagnostic.Message

	// Fix misplaced program declaration
	if strings.Contains(message, "Program declaration must be on the first line") || 
	   (diagnostic.Code == "program-position") {
		action := createMoveProgramToTopAction(doc, diagnostic)
		if action != nil {
			actions = append(actions, *action)
		}
	}

	// Fix missing 'do' keyword
	if strings.Contains(message, "expected 'do'") || strings.Contains(message, "missing 'do'") {
		action := protocol.CodeAction{
			Title: "Add 'do' keyword",
			Kind:  protocol.QuickFix,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					doc.URI: {
						{
							Range: protocol.Range{
								Start: diagnostic.Range.End,
								End:   diagnostic.Range.End,
							},
							NewText: " do",
						},
					},
				},
			},
			Diagnostics: []protocol.Diagnostic{diagnostic},
		}
		actions = append(actions, action)
	}

	// Fix missing 'end' keyword
	if strings.Contains(message, "expected 'end'") || strings.Contains(message, "missing 'end'") {
		action := protocol.CodeAction{
			Title: "Add 'end' keyword",
			Kind:  protocol.QuickFix,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					doc.URI: {
						{
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      diagnostic.Range.End.Line + 1,
									Character: 0,
								},
								End: protocol.Position{
									Line:      diagnostic.Range.End.Line + 1,
									Character: 0,
								},
							},
							NewText: "end\n",
						},
					},
				},
			},
			Diagnostics: []protocol.Diagnostic{diagnostic},
		}
		actions = append(actions, action)
	}

	// Fix missing 'then' keyword
	if strings.Contains(message, "expected 'then'") || strings.Contains(message, "missing 'then'") {
		action := protocol.CodeAction{
			Title: "Add 'then' keyword",
			Kind:  protocol.QuickFix,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					doc.URI: {
						{
							Range: protocol.Range{
								Start: diagnostic.Range.End,
								End:   diagnostic.Range.End,
							},
							NewText: " then",
						},
					},
				},
			},
			Diagnostics: []protocol.Diagnostic{diagnostic},
		}
		actions = append(actions, action)
	}

	// Fix missing colon in assignment
	if strings.Contains(message, "expected ':'") || strings.Contains(message, "missing assignment") {
		action := protocol.CodeAction{
			Title: "Add ':' for assignment",
			Kind:  protocol.QuickFix,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					doc.URI: {
						{
							Range:   diagnostic.Range,
							NewText: ": ",
						},
					},
				},
			},
			Diagnostics: []protocol.Diagnostic{diagnostic},
		}
		actions = append(actions, action)
	}

	// Suggest replacing common mistakes (disabled for now to prevent hangs)
	// This feature was causing performance issues
	/*
		if strings.Contains(message, "undefined") {
			// Extract variable name from message
			parts := strings.Split(message, "'")
			if len(parts) >= 2 {
				undefinedName := parts[1]
				// Suggest similar variable names
				suggestions := findSimilarNames(doc.SymbolTable, undefinedName)
				for _, suggestion := range suggestions {
					action := protocol.CodeAction{
						Title: fmt.Sprintf("Did you mean '%s'?", suggestion),
						Kind:  protocol.QuickFix,
						Edit: &protocol.WorkspaceEdit{
							Changes: map[protocol.DocumentURI][]protocol.TextEdit{
								doc.URI: {
									{
										Range:   diagnostic.Range,
										NewText: suggestion,
									},
								},
							},
						},
						Diagnostics: []protocol.Diagnostic{diagnostic},
					}
					actions = append(actions, action)
				}
			}
		}
	*/

	return actions
}

func generateContextActions(doc *Document, rng protocol.Range) []protocol.CodeAction {
	actions := []protocol.CodeAction{}

	// Safety checks
	if doc == nil || doc.Lines == nil {
		return actions
	}

	// Get the line content from cached lines
	if int(rng.Start.Line) >= len(doc.Lines) || int(rng.Start.Line) < 0 {
		return actions
	}

	line := doc.Lines[rng.Start.Line]
	
	// Sanity check line length to prevent issues
	if len(line) > 10000 {
		return actions
	}

	// Sanity check line length to prevent issues
	if len(line) > 5000 {
		return actions
	}

	// Extract function refactoring
	if strings.Contains(line, "func ") {
		// Offer to add documentation
		action := protocol.CodeAction{
			Title: "Add function documentation",
			Kind:  protocol.Refactor,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					doc.URI: {
						{
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      rng.Start.Line,
									Character: 0,
								},
								End: protocol.Position{
									Line:      rng.Start.Line,
									Character: 0,
								},
							},
							NewText: "? Function description\n",
						},
					},
				},
			},
		}
		actions = append(actions, action)
	}

	// Convert word operators to symbols
	if strings.Contains(line, " plus ") {
		action := protocol.CodeAction{
			Title: "Convert 'plus' to '+'",
			Kind:  protocol.Refactor,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					doc.URI: {
						{
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      rng.Start.Line,
									Character: 0,
								},
								End: protocol.Position{
									Line:      rng.Start.Line,
									Character: uint32(len(line)),
								},
							},
							NewText: strings.ReplaceAll(line, " plus ", " + "),
						},
					},
				},
			},
		}
		actions = append(actions, action)
	}

	if strings.Contains(line, " minus ") {
		action := protocol.CodeAction{
			Title: "Convert 'minus' to '-'",
			Kind:  protocol.Refactor,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					doc.URI: {
						{
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      rng.Start.Line,
									Character: 0,
								},
								End: protocol.Position{
									Line:      rng.Start.Line,
									Character: uint32(len(line)),
								},
							},
							NewText: strings.ReplaceAll(line, " minus ", " - "),
						},
					},
				},
			},
		}
		actions = append(actions, action)
	}

	if strings.Contains(line, " times ") {
		action := protocol.CodeAction{
			Title: "Convert 'times' to '*'",
			Kind:  protocol.Refactor,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					doc.URI: {
						{
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      rng.Start.Line,
									Character: 0,
								},
								End: protocol.Position{
									Line:      rng.Start.Line,
									Character: uint32(len(line)),
								},
							},
							NewText: strings.ReplaceAll(line, " times ", " * "),
						},
					},
				},
			},
		}
		actions = append(actions, action)
	}

	if strings.Contains(line, " is ") {
		action := protocol.CodeAction{
			Title: "Convert 'is' to '=='",
			Kind:  protocol.Refactor,
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentURI][]protocol.TextEdit{
					doc.URI: {
						{
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      rng.Start.Line,
									Character: 0,
								},
								End: protocol.Position{
									Line:      rng.Start.Line,
									Character: uint32(len(line)),
								},
							},
							NewText: strings.ReplaceAll(line, " is ", " == "),
						},
					},
				},
			},
		}
		actions = append(actions, action)
	}

	return actions
}

// findSimilarNames finds variable names similar to the given name using Levenshtein distance
func findSimilarNames(symbolTable *SymbolTable, name string) []string {
	if symbolTable == nil {
		return nil
	}

	suggestions := []string{}
	allSymbols := symbolTable.GetAllSymbols()

	// Strict limit to prevent hanging
	maxCheck := 50
	checked := 0

	for _, sym := range allSymbols {
		if checked >= maxCheck {
			break
		}

		if sym == nil {
			continue
		}

		if sym.Kind == SymbolKindVariable || sym.Kind == SymbolKindFunction || sym.Kind == SymbolKindParameter {
			checked++
			// Calculate similarity (simple version)
			if isSimilar(name, sym.Name) {
				suggestions = append(suggestions, sym.Name)
				// Stop early if we have enough
				if len(suggestions) >= 2 {
					break
				}
			}
		}
	}

	return suggestions
}

// isSimilar checks if two names are similar (simple heuristic)
func isSimilar(a, b string) bool {
	// Same length and similar characters
	if len(a) == len(b) {
		diff := 0
		for i := 0; i < len(a); i++ {
			if a[i] != b[i] {
				diff++
			}
		}
		return diff <= 2
	}

	// One character different in length
	if abs(len(a)-len(b)) == 1 {
		// Check if one is substring of other with one char difference
		longer, shorter := a, b
		if len(b) > len(a) {
			longer, shorter = b, a
		}

		for i := 0; i < len(longer); i++ {
			// Try removing character at position i
			test := longer[:i] + longer[i+1:]
			if test == shorter {
				return true
			}
		}
	}

	// Check if names start the same way
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	if minLen >= 3 {
		same := 0
		for i := 0; i < minLen; i++ {
			if a[i] == b[i] {
				same++
			} else {
				break
			}
		}
		// At least 60% of shorter string matches from start
		return float64(same)/float64(minLen) >= 0.6
	}

	return false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Helper function to check if a node at a position matches a pattern
func nodeMatchesPattern(node *ahoy.ASTNode, pattern string) bool {
	if node == nil {
		return false
	}

	switch pattern {
	case "assignment":
		return node.Type == ahoy.NODE_ASSIGNMENT || node.Type == ahoy.NODE_VARIABLE_DECLARATION
	case "function":
		return node.Type == ahoy.NODE_FUNCTION
	case "if":
		return node.Type == ahoy.NODE_IF_STATEMENT
	case "loop":
		return node.Type == ahoy.NODE_WHILE_LOOP ||
			node.Type == ahoy.NODE_FOR_LOOP ||
			node.Type == ahoy.NODE_FOR_RANGE_LOOP ||
			node.Type == ahoy.NODE_FOR_COUNT_LOOP
	default:
		return false
	}
}

// createMoveProgramToTopAction creates a code action to move program declaration to line 1
func createMoveProgramToTopAction(doc *Document, diagnostic protocol.Diagnostic) *protocol.CodeAction {
	if doc.AST == nil {
		return nil
	}

	// Find the program declaration node
	var programNode *ahoy.ASTNode
	var programText string
	var findProgram func(*ahoy.ASTNode)
	findProgram = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}
		if node.Type == ahoy.NODE_PROGRAM_DECLARATION {
			programNode = node
			return
		}
		for _, child := range node.Children {
			if programNode == nil {
				findProgram(child)
			}
		}
	}
	findProgram(doc.AST)

	if programNode == nil {
		return nil
	}

	// Get the program declaration text from the document
	if programNode.Line > 0 && programNode.Line <= len(doc.Lines) {
		programText = strings.TrimSpace(doc.Lines[programNode.Line-1])
	}

	if programText == "" {
		return nil
	}

	// Create text edits to:
	// 1. Remove program declaration from its current location
	// 2. Add it to the top of the file

	edits := []protocol.TextEdit{
		// Remove from current location
		{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(programNode.Line - 1),
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(programNode.Line), // Include the newline
					Character: 0,
				},
			},
			NewText: "",
		},
		// Add to top of file
		{
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 0},
			},
			NewText: programText + "\n\n",
		},
	}

	action := &protocol.CodeAction{
		Title:       "Move program declaration to top of file",
		Kind:        protocol.QuickFix,
		Diagnostics: []protocol.Diagnostic{diagnostic},
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				doc.URI: edits,
			},
		},
	}

	return action
}
