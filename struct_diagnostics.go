package main

import (
	"strings"

	"ahoy"

	"go.lsp.dev/protocol"
)

// checkStructMemberAccess validates accessing struct/object properties
func checkStructMemberAccess(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil || doc.SymbolTable == nil {
		return diagnostics
	}

	var checkNode func(*ahoy.ASTNode)
	checkNode = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		// Check member access: object.property
		if node.Type == ahoy.NODE_MEMBER_ACCESS && len(node.Children) >= 2 {
			// First child is the object, second is the property name
			objNode := node.Children[0]
			propNode := node.Children[1]

			// Get the type of the object being accessed
			var objType string
			if objNode.Type == ahoy.NODE_IDENTIFIER {
				sym := doc.SymbolTable.GlobalScope.Lookup(objNode.Value)
				if sym != nil {
					objType = sym.Type
				}
			}

			// Check if it's a struct type
			if strings.HasPrefix(objType, "struct:") {
				structTypeName := strings.TrimPrefix(objType, "struct:")
				structSym := doc.SymbolTable.GlobalScope.Lookup(structTypeName)

				if structSym != nil && structSym.Kind == SymbolKindStruct {
					propName := propNode.Value

					// Check if property exists in struct
					if _, exists := structSym.Fields[propName]; !exists {
						lineText := ""
						if node.Line > 0 && node.Line <= len(doc.Lines) {
							lineText = doc.Lines[node.Line-1]
						}
						endChar := uint32(len(lineText))
						if endChar == 0 {
							endChar = uint32(len(objNode.Value) + len(propName) + 10)
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
							Message:  "accessing property '" + propName + "' which does not exist on type '" + structTypeName + "'",
							Code:     "undefined-property",
						}
						diagnostics = append(diagnostics, diagnostic)
					}
				}
			} else if objType == "object" {
				// For object literals, we track what properties exist but don't error on access
				// Only error when trying to add new properties via assignment (handled below)
			}
		}

		// Check assignment to struct properties
		if node.Type == ahoy.NODE_ASSIGNMENT && len(node.Children) > 0 {
			// Check if the variable being assigned is a member access
			if node.Value != "" && strings.Contains(node.Value, ".") {
				// This is a property assignment like: obj.property: value
				parts := strings.SplitN(node.Value, ".", 2)
				if len(parts) == 2 {
					objName := parts[0]
					propName := parts[1]

					sym := doc.SymbolTable.GlobalScope.Lookup(objName)
					if sym != nil {
						objType := sym.Type

						// Check struct types
						if strings.HasPrefix(objType, "struct:") {
							structTypeName := strings.TrimPrefix(objType, "struct:")
							structSym := doc.SymbolTable.GlobalScope.Lookup(structTypeName)

							if structSym != nil && structSym.Kind == SymbolKindStruct {
								// Check if property exists
								field, exists := structSym.Fields[propName]
								if !exists {
									lineText := ""
									if node.Line > 0 && node.Line <= len(doc.Lines) {
										lineText = doc.Lines[node.Line-1]
									}
									endChar := uint32(len(lineText))
									if endChar == 0 {
										endChar = 50
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
										Message:  "type '" + structTypeName + "' does not have property '" + propName + "'",
										Code:     "undefined-property",
									}
									diagnostics = append(diagnostics, diagnostic)
								} else {
									// Check type compatibility
									if len(node.Children) > 0 {
										actualType := inferExpressionType(node.Children[0])
										expectedType := field.Type

										// Allow int->float casting
										if actualType != "unknown" && actualType != expectedType {
											if !(expectedType == "float" && actualType == "int") {
												lineText := ""
												if node.Line > 0 && node.Line <= len(doc.Lines) {
													lineText = doc.Lines[node.Line-1]
												}
												endChar := uint32(len(lineText))
												if endChar == 0 {
													endChar = 50
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
													Message:  propName + " is of type " + expectedType + " and cannot be assigned a " + actualType + " value",
													Code:     "type-mismatch",
												}
												diagnostics = append(diagnostics, diagnostic)
											}
										}
									}
								}
							}
						} else if objType == "object" {
							// Object literals can't have new properties added
							lineText := ""
							if node.Line > 0 && node.Line <= len(doc.Lines) {
								lineText = doc.Lines[node.Line-1]
							}
							endChar := uint32(len(lineText))
							if endChar == 0 {
								endChar = 50
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
								Message:  "type object literal cannot have new properties added to it at runtime",
								Code:     "object-literal-immutable",
							}
							diagnostics = append(diagnostics, diagnostic)
						}
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

// checkObjectPropertyAssignment validates object literal instantiation
func checkObjectPropertyAssignment(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil || doc.SymbolTable == nil {
		return diagnostics
	}

	var checkNode func(*ahoy.ASTNode)
	checkNode = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		// Check variable declarations with object literals
		if node.Type == ahoy.NODE_VARIABLE_DECLARATION && node.DataType != "" {
			varType := node.DataType

			// Check if it's a struct type
			if strings.HasPrefix(varType, "struct:") {
				structTypeName := strings.TrimPrefix(varType, "struct:")
				structSym := doc.SymbolTable.GlobalScope.Lookup(structTypeName)

				if structSym != nil && structSym.Kind == SymbolKindStruct {
					// Check if initialized with object literal
					if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_OBJECT_LITERAL {
						objLiteral := node.Children[0]

						// Validate each property in the object literal
						for _, prop := range objLiteral.Children {
							if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
								propName := prop.Value

								// Check if property exists in struct
								field, exists := structSym.Fields[propName]
								if !exists {
									lineText := ""
									if node.Line > 0 && node.Line <= len(doc.Lines) {
										lineText = doc.Lines[node.Line-1]
									}
									endChar := uint32(len(lineText))
									if endChar == 0 {
										endChar = 50
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
										Message:  propName + " does not exist in " + structTypeName + " type",
										Code:     "undefined-property",
									}
									diagnostics = append(diagnostics, diagnostic)
								} else {
									// Check type compatibility
									if len(prop.Children) > 0 {
										actualType := inferExpressionType(prop.Children[0])
										expectedType := field.Type

										// Allow int->float casting
										if actualType != "unknown" && actualType != expectedType {
											if !(expectedType == "float" && actualType == "int") {
												lineText := ""
												if node.Line > 0 && node.Line <= len(doc.Lines) {
													lineText = doc.Lines[node.Line-1]
												}
												endChar := uint32(len(lineText))
												if endChar == 0 {
													endChar = 50
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
													Message:  "expected " + expectedType + " for property '" + propName + "', got " + actualType,
													Code:     "type-mismatch",
												}
												diagnostics = append(diagnostics, diagnostic)
											}
										}
									}
								}
							}
						}
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
