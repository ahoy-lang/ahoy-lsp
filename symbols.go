package main

import (
	"context"
	"encoding/json"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (s *Server) handleDocumentSymbol(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DocumentSymbolParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil || doc.SymbolTable == nil {
		return reply(ctx, nil, nil)
	}

	// Build document symbols from symbol table
	symbols := []protocol.DocumentSymbol{}

	// Get all symbols and organize them hierarchically
	allSymbols := doc.SymbolTable.GetAllSymbols()

	for _, sym := range allSymbols {
		// Only include top-level symbols (functions, enums, structs, constants)
		if shouldIncludeInOutline(sym) {
			docSymbol := symbolToDocumentSymbol(sym)
			symbols = append(symbols, docSymbol)
		}
	}

	return reply(ctx, symbols, nil)
}

func shouldIncludeInOutline(sym *Symbol) bool {
	switch sym.Kind {
	case SymbolKindFunction, SymbolKindEnum, SymbolKindStruct, SymbolKindConstant, SymbolKindVariable:
		return true
	default:
		return false
	}
}

func symbolToDocumentSymbol(sym *Symbol) protocol.DocumentSymbol {
	docSymbol := protocol.DocumentSymbol{
		Name: sym.Name,
		Kind: symbolKindToProtocol(sym.Kind),
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(sym.Line - 1),
				Character: uint32(sym.Column),
			},
			End: protocol.Position{
				Line:      uint32(sym.Line - 1),
				Character: uint32(sym.Column + len(sym.Name)),
			},
		},
		SelectionRange: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(sym.Line - 1),
				Character: uint32(sym.Column),
			},
			End: protocol.Position{
				Line:      uint32(sym.Line - 1),
				Character: uint32(sym.Column + len(sym.Name)),
			},
		},
	}

	// Add type detail
	if sym.Type != "" {
		docSymbol.Detail = sym.Type
	}

	// Note: Children handling removed to prevent circular references and memory leaks
	// Symbols are now shown flat without hierarchical nesting

	return docSymbol
}

func shouldIncludeAsChild(sym *Symbol) bool {
	switch sym.Kind {
	case SymbolKindParameter, SymbolKindEnumValue, SymbolKindStructField:
		return true
	default:
		return false
	}
}

func symbolKindToProtocol(kind SymbolKind) protocol.SymbolKind {
	switch kind {
	case SymbolKindFunction:
		return protocol.SymbolKindFunction
	case SymbolKindVariable:
		return protocol.SymbolKindVariable
	case SymbolKindParameter:
		return protocol.SymbolKindVariable
	case SymbolKindEnum:
		return protocol.SymbolKindEnum
	case SymbolKindEnumValue:
		return protocol.SymbolKindEnumMember
	case SymbolKindStruct:
		return protocol.SymbolKindStruct
	case SymbolKindStructField:
		return protocol.SymbolKindField
	case SymbolKindConstant:
		return protocol.SymbolKindConstant
	default:
		return protocol.SymbolKindVariable
	}
}
