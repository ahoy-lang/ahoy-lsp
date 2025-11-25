package main

import (
	"fmt"
	"strings"

	"ahoy"

	"go.lsp.dev/protocol"
)



func checkTypeTypos(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	// Collect valid type names
	validTypes := make(map[string]bool)
	
	// Built-in types
	builtinTypes := []string{
		"int", "float", "bool", "char", "string", "void",
		"array", "dict", "HashMap", "AhoyArray",
		"int64", "float64", "uint8", "uint16", "uint32", "uint64",
		"infer", "generic", // Special types for type inference
		"object", // Anonymous object type
	}
	for _, t := range builtinTypes {
		validTypes[t] = true
	}

	// Collect struct types, enums, unions, and aliases (including nested types)
	var collectTypes func(*ahoy.ASTNode, string)
	collectTypes = func(node *ahoy.ASTNode, parentStruct string) {
		if node == nil {
			return
		}
		if node.Type == ahoy.NODE_STRUCT_DECLARATION {
			structName := node.Value
			validTypes[structName] = true
			// Also add capitalized version
			validTypes[capitalizeFirst(structName)] = true
			
			// Check for nested type declarations within struct
			for _, child := range node.Children {
				// Nested types are NODE_TYPE (not NODE_STRUCT_DECLARATION)
				if child.Type == ahoy.NODE_TYPE {
					// Nested type: parent:child
					nestedName := structName + ":" + child.Value
					validTypes[nestedName] = true
					validTypes[child.Value] = true
				}
			}
		} else if node.Type == ahoy.NODE_ALIAS_DECLARATION {
			// Add type aliases
			validTypes[node.Value] = true
		} else if node.Type == ahoy.NODE_ENUM_DECLARATION {
			// Add enum types
			validTypes[node.Value] = true
		} else if node.Type == ahoy.NODE_UNION_DECLARATION {
			// Add union types
			validTypes[node.Value] = true
		}
		for _, child := range node.Children {
			collectTypes(child, parentStruct)
		}
	}
	collectTypes(doc.AST, "")

	// Collect C types from imports
	if doc.CHeaderGlobal != nil {
		for typeName := range doc.CHeaderGlobal.Structs {
			validTypes[typeName] = true
			validTypes[strings.ToLower(typeName)] = true // Also add lowercase version
		}
		for typeName := range doc.CHeaderGlobal.Enums {
			validTypes[typeName] = true
			validTypes[strings.ToLower(typeName)] = true
		}
	}
	
	// Also collect from namespaced C headers
	if doc.CHeaders != nil {
		for _, cHeader := range doc.CHeaders {
			for typeName := range cHeader.Structs {
				validTypes[typeName] = true
				validTypes[strings.ToLower(typeName)] = true
			}
			for typeName := range cHeader.Enums {
				validTypes[typeName] = true
				validTypes[strings.ToLower(typeName)] = true
			}
		}
	}

	// Check for type typos in variable declarations
	var checkNode func(node *ahoy.ASTNode)
	checkNode = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		if node.Type == ahoy.NODE_ASSIGNMENT && node.DataType != "" {
			typeStr := node.DataType
			
			// Extract base type from array[T] or dict<K,V>
			baseType := extractBaseType(typeStr)
			
			if baseType != "" && !validTypes[baseType] {
				// Find closest match
				closestMatch := findClosestType(baseType, validTypes)
				var message string
				if closestMatch != "" {
					message = fmt.Sprintf("Type '%s' does not exist, did you mean '%s'?", baseType, closestMatch)
				} else {
					message = fmt.Sprintf("Struct type '%s' does not exist", baseType)
				}
				diag := protocol.Diagnostic{
					Range: protocol.Range{
						Start: protocol.Position{Line: uint32(node.Line - 1), Character: 0},
						End:   protocol.Position{Line: uint32(node.Line - 1), Character: 100},
					},
					Severity: protocol.DiagnosticSeverityError,
					Source:   "ahoy",
					Message:  message,
				}
				diagnostics = append(diagnostics, diag)
			}
		}

		// Check function parameters
		if node.Type == ahoy.NODE_FUNCTION && len(node.Children) > 0 {
			params := node.Children[0]
			for _, param := range params.Children {
				if param.DataType != "" {
					baseType := extractBaseType(param.DataType)
					if baseType != "" && !validTypes[baseType] {
						closestMatch := findClosestType(baseType, validTypes)
						var message string
						if closestMatch != "" {
							message = fmt.Sprintf("Type '%s' does not exist, did you mean '%s'?", baseType, closestMatch)
						} else {
							message = fmt.Sprintf("Struct type '%s' does not exist", baseType)
						}
						diag := protocol.Diagnostic{
							Range: protocol.Range{
								Start: protocol.Position{Line: uint32(param.Line - 1), Character: 0},
								End:   protocol.Position{Line: uint32(param.Line - 1), Character: 100},
							},
							Severity: protocol.DiagnosticSeverityError,
							Source:   "ahoy",
							Message:  message,
						}
						diagnostics = append(diagnostics, diag)
					}
				}
			}
		}

		// Check object literal types and properties
		if node.Type == ahoy.NODE_OBJECT_LITERAL && node.Value != "" {
			// Check if the struct type exists
			typeName := node.Value
			if !validTypes[typeName] {
				closestMatch := findClosestType(typeName, validTypes)
				var message string
				if closestMatch != "" {
					message = fmt.Sprintf("Struct type '%s' does not exist, did you mean '%s'?", typeName, closestMatch)
				} else {
					message = fmt.Sprintf("Struct type '%s' does not exist", typeName)
				}
				diag := protocol.Diagnostic{
					Range: protocol.Range{
						Start: protocol.Position{Line: uint32(node.Line - 1), Character: 0},
						End:   protocol.Position{Line: uint32(node.Line - 1), Character: 100},
					},
					Severity: protocol.DiagnosticSeverityError,
					Source:   "ahoy",
					Message:  message,
				}
				diagnostics = append(diagnostics, diag)
			} else {
				// Type exists, now check property names
				propDiags := checkObjectProperties(doc, node, typeName)
				diagnostics = append(diagnostics, propDiags...)
			}
		}

		for _, child := range node.Children {
			checkNode(child)
		}
	}

	checkNode(doc.AST)
	return diagnostics
}

