package main

import (
	"strings"

	"ahoy"

	"go.lsp.dev/protocol"
)

// checkUnhandledMultiReturns checks for function calls that return multiple values but the values are not captured
func checkUnhandledMultiReturns(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	st := getSymbolTable(doc)
	if st == nil {
		return diagnostics
	}

	// Build function signature map
	funcSignatures := make(map[string]*FunctionSignature)
	var collectFuncs func(*ahoy.ASTNode)
	collectFuncs = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}
		if node.Type == ahoy.NODE_FUNCTION {
			funcName := node.Value
			sig := &FunctionSignature{
				Name:       funcName,
				Parameters: []ParameterInfo{},
				ReturnType: node.DataType,
			}

			// Get parameters
			if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_BLOCK {
				for _, param := range node.Children[0].Children {
					if param.Type == ahoy.NODE_IDENTIFIER {
						sig.Parameters = append(sig.Parameters, ParameterInfo{
							Name: param.Value,
							Type: param.DataType,
						})
					}
				}
			}
			funcSignatures[funcName] = sig
		}
		for _, child := range node.Children {
			collectFuncs(child)
		}
	}
	collectFuncs(doc.AST)

	// Track lines we've already reported
	reportedLines := make(map[int]bool)

	// Check for standalone function calls (not in assignment)
	var checkNode func(*ahoy.ASTNode, bool) // bool indicates if we're inside an assignment context
	checkNode = func(node *ahoy.ASTNode, inAssignment bool) {
		if node == nil {
			return
		}

		// Update inAssignment for children based on current node type
		childInAssignment := inAssignment
		switch node.Type {
		case ahoy.NODE_VARIABLE_DECLARATION, ahoy.NODE_TUPLE_ASSIGNMENT, ahoy.NODE_ASSIGNMENT:
			childInAssignment = true
		}

		// Check if this is a standalone call (not part of an assignment or expression)
		if node.Type == ahoy.NODE_CALL {
			// A call is standalone if it's not inside an assignment
			isStandalone := !inAssignment

			if isStandalone && !reportedLines[node.Line] {
				funcName := node.Value

				// Check if function has multiple return values
				returnTypes := []string{}

				// Check local functions
				if sig, exists := funcSignatures[funcName]; exists && sig.ReturnType != "" {
					returnTypes = parseMultiReturnTypes(sig.ReturnType)
				}

				// Also check via symbol table
				if len(returnTypes) <= 1 {
					if sym := st.Lookup(funcName); sym != nil && sym.Kind == SymbolKindFunction {
						returnType := sym.Type
						if returnType != "" && returnType != "void" && returnType != "infer" && returnType != "any" && returnType != "generic" {
							returnTypes = parseMultiReturnTypes(returnType)
						}
					}
				}

				if len(returnTypes) > 1 {
					// Format the expected types for error message
					typesStr := "["
					for i, rt := range returnTypes {
						if i > 0 {
							typesStr += ","
						}
						typesStr += rt
					}
					typesStr += "]"

					diag := protocol.Diagnostic{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(node.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(node.Line - 1), Character: 100},
						},
						Severity: protocol.DiagnosticSeverityError,
						Source:   "ahoy",
						Message:  "Unhandled return values: function '" + funcName + "' returns multiple values, expected " + typesStr,
					}
					diagnostics = append(diagnostics, diag)
					reportedLines[node.Line] = true
				}
			}
		}

		// Recurse to children
		for _, child := range node.Children {
			checkNode(child, childInAssignment)
		}
	}

	checkNode(doc.AST, false)

	return diagnostics
}

// parseMultiReturnTypes parses a comma-separated return type string into individual types
func parseMultiReturnTypes(returnType string) []string {
	if returnType == "" {
		return []string{}
	}

	var types []string
	depth := 0
	current := ""

	for _, ch := range returnType {
		switch ch {
		case '<', '[':
			depth++
			current += string(ch)
		case '>', ']':
			depth--
			current += string(ch)
		case ',':
			if depth == 0 {
				if trimmed := strings.TrimSpace(current); trimmed != "" {
					types = append(types, trimmed)
				}
				current = ""
			} else {
				current += string(ch)
			}
		default:
			current += string(ch)
		}
	}

	// Add last type
	if trimmed := strings.TrimSpace(current); trimmed != "" {
		types = append(types, trimmed)
	}

	return types
}
