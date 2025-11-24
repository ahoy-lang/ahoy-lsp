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
	}
	for _, t := range builtinTypes {
		validTypes[t] = true
	}

	// Collect struct types, enums, unions, and aliases
	var collectTypes func(*ahoy.ASTNode)
	collectTypes = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}
		if node.Type == ahoy.NODE_STRUCT_DECLARATION {
			validTypes[node.Value] = true
			// Also add capitalized version
			validTypes[capitalizeFirst(node.Value)] = true
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
			collectTypes(child)
		}
	}
	collectTypes(doc.AST)

	// Collect C types from imports
	if doc.CHeaderGlobal != nil {
		for typeName := range doc.CHeaderGlobal.Structs {
			validTypes[typeName] = true
		}
		for typeName := range doc.CHeaderGlobal.Enums {
			validTypes[typeName] = true
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
				if closestMatch != "" {
					diag := protocol.Diagnostic{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(node.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(node.Line - 1), Character: 100},
						},
						Severity: protocol.DiagnosticSeverityError,
						Source:   "ahoy",
						Message:  fmt.Sprintf("Type '%s' does not exist, did you mean '%s'?", baseType, closestMatch),
					}
					diagnostics = append(diagnostics, diag)
				}
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
						if closestMatch != "" {
							diag := protocol.Diagnostic{
								Range: protocol.Range{
									Start: protocol.Position{Line: uint32(param.Line - 1), Character: 0},
									End:   protocol.Position{Line: uint32(param.Line - 1), Character: 100},
								},
								Severity: protocol.DiagnosticSeverityError,
								Source:   "ahoy",
								Message:  fmt.Sprintf("Type '%s' does not exist, did you mean '%s'?", baseType, closestMatch),
							}
							diagnostics = append(diagnostics, diag)
						}
					}
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
