package main

import (
	"strings"

	"ahoy"
)

// resolveAllInferReturnTypes walks the AST and resolves return types for all functions with 'infer' keyword
// This must be called BEFORE building the symbol table so that function symbols have correct types
func resolveAllInferReturnTypes(ast *ahoy.ASTNode, doc *Document) map[string][]string {
	resolvedTypes := make(map[string][]string)

	if ast == nil {
		return resolvedTypes
	}

	// First, collect all functions and their explicit return types
	functionNodes := make(map[string]*ahoy.ASTNode)
	var collectFunctions func(*ahoy.ASTNode)
	collectFunctions = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		if node.Type == ahoy.NODE_FUNCTION {
			funcName := node.Value
			functionNodes[funcName] = node

			// If function has explicit return types (not infer/any), store them
			if node.DataType != "" && node.DataType != "infer" && node.DataType != "generic" && node.DataType != "any" {
				// Split multiple return types (e.g., "dict<string,int>,dict<string,int>")
				returnTypes := splitReturnTypesHelper(node.DataType)
				resolvedTypes[funcName] = returnTypes
			}
		}

		for _, child := range node.Children {
			collectFunctions(child)
		}
	}
	collectFunctions(ast)

	// Now resolve functions with 'infer' return types
	// We may need multiple passes if functions depend on each other
	maxPasses := 10
	changed := true
	for pass := 0; pass < maxPasses && changed; pass++ {
		changed = false

		for funcName, funcNode := range functionNodes {
			// Skip if already resolved
			if _, resolved := resolvedTypes[funcName]; resolved {
				continue
			}

			// Only process functions with 'infer' return type
			if funcNode.DataType != "infer" && funcNode.DataType != "" {
				continue
			}

			// Try to infer the return types
			inferredTypes := inferFunctionReturnTypesWithContext(funcNode, []string{}, doc, resolvedTypes)
			if len(inferredTypes) > 0 {
				// Check if all inferred types are concrete (not "any" or "unknown")
				allConcrete := true
				for _, t := range inferredTypes {
					if t == "any" || t == "unknown" || t == "" {
						allConcrete = false
						break
					}
				}

				if allConcrete {
					resolvedTypes[funcName] = inferredTypes
					changed = true
				}
			}
		}
	}

	return resolvedTypes
}

// inferFunctionReturnTypesWithContext is like inferFunctionReturnTypes but uses a context map of already-resolved types
func inferFunctionReturnTypesWithContext(funcNode *ahoy.ASTNode, argTypes []string, doc *Document, resolvedTypes map[string][]string) []string {
	if funcNode == nil || funcNode.Type != ahoy.NODE_FUNCTION {
		return []string{}
	}

	// Get parameters and body
	if len(funcNode.Children) < 2 {
		return []string{}
	}

	params := funcNode.Children[0]
	body := funcNode.Children[1]

	// Build parameter type map
	paramTypes := make(map[string]string)
	if params != nil && params.Type == ahoy.NODE_BLOCK {
		for i, param := range params.Children {
			if param.Type == ahoy.NODE_IDENTIFIER {
				paramName := param.Value
				paramType := param.DataType

				// If parameter has no explicit type, use inferred type from arguments
				if (paramType == "" || paramType == "generic" || paramType == "any" || paramType == "infer") && i < len(argTypes) {
					paramType = argTypes[i]
				}

				if paramType != "" {
					paramTypes[paramName] = paramType
				}
			}
		}
	}

	// Find return statement and infer types
	returnStmt := findReturnStatementInNode(body)
	if returnStmt == nil {
		return []string{}
	}

	// Infer types from each returned expression
	types := []string{}
	for _, child := range returnStmt.Children {
		inferredType := inferExpressionTypeWithContext(child, paramTypes, doc, resolvedTypes)
		types = append(types, inferredType)
	}

	return types
}

// inferExpressionTypeWithContext infers expression type using a context map of resolved function return types
func inferExpressionTypeWithContext(node *ahoy.ASTNode, paramTypes map[string]string, doc *Document, resolvedTypes map[string][]string) string {
	if node == nil {
		return "unknown"
	}

	switch node.Type {
	case ahoy.NODE_NUMBER:
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
		// Check if it's a parameter with inferred type
		if paramType, exists := paramTypes[node.Value]; exists && paramType != "" {
			return paramType
		}

		// Could also check variables declared in the function, but for now return unknown
		return "unknown"

	case ahoy.NODE_CALL:
		// Look up function return type from resolved types
		funcName := node.Value
		if returnTypes, exists := resolvedTypes[funcName]; exists && len(returnTypes) > 0 {
			return returnTypes[0] // Return first type for single-value context
		}
		return "unknown"

	default:
		return "unknown"
	}
}

// findReturnStatementInNode finds the first return statement in a node tree
func findReturnStatementInNode(node *ahoy.ASTNode) *ahoy.ASTNode {
	if node == nil {
		return nil
	}

	if node.Type == ahoy.NODE_RETURN_STATEMENT {
		return node
	}

	for _, child := range node.Children {
		if result := findReturnStatementInNode(child); result != nil {
			return result
		}
	}

	return nil
}

// splitReturnTypesHelper splits a comma-separated list of return types, handling nested <> and []
func splitReturnTypesHelper(typeStr string) []string {
	if typeStr == "" {
		return []string{}
	}

	var types []string
	var current strings.Builder
	depth := 0

	for i := 0; i < len(typeStr); i++ {
		ch := typeStr[i]
		switch ch {
		case '<', '[':
			depth++
			current.WriteByte(ch)
		case '>', ']':
			depth--
			current.WriteByte(ch)
		case ',':
			if depth == 0 {
				// Top-level comma, split here
				types = append(types, strings.TrimSpace(current.String()))
				current.Reset()
			} else {
				// Nested comma, keep it
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}

	// Add the last type
	if current.Len() > 0 {
		types = append(types, strings.TrimSpace(current.String()))
	}

	return types
}
