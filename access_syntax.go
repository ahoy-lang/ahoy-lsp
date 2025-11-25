package main

import (
	"strings"

	"ahoy"

	"go.lsp.dev/protocol"
)

// checkAccessSyntax validates that the correct access syntax is used for arrays, dicts, and objects
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
			// Get the variable being accessed
			varName := node.Value
			
			// Determine what type it is
			varType := ""
			if sym := doc.SymbolTable.GlobalScope.Lookup(varName); sym != nil {
				varType = sym.Type
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
		}
		
		for _, child := range node.Children {
			checkNode(child)
		}
	}
	
	checkNode(doc.AST)
	return diagnostics
}
