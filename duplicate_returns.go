package main

import (
	"fmt"
	"strings"

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

					// Check for duplicates
					for varName, count := range returnedVars {
						if count > 1 {
							// Check if this is a heap-allocated type (primitive duplicates are OK)
							varType := ""

							// Try to find the variable type from symbol table
							symbolTable := getSymbolTable(doc)
							if symbolTable != nil && symbolTable.GlobalScope != nil {
								if sym := symbolTable.GlobalScope.Lookup(varName); sym != nil {
									varType = sym.Type
								}
							}

							// Check if it's a heap-allocated type
							isHeapType := false
							if strings.HasPrefix(varType, "array") ||
								strings.HasPrefix(varType, "dict") ||
								strings.HasPrefix(varType, "HashMap") ||
								strings.HasPrefix(varType, "AhoyArray") {
								isHeapType = true
							}

							// Only warn for heap-allocated types (primitives are fine)
							if isHeapType {
								diag := protocol.Diagnostic{
									Range: protocol.Range{
										Start: protocol.Position{Line: uint32(n.Line - 1), Character: 0},
										End:   protocol.Position{Line: uint32(n.Line - 1), Character: 100},
									},
									Severity: protocol.DiagnosticSeverityError,
									Source:   "ahoy",
									Message:  fmt.Sprintf("Cannot return heap-allocated variable '%s' multiple times: this will cause double-free", varName),
								}
								diagnostics = append(diagnostics, diag)
							}
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
