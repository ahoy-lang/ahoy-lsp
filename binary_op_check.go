package main

import (
	"ahoy"

	"go.lsp.dev/protocol"
)

// checkBinaryOperationTypes validates that binary operations use compatible types
func checkBinaryOperationTypes(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	var checkNode func(*ahoy.ASTNode, int)
	checkNode = func(node *ahoy.ASTNode, depth int) {
		if node == nil || depth > 500 {
			return
		}

		if node.Type == ahoy.NODE_BINARY_OP {
			op := node.Value

			// Check arithmetic operations: +, -, *, /, %
			if op == "+" || op == "-" || op == "*" || op == "/" || op == "%" {
				if len(node.Children) >= 2 {
					leftType := inferExpressionType(node.Children[0], doc)
					rightType := inferExpressionType(node.Children[1], doc)

					// Skip if either type is unknown
					if leftType == "unknown" || rightType == "unknown" {
						// Still recurse into children
						for _, child := range node.Children {
							checkNode(child, depth+1)
						}
						return
					}

					// Get line number - if 0, try to get from first child
					lineNum := node.Line
					if lineNum == 0 && len(node.Children) > 0 {
						lineNum = node.Children[0].Line
					}

					// Check for string + number errors
					if op == "+" {
						// string + int
						if leftType == "string" && rightType == "int" {
							diagnostics = append(diagnostics, makeBinaryOpDiagnostic(lineNum, "Cannot add string and int; use f-string or explicit string|int| cast"))
						}
						// int + string
						if leftType == "int" && rightType == "string" {
							diagnostics = append(diagnostics, makeBinaryOpDiagnostic(lineNum, "Cannot add int and string; use f-string or explicit string|int| cast"))
						}
						// string + float
						if leftType == "string" && rightType == "float" {
							diagnostics = append(diagnostics, makeBinaryOpDiagnostic(lineNum, "Cannot add string and float; use f-string or explicit string|float| cast"))
						}
						// float + string
						if leftType == "float" && rightType == "string" {
							diagnostics = append(diagnostics, makeBinaryOpDiagnostic(lineNum, "Cannot add float and string; use f-string or explicit string|float| cast"))
						}
					}

					// Check for other arithmetic operations with strings
					if op == "-" || op == "*" || op == "/" || op == "%" {
						if leftType == "string" || rightType == "string" {
							diagnostics = append(diagnostics, makeBinaryOpDiagnostic(lineNum, "Cannot perform arithmetic operation '"+op+"' with string type"))
						}
					}
				}
			}
		}

		// Recurse into children
		for _, child := range node.Children {
			checkNode(child, depth+1)
		}
	}

	checkNode(doc.AST, 0)
	return diagnostics
}

func makeBinaryOpDiagnostic(lineNum int, message string) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: uint32(lineNum - 1), Character: 0},
			End:   protocol.Position{Line: uint32(lineNum - 1), Character: 100},
		},
		Severity: protocol.DiagnosticSeverityError,
		Source:   "ahoy",
		Message:  message,
		Code:     "type-mismatch",
	}
}
