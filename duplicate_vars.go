package main

import (
	"fmt"

	"ahoy"
	"go.lsp.dev/protocol"
)

// checkDuplicateVariableDeclarations checks for duplicate variable declarations in the same scope
func checkDuplicateVariableDeclarations(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	// Track variable declarations by scope
	type varDecl struct {
		name string
		line int
		node *ahoy.ASTNode
	}

	// Walk the AST and check for duplicate variable declarations within each function scope
	var checkNode func(node *ahoy.ASTNode, inFunction bool, seenVars map[string]varDecl)
	checkNode = func(node *ahoy.ASTNode, inFunction bool, seenVars map[string]varDecl) {
		if node == nil {
			return
		}

		switch node.Type {
		case ahoy.NODE_FUNCTION:
			// Start a new scope for this function
			funcVars := make(map[string]varDecl)
			
			// Add parameters to the function scope
			if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_BLOCK {
				for _, param := range node.Children[0].Children {
					if param.Type == ahoy.NODE_IDENTIFIER {
						funcVars[param.Value] = varDecl{
							name: param.Value,
							line: param.Line,
							node: param,
						}
					}
				}
			}

			// Check function body with function scope
			if len(node.Children) > 1 {
				checkNode(node.Children[1], true, funcVars)
			}
			return

		case ahoy.NODE_IF_STATEMENT:
			// Handle if/else branches - allow same variable in each branch if not already in parent scope
			debugLog.Printf("Found NODE_IF_STATEMENT at line %d, inFunction=%v, seenVars=%v", node.Line, inFunction, seenVars != nil)
			if inFunction && seenVars != nil {
				// Process if/elif/else branches separately to allow same variable in each
				debugLog.Printf("Processing %d if/else branches", len(node.Children))
				for i, child := range node.Children {
					// Each branch gets its own scope (copy of parent scope)
					branchVars := make(map[string]varDecl)
					for k, v := range seenVars {
						branchVars[k] = v
					}
					debugLog.Printf("Processing branch %d with %d parent vars", i, len(seenVars))
					checkNode(child, inFunction, branchVars)
				}
				return
			}

		case ahoy.NODE_SWITCH_STATEMENT:
			// Handle switch cases - allow same variable in each case if not already in parent scope
			if inFunction && seenVars != nil {
				// Skip the condition (first child)
				if len(node.Children) > 0 {
					// Process each case separately
					for i := 1; i < len(node.Children); i++ {
						// Each case gets its own scope (copy of parent scope)
						caseVars := make(map[string]varDecl)
						for k, v := range seenVars {
							caseVars[k] = v
						}
						checkNode(node.Children[i], inFunction, caseVars)
					}
				}
				return
			}

		case ahoy.NODE_VARIABLE_DECLARATION:
			// Only check if we're inside a function
			if inFunction && seenVars != nil {
				varName := node.Value
				if varName != "" && varName != "_" {
					// DEBUG: Log what we're checking
					if varName == "idx" || varName == "dir_row" || varName == "card_tex" {
						debugLog.Printf("Checking %s at line %d, seenVars has %d entries, found=%v", 
							varName, node.Line, len(seenVars), seenVars[varName].line > 0)
					}
					
					// Check if already declared in this scope
					if existing, found := seenVars[varName]; found {
						// Found a duplicate declaration
						lineText := ""
						if node.Line > 0 && node.Line <= len(doc.Lines) {
							lineText = doc.Lines[node.Line-1]
						}
						endChar := uint32(len(lineText))
						if endChar == 0 {
							endChar = uint32(len(varName) + 10)
						}

						diagnostic := protocol.Diagnostic{
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      uint32(node.Line - 1),
									Character: 0,
								},
								End: protocol.Position{
									Line:      uint32(node.Line - 1),
									Character: endChar,
								},
							},
							Severity: protocol.DiagnosticSeverityError,
							Source:   "ahoy",
							Message:  fmt.Sprintf("variable '%s' already declared on line %d; use \"=\" to update variable", varName, existing.line),
							Code:     "duplicate-variable-declaration",
						}
						diagnostics = append(diagnostics, diagnostic)
					} else {
						// Add to seen variables
						seenVars[varName] = varDecl{
							name: varName,
							line: node.Line,
							node: node,
						}
					}
				}
			}
			// Don't recurse further - we already handle children in the recursive call below
			return
		}

		// Recursively check children (but skip if already handled specially)
		if node.Type != ahoy.NODE_FUNCTION && node.Type != ahoy.NODE_IF_STATEMENT && node.Type != ahoy.NODE_SWITCH_STATEMENT {
			for _, child := range node.Children {
				checkNode(child, inFunction, seenVars)
			}
		}
	}

	// Start checking from the root
	checkNode(doc.AST, false, nil)

	return diagnostics
}
