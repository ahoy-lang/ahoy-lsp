package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// InlayHint types for LSP (not in protocol library yet)
type InlayHintParams struct {
	TextDocument protocol.TextDocumentIdentifier `json:"textDocument"`
	Range        protocol.Range                  `json:"range"`
}

type InlayHint struct {
	Position     protocol.Position `json:"position"`
	Label        string            `json:"label"`
	Kind         int               `json:"kind,omitempty"`
	PaddingLeft  bool              `json:"paddingLeft,omitempty"`
	PaddingRight bool              `json:"paddingRight,omitempty"`
}

const (
	InlayHintKindType      = 1
	InlayHintKindParameter = 2
)

func (s *Server) handleInlayHint(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params InlayHintParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		debugLog.Printf("Failed to unmarshal inlay hint params: %v", err)
		return reply(ctx, nil, nil)
	}

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, []InlayHint{}, nil)
	}

	var hints []InlayHint

	// Find all $ characters in the visible range and add hints for them
	startLine := int(params.Range.Start.Line)
	endLine := int(params.Range.End.Line)

	if startLine < 0 {
		startLine = 0
	}
	if endLine >= len(doc.Lines) {
		endLine = len(doc.Lines) - 1
	}

	for lineNum := startLine; lineNum <= endLine; lineNum++ {
		line := doc.Lines[lineNum]
		
		// Find all $ characters on this line
		for charPos, ch := range line {
			if ch == '$' {
				// Find the block start for this $
				if blockStart := findBlockStart(doc, lineNum); blockStart != nil {
					startLineNum := int(blockStart.Range.Start.Line)
					if startLineNum >= 0 && startLineNum < len(doc.Lines) {
						startLineContent := strings.TrimSpace(doc.Lines[startLineNum])
						
						// Create inlay hint text
						hintText := fmt.Sprintf(" %s (line %d)", startLineContent, startLineNum+1)
						
						// Add hint right after the $
						hint := InlayHint{
							Position: protocol.Position{
								Line:      uint32(lineNum),
								Character: uint32(charPos + 1),
							},
							Label:        hintText,
							Kind:         InlayHintKindType,
							PaddingLeft:  false,
							PaddingRight: false,
						}
						
						hints = append(hints, hint)
					}
				}
			}
		}
	}

	return reply(ctx, hints, nil)
}
