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

	// Only check if we're in a package (has PackageSymbols)
	if doc.PackageSymbols == nil {
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

	// Check against all functions in the package
	for funcName, thisDef := range thisFuncs {
		// DEBUG
		if funcName == "draw_game_card" {
			debugLog.Printf("Checking function %s at line %d in %s", funcName, thisDef.line, thisDef.file)
		}
		
		// Look up in package symbols
		if sym := doc.PackageSymbols.GlobalScope.Lookup(funcName); sym != nil {
			if funcName == "draw_game_card" {
				debugLog.Printf("  Found symbol: Kind=%d, File=%s, Line=%d", sym.Kind, sym.File, sym.Line)
			}
			
			if sym.Kind == SymbolKindFunction {
				// DEBUG
				if funcName == "draw_game_card" {
					debugLog.Printf("  Comparing: sym.File='%s' vs doc.URI='%s', sym.Line=%d vs thisDef.line=%d", 
						sym.File, string(doc.URI), sym.Line, thisDef.line)
					debugLog.Printf("  Files equal? %v, Lines equal? %v", 
						sym.File == string(doc.URI), sym.Line == thisDef.line)
				}
				
				// Check if this function is defined in a different location
				// (different file OR different line in same file)
				if sym.File != "" && (sym.File != string(doc.URI) || sym.Line != thisDef.line) {
					// Found a duplicate function definition
					
					if funcName == "draw_game_card" {
						debugLog.Printf("  REPORTING DUPLICATE!")
					}
					
					// Get the other file name (just filename, not full path)
					otherFile := sym.File
					if idx := strings.LastIndex(sym.File, "/"); idx != -1 {
						otherFile = sym.File[idx+1:]
					}

					// Get current file name
					thisFile := string(doc.URI)
					if idx := strings.LastIndex(thisFile, "/"); idx != -1 {
						thisFile = thisFile[idx+1:]
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
						funcName, otherFile, sym.Line)

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
				}
			}
		}
	}

	return diagnostics
}
