package main

import (
	"fmt"
	"strings"

	"ahoy"

	"go.lsp.dev/protocol"
)

func checkReturnValueCounts(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	// Collect function return counts
	funcReturnCounts := make(map[string]int)

	var collectFunctions func(*ahoy.ASTNode)
	collectFunctions = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		if node.Type == ahoy.NODE_FUNCTION {
			funcName := node.Value
			returnType := node.DataType

			// Count return values by splitting on comma (considering nested types)
			if returnType != "" && returnType != "void" {
				if strings.Contains(returnType, ",") {
					// Multiple returns - count them smartly
					count := 1
					depth := 0
					for _, ch := range returnType {
						if ch == '<' || ch == '[' {
							depth++
						} else if ch == '>' || ch == ']' {
							depth--
						} else if ch == ',' && depth == 0 {
							count++
						}
					}
					funcReturnCounts[funcName] = count
				} else {
					funcReturnCounts[funcName] = 1
				}
			} else {
				funcReturnCounts[funcName] = 0
			}
		}

		for _, child := range node.Children {
			collectFunctions(child)
		}
	}

	collectFunctions(doc.AST)

	// Check assignments
	var checkNode func(node *ahoy.ASTNode)
	checkNode = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		// Check tuple assignments from function calls
		if node.Type == ahoy.NODE_TUPLE_ASSIGNMENT && len(node.Children) > 0 {
			callNode := node.Children[0]
			if callNode.Type == ahoy.NODE_CALL {
				funcName := callNode.Value

				if expectedCount, exists := funcReturnCounts[funcName]; exists {
					varNames := strings.Split(node.Value, ",")
					actualCount := len(varNames)

					if actualCount != expectedCount {
						diag := protocol.Diagnostic{
							Range: protocol.Range{
								Start: protocol.Position{Line: uint32(node.Line - 1), Character: 0},
								End:   protocol.Position{Line: uint32(node.Line - 1), Character: 100},
							},
							Severity: protocol.DiagnosticSeverityError,
							Source:   "ahoy",
							Message:  fmt.Sprintf("Expected %d return values from '%s' but got %d", expectedCount, funcName, actualCount),
						}
						diagnostics = append(diagnostics, diag)
					}
				}
			}
		}

		// Check single variable assignments from function calls
		if node.Type == ahoy.NODE_ASSIGNMENT && len(node.Children) > 0 {
			valueNode := node.Children[0]
			if valueNode.Type == ahoy.NODE_CALL {
				funcName := valueNode.Value

				if expectedCount, exists := funcReturnCounts[funcName]; exists && expectedCount > 1 {
					diag := protocol.Diagnostic{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(node.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(node.Line - 1), Character: 100},
						},
						Severity: protocol.DiagnosticSeverityError,
						Source:   "ahoy",
						Message:  fmt.Sprintf("Expected %d return variables from '%s' but got 1", expectedCount, funcName),
					}
					diagnostics = append(diagnostics, diag)
				}
			}
		}

		for _, child := range node.Children {
			checkNode(child)
		}
	}

	checkNode(doc.AST)
	return diagnostics
}
