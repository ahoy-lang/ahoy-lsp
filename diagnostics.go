package main

import (
	"context"
	"strings"

	"ahoy"

	"go.lsp.dev/protocol"
)

// getSymbolTable returns the appropriate symbol table for the document
// Uses PackageSymbols if available (for multi-file packages), otherwise SymbolTable
func getSymbolTable(doc *Document) *SymbolTable {
	if doc.PackageSymbols != nil {
		return doc.PackageSymbols
	}
	return doc.SymbolTable
}

func (s *Server) publishDiagnostics(ctx context.Context, doc *Document) {
	diagnostics := []protocol.Diagnostic{}

	// Check for program declaration position
	if doc.AST != nil {
		programDiag := checkProgramDeclarationPosition(doc)
		if programDiag != nil {
			diagnostics = append(diagnostics, *programDiag)
		}

		// Check for const reassignment and variable/const name collisions
		if doc.SymbolTable != nil {
			constDiags := checkConstReassignment(doc)
			diagnostics = append(diagnostics, constDiags...)

			// Check const method calls
			constMethodDiags := checkConstMethodCalls(doc)
			diagnostics = append(diagnostics, constMethodDiags...)

			// Check invalid method calls
			invalidMethodDiags := checkInvalidMethodCalls(doc)
			diagnostics = append(diagnostics, invalidMethodDiags...)

			// Check return type violations
			returnDiags := checkReturnTypeViolations(doc)
			diagnostics = append(diagnostics, returnDiags...)

			// Check enum duplicates
			enumDiags := checkEnumDuplicates(doc)
			diagnostics = append(diagnostics, enumDiags...)

			// Check enum name duplicates
			enumNameDiags := checkEnumNameDuplicates(doc)
			diagnostics = append(diagnostics, enumNameDiags...)

			// Check undefined function calls
			undefinedFuncDiags := checkUndefinedFunctions(doc)
			diagnostics = append(diagnostics, undefinedFuncDiags...)

			// Check undeclared identifiers (variables, constants, enums)
			undeclaredDiags := checkUndeclaredIdentifiers(doc)
			diagnostics = append(diagnostics, undeclaredDiags...)

			// Check function call argument counts
			argCountDiags := checkFunctionCallArgumentCounts(doc)
			diagnostics = append(diagnostics, argCountDiags...)

			// Check function call argument types
			argTypeDiags := checkFunctionCallArgumentTypes(doc)
			diagnostics = append(diagnostics, argTypeDiags...)

			// Check variable/constant type mismatches
			typeMismatchDiags := checkTypeMismatches(doc)
			diagnostics = append(diagnostics, typeMismatchDiags...)
			
			// Check struct member access and property validation
			memberAccessDiags := checkStructMemberAccess(doc)
			diagnostics = append(diagnostics, memberAccessDiags...)
			
			// Check object literal property assignment
			objectPropDiags := checkObjectPropertyAssignment(doc)
			diagnostics = append(diagnostics, objectPropDiags...)
		}
	}

	// Convert parse errors to LSP diagnostics
	for _, err := range doc.Errors {
		severity := protocol.DiagnosticSeverityError

		// Ensure column doesn't go negative when converting to 0-based
		startCol := err.Column - 1
		if startCol < 0 {
			startCol = 0
		}
		endCol := err.Column + 10
		if endCol < startCol {
			endCol = startCol + 1
		}

		diagnostic := protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(err.Line - 1), // LSP is 0-based, parser is 1-based
					Character: uint32(startCol),
				},
				End: protocol.Position{
					Line:      uint32(err.Line - 1),
					Character: uint32(endCol),
				},
			},
			Severity: severity,
			Source:   "ahoy",
			Message:  err.Message,
		}

		diagnostics = append(diagnostics, diagnostic)
	}

	// Send diagnostics to the editor
	params := protocol.PublishDiagnosticsParams{
		URI:         doc.URI,
		Diagnostics: diagnostics,
	}

	// Notify the client (no reply expected)
	s.conn.Notify(ctx, protocol.MethodTextDocumentPublishDiagnostics, params)
}

// checkProgramDeclarationPosition checks if program declaration is on the first line
func checkProgramDeclarationPosition(doc *Document) *protocol.Diagnostic {
	if doc.AST == nil {
		return nil
	}

	// Find program_declaration node in AST
	var programNode *ahoy.ASTNode
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

	// Check if program declaration is NOT on line 1
	// Allow empty lines or comments before it
	firstNonEmptyLine := 0
	for i, line := range doc.Lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "?") {
			firstNonEmptyLine = i
			break
		}
	}

	// If program node is not on the first non-empty line, create diagnostic
	if programNode.Line > firstNonEmptyLine+1 {
		// Get the line text to calculate end column
		lineText := ""
		if programNode.Line > 0 && programNode.Line <= len(doc.Lines) {
			lineText = doc.Lines[programNode.Line-1]
		}
		endChar := uint32(len(lineText))
		if endChar == 0 {
			endChar = 20 // Default if we can't get line text
		}

		return &protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(programNode.Line - 1),
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(programNode.Line - 1),
					Character: endChar,
				},
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "ahoy",
			Message:  "Program declaration must be on the first line of the file",
			Code:     "program-position",
		}
	}

	return nil
}

