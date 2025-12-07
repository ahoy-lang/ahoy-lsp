package main

import (
	"fmt"
	"strings"

	"ahoy"
	"go.lsp.dev/protocol"
)

// checkDuplicateFunctionDefinitions checks for duplicate function definitions in the same package
func checkDuplicateFunctionDefinitions(doc *Document) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}

	if doc.AST == nil {
		return diagnostics
	}

	// Only check if we're in a package (has PackageFiles)
	if doc.PackageFiles == nil || len(doc.PackageFiles) == 0 {
		return diagnostics
	}

	// Collect all functions defined in this file
	type funcDef struct {
		name string
		line int
		file string
	}

	thisFuncs := make(map[string]funcDef)
	
	var collectFuncs func(node *ahoy.ASTNode)
	collectFuncs = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}

		if node.Type == ahoy.NODE_FUNCTION {
			funcName := node.Value
			if funcName != "" {
				thisFuncs[funcName] = funcDef{
					name: funcName,
					line: node.Line,
					file: string(doc.URI),
				}
			}
		}

		for _, child := range node.Children {
			collectFuncs(child)
		}
	}

	collectFuncs(doc.AST)

	// Check against all functions in ALL package files
	for funcName, thisDef := range thisFuncs {
		debugLog.Printf("[DupFunc] Checking function %s at line %d in %s", funcName, thisDef.line, thisDef.file)
		
		// Check each package file for this function
		for pkgURI, pkgFile := range doc.PackageFiles {
			if pkgFile.AST == nil {
				continue
			}
			
			// Collect functions from this package file
			var findFunc func(node *ahoy.ASTNode) *ahoy.ASTNode
			findFunc = func(node *ahoy.ASTNode) *ahoy.ASTNode {
				if node == nil {
					return nil
				}
				if node.Type == ahoy.NODE_FUNCTION && node.Value == funcName {
					return node
				}
				for _, child := range node.Children {
					if found := findFunc(child); found != nil {
						return found
					}
				}
				return nil
			}
			
			if foundFunc := findFunc(pkgFile.AST); foundFunc != nil {
				// Found the same function name in another file
				debugLog.Printf("[DupFunc]   Found duplicate in %s at line %d", pkgURI, foundFunc.Line)
				
				// Get the other file name (just filename, not full path)
				otherFile := string(pkgURI)
				if idx := strings.LastIndex(otherFile, "/"); idx != -1 {
					otherFile = otherFile[idx+1:]
				}

				lineText := ""
				if thisDef.line > 0 && thisDef.line <= len(doc.Lines) {
					lineText = doc.Lines[thisDef.line-1]
				}
				endChar := uint32(len(lineText))
				if endChar == 0 {
					endChar = uint32(len(funcName) + 20)
				}

				message := fmt.Sprintf("function '%s' already defined in %s line %d; function overloading not supported in ahoy", 
					funcName, otherFile, foundFunc.Line)

				diagnostic := protocol.Diagnostic{
					Range: protocol.Range{
						Start: protocol.Position{
							Line:      uint32(thisDef.line - 1),
							Character: 0,
						},
						End: protocol.Position{
							Line:      uint32(thisDef.line - 1),
							Character: endChar,
						},
					},
					Severity: protocol.DiagnosticSeverityError,
					Source:   "ahoy",
					Message:  message,
					Code:     "duplicate-function-definition",
				}
				diagnostics = append(diagnostics, diagnostic)
				break // Only report once per function
			}
		}
	}

	return diagnostics
}