// checkObjectProperties checks if properties in object literal exist in struct definition
func checkObjectProperties(doc *Document, objNode *ahoy.ASTNode, typeName string) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}
	
	// Get struct definition
	var structNode *ahoy.ASTNode
	var findStruct func(*ahoy.ASTNode) *ahoy.ASTNode
	findStruct = func(node *ahoy.ASTNode) *ahoy.ASTNode {
		if node == nil {
			return nil
		}
		if node.Type == ahoy.NODE_STRUCT_DECLARATION && node.Value == typeName {
			return node
		}
		// Check for nested types (NODE_TYPE within struct)
		if node.Type == ahoy.NODE_STRUCT_DECLARATION {
			for _, child := range node.Children {
				// Nested types are NODE_TYPE
				if child.Type == ahoy.NODE_TYPE {
					// Check both "typename" and "parent:typename"
					if child.Value == typeName || (node.Value + ":" + child.Value) == typeName {
						return child
					}
				}
			}
		}
		// Recursively check children
		for _, child := range node.Children {
			if result := findStruct(child); result != nil {
				return result
			}
		}
		return nil
	}
	
	structNode = findStruct(doc.AST)
	
	// Also check C header structs
	if structNode == nil && doc.CHeaderGlobal != nil {
		if cStruct, exists := doc.CHeaderGlobal.Structs[typeName]; exists {
			// Collect valid C struct field names
			validProps := make(map[string]bool)
			for _, field := range cStruct.Fields {
				validProps[field.Name] = true
			}
			
			// Check each property in object literal
			for _, prop := range objNode.Children {
				if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
					propName := prop.Value
					if !validProps[propName] {
						closestMatch := findClosestProperty(propName, validProps)
						var message string
						if closestMatch != "" {
							message = fmt.Sprintf("Property '%s' does not exist on struct '%s', did you mean '%s'?", propName, typeName, closestMatch)
						} else {
							message = fmt.Sprintf("Property '%s' not found on struct type '%s'", propName, typeName)
						}
						diag := protocol.Diagnostic{
							Range: protocol.Range{
								Start: protocol.Position{Line: uint32(prop.Line - 1), Character: 0},
								End:   protocol.Position{Line: uint32(prop.Line - 1), Character: 100},
							},
							Severity: protocol.DiagnosticSeverityError,
							Source:   "ahoy",
							Message:  message,
						}
						diagnostics = append(diagnostics, diag)
					}
				}
			}
			return diagnostics
		}
	}
	
	if structNode == nil {
		return diagnostics
	}
	
	// Collect valid property names from struct
	validProps := make(map[string]bool)
	
	// First, check if this is a nested type and find its parent
	var parentNode *ahoy.ASTNode
	if strings.Contains(typeName, ":") {
		// This is a nested type reference like "particle:smoke_particle"
		parts := strings.Split(typeName, ":")
		parentName := parts[0]
		var findParent func(*ahoy.ASTNode) *ahoy.ASTNode
		findParent = func(node *ahoy.ASTNode) *ahoy.ASTNode {
			if node == nil {
				return nil
			}
			if node.Type == ahoy.NODE_STRUCT_DECLARATION && node.Value == parentName {
				return node
			}
			for _, child := range node.Children {
				if result := findParent(child); result != nil {
					return result
				}
			}
			return nil
		}
		parentNode = findParent(doc.AST)
	}
	
	// Also check if structNode itself is a nested type (NODE_TYPE)
	// In that case, find the parent struct
	if structNode.Type == ahoy.NODE_TYPE && parentNode == nil {
		// Search upward to find parent struct
		var findParentOfNested func(*ahoy.ASTNode) *ahoy.ASTNode
		findParentOfNested = func(node *ahoy.ASTNode) *ahoy.ASTNode {
			if node == nil {
				return nil
			}
			if node.Type == ahoy.NODE_STRUCT_DECLARATION {
				// Check if this struct contains our nested type
				for _, child := range node.Children {
					if child == structNode || (child.Type == ahoy.NODE_TYPE && child.Value == structNode.Value) {
						return node
					}
				}
			}
			for _, child := range node.Children {
				if result := findParentOfNested(child); result != nil {
					return result
				}
			}
			return nil
		}
		parentNode = findParentOfNested(doc.AST)
	}
	
	// Add parent struct fields first (inherited fields)
	if parentNode != nil {
		for _, child := range parentNode.Children {
			if child.Type == ahoy.NODE_IDENTIFIER {
				validProps[child.Value] = true
			}
		}
	}
	
	// Add fields from the struct/nested type itself
	for _, child := range structNode.Children {
		// Fields are direct children as NODE_IDENTIFIER
		if child.Type == ahoy.NODE_IDENTIFIER {
			validProps[child.Value] = true
		}
		// Nested types are NODE_TYPE with fields as children
		if child.Type == ahoy.NODE_TYPE {
			for _, field := range child.Children {
				if field.Type == ahoy.NODE_IDENTIFIER {
					validProps[field.Value] = true
				}
			}
		}
	}
	
	// Check each property in object literal
	for _, prop := range objNode.Children {
		if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
			propName := prop.Value
			if !validProps[propName] {
				closestMatch := findClosestProperty(propName, validProps)
				var message string
				if closestMatch != "" {
					message = fmt.Sprintf("Property '%s' does not exist on struct '%s', did you mean '%s'?", propName, typeName, closestMatch)
				} else {
					message = fmt.Sprintf("Property '%s' not found on struct type '%s'", propName, typeName)
				}
				diag := protocol.Diagnostic{
					Range: protocol.Range{
						Start: protocol.Position{Line: uint32(prop.Line - 1), Character: 0},
						End:   protocol.Position{Line: uint32(prop.Line - 1), Character: 100},
					},
					Severity: protocol.DiagnosticSeverityError,
					Source:   "ahoy",
					Message:  message,
				}
				diagnostics = append(diagnostics, diag)
			}
		}
	}
	
	return diagnostics
}

