package main

import (
	"context"
	"strings"

	"ahoy"

	"go.lsp.dev/protocol"
)

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

			// Check return type violations
			returnDiags := checkReturnTypeViolations(doc)
			diagnostics = append(diagnostics, returnDiags...)

			// Check enum duplicates
			enumDiags := checkEnumDuplicates(doc)
			diagnostics = append(diagnostics, enumDiags...)

			// Check undefined function calls
			undefinedFuncDiags := checkUndefinedFunctions(doc)
			diagnostics = append(diagnostics, undefinedFuncDiags...)

			// Check function call argument counts
			argCountDiags := checkFunctionCallArgumentCounts(doc)
			diagnostics = append(diagnostics, argCountDiags...)

			// Check function call argument types
			argTypeDiags := checkFunctionCallArgumentTypes(doc)
			diagnostics = append(diagnostics, argTypeDiags...)

			// Check variable/constant type mismatches
			typeMismatchDiags := checkTypeMismatches(doc)
			diagnostics = append(diagnostics, typeMismatchDiags...)
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

	if doc.AST == nil || doc.SymbolTable == nil {
		return diagnostics
	}

	// Walk the AST looking for assignments
	var checkNode func(*ahoy.ASTNode)
	checkNode = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		switch node.Type {
		case ahoy.NODE_ASSIGNMENT:
			// Check if assigning to a constant
			varName := node.Value
			if varName != "" {
				// Look up the symbol
				sym := doc.SymbolTable.GlobalScope.Lookup(varName)
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
			checkNode(child)
		}
	}

	checkNode(doc.AST)
	return diagnostics
}

// checkReturnTypeViolations checks for return type mismatches
func checkReturnTypeViolations(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	// Walk the AST looking for functions
	var checkNode func(*ahoy.ASTNode)
	checkNode = func(node *ahoy.ASTNode) {
		if node == nil {
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
			checkNode(child)
		}
	}

	checkNode(doc.AST)
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
	var checkNode func(*ahoy.ASTNode)
	checkNode = func(node *ahoy.ASTNode) {
		if node == nil {
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
			checkNode(child)
		}
	}

	checkNode(doc.AST)
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

	if doc.AST == nil || doc.SymbolTable == nil {
		return diagnostics
	}

	// Collect all available function names (built-ins + user-defined)
	availableFuncs := make([]string, 0)
	availableFuncs = append(availableFuncs, builtinFunctions...)

	// Add user-defined functions from symbol table
	for _, sym := range doc.SymbolTable.GlobalScope.Symbols {
		if sym.Kind == SymbolKindFunction {
			availableFuncs = append(availableFuncs, sym.Name)
		}
	}

	// Walk the AST looking for function calls
	var checkNode func(*ahoy.ASTNode)
	checkNode = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		if node.Type == ahoy.NODE_CALL {
			funcName := node.Value

			// Check if function exists (built-in or user-defined)
			if !isBuiltinFunction(funcName) {
				sym := doc.SymbolTable.GlobalScope.Lookup(funcName)
				if sym == nil || sym.Kind != SymbolKindFunction {
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
			checkNode(child)
		}
	}

	checkNode(doc.AST)
	return diagnostics
}

// checkFunctionCallArgumentCounts checks if function calls have the correct number of arguments
func checkFunctionCallArgumentCounts(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil || doc.SymbolTable == nil {
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

				message := ""
				if argCount < expectedMin {
					if expectedMin == expectedMax {
						if expectedMin == 0 {
							message = "expected no arguments, got " + intToString(argCount)
						} else if expectedMin == 1 {
							message = "expected 1 argument, got none"
						} else {
							message = "expected " + intToString(expectedMin) + " arguments, got " + intToString(argCount)
						}
					} else {
						message = "expected " + intToString(expectedMin) + "-" + intToString(expectedMax) +
							" arguments, got " + intToString(argCount)
					}
				} else if argCount > expectedMax {
					if expectedMin == expectedMax {
						if expectedMax == 1 {
							message = "expected 1 argument, got " + intToString(argCount)
						} else {
							message = "expected " + intToString(expectedMax) + " arguments, got " + intToString(argCount)
						}
					} else {
						message = "expected " + intToString(expectedMin) + "-" + intToString(expectedMax) +
							" arguments, got " + intToString(argCount)
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

	if doc.AST == nil || doc.SymbolTable == nil {
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
					for _, arg := range node.Children {
						actualType := inferExpressionType(arg)
						actualTypes = append(actualTypes, actualType)
					}

					// Check for type mismatches
					mismatch := false
					for i := 0; i < len(expectedTypes) && i < len(actualTypes); i++ {
						expected := expectedTypes[i]
						actual := actualTypes[i]

						// Skip if either type is unknown
						if expected == "" || expected == "unknown" || actual == "unknown" {
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
								expectedStr += ", "
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
								actualStr += ", "
							}
							actualStr += t
						}
						actualStr += "]"

						message := "expected function arguments " + expectedStr + " got " + actualStr

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

	var checkNode func(*ahoy.ASTNode)
	checkNode = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		// Check assignments (variables)
		if node.Type == ahoy.NODE_ASSIGNMENT && node.DataType != "" {
			// Has explicit type annotation
			expectedType := node.DataType

			if len(node.Children) > 0 {
				actualType := inferExpressionType(node.Children[0])

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
				actualType := inferExpressionType(node.Children[0])

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
			checkNode(child)
		}
	}

	checkNode(doc.AST)
	return diagnostics
}

// inferExpressionType infers the type of an expression
func inferExpressionType(node *ahoy.ASTNode) string {
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
		// Could look up in symbol table, for now return unknown
		return "unknown"

	case ahoy.NODE_CALL:
		// Could look up function return type, for now return unknown
		return "unknown"

	case ahoy.NODE_BINARY_OP:
		// Infer based on operands
		if len(node.Children) >= 2 {
			leftType := inferExpressionType(node.Children[0])
			rightType := inferExpressionType(node.Children[1])

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

	default:
		return "unknown"
	}
}
