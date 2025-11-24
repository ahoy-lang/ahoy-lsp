package main

import (
	"fmt"

	"ahoy"

	"go.lsp.dev/protocol"
)

func checkDuplicateReturns(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	var checkFunction func(node *ahoy.ASTNode)
	checkFunction = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		if node.Type == ahoy.NODE_FUNCTION {
			if len(node.Children) < 2 {
				return
			}

			body := node.Children[1]

			// Find all return statements in this function
			var checkReturns func(*ahoy.ASTNode)
			checkReturns = func(n *ahoy.ASTNode) {
				if n == nil {
					return
				}

				if n.Type == ahoy.NODE_RETURN_STATEMENT {
					// Track returned variable names
					returnedVars := make(map[string]int)    // var name -> count
					returnedVarLines := make(map[string]int) // var name -> line

					// Check each returned value
					for _, child := range n.Children {
						if child.Type == ahoy.NODE_IDENTIFIER {
							varName := child.Value
							returnedVars[varName]++
							if returnedVars[varName] == 1 {
								returnedVarLines[varName] = n.Line
							}
						}
					}

					// Check for duplicates - error on ANY duplicate variable return
					for varName, count := range returnedVars {
						if count > 1 {
							diag := protocol.Diagnostic{
								Range: protocol.Range{
									Start: protocol.Position{Line: uint32(n.Line - 1), Character: 0},
									End:   protocol.Position{Line: uint32(n.Line - 1), Character: 100},
								},
								Severity: protocol.DiagnosticSeverityError,
								Source:   "ahoy",
								Message:  fmt.Sprintf("Cannot return the same variable '%s' multiple times", varName),
							}
							diagnostics = append(diagnostics, diag)
						}
					}
				}

				for _, c := range n.Children {
					checkReturns(c)
				}
			}

			checkReturns(body)
		}

		// Recursively check other functions
		for _, child := range node.Children {
			checkFunction(child)
		}
	}

	checkFunction(doc.AST)
	return diagnostics
}