// checkConstReassignment checks for const reassignment and variable/const name collisions
func checkConstReassignment(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	symbolTable := getSymbolTable(doc)
	if doc.AST == nil || symbolTable == nil {
		return diagnostics
	}

	// Walk the AST looking for assignments
	var checkNode func(*ahoy.ASTNode, int)
	checkNode = func(node *ahoy.ASTNode, depth int) {
		if node == nil {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		switch node.Type {
		case ahoy.NODE_ASSIGNMENT:
			// Check if assigning to a constant
			varName := node.Value
			if varName != "" {
				// Look up the symbol
				sym := symbolTable.GlobalScope.Lookup(varName)
				if sym != nil && sym.Kind == SymbolKindConstant {
					// Error: trying to reassign a constant
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
						Message:  "Cannot reassign constant '" + varName + "'",
						Code:     "const-reassignment",
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}

		case ahoy.NODE_VARIABLE_DECLARATION:
			// Check if variable name conflicts with a constant
			varName := node.Value
			if varName != "" {
				// Look up the symbol in parent scopes
				sym := doc.SymbolTable.GlobalScope.Lookup(varName)
				if sym != nil && sym.Kind == SymbolKindConstant && sym.Line < node.Line {
					// Error: variable name already used by constant
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
						Message:  "Cannot declare variable '" + varName + "' - already declared as constant",
						Code:     "variable-const-collision",
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}

		case ahoy.NODE_CONSTANT_DECLARATION:
			// Check if constant is being redeclared
			constName := node.Value
			if constName != "" {
				// Look for previous declarations
				sym := doc.SymbolTable.GlobalScope.Lookup(constName)
				if sym != nil && sym.Kind == SymbolKindConstant && sym.Line < node.Line {
					// Error: constant already declared
					lineText := ""
					if node.Line > 0 && node.Line <= len(doc.Lines) {
						lineText = doc.Lines[node.Line-1]
					}
					endChar := uint32(len(lineText))
					if endChar == 0 {
						endChar = uint32(len(constName) + 10)
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
						Message:  "Cannot redeclare constant '" + constName + "'",
						Code:     "const-redeclaration",
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}
		}

		// Recursively check children
		for _, child := range node.Children {
			checkNode(child, depth+1)
		}
	}

	checkNode(doc.AST, 0)
	return diagnostics
}

// checkConstMethodCalls checks for method calls on constants
func checkConstMethodCalls(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil || doc.SymbolTable == nil {
		return diagnostics
	}

	// Walk the AST looking for method calls and member accesses
	var checkNode func(*ahoy.ASTNode, int)
	checkNode = func(node *ahoy.ASTNode, depth int) {
		if node == nil {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		switch node.Type {
		case ahoy.NODE_METHOD_CALL, ahoy.NODE_MEMBER_ACCESS:
			// Check if the target of the method/member access is a constant
			// The first child should be the target (identifier)
			if len(node.Children) > 0 {
				target := node.Children[0]
				if target != nil && target.Type == ahoy.NODE_IDENTIFIER {
					varName := target.Value
					// Look up the symbol
					sym := doc.SymbolTable.GlobalScope.Lookup(varName)
					if sym != nil && sym.Kind == SymbolKindConstant {
						// Error: trying to call method on constant
						lineText := ""
						if node.Line > 0 && node.Line <= len(doc.Lines) {
							lineText = doc.Lines[node.Line-1]
						}
						endChar := uint32(len(lineText))
						if endChar == 0 {
							endChar = uint32(len(varName) + 20)
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
							Message:  "Cannot call methods on constant '" + varName + "'",
							Code:     "const-method-call",
						}
						diagnostics = append(diagnostics, diagnostic)
					}
				}
			}
		}

		// Recursively check children
		for _, child := range node.Children {
			checkNode(child, depth+1)
		}
	}

	checkNode(doc.AST, 0)
	return diagnostics
}

// getValidStringMethods returns list of valid string methods
func getValidStringMethods() []string {
	return []string{
		"length", "upper", "lower", "replace", "contains",
		"camel_case", "snake_case", "pascal_case", "kebab_case",
		"match", "split", "count", "lpad", "rpad", "pad", "strip", "get_file",
	}
}

// getValidArrayMethods returns list of valid array methods
func getValidArrayMethods() []string {
	return []string{
		"length", "push", "pop", "sort", "reverse", "contains",
		"find", "filter", "map", "join", "slice",
	}
}

// getValidDictMethods returns list of valid dict methods
func getValidDictMethods() []string {
	return []string{
		"size", "clear", "has", "has_all", "keys", "values",
		"sort", "stable_sort", "merge",
	}
}

// checkInvalidMethodCalls checks for calls to non-existent methods on strings, arrays, and dicts
func checkInvalidMethodCalls(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil || doc.SymbolTable == nil {
		return diagnostics
	}

	// Walk the AST looking for method calls
	var checkNode func(*ahoy.ASTNode, int)
	checkNode = func(node *ahoy.ASTNode, depth int) {
		if node == nil {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		if node.Type == ahoy.NODE_METHOD_CALL {
			// Get the method name from the node value
			methodName := node.Value

			// Get the target (first child should be the object)
			if len(node.Children) > 0 {
				target := node.Children[0]
				
				// Determine the type of the target
				var targetType string
				if target != nil {
					if target.Type == ahoy.NODE_IDENTIFIER {
						// Look up the symbol to get its type
						sym := doc.SymbolTable.GlobalScope.Lookup(target.Value)
						if sym != nil {
							targetType = sym.Type
						}
					} else if target.Type == ahoy.NODE_STRING {
						targetType = "string"
					} else if target.Type == ahoy.NODE_ARRAY_LITERAL {
						targetType = "array"
					} else if target.Type == ahoy.NODE_DICT_LITERAL {
						targetType = "dict"
					}
				}

				// Check if method exists for the type
				var validMethods []string
				switch targetType {
				case "string":
					validMethods = getValidStringMethods()
				case "array":
					validMethods = getValidArrayMethods()
				case "dict":
					validMethods = getValidDictMethods()
				default:
					// Unknown type, skip validation
					for _, child := range node.Children {
						checkNode(child, depth+1)
					}
					return
				}

				// Check if method is valid
				methodExists := false
				for _, validMethod := range validMethods {
					if validMethod == methodName {
						methodExists = true
						break
					}
				}

				if !methodExists {
					// Method doesn't exist - find similar method using Levenshtein distance
					bestMatch := ""
					bestDistance := 1000000
					
					for _, validMethod := range validMethods {
						distance := levenshteinDistance(methodName, validMethod)
						if distance < bestDistance {
							bestDistance = distance
							bestMatch = validMethod
						}
					}

					// Build error message
					message := "Method '" + methodName + "' does not exist"
					
					// Suggest similar method if distance is reasonable
					// Threshold: max 3 edits or 40% of method name length
					threshold := 3
					if len(methodName) > 7 {
						threshold = (len(methodName) * 2) / 5
					}
					
					if bestDistance <= threshold && bestMatch != "" {
						message += ", did you mean '" + bestMatch + "'?"
					}

					lineText := ""
					if node.Line > 0 && node.Line <= len(doc.Lines) {
						lineText = doc.Lines[node.Line-1]
					}
					endChar := uint32(len(lineText))
					if endChar == 0 {
						endChar = uint32(len(methodName) + 20)
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
						Message:  message,
						Code:     "invalid-method",
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}
		}

		// Recursively check children
		for _, child := range node.Children {
			checkNode(child, depth+1)
		}
	}

	checkNode(doc.AST, 0)
	return diagnostics
}

// checkReturnTypeViolations checks for return type mismatches
func checkReturnTypeViolations(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	// Walk the AST looking for functions
	var checkNode func(*ahoy.ASTNode, int)
	checkNode = func(node *ahoy.ASTNode, depth int) {
		if node == nil {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		if node.Type == ahoy.NODE_FUNCTION {
			returnType := node.DataType
			hasReturn := false

			// Check if function body has return statements
			if len(node.Children) >= 2 {
				body := node.Children[1] // Function body

				var checkReturns func(*ahoy.ASTNode)
				checkReturns = func(n *ahoy.ASTNode) {
					if n == nil {
						return
					}

					if n.Type == ahoy.NODE_RETURN_STATEMENT {
						hasReturn = true

						// Skip checking if return type is "infer"
						if returnType == "infer" {
							// No validation needed for inferred types
						} else if returnType == "void" && len(n.Children) > 0 {
							// Check if void function returns a value
							returnedType := inferReturnType(n.Children[0])

							lineText := ""
							if n.Line > 0 && n.Line <= len(doc.Lines) {
								lineText = doc.Lines[n.Line-1]
							}
							endChar := uint32(len(lineText))
							if endChar == 0 {
								endChar = 30
							}

							diagnostic := protocol.Diagnostic{
								Range: protocol.Range{
									Start: protocol.Position{
										Line:      uint32(n.Line - 1),
										Character: 0,
									},
									End: protocol.Position{
										Line:      uint32(n.Line - 1),
										Character: endChar,
									},
								},
								Severity: protocol.DiagnosticSeverityError,
								Source:   "ahoy",
								Message:  "Expected void, got return type " + returnedType,
								Code:     "void-return-violation",
							}
							diagnostics = append(diagnostics, diagnostic)
						} else if returnType != "" && returnType != "void" && len(n.Children) > 0 {
							// Check if return type matches
							returnedType := inferReturnType(n.Children[0])

							// Check if types match (handle multiple return types)
							expectedTypes := strings.Split(returnType, ",")
							matches := false
							for _, et := range expectedTypes {
								if strings.TrimSpace(et) == returnedType || strings.TrimSpace(et) == "generic" {
									matches = true
									break
								}
							}

							if !matches && returnedType != "unknown" {
								lineText := ""
								if n.Line > 0 && n.Line <= len(doc.Lines) {
									lineText = doc.Lines[n.Line-1]
								}
								endChar := uint32(len(lineText))
								if endChar == 0 {
									endChar = 30
								}

								diagnostic := protocol.Diagnostic{
									Range: protocol.Range{
										Start: protocol.Position{
											Line:      uint32(n.Line - 1),
											Character: 0,
										},
										End: protocol.Position{
											Line:      uint32(n.Line - 1),
											Character: endChar,
										},
									},
									Severity: protocol.DiagnosticSeverityError,
									Source:   "ahoy",
									Message:  "Expected return type " + returnType + ", got " + returnedType,
									Code:     "return-type-mismatch",
								}
								diagnostics = append(diagnostics, diagnostic)
							}
						}
					}

					for _, child := range n.Children {
						checkReturns(child)
					}
				}

				checkReturns(body)
			}

			// Check if non-void, non-infer function has return statement
			if returnType != "" && returnType != "void" && returnType != "infer" && !hasReturn {
				lineText := ""
				if node.Line > 0 && node.Line <= len(doc.Lines) {
					lineText = doc.Lines[node.Line-1]
				}
				endChar := uint32(len(lineText))
				if endChar == 0 {
					endChar = 30
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
					Message:  "Function with return type " + returnType + " must return a value",
					Code:     "missing-return",
				}
				diagnostics = append(diagnostics, diagnostic)
			}
		}

		// Recursively check children
		for _, child := range node.Children {
			checkNode(child, depth+1)
		}
	}

	checkNode(doc.AST, 0)
	return diagnostics
}

// inferReturnType infers the type of a return expression
func inferReturnType(node *ahoy.ASTNode) string {
	if node == nil {
		return "void"
	}

	switch node.Type {
	case ahoy.NODE_NUMBER:
		// Check if it's a float or int
		if strings.Contains(node.Value, ".") {
			return "float"
		}
		return "int"
	case ahoy.NODE_STRING:
		return "string"
	case ahoy.NODE_BOOLEAN:
		return "bool"
	case ahoy.NODE_IDENTIFIER:
		// Would need symbol table lookup for proper type
		return "unknown"
	case ahoy.NODE_CALL:
		// Would need function signature lookup
		return "unknown"
	default:
		return "unknown"
	}
}

// checkEnumDuplicates checks for duplicate enum members
func checkEnumDuplicates(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	// Walk the AST looking for enums
	var checkNode func(*ahoy.ASTNode, int)
	checkNode = func(node *ahoy.ASTNode, depth int) {
		if node == nil {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		if node.Type == ahoy.NODE_ENUM_DECLARATION {
			// Track member names and their lines
			memberMap := make(map[string][]int)

			// Collect all member names
			for _, child := range node.Children {
				if child.Type == ahoy.NODE_IDENTIFIER {
					memberName := child.Value
					memberMap[memberName] = append(memberMap[memberName], child.Line)
				}
			}

			// Check for duplicates
			for memberName, lines := range memberMap {
				if len(lines) > 1 {
					// Report error for each duplicate occurrence (except the first)
					for i := 1; i < len(lines); i++ {
						line := lines[i]
						lineText := ""
						if line > 0 && line <= len(doc.Lines) {
							lineText = doc.Lines[line-1]
						}
						endChar := uint32(len(lineText))
						if endChar == 0 {
							endChar = uint32(len(memberName) + 10)
						}

						diagnostic := protocol.Diagnostic{
							Range: protocol.Range{
								Start: protocol.Position{
									Line:      uint32(line - 1),
									Character: 0,
								},
								End: protocol.Position{
									Line:      uint32(line - 1),
									Character: endChar,
								},
							},
							Severity: protocol.DiagnosticSeverityError,
							Source:   "ahoy",
							Message:  "Duplicate enum member '" + memberName + "'",
							Code:     "enum-duplicate-member",
						}
						diagnostics = append(diagnostics, diagnostic)
					}
				}
			}
		}

		// Recursively check children
		for _, child := range node.Children {
			checkNode(child, depth+1)
		}
	}

	checkNode(doc.AST, 0)
	return diagnostics
}

// checkEnumNameDuplicates checks for duplicate enum declarations
func checkEnumNameDuplicates(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	// Track enum names and their lines
	enumMap := make(map[string][]int)

	// Walk the AST looking for enum declarations
	var checkNode func(*ahoy.ASTNode, int)
	checkNode = func(node *ahoy.ASTNode, depth int) {
		if node == nil {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		if node.Type == ahoy.NODE_ENUM_DECLARATION {
			enumName := node.Value
			if enumName != "" {
				enumMap[enumName] = append(enumMap[enumName], node.Line)
			}
		}

		// Recursively check children
		for _, child := range node.Children {
			checkNode(child, depth+1)
		}
	}

	checkNode(doc.AST, 0)

	// Check for duplicate enum names
	for enumName, lines := range enumMap {
		if len(lines) > 1 {
			// Report error for each duplicate occurrence (except the first)
			for i := 1; i < len(lines); i++ {
				line := lines[i]
				lineText := ""
				if line > 0 && line <= len(doc.Lines) {
					lineText = doc.Lines[line-1]
				}
				endChar := uint32(len(lineText))
				if endChar == 0 {
					endChar = uint32(len(enumName) + 10)
				}

				diagnostic := protocol.Diagnostic{
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(line - 1),
							Character: 0,
						},
						End: protocol.Position{
							Line:      uint32(line - 1),
							Character: endChar,
						},
					},
					Severity: protocol.DiagnosticSeverityError,
					Source:   "ahoy",
					Message:  "Enum '" + enumName + "' declared twice",
					Code:     "enum-duplicate-declaration",
				}
				diagnostics = append(diagnostics, diagnostic)
			}
		}
	}

	return diagnostics
}

// levenshteinDistance calculates the edit distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}

	// Initialize first row and column
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			deletion := matrix[i-1][j] + 1
			insertion := matrix[i][j-1] + 1
			substitution := matrix[i-1][j-1] + cost

			min := deletion
			if insertion < min {
				min = insertion
			}
			if substitution < min {
				min = substitution
			}

			matrix[i][j] = min
		}
	}

	return matrix[len(s1)][len(s2)]
}

// builtinFunctions is a list of all built-in functions in Ahoy
var builtinFunctions = []string{
	"print",
	"sprintf",
	"ahoy",
}

// isBuiltinFunction checks if a function name is a built-in function
func isBuiltinFunction(name string) bool {
	for _, builtin := range builtinFunctions {
		if builtin == name {
			return true
		}
	}
	return false
}

// findSimilarFunction finds the most similar function name using Levenshtein distance
func findSimilarFunction(name string, availableFuncs []string) (string, int) {
	bestMatch := ""
	bestDistance := 1000000

	for _, funcName := range availableFuncs {
		distance := levenshteinDistance(name, funcName)
		if distance < bestDistance {
			bestDistance = distance
			bestMatch = funcName
		}
	}

	return bestMatch, bestDistance
}

// checkUndefinedFunctions checks for calls to undefined functions
func checkUndefinedFunctions(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	// Use PackageSymbols if available, otherwise use SymbolTable
	symbolTable := doc.SymbolTable
	if doc.PackageSymbols != nil {
		symbolTable = doc.PackageSymbols
	}

	if doc.AST == nil || symbolTable == nil {
		return diagnostics
	}

	// Collect all available function names (built-ins + user-defined + C imports)
	availableFuncs := make([]string, 0)
	availableFuncs = append(availableFuncs, builtinFunctions...)

	// Add user-defined functions from symbol table (includes package symbols)
	for _, sym := range symbolTable.GlobalScope.Symbols {
		if sym.Kind == SymbolKindFunction {
			availableFuncs = append(availableFuncs, sym.Name)
		}
	}

	// Add C header functions (global imports - snake_case names)
	if doc.CHeaderGlobal != nil {
		for funcName := range doc.CHeaderGlobal.Functions {
			snakeName := ahoy.PascalToSnake(funcName)
			availableFuncs = append(availableFuncs, snakeName)
		}
	}

	// Walk the AST looking for function calls
	var checkNode func(*ahoy.ASTNode, int)
	checkNode = func(node *ahoy.ASTNode, depth int) {
		if node == nil {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		if node.Type == ahoy.NODE_CALL {
			funcName := node.Value

			// Check if function exists (built-in or user-defined or C import)
			if !isBuiltinFunction(funcName) {
				sym := symbolTable.GlobalScope.Lookup(funcName)
				isUserDefined := sym != nil && sym.Kind == SymbolKindFunction
				
				// Check if it's a C function (global import)
				isCFunction := false
				if doc.CHeaderGlobal != nil {
					for cFuncName := range doc.CHeaderGlobal.Functions {
						if ahoy.PascalToSnake(cFuncName) == funcName {
							isCFunction = true
							break
						}
					}
				}
				
				// Also check namespaced C functions (e.g., rl.function_name)
				// These would come through as method calls, but we'll handle them separately
				
				if !isUserDefined && !isCFunction {
					// Function not found - find similar function
					similarFunc, distance := findSimilarFunction(funcName, availableFuncs)

					lineText := ""
					if node.Line > 0 && node.Line <= len(doc.Lines) {
						lineText = doc.Lines[node.Line-1]
					}
					endChar := uint32(len(lineText))
					if endChar == 0 {
						endChar = uint32(len(funcName) + 10)
					}

					message := funcName + " func not found"

					// If we found a similar function within reasonable distance, suggest it
					// Threshold: max 3 edits or 30% of the function name length
					threshold := 3
					if len(funcName) > 10 {
						threshold = len(funcName) / 3
					}

					if distance <= threshold && similarFunc != "" {
						message += ", did you mean " + similarFunc
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
						Message:  message,
						Code:     "undefined-function",
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}
		}

		// Recursively check children
		for _, child := range node.Children {
			checkNode(child, depth+1)
		}
	}

	checkNode(doc.AST, 0)
	return diagnostics
}

// checkUndeclaredIdentifiers checks for use of undeclared variables, constants, and enums
func checkUndeclaredIdentifiers(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	symbolTable := getSymbolTable(doc)
	if doc.AST == nil || symbolTable == nil {
		return diagnostics
	}

	// Track scope stack as we traverse
	type scopeInfo struct {
		scope      *Scope
		childIndex int // Which child scope we're currently using
	}
	
	scopeStack := []scopeInfo{{scope: symbolTable.GlobalScope, childIndex: 0}}

	// Walk the AST looking for identifier usage
	var checkNode func(*ahoy.ASTNode, int)
	checkNode = func(node *ahoy.ASTNode, depth int) {
		if node == nil {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		currentInfo := &scopeStack[len(scopeStack)-1]
		scope := currentInfo.scope

		// Check identifiers in various contexts
		switch node.Type {
		case ahoy.NODE_IDENTIFIER:
			identifierName := node.Value
			
			// Look up the identifier in the current scope (searches parent scopes too)
			sym := scope.Lookup(identifierName)
			
			// If not found in symbol table, check C header enums/defines
			if sym == nil {
				// Check C header enums and defines
				foundInCHeader := false
				if doc.CHeaderGlobal != nil {
					// Check if it's an enum VALUE (not enum name)
					for _, enum := range doc.CHeaderGlobal.Enums {
						if _, ok := enum.Values[identifierName]; ok {
							foundInCHeader = true
							break
						}
					}
					// Check defines
					if !foundInCHeader {
						if _, ok := doc.CHeaderGlobal.Defines[identifierName]; ok {
							foundInCHeader = true
						}
					}
				}
				
				// Also check namespaced headers
				if !foundInCHeader {
					for _, headerInfo := range doc.CHeaders {
						// Check enum values
						for _, enum := range headerInfo.Enums {
							if _, ok := enum.Values[identifierName]; ok {
								foundInCHeader = true
								break
							}
						}
						if foundInCHeader {
							break
						}
						// Check defines
						if _, ok := headerInfo.Defines[identifierName]; ok {
							foundInCHeader = true
							break
						}
					}
				}
				
				if foundInCHeader {
					// Found in C headers, no error
					sym = &Symbol{Name: identifierName, Kind: SymbolKindCEnum}
				}
			}
			
			if sym == nil {
				// Identifier not found
				identifierType := "variable"
				
				// Check naming convention
				isAllCaps := true
				for _, ch := range identifierName {
					if ch >= 'a' && ch <= 'z' {
						isAllCaps = false
						break
					}
				}
				
				if isAllCaps && len(identifierName) > 1 {
					identifierType = "constant"
				}
				
				message := "Use of undeclared " + identifierType + " '" + identifierName + "'"
				
				lineText := ""
				if node.Line > 0 && node.Line <= len(doc.Lines) {
					lineText = doc.Lines[node.Line-1]
				}
				endChar := uint32(len(lineText))
				if endChar == 0 {
					endChar = uint32(len(identifierName) + 10)
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
					Message:  message,
					Code:     "undeclared-identifier",
				}
				diagnostics = append(diagnostics, diagnostic)
			}

		case ahoy.NODE_ASSIGNMENT, ahoy.NODE_VARIABLE_DECLARATION:
			// For assignments, check the right side but not the left side
			for i := 1; i < len(node.Children); i++ {
				checkNode(node.Children[i], depth+1)
			}
			return

		case ahoy.NODE_CONSTANT_DECLARATION:
			// For const declarations, check the right side but not the name
			for i := 1; i < len(node.Children); i++ {
				checkNode(node.Children[i], depth+1)
			}
			return

		case ahoy.NODE_FUNCTION:
			// Push function scope (take next child scope)
			if currentInfo.childIndex < len(scope.Children) {
				funcScope := scope.Children[currentInfo.childIndex]
				currentInfo.childIndex++
				scopeStack = append(scopeStack, scopeInfo{scope: funcScope, childIndex: 0})
				
				// Check function body
				if len(node.Children) > 1 {
					checkNode(node.Children[1], depth+1)
				}
				
				scopeStack = scopeStack[:len(scopeStack)-1]
			}
			return

		case ahoy.NODE_WHILE_LOOP, ahoy.NODE_FOR_RANGE_LOOP, ahoy.NODE_FOR_COUNT_LOOP,
			ahoy.NODE_FOR_IN_ARRAY_LOOP, ahoy.NODE_FOR_IN_DICT_LOOP:
			// Push loop scope (take next child scope from current scope)
			if currentInfo.childIndex < len(scope.Children) {
				loopScope := scope.Children[currentInfo.childIndex]
				currentInfo.childIndex++
				scopeStack = append(scopeStack, scopeInfo{scope: loopScope, childIndex: 0})
				
				// Determine which children to skip (variable declarations)
				startIdx := 0
				switch node.Type {
				case ahoy.NODE_FOR_RANGE_LOOP:
					startIdx = 1 // Skip loop var
				case ahoy.NODE_WHILE_LOOP:
					if len(node.Children) >= 3 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
						startIdx = 1 // Skip loop var
					}
				case ahoy.NODE_FOR_COUNT_LOOP:
					if len(node.Children) >= 2 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
						startIdx = 1 // Skip loop var
					}
				case ahoy.NODE_FOR_IN_ARRAY_LOOP:
					startIdx = 1 // Skip element var
				case ahoy.NODE_FOR_IN_DICT_LOOP:
					startIdx = 2 // Skip key and value vars
				}
				
				// Check children with loop scope
				for i := startIdx; i < len(node.Children); i++ {
					checkNode(node.Children[i], depth+1)
				}
				
				scopeStack = scopeStack[:len(scopeStack)-1]
			} else {
				// No more child scopes, check with current scope
				for i := 0; i < len(node.Children); i++ {
					checkNode(node.Children[i], depth+1)
				}
			}
			return

		case ahoy.NODE_ENUM_DECLARATION, ahoy.NODE_STRUCT_DECLARATION:
			return

		case ahoy.NODE_CALL:
			for _, child := range node.Children {
				checkNode(child, depth+1)
			}
			return
		}

		// Recursively check children
		for _, child := range node.Children {
			checkNode(child, depth+1)
		}
	}

	checkNode(doc.AST, 0)
	return diagnostics
}

// findScopeForLine finds the most specific scope that contains the given line
func findScopeForLine(scope *Scope, line int) *Scope {
	if scope == nil {
		return nil
	}
	
	// First check direct children
	for _, child := range scope.Children {
		if child.StartLine <= line && (child.EndLine == 0 || child.EndLine >= line) {
			// Recursively search in child scopes for a more specific match
			if nestedScope := findScopeForLine(child, line); nestedScope != nil {
				return nestedScope
			}
			return child
		}
	}
	
	// If current scope contains the line, return it
	if scope.StartLine <= line && (scope.EndLine == 0 || scope.EndLine >= line) {
		return scope
	}
	
	return nil
}

// findFunctionScope finds the child scope that contains the given line (function scope)
func findFunctionScope(scope *Scope, line int) *Scope {
	if scope == nil {
		return nil
	}
	
	// Check each child scope to see if it contains this line
	for _, child := range scope.Children {
		if child.StartLine <= line && (child.EndLine == 0 || child.EndLine >= line) {
			return child
		}
	}
	
	return nil
}

// inferArgType attempts to infer the type of an argument node
func inferArgType(node *ahoy.ASTNode) string {
	if node == nil {
		return "unknown"
	}
	
	switch node.Type {
	case ahoy.NODE_NUMBER:
		// Check if it's float or int based on value
		if strings.Contains(node.Value, ".") {
			return "float"
		}
		return "int"
	case ahoy.NODE_STRING:
		return "string"
	case ahoy.NODE_BOOLEAN:
		return "bool"
	case ahoy.NODE_IDENTIFIER:
		// Could look up in symbol table, but for now return identifier type if available
		if node.DataType != "" {
			return mapCTypeToAhoyType(node.DataType)
		}
		return "identifier"
	case ahoy.NODE_ARRAY_LITERAL:
		return "array"
	case ahoy.NODE_DICT_LITERAL:
		return "dict"
	case ahoy.NODE_OBJECT_LITERAL:
		// Return the struct type if available (Value holds the struct type name)
		if node.Value != "" {
			return strings.ToLower(node.Value)
		}
		if node.DataType != "" {
			return strings.ToLower(node.DataType)
		}
		return "object"
	default:
		if node.DataType != "" {
			return mapCTypeToAhoyType(node.DataType)
		}
		return "unknown"
	}
}

// checkFunctionCallArgumentCounts checks if function calls have the correct number of arguments
func checkFunctionCallArgumentCounts(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	symbolTable := getSymbolTable(doc)
	if doc.AST == nil || symbolTable == nil {
		return diagnostics
	}

	funcSignatures := make(map[string]*FunctionSignature)

	var collectFunctions func(*ahoy.ASTNode)
	collectFunctions = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		if node.Type == ahoy.NODE_FUNCTION {
			funcName := node.Value
			sig := &FunctionSignature{
				Name:       funcName,
				ReturnType: node.DataType,
			}

			if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_BLOCK {
				params := node.Children[0]
				for _, param := range params.Children {
					if param.Type == ahoy.NODE_IDENTIFIER {
						paramInfo := ParameterInfo{
							Name:       param.Value,
							Type:       param.DataType,
							HasDefault: param.DefaultValue != nil,
						}
						sig.Parameters = append(sig.Parameters, paramInfo)
						if !paramInfo.HasDefault {
							sig.RequiredParams++
						}
					}
				}
				sig.TotalParams = len(sig.Parameters)
			}

			funcSignatures[funcName] = sig
		}

		for _, child := range node.Children {
			collectFunctions(child)
		}
	}

	collectFunctions(doc.AST)
	
	// Also collect functions from package files
	if doc.PackageFiles != nil {
		for _, pkgFile := range doc.PackageFiles {
			if pkgFile.AST != nil {
				collectFunctions(pkgFile.AST)
			}
		}
	}
	
	// Add C function signatures from imported headers
	if doc.CHeaderGlobal != nil {
		for cFuncName, cFunc := range doc.CHeaderGlobal.Functions {
			snakeName := ahoy.PascalToSnake(cFuncName)
			sig := &FunctionSignature{
				Name:          snakeName,
				ReturnType:    cFunc.ReturnType,
				RequiredParams: len(cFunc.Parameters),
				TotalParams:   len(cFunc.Parameters),
			}
			for _, param := range cFunc.Parameters {
				sig.Parameters = append(sig.Parameters, ParameterInfo{
					Name:       param.Name,
					Type:       param.Type,
					HasDefault: false,
				})
			}
			funcSignatures[snakeName] = sig
		}
	}
	
	// Add namespaced C functions
	for _, headerInfo := range doc.CHeaders {
		for cFuncName, cFunc := range headerInfo.Functions {
			snakeName := ahoy.PascalToSnake(cFuncName)
			sig := &FunctionSignature{
				Name:          snakeName,
				ReturnType:    cFunc.ReturnType,
				RequiredParams: len(cFunc.Parameters),
				TotalParams:   len(cFunc.Parameters),
			}
			for _, param := range cFunc.Parameters {
				sig.Parameters = append(sig.Parameters, ParameterInfo{
					Name:       param.Name,
					Type:       param.Type,
					HasDefault: false,
				})
			}
			funcSignatures[snakeName] = sig
		}
	}

	var checkCalls func(*ahoy.ASTNode)
	checkCalls = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		if node.Type == ahoy.NODE_CALL {
			funcName := node.Value
			argCount := len(node.Children)

			if sig, exists := funcSignatures[funcName]; exists {
				expectedMin := sig.RequiredParams
				expectedMax := sig.TotalParams

				// Build expected types string
				expectedTypes := "["
				for i, param := range sig.Parameters {
					if i > 0 {
						expectedTypes += ","
					}
					expectedTypes += param.Type
				}
				expectedTypes += "]"
				
				// Build actual types string (approximation based on arg nodes)
				actualTypes := "["
				for i, arg := range node.Children {
					if i > 0 {
						actualTypes += ","
					}
					// Try to infer type from argument
					argType := inferExpressionType(arg, doc)
					actualTypes += argType
				}
				actualTypes += "]"

				message := ""
				if argCount < expectedMin {
					if expectedMin == expectedMax {
						if expectedMin == 0 {
							message = "expected no arguments, got " + intToString(argCount)
						} else if expectedMin == 1 {
							message = "expected 1 argument " + expectedTypes + ", got none"
						} else {
							message = "expected " + intToString(expectedMin) + " arguments " + expectedTypes + ", got " + intToString(argCount) + " " + actualTypes
						}
					} else {
						message = "expected " + intToString(expectedMin) + "-" + intToString(expectedMax) +
							" arguments " + expectedTypes + ", got " + intToString(argCount) + " " + actualTypes
					}
				} else if argCount > expectedMax {
					if expectedMin == expectedMax {
						if expectedMax == 1 {
							message = "expected 1 argument " + expectedTypes + ", got " + intToString(argCount) + " " + actualTypes
						} else {
							message = "expected " + intToString(expectedMax) + " arguments " + expectedTypes + ", got " + intToString(argCount) + " " + actualTypes
						}
					} else {
						message = "expected " + intToString(expectedMin) + "-" + intToString(expectedMax) +
							" arguments " + expectedTypes + ", got " + intToString(argCount) + " " + actualTypes
					}
				}

				if message != "" {
					lineText := ""
					if node.Line > 0 && node.Line <= len(doc.Lines) {
						lineText = doc.Lines[node.Line-1]
					}
					endChar := uint32(len(lineText))
					if endChar == 0 {
						endChar = uint32(len(funcName) + 10)
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
						Message:  message,
						Code:     "argument-count-mismatch",
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}
		}

		for _, child := range node.Children {
			checkCalls(child)
		}
	}

	checkCalls(doc.AST)
	return diagnostics
}

// checkFunctionCallArgumentTypes checks if function call arguments match parameter types
func checkFunctionCallArgumentTypes(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	symbolTable := getSymbolTable(doc)
	if doc.AST == nil || symbolTable == nil {
		return diagnostics
	}

	funcSignatures := make(map[string]*FunctionSignature)

	var collectFunctions func(*ahoy.ASTNode)
	collectFunctions = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		if node.Type == ahoy.NODE_FUNCTION {
			funcName := node.Value
			sig := &FunctionSignature{
				Name:       funcName,
				ReturnType: node.DataType,
			}

			if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_BLOCK {
				params := node.Children[0]
				for _, param := range params.Children {
					if param.Type == ahoy.NODE_IDENTIFIER {
						paramInfo := ParameterInfo{
							Name:       param.Value,
							Type:       param.DataType,
							HasDefault: param.DefaultValue != nil,
						}
						sig.Parameters = append(sig.Parameters, paramInfo)
						if !paramInfo.HasDefault {
							sig.RequiredParams++
						}
					}
				}
				sig.TotalParams = len(sig.Parameters)
			}

			funcSignatures[funcName] = sig
		}

		for _, child := range node.Children {
			collectFunctions(child)
		}
	}

	collectFunctions(doc.AST)
	
	// Also collect functions from package files
	if doc.PackageFiles != nil {
		for _, pkgFile := range doc.PackageFiles {
			if pkgFile.AST != nil {
				collectFunctions(pkgFile.AST)
			}
		}
	}

	// Add C imported functions (global)
	if doc.CHeaderGlobal != nil {
		for cFuncName, cFunc := range doc.CHeaderGlobal.Functions {
			snakeName := ahoy.PascalToSnake(cFuncName)
			sig := &FunctionSignature{
				Name:           snakeName,
				ReturnType:     cFunc.ReturnType,
				RequiredParams: len(cFunc.Parameters),
				TotalParams:    len(cFunc.Parameters),
			}
			for _, param := range cFunc.Parameters {
				// Map C types to Ahoy types
				ahoyType := mapCTypeToAhoyType(param.Type)
				sig.Parameters = append(sig.Parameters, ParameterInfo{
					Name:       param.Name,
					Type:       ahoyType,
					HasDefault: false,
				})
			}
			funcSignatures[snakeName] = sig
		}
	}

	// Add C imported functions (namespaced)
	for _, cHeader := range doc.CHeaders {
		for cFuncName, cFunc := range cHeader.Functions {
			snakeName := ahoy.PascalToSnake(cFuncName)
			sig := &FunctionSignature{
				Name:           snakeName,
				ReturnType:     cFunc.ReturnType,
				RequiredParams: len(cFunc.Parameters),
				TotalParams:    len(cFunc.Parameters),
			}
			for _, param := range cFunc.Parameters {
				ahoyType := mapCTypeToAhoyType(param.Type)
				sig.Parameters = append(sig.Parameters, ParameterInfo{
					Name:       param.Name,
					Type:       ahoyType,
					HasDefault: false,
				})
			}
			funcSignatures[snakeName] = sig
		}
	}

	var checkCalls func(*ahoy.ASTNode)
	checkCalls = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		if node.Type == ahoy.NODE_CALL {
			funcName := node.Value

			if sig, exists := funcSignatures[funcName]; exists {
				// Check if we have type information for parameters
				hasTypeInfo := false
				for _, param := range sig.Parameters {
					if param.Type != "" {
						hasTypeInfo = true
						break
					}
				}

				if hasTypeInfo && len(node.Children) > 0 {
					// Build expected and actual type lists
					expectedTypes := []string{}
					actualTypes := []string{}

					// Get expected types from signature
					for i := 0; i < len(sig.Parameters) && i < len(node.Children); i++ {
						expectedTypes = append(expectedTypes, sig.Parameters[i].Type)
					}

					// Infer actual types from arguments
					for i, arg := range node.Children {
						actualType := inferExpressionType(arg, doc)
						actualTypes = append(actualTypes, actualType)
						
						// Additional validation for object literals passed to functions
						if arg.Type == ahoy.NODE_OBJECT_LITERAL && i < len(expectedTypes) {
							expectedType := expectedTypes[i]
							// Get the actual struct type from the object literal's Value field
							actualStructType := strings.ToLower(arg.Value)
							
							// Check if the object literal has the correct properties for struct types
							if expectedType != "object" && expectedType != "unknown" && expectedType != "" {
								// Check C structs
								if doc.CHeaderGlobal != nil {
									for cStructName, cStruct := range doc.CHeaderGlobal.Structs {
										if strings.ToLower(cStructName) == actualStructType {
											// Validate object properties against C struct fields
											objectProps := make(map[string]bool)
											expectedProps := []string{}
											
											for _, field := range cStruct.Fields {
												expectedProps = append(expectedProps, field.Name)
											}
											
											for _, prop := range arg.Children {
												if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
													objectProps[prop.Value] = true
												}
											}
											
											// Check if all expected properties are present
											actualProps := []string{}
											for prop := range objectProps {
												actualProps = append(actualProps, prop)
											}
											
											// Compare properties
											if len(expectedProps) > 0 && len(actualProps) > 0 {
												propsMatch := true
												if len(expectedProps) != len(actualProps) {
													propsMatch = false
												} else {
													expectedPropsMap := make(map[string]bool)
													for _, p := range expectedProps {
														expectedPropsMap[p] = true
													}
													for _, p := range actualProps {
														if !expectedPropsMap[p] {
															propsMatch = false
															break
														}
													}
												}
												
												if !propsMatch {
													expectedPropsStr := strings.Join(expectedProps, ",")
													actualPropsStr := strings.Join(actualProps, ",")
													message := "Object properties mismatch: expected " + expectedPropsStr + " got " + actualPropsStr
													
													lineText := ""
													if node.Line > 0 && node.Line <= len(doc.Lines) {
														lineText = doc.Lines[node.Line-1]
													}
													endChar := uint32(len(lineText))
													if endChar == 0 {
														endChar = uint32(len(funcName) + 10)
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
														Message:  message,
														Code:     "object-property-mismatch",
													}
													diagnostics = append(diagnostics, diagnostic)
												}
											}
											break
										}
									}
								}
							}
						}
					}

					// Check for type mismatches
					mismatch := false
					for i := 0; i < len(expectedTypes) && i < len(actualTypes); i++ {
						expected := expectedTypes[i]
						actual := actualTypes[i]

						// Skip if either type is unknown, generic, or infer
						if expected == "" || expected == "unknown" || actual == "unknown" ||
							expected == "generic" || actual == "generic" ||
							expected == "infer" || actual == "infer" {
							continue
						}
						
						// Allow int->float implicit conversion
						if expected == "float" && actual == "int" {
							continue
						}
						
						// Allow string to match char (C char* can accept string literals)
						if expected == "char" && actual == "string" {
							continue
						}

						// Check for mismatch
						if expected != actual {
							mismatch = true
							break
						}
					}

					if mismatch {
						// Build type list strings
						expectedStr := "["
						for i, t := range expectedTypes {
							if i > 0 {
								expectedStr += ","
							}
							if t == "" {
								expectedStr += "unknown"
							} else {
								expectedStr += t
							}
						}
						expectedStr += "]"

						actualStr := "["
						for i, t := range actualTypes {
							if i > 0 {
								actualStr += ","
							}
							actualStr += t
						}
						actualStr += "]"

						message := "Argument type mismatch: expected " + expectedStr + ", got " + actualStr

						lineText := ""
						if node.Line > 0 && node.Line <= len(doc.Lines) {
							lineText = doc.Lines[node.Line-1]
						}
						endChar := uint32(len(lineText))
						if endChar == 0 {
							endChar = uint32(len(funcName) + 10)
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
							Message:  message,
							Code:     "argument-type-mismatch",
						}
						diagnostics = append(diagnostics, diagnostic)
					}
				}
			}
		}

		for _, child := range node.Children {
			checkCalls(child)
		}
	}

	checkCalls(doc.AST)
	return diagnostics
}

type FunctionSignature struct {
	Name           string
	Parameters     []ParameterInfo
	RequiredParams int
	TotalParams    int
	ReturnType     string
}

type ParameterInfo struct {
	Name       string
	Type       string
	HasDefault bool
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}

	negative := n < 0
	if negative {
		n = -n
	}

	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	if negative {
		digits = append([]byte{'-'}, digits...)
	}

	return string(digits)
}

// checkTypeMismatches checks if variable/constant assignments match their declared types
func checkTypeMismatches(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	var checkNode func(*ahoy.ASTNode, int)
	checkNode = func(node *ahoy.ASTNode, depth int) {
		if node == nil {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Prevent excessive recursion
		if depth > 500 {
			return
		}

		// Check assignments (variables)
		if node.Type == ahoy.NODE_ASSIGNMENT && node.DataType != "" {
			// Has explicit type annotation
			expectedType := node.DataType

			if len(node.Children) > 0 {
				actualType := inferExpressionType(node.Children[0], doc)

				if actualType != "unknown" && actualType != expectedType && expectedType != "generic" {
					lineText := ""
					if node.Line > 0 && node.Line <= len(doc.Lines) {
						lineText = doc.Lines[node.Line-1]
					}
					endChar := uint32(len(lineText))
					if endChar == 0 {
						endChar = 30
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
						Message:  "expected " + expectedType + " got " + actualType,
						Code:     "type-mismatch",
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}
		}

		// Check constant declarations
		if node.Type == ahoy.NODE_CONSTANT_DECLARATION && node.DataType != "" {
			// Has explicit type annotation
			expectedType := node.DataType

			if len(node.Children) > 0 {
				actualType := inferExpressionType(node.Children[0], doc)

				if actualType != "unknown" && actualType != expectedType && expectedType != "generic" {
					lineText := ""
					if node.Line > 0 && node.Line <= len(doc.Lines) {
						lineText = doc.Lines[node.Line-1]
					}
					endChar := uint32(len(lineText))
					if endChar == 0 {
						endChar = 30
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
						Message:  "expected " + expectedType + " got " + actualType,
						Code:     "type-mismatch",
					}
					diagnostics = append(diagnostics, diagnostic)
				}
			}
		}

		// Recursively check children
		for _, child := range node.Children {
			checkNode(child, depth+1)
		}
	}

	checkNode(doc.AST, 0)
	return diagnostics
}

// inferExpressionType infers the type of an expression
func inferExpressionType(node *ahoy.ASTNode, doc *Document) string {
	if node == nil {
		return "unknown"
	}

	switch node.Type {
	case ahoy.NODE_NUMBER:
		// Check if it contains a decimal point
		for i := 0; i < len(node.Value); i++ {
			if node.Value[i] == '.' {
				return "float"
			}
		}
		return "int"

	case ahoy.NODE_STRING, ahoy.NODE_F_STRING:
		return "string"

	case ahoy.NODE_CHAR:
		return "char"

	case ahoy.NODE_BOOLEAN:
		return "bool"

	case ahoy.NODE_ARRAY_LITERAL:
		return "array"

	case ahoy.NODE_DICT_LITERAL:
		return "dict"

	case ahoy.NODE_IDENTIFIER:
		// Look up the identifier in the symbol table
		if doc != nil && doc.SymbolTable != nil && doc.SymbolTable.GlobalScope != nil {
			if sym := doc.SymbolTable.GlobalScope.Lookup(node.Value); sym != nil {
				return sym.Type
			}
		}
		
		// Check if it's a C #define constant
		if doc != nil && doc.CHeaderGlobal != nil {
			if define, exists := doc.CHeaderGlobal.Defines[node.Value]; exists {
				// Try to extract type from the define value
				// e.g., "CLITERAL(Color){ ... }" -> "color"
				return extractTypeFromDefine(define.Value)
			}
		}
		
		// Check namespaced C headers
		if doc != nil {
			for _, cHeader := range doc.CHeaders {
				if define, exists := cHeader.Defines[node.Value]; exists {
					return extractTypeFromDefine(define.Value)
				}
			}
		}
		
		return "unknown"

	case ahoy.NODE_CALL:
		// Look up function return type and infer from arguments if needed
		funcName := node.Value
		
		// Build function signatures map if not already done
		funcSignatures := make(map[string]*FunctionSignature)
		if doc != nil && doc.AST != nil {
			var collectFunctions func(*ahoy.ASTNode)
			collectFunctions = func(n *ahoy.ASTNode) {
				if n == nil {
					return
				}
				if n.Type == ahoy.NODE_FUNCTION {
					sig := &FunctionSignature{
						Name:       n.Value,
						ReturnType: n.DataType,
					}
					if len(n.Children) > 0 && n.Children[0].Type == ahoy.NODE_BLOCK {
						params := n.Children[0]
						for _, param := range params.Children {
							if param.Type == ahoy.NODE_IDENTIFIER {
								paramInfo := ParameterInfo{
									Name:       param.Value,
									Type:       param.DataType,
									HasDefault: param.DefaultValue != nil,
								}
								sig.Parameters = append(sig.Parameters, paramInfo)
							}
						}
					}
					funcSignatures[n.Value] = sig
				}
				for _, child := range n.Children {
					collectFunctions(child)
				}
			}
			collectFunctions(doc.AST)
		}
		
		// Look up the function
		if sig, exists := funcSignatures[funcName]; exists {
			returnType := sig.ReturnType
			
			// If return type is "infer" or empty (generic), infer from arguments
			if returnType == "infer" || returnType == "" || returnType == "generic" {
				// For infer/generic returns, we need to trace through the function body
				// to determine what's actually returned based on the argument types
				// For now, we'll look at the arguments to infer parameter types
				if len(node.Children) > 0 {
					// Infer parameter types from actual arguments
					inferredParamTypes := make(map[string]string)
					for i, arg := range node.Children {
						if i < len(sig.Parameters) {
							paramName := sig.Parameters[i].Name
							paramType := sig.Parameters[i].Type
							
							// If parameter type is empty/generic, infer from argument
							if paramType == "" || paramType == "generic" {
								argType := inferExpressionType(arg, doc)
								inferredParamTypes[paramName] = argType
							}
						}
					}
					
					// Now we'd need to trace through the function body with inferred types
					// This is complex - for now return "infer" to indicate it needs inference
					return "infer"
				}
			}
			
			return returnType
		}
		
		return "unknown"

	case ahoy.NODE_BINARY_OP:
		// Infer based on operands
		if len(node.Children) >= 2 {
			leftType := inferExpressionType(node.Children[0], doc)
			rightType := inferExpressionType(node.Children[1], doc)

			// Arithmetic operations
			if node.Value == "+" || node.Value == "-" || node.Value == "*" || node.Value == "/" || node.Value == "%" {
				if leftType == "float" || rightType == "float" {
					return "float"
				}
				if leftType == "int" || rightType == "int" {
					return "int"
				}
			}

			// Comparison operations return bool
			if node.Value == "<" || node.Value == ">" || node.Value == "<=" || node.Value == ">=" ||
				node.Value == "is" || node.Value == "not" {
				return "bool"
			}
		}
		return "unknown"

	case ahoy.NODE_ARRAY_ACCESS:
		// Return type depends on array element type - unknown for now
		return "unknown"

	case ahoy.NODE_OBJECT_LITERAL:
		// Check if it has a struct type name in Value (e.g., rectangle, vector2)
		if node.Value != "" {
			return strings.ToLower(node.Value)
		}
		// Fallback to DataType if Value is not set
		if node.DataType != "" {
			return node.DataType
		}
		return "object"

	default:
		return "unknown"
	}
}

// mapCTypeToAhoyType maps C types to Ahoy types
func mapCTypeToAhoyType(cType string) string {
	// Trim spaces and pointer/const qualifiers
	cType = strings.TrimSpace(cType)
	cType = strings.TrimPrefix(cType, "const ")
	cType = strings.TrimSpace(cType)
	
	// Handle pointers to types (like char* for string)
	if strings.Contains(cType, "char") && strings.Contains(cType, "*") {
		return "string"
	}
	
	// Remove pointer notation for other types
	cType = strings.ReplaceAll(cType, "*", "")
	cType = strings.TrimSpace(cType)
	
	switch cType {
	case "int", "short", "long", "unsigned int", "unsigned short", "unsigned long",
		"int8_t", "int16_t", "int32_t", "int64_t",
		"uint8_t", "uint16_t", "uint32_t", "uint64_t",
		"size_t":
		return "int"
	case "float", "double":
		return "float"
	case "char":
		return "char"
	case "bool", "_Bool":
		return "bool"
	case "void":
		return "void"
	default:
		// Check if it's a struct type (starts with uppercase or is a known type)
		if len(cType) > 0 && cType[0] >= 'A' && cType[0] <= 'Z' {
			// It's likely a struct type, return as lowercase for matching
			return strings.ToLower(cType)
		}
		return cType
	}
}

// extractTypeFromDefine extracts type information from a C #define value
// e.g., "CLITERAL(Color){ 80, 80, 80, 255 }" -> "color"
func extractTypeFromDefine(defineValue string) string {
	// Look for CLITERAL(Type) pattern
	if strings.Contains(defineValue, "CLITERAL(") {
		start := strings.Index(defineValue, "CLITERAL(") + len("CLITERAL(")
		end := strings.Index(defineValue[start:], ")")
		if end > 0 {
			typeName := strings.TrimSpace(defineValue[start : start+end])
			return strings.ToLower(typeName)
		}
	}
	
	// Look for (Type){ ... } pattern
	if strings.Contains(defineValue, ")(") || strings.Contains(defineValue, "){") {
		// Try to extract type from cast pattern like (Color){ ... }
		start := strings.Index(defineValue, "(")
		if start >= 0 {
			end := strings.Index(defineValue[start+1:], ")")
			if end > 0 {
				typeName := strings.TrimSpace(defineValue[start+1 : start+1+end])
				// Check if it looks like a type name (starts with uppercase)
				if len(typeName) > 0 && typeName[0] >= 'A' && typeName[0] <= 'Z' {
					return strings.ToLower(typeName)
				}
			}
		}
	}
	
	return "unknown"
}