// findClosestProperty finds the closest matching property name
func findClosestProperty(typo string, validProps map[string]bool) string {
	closestMatch := ""
	minDistance := 999
	
	for prop := range validProps {
		distance := levenshteinDistance(typo, prop)
		if distance < minDistance && distance <= 3 {
			minDistance = distance
			closestMatch = prop
		}
	}
	
	return closestMatch
}

// extractBaseType extracts base type from complex types like array[string] or dict<int,string>
func extractBaseType(typeStr string) string {
	// Handle array[T]
	if strings.HasPrefix(typeStr, "array[") {
		// Extract the type inside brackets
		start := strings.Index(typeStr, "[")
		end := strings.LastIndex(typeStr, "]")
		if start != -1 && end != -1 && end > start {
			innerType := typeStr[start+1 : end]
			// Recursively check inner type
			return extractBaseType(innerType)
		}
		return "array"
	}

	// Handle dict<K,V>
	if strings.HasPrefix(typeStr, "dict<") || strings.HasPrefix(typeStr, "dict[") {
		// Extract types inside <>
		start := strings.IndexAny(typeStr, "<[")
		end := strings.LastIndexAny(typeStr, ">]")
		if start != -1 && end != -1 && end > start {
			innerTypes := typeStr[start+1 : end]
			// Split by comma, but respect nesting
			types := smartSplit(innerTypes, ',')
			for _, t := range types {
				// Check each type
				extracted := extractBaseType(strings.TrimSpace(t))
				if extracted != "" {
					return extracted
				}
			}
		}
		return "dict"
	}

	// Simple type
	return typeStr
}

// smartSplit splits on delimiter but respects < > and [ ] nesting
func smartSplit(s string, delim rune) []string {
	var result []string
	var current strings.Builder
	depth := 0

	for _, ch := range s {
		if ch == '<' || ch == '[' {
			depth++
		} else if ch == '>' || ch == ']' {
			depth--
		}

		if ch == delim && depth == 0 {
			result = append(result, current.String())
			current.Reset()
		} else {
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

func findClosestType(typo string, validTypes map[string]bool) string {
	closestMatch := ""
	minDistance := 999

	for validType := range validTypes {
		distance := levenshteinDistance(typo, validType)
		// Only suggest if distance is small (typo is close)
		if distance < minDistance && distance <= 3 {
			minDistance = distance
			closestMatch = validType
		}
	}

	return closestMatch
}

func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
