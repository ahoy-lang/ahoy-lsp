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

		diagnostic := protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{
					Line:      uint32(err.Line - 1),   // LSP is 0-based, parser is 1-based
					Character: uint32(err.Column - 1), // LSP is 0-based
				},
				End: protocol.Position{
					Line:      uint32(err.Line - 1),
					Character: uint32(err.Column + 10), // Estimate error length
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
