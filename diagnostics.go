package main

import (
	"context"
	"strings"

	"ahoy"
	"go.lsp.dev/protocol"
)

func (s *Server) publishDiagnostics(ctx context.Context, doc *Document) {
	diagnostics := []protocol.Diagnostic{}

	// Check for program declaration position
	if doc.AST != nil {
		programDiag := checkProgramDeclarationPosition(doc)
		if programDiag != nil {
			diagnostics = append(diagnostics, *programDiag)
		}
	}

	// Convert parse errors to LSP diagnostics
	for _, err := range doc.Errors {
		severity := protocol.DiagnosticSeverityError

		// Ensure column doesn't go negative when converting to 0-based
		startCol := err.Column - 1
		if startCol < 0 {
			startCol = 0
		}
		endCol := err.Column + 10
		if endCol < startCol {
			endCol = startCol + 1
		}

		diagnostic := protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(err.Line - 1),   // LSP is 0-based, parser is 1-based
					Character: uint32(startCol),
				},
				End: protocol.Position{
					Line:      uint32(err.Line - 1),
					Character: uint32(endCol),
				},
			},
			Severity: severity,
			Source:   "ahoy",
			Message:  err.Message,
		}

		diagnostics = append(diagnostics, diagnostic)
	}

	// Send diagnostics to the editor
	params := protocol.PublishDiagnosticsParams{
		URI:         doc.URI,
		Diagnostics: diagnostics,
	}

	// Notify the client (no reply expected)
	s.conn.Notify(ctx, protocol.MethodTextDocumentPublishDiagnostics, params)
}

// checkProgramDeclarationPosition checks if program declaration is on the first line
func checkProgramDeclarationPosition(doc *Document) *protocol.Diagnostic {
	if doc.AST == nil {
		return nil
	}

	// Find program_declaration node in AST
	var programNode *ahoy.ASTNode
	var findProgram func(*ahoy.ASTNode)
	findProgram = func(node *ahoy.ASTNode) {
		if node == nil {
			return
		}
		if node.Type == ahoy.NODE_PROGRAM_DECLARATION {
			programNode = node
			return
		}
		for _, child := range node.Children {
			if programNode == nil {
				findProgram(child)
			}
		}
	}
	findProgram(doc.AST)

	if programNode == nil {
		return nil
	}

	// Check if program declaration is NOT on line 1
	// Allow empty lines or comments before it
	firstNonEmptyLine := 0
	for i, line := range doc.Lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "?") {
			firstNonEmptyLine = i
			break
		}
	}

	// If program node is not on the first non-empty line, create diagnostic
	if programNode.Line > firstNonEmptyLine+1 {
		// Get the line text to calculate end column
		lineText := ""
		if programNode.Line > 0 && programNode.Line <= len(doc.Lines) {
			lineText = doc.Lines[programNode.Line-1]
		}
		endChar := uint32(len(lineText))
		if endChar == 0 {
			endChar = 20 // Default if we can't get line text
		}

		return &protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(programNode.Line - 1),
					Character: 0,
				},
				End: protocol.Position{
					Line:      uint32(programNode.Line - 1),
					Character: endChar,
				},
			},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "ahoy",
			Message:  "Program declaration must be on the first line of the file",
			Code:     "program-position",
		}
	}

	return nil
}
