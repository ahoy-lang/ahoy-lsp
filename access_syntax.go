package main

import (
	"strconv"
	"strings"

	"ahoy"

	"go.lsp.dev/protocol"
)

// getArrayDimension returns the number of dimensions of an array type
// e.g., "array[int]" -> 1, "array[array[int]]" -> 2
func getArrayDimension(typeName string) int {
	if !strings.HasPrefix(typeName, "array") {
		return 0
	}
	dim := 1
	// Count nested array[ occurrences
	inner := typeName
	for {
		start := strings.Index(inner, "array[")
		if start == -1 {
			break
		}
		inner = inner[start+6:] // Skip "array["
		if strings.HasPrefix(inner, "array[") {
			dim++
		} else {
			break
		}
	}
	return dim
}

// checkAccessSyntax validates that the correct access syntax is used for arrays, dicts, and objects
// Also validates array bounds and dimension access
func checkAccessSyntax(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}
	
	if doc.AST == nil || doc.SymbolTable == nil {
		return diagnostics
	}
	
	var checkNode func(node *ahoy.ASTNode)
	checkNode = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}
		
		// Check array access nodes
		if node.Type == ahoy.NODE_ARRAY_ACCESS {
			// Count access dimensions (how many levels of [])
			accessDimensions := countArrayAccessDimensions(node)
			
			// Get the root variable name
			varName := getRootArrayName(node)
			
			// Determine what type it is
			varType := ""
			if varName != "" {
				if sym := doc.SymbolTable.GlobalScope.Lookup(varName); sym != nil {
					varType = sym.Type
				}
			}
			
			// Check dimension mismatch - only if we have explicit type information
			// Don't check if type is just "array" (could be inferred, might be 2D)
			if varType != "" && varType != "array" {
				arrayDimensions := getArrayDimension(varType)
				
				// Only report error if we have explicit dimensionality info
				// and the access exceeds it
				if arrayDimensions > 0 && accessDimensions > arrayDimensions {
					diag := protocol.Diagnostic{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(node.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(node.Line - 1), Character: 100},
						},
						Severity: protocol.DiagnosticSeverityError,
						Source:   "ahoy",
						Message:  "Cannot access " + strconv.Itoa(accessDimensions) + " dimensions on " + strconv.Itoa(arrayDimensions) + "D array '" + varName + "'",
					}
					diagnostics = append(diagnostics, diag)
				}
			}
			
			// Check if it's an object (struct) - objects should use {} not []
			if varType != "" && varType != "array" && varType != "dict" && !strings.HasPrefix(varType, "array[") && !strings.HasPrefix(varType, "dict<") {
				// Check if it's a struct type by looking it up
				isStruct := false
				if typeSym := doc.SymbolTable.GlobalScope.Lookup(varType); typeSym != nil {
					if typeSym.Kind == SymbolKindStruct {
						isStruct = true
					}
				}
				
				// Also check C header structs
				if !isStruct && doc.CHeaderGlobal != nil {
					if _, exists := doc.CHeaderGlobal.Structs[varType]; exists {
						isStruct = true
					}
				}
				
				if isStruct {
					diag := protocol.Diagnostic{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(node.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(node.Line - 1), Character: 100},
						},
						Severity: protocol.DiagnosticSeverityError,
						Source:   "ahoy",
						Message:  "Invalid object access syntax, use object{} instead of array[]",
					}
					diagnostics = append(diagnostics, diag)
				}
			}
			
			// Check compile-time bounds for multidimensional arrays
			diagnostics = append(diagnostics, checkMultiDimensionalBounds(node, doc)...)
		}
		
		for _, child := range node.Children {
			checkNode(child)
		}
	}
	
	checkNode(doc.AST)
	return diagnostics
}

// countArrayAccessDimensions counts how many dimensions are being accessed
// e.g., arr[0] -> 1, arr[0][1] -> 2, arr[0][1][2] -> 3
func countArrayAccessDimensions(node *ahoy.ASTNode) int {
	if node.Type != ahoy.NODE_ARRAY_ACCESS {
		return 0
	}
	
	// If Value is empty, it's a chained access (2D+)
	if node.Value == "" && len(node.Children) == 2 {
		// Children[0] is the inner access, Children[1] is the outer index
		return 1 + countArrayAccessDimensions(node.Children[0])
	}
	
	// Simple 1D access
	return 1
}

