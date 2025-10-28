package main

import (
	"context"
	"encoding/json"
	"strings"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (s *Server) handleCompletion(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.CompletionParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil {
		return reply(ctx, nil, nil)
	}

	items := []protocol.CompletionItem{}

	// Get the current line content
	lines := strings.Split(doc.Content, "\n")
	if int(params.Position.Line) >= len(lines) {
		return reply(ctx, protocol.CompletionList{Items: items}, nil)
	}

	currentLine := lines[params.Position.Line]
	if int(params.Position.Character) > len(currentLine) {
		return reply(ctx, protocol.CompletionList{Items: items}, nil)
	}

	// Get the word being typed
	prefix := ""
	if params.Position.Character > 0 {
		start := int(params.Position.Character) - 1
		for start >= 0 && (isIdentifierChar(rune(currentLine[start])) || currentLine[start] == '_') {
			start--
		}
		start++
		prefix = currentLine[start:params.Position.Character]
	}

	// Add keyword completions
	keywords := []string{
		"if", "else", "elseif", "anif", "then",
		"loop", "in", "to", "do",
		"func", "return",
		"switch", "on",
		"when",
		"import",
		"ahoy",
		"is", "not", "and", "or",
		"break", "skip",
		"true", "false",
		"enum", "struct", "type",
		"int", "float", "string", "bool", "dict", "vector2", "color",
	}

	for _, kw := range keywords {
		if prefix == "" || strings.HasPrefix(kw, prefix) {
			items = append(items, protocol.CompletionItem{
				Label:  kw,
				Kind:   protocol.CompletionItemKindKeyword,
				Detail: "keyword",
			})
		}
	}

	// Add operator completions (word-based operators)
	operators := []struct {
		label  string
		detail string
	}{
		{"plus", "addition operator (+)"},
		{"minus", "subtraction operator (-)"},
		{"times", "multiplication operator (*)"},
		{"div", "division operator (/)"},
		{"mod", "modulo operator (%)"},
		{"lesser", "less than operator (<)"},
		{"greater", "greater than operator (>)"},
	}

	for _, op := range operators {
		if prefix == "" || strings.HasPrefix(op.label, prefix) {
			items = append(items, protocol.CompletionItem{
				Label:  op.label,
				Kind:   protocol.CompletionItemKindOperator,
				Detail: op.detail,
			})
		}
	}

	// TODO: Add completions for:
	// - Variables in scope (from symbol table)
	// - Functions in scope
	// - Struct members
	// - Enum values

	result := protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}

	return reply(ctx, result, nil)
}

func isIdentifierChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}
