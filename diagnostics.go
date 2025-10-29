package main

import (
	"context"

	"go.lsp.dev/protocol"
)

func (s *Server) publishDiagnostics(ctx context.Context, doc *Document) {
	diagnostics := []protocol.Diagnostic{}

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
