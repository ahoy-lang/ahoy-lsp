package main

import (
	"ahoy"
)

// inferFunctionReturnTypes infers actual return types for functions with 'infer' return type
// by analyzing what's actually returned and the types of parameters passed
func inferFunctionReturnTypes(funcNode *ahoy.ASTNode, argTypes []string, doc *Document) []string {
	if funcNode == nil || funcNode.Type != ahoy.NODE_FUNCTION {
		return []string{}
	}

	// Find the function body (skip parameter block)
	var body *ahoy.ASTNode
	for _, child := range funcNode.Children {
		if child.Type == ahoy.NODE_BLOCK && len(child.Children) > 0 {
			// Parameter block has identifiers as first children
			// Body block has statements
			if child.Children[0].Type != ahoy.NODE_IDENTIFIER {
				body = child
				break
			}
		}
	}

	if body == nil {
		return []string{}
	}

	// Build parameter type map from function definition and actual arguments
	paramTypes := make(map[string]string)
	if len(funcNode.Children) > 0 && funcNode.Children[0].Type == ahoy.NODE_BLOCK {
		params := funcNode.Children[0]
		for i, param := range params.Children {
			if param.Type == ahoy.NODE_IDENTIFIER {
				paramName := param.Value
				paramType := param.DataType
				
				// If parameter has no explicit type or is generic, use inferred type from arguments
				if (paramType == "" || paramType == "generic" || paramType == "infer") && i < len(argTypes) {
					paramType = argTypes[i]
				}
				
				if paramType != "" {
					paramTypes[paramName] = paramType
				}
			}
		}
	}

	// Find return statement and infer types of returned expressions
	var returnTypes []string
	var findReturn func(*ahoy.ASTNode) bool
	findReturn = func(node *ahoy.ASTNode) bool {
		if node == nil {
			return false
		}

		if node.Type == ahoy.NODE_RETURN_STATEMENT {
			// Analyze what's being returned
			for _, child := range node.Children {
				inferredType := inferExpressionTypeWithParams(child, paramTypes, doc)
				returnTypes = append(returnTypes, inferredType)
			}
			return true
		}

		// Recursively search in children
		for _, child := range node.Children {
			if findReturn(child) {
				return true
			}
		}
		return false
	}

	findReturn(body)
	return returnTypes
}

// inferExpressionTypeWithParams infers expression type considering parameter types
func inferExpressionTypeWithParams(node *ahoy.ASTNode, paramTypes map[string]string, doc *Document) string {
	if node == nil {
		return "unknown"
	}

	switch node.Type {
	case ahoy.NODE_NUMBER:
		// Check if it's a float (contains decimal point)
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
		// First check if it's a parameter with inferred type
		if paramType, exists := paramTypes[node.Value]; exists && paramType != "" {
			return paramType
		}
		
		// Then look up in symbol table if available
		if doc != nil && doc.SymbolTable != nil {
			if sym := doc.SymbolTable.Lookup(node.Value); sym != nil {
				if sym.Type != "" && sym.Type != "infer" && sym.Type != "generic" {
					return sym.Type
				}
			}
		}
		
		return "unknown"

	case ahoy.NODE_BINARY_OP:
		// For arithmetic operations, infer based on operands
		if len(node.Children) >= 2 {
			leftType := inferExpressionTypeWithParams(node.Children[0], paramTypes, doc)
			rightType := inferExpressionTypeWithParams(node.Children[1], paramTypes, doc)
			
			if node.Value == "+" || node.Value == "-" || node.Value == "*" || node.Value == "/" {
				// If either is float, result is float
				if leftType == "float" || rightType == "float" {
					return "float"
				}
				// Both int, result is int
				if leftType == "int" && rightType == "int" {
					return "int"
				}
			}
		}
		return "unknown"

	case ahoy.NODE_OBJECT_LITERAL:
		// Check if it has a type name
		if node.Value != "" {
			return node.Value
		}
		if node.DataType != "" {
			return node.DataType
		}
		return "object"

	default:
		// Fall back to regular inference
		return inferExpressionType(node, doc)
	}
}