// getRootArrayName gets the variable name from a potentially chained array access
func getRootArrayName(node *ahoy.ASTNode) string {
	if node.Type != ahoy.NODE_ARRAY_ACCESS {
		return ""
	}
	
	// If Value is not empty, this is the root
	if node.Value != "" {
		return node.Value
	}
	
	// Otherwise, recurse into the first child
	if len(node.Children) > 0 {
		return getRootArrayName(node.Children[0])
	}
	
	return ""
}

// checkMultiDimensionalBounds checks compile-time bounds for all dimensions of array access
func checkMultiDimensionalBounds(node *ahoy.ASTNode, doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}
	
	if node.Type != ahoy.NODE_ARRAY_ACCESS {
		return diagnostics
	}
	
	// Get all indices in order from innermost to outermost
	indices := collectArrayIndices(node)
	varName := getRootArrayName(node)
	
	if varName == "" || len(indices) == 0 {
		return diagnostics
	}
	
	// Get the array symbol to check bounds
	sym := doc.SymbolTable.GlobalScope.Lookup(varName)
	if sym == nil {
		return diagnostics
	}
	
	// Get array sizes from declarations
	arraySizes := getArraySizes(varName, doc)
	
	// Check each index against its corresponding size
	for i, idx := range indices {
		if idx.Type == ahoy.NODE_NUMBER {
			indexVal, err := strconv.Atoi(idx.Value)
			if err != nil {
				continue
			}
			
			// Check if we have size information for this dimension
			if i < len(arraySizes) && arraySizes[i] > 0 {
				if indexVal < 0 || indexVal >= arraySizes[i] {
					diag := protocol.Diagnostic{
						Range: protocol.Range{
							Start: protocol.Position{Line: uint32(node.Line - 1), Character: 0},
							End:   protocol.Position{Line: uint32(node.Line - 1), Character: 100},
						},
						Severity: protocol.DiagnosticSeverityError,
						Source:   "ahoy",
						Message:  "Array index " + strconv.Itoa(indexVal) + " out of bounds for dimension " + strconv.Itoa(i+1) + " (size " + strconv.Itoa(arraySizes[i]) + ")",
					}
					diagnostics = append(diagnostics, diag)
				}
			}
		}
	}
	
	return diagnostics
}

// collectArrayIndices collects all index expressions from a chained array access
func collectArrayIndices(node *ahoy.ASTNode) []*ahoy.ASTNode {
	indices := []*ahoy.ASTNode{}
	
	if node.Type != ahoy.NODE_ARRAY_ACCESS {
		return indices
	}
	
	// If Value is empty, it's a chained access
	if node.Value == "" && len(node.Children) == 2 {
		// Collect from inner access first
		innerIndices := collectArrayIndices(node.Children[0])
		indices = append(indices, innerIndices...)
		// Then add the outer index
		indices = append(indices, node.Children[1])
	} else if len(node.Children) > 0 {
		// Simple access - just the one index
		indices = append(indices, node.Children[0])
	}
	
	return indices
}

// getArraySizes returns the sizes of each dimension for an array variable
func getArraySizes(varName string, doc *Document) []int {
	sizes := []int{}
	
	// Find the variable declaration in the AST
	var findDecl func(*ahoy.ASTNode) *ahoy.ASTNode
	findDecl = func(node *ahoy.ASTNode) *ahoy.ASTNode {
		if node == nil {
			return nil
		}
		
		if (node.Type == ahoy.NODE_ASSIGNMENT || node.Type == ahoy.NODE_VARIABLE_DECLARATION) && node.Value == varName {
			return node
		}
		
		for _, child := range node.Children {
			if result := findDecl(child); result != nil {
				return result
			}
		}
		return nil
	}
	
	declNode := findDecl(doc.AST)
	if declNode == nil || len(declNode.Children) == 0 {
		return sizes
	}
	
	// Get the value node
	valueNode := declNode.Children[0]
	
	// Count elements in array literal
	sizes = countArrayDimensionSizes(valueNode)
	
	return sizes
}

// countArrayDimensionSizes counts the size of each dimension in a nested array literal
func countArrayDimensionSizes(node *ahoy.ASTNode) []int {
	sizes := []int{}
	
	if node == nil {
		return sizes
	}
	
	if node.Type == ahoy.NODE_ARRAY_LITERAL {
		sizes = append(sizes, len(node.Children))
		
		// Check if first child is also an array literal (nested array)
		if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_ARRAY_LITERAL {
			innerSizes := countArrayDimensionSizes(node.Children[0])
			sizes = append(sizes, innerSizes...)
		}
	}
	
	return sizes
}
