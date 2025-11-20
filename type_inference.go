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

// inferParameterTypes analyzes function bodies and call sites to infer parameter types
func inferParameterTypes(ast *ahoy.ASTNode, doc *Document) map[string][]string {
result := make(map[string][]string)

if ast == nil {
return result
}

// First pass: collect functions and initialize with generic types
functions := make(map[string]*ahoy.ASTNode)
var collectFuncs func(*ahoy.ASTNode)
collectFuncs = func(node *ahoy.ASTNode) {
if node == nil {
return
}
if node.Type == ahoy.NODE_FUNCTION {
funcName := node.Value
functions[funcName] = node

// Initialize with generic types
if len(node.Children) > 0 {
params := node.Children[0]
paramTypes := make([]string, len(params.Children))
for i := range paramTypes {
paramTypes[i] = "generic"
}
result[funcName] = paramTypes
}
}
for _, child := range node.Children {
collectFuncs(child)
}
}
collectFuncs(ast)

// Second pass: analyze usage within function bodies
for funcName, funcNode := range functions {
if len(funcNode.Children) < 2 {
continue
}
params := funcNode.Children[0]
body := funcNode.Children[1]

paramNames := make(map[string]int)
for i, param := range params.Children {
paramNames[param.Value] = i
}

analyzeParameterUsageInBody(body, funcName, paramNames, result)
}

// Third pass: analyze function calls
var analyzeCalls func(*ahoy.ASTNode)
analyzeCalls = func(node *ahoy.ASTNode) {
if node == nil {
return
}

if node.Type == ahoy.NODE_CALL {
funcName := node.Value
if paramTypes, exists := result[funcName]; exists {
// Analyze arguments
for i, arg := range node.Children {
if i < len(paramTypes) && paramTypes[i] == "generic" {
argType := inferExpressionType(arg, doc)
if argType != "generic" && argType != "unknown" {
paramTypes[i] = argType
}
}
}
}
}

for _, child := range node.Children {
analyzeCalls(child)
}
}
analyzeCalls(ast)

return result
}

// analyzeParameterUsageInBody walks the function body and infers types from usage
func analyzeParameterUsageInBody(node *ahoy.ASTNode, funcName string, paramNames map[string]int, result map[string][]string) {
if node == nil {
return
}

inferredTypes := result[funcName]

// Check if parameter is used in boolean context (if condition)
if node.Type == ahoy.NODE_IF_STATEMENT || node.Type == ahoy.NODE_WHILE_LOOP {
if len(node.Children) > 0 {
condition := node.Children[0]
if condition.Type == ahoy.NODE_IDENTIFIER {
if paramIdx, exists := paramNames[condition.Value]; exists {
if inferredTypes[paramIdx] == "generic" {
inferredTypes[paramIdx] = "bool"
}
}
}
}
}

// Check for method calls (e.g., param.length||)
if node.Type == ahoy.NODE_METHOD_CALL && len(node.Children) > 0 {
if node.Children[0].Type == ahoy.NODE_IDENTIFIER {
paramName := node.Children[0].Value
if paramIdx, exists := paramNames[paramName]; exists {
method := node.Value

// Array methods
if method == "length" || method == "push" || method == "pop" {
if inferredTypes[paramIdx] == "generic" {
inferredTypes[paramIdx] = "array"
}
} else if method == "has" || method == "size" || method == "keys" || method == "values" {
// Dict methods
if inferredTypes[paramIdx] == "generic" {
inferredTypes[paramIdx] = "dict"
}
}
}
}
}

// Check for array access
if node.Type == ahoy.NODE_ARRAY_ACCESS {
if paramIdx, exists := paramNames[node.Value]; exists {
if inferredTypes[paramIdx] == "generic" {
inferredTypes[paramIdx] = "array"
}
}
}

// Check for dict/object access
if node.Type == ahoy.NODE_DICT_ACCESS || node.Type == ahoy.NODE_OBJECT_ACCESS {
if paramIdx, exists := paramNames[node.Value]; exists {
if inferredTypes[paramIdx] == "generic" {
inferredTypes[paramIdx] = "dict"
}
}
}

// Check for dict iteration
if node.Type == ahoy.NODE_FOR_IN_DICT_LOOP && len(node.Children) > 2 {
if node.Children[2].Type == ahoy.NODE_IDENTIFIER {
paramName := node.Children[2].Value
if paramIdx, exists := paramNames[paramName]; exists {
if inferredTypes[paramIdx] == "generic" {
inferredTypes[paramIdx] = "dict"
}
}
}
}

// Check for array iteration
if node.Type == ahoy.NODE_FOR_IN_ARRAY_LOOP && len(node.Children) > 1 {
if node.Children[1].Type == ahoy.NODE_IDENTIFIER {
paramName := node.Children[1].Value
if paramIdx, exists := paramNames[paramName]; exists {
if inferredTypes[paramIdx] == "generic" {
inferredTypes[paramIdx] = "array"
}
}
}
}

// Recurse
for _, child := range node.Children {
analyzeParameterUsageInBody(child, funcName, paramNames, result)
}
}

// Helper to check if a node is used in a boolean context
func isUsedInBooleanContext(node *ahoy.ASTNode, targetIdentifier string) bool {
if node == nil {
return false
}

// Check if used in if/while/loop conditions
if node.Type == ahoy.NODE_IF_STATEMENT || node.Type == ahoy.NODE_WHILE_LOOP {
if len(node.Children) > 0 && containsIdentifier(node.Children[0], targetIdentifier) {
return true
}
}

// Check if used in boolean operations
if node.Type == ahoy.NODE_BINARY_OP {
if node.Value == "and" || node.Value == "or" || node.Value == "not" {
return containsIdentifier(node, targetIdentifier)
}
}

// Recurse
for _, child := range node.Children {
if isUsedInBooleanContext(child, targetIdentifier) {
return true
}
}

return false
}

func containsIdentifier(node *ahoy.ASTNode, name string) bool {
if node == nil {
return false
}

if node.Type == ahoy.NODE_IDENTIFIER && node.Value == name {
return true
}

for _, child := range node.Children {
if containsIdentifier(child, name) {
return true
}
}

return false
}
