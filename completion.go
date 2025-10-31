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
	if doc == nil || doc.Lines == nil {
		return reply(ctx, protocol.CompletionList{Items: []protocol.CompletionItem{}}, nil)
	}

	items := []protocol.CompletionItem{}

	// Get the current line content from cached lines
	if int(params.Position.Line) >= len(doc.Lines) || int(params.Position.Line) < 0 {
		return reply(ctx, protocol.CompletionList{Items: items}, nil)
	}

	currentLine := doc.Lines[params.Position.Line]

	// Additional safety checks
	if len(currentLine) > 10000 {
		return reply(ctx, protocol.CompletionList{Items: items}, nil)
	}

	if int(params.Position.Character) > len(currentLine) || int(params.Position.Character) < 0 {
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
	
	// Check if we're after a dot (.) for method completion
	isDotCompletion := false
	beforePrefix := ""
	if params.Position.Character > 0 {
		// Look back from the prefix to find if there's a dot
		checkPos := int(params.Position.Character) - len(prefix) - 1
		if checkPos >= 0 && checkPos < len(currentLine) && currentLine[checkPos] == '.' {
			// We're after a dot, find the identifier before the dot
			identEnd := checkPos - 1
			identStart := identEnd
			for identStart >= 0 && (isIdentifierChar(rune(currentLine[identStart])) || currentLine[identStart] == '_') {
				identStart--
			}
			identStart++
			if identStart <= identEnd {
				beforePrefix = currentLine[identStart : identEnd+1]
				isDotCompletion = true
			}
		}
	}
	
	// If we're after a dot, provide completions based on the type
	if isDotCompletion {
		// Build symbol table to look up the type
		if doc.AST != nil {
			symbolTable := BuildSymbolTable(doc.AST)
			defer symbolTable.Clear()
			
			// Look up the variable/identifier before the dot
			if sym := symbolTable.Lookup(beforePrefix); sym != nil {
				// Check if it's a struct type
				if sym.Kind == SymbolKindVariable || sym.Kind == SymbolKindConstant {
					// Get struct fields based on the variable's type
					fields := symbolTable.GetStructFields(sym.Type)
					if fields != nil && len(fields) > 0 {
						// Add struct field completions
						for fieldName, field := range fields {
							if prefix == "" || strings.HasPrefix(fieldName, prefix) {
								items = append(items, protocol.CompletionItem{
									Label:  fieldName,
									Kind:   protocol.CompletionItemKindField,
									Detail: field.Type,
								})
							}
						}
						
						// Return early with struct field completions
						result := protocol.CompletionList{
							IsIncomplete: false,
							Items:        items,
						}
						return reply(ctx, result, nil)
					}
				}
				
				// Check if it's a string type for string methods
				if sym.Type == "string" {
					stringMethods := []struct {
						label       string
						detail      string
						description string
						params      string
					}{
						{"length", "Get string length", "Returns the number of characters in the string", "||"},
						{"upper", "Convert to uppercase", "Returns the string in uppercase", "||"},
						{"lower", "Convert to lowercase", "Returns the string in lowercase", "||"},
						{"replace", "Replace substring", "Replaces occurrences of a substring with another", "|old, new|"},
						{"contains", "Check if contains substring", "Returns true if the string contains the substring", "|substring|"},
						{"camel_case", "Convert to camelCase", "Converts the string to camelCase", "||"},
						{"snake_case", "Convert to snake_case", "Converts the string to snake_case", "||"},
						{"pascal_case", "Convert to PascalCase", "Converts the string to PascalCase", "||"},
						{"kebab_case", "Convert to kebab-case", "Converts the string to kebab-case", "||"},
						{"match", "Match regex pattern", "Tests if the string matches a regular expression", "|pattern|"},
						{"split", "Split string", "Splits the string by a delimiter", "|delimiter|"},
						{"count", "Count occurrences", "Counts occurrences of a character or substring", "|substring|"},
						{"lpad", "Left pad string", "Pads the string on the left to a specified length", "|length, char|"},
						{"rpad", "Right pad string", "Pads the string on the right to a specified length", "|length, char|"},
						{"pad", "Pad string both sides", "Pads the string on both sides to a specified length", "|length, char|"},
						{"strip", "Trim whitespace", "Removes leading and trailing whitespace", "||"},
						{"get_file", "Get filename from path", "Extracts the filename from a file path", "||"},
					}
					
					for _, method := range stringMethods {
						if prefix == "" || strings.HasPrefix(method.label, prefix) {
							items = append(items, protocol.CompletionItem{
								Label:         method.label,
								Kind:          protocol.CompletionItemKindMethod,
								Detail:        method.detail,
								Documentation: method.description,
								InsertText:    method.label + method.params,
							})
						}
					}
					
					// Return early with method completions
					result := protocol.CompletionList{
						IsIncomplete: false,
						Items:        items,
					}
					return reply(ctx, result, nil)
				}

				// Check if it's a dict type for dictionary methods
				if sym.Type == "dict" {
					dictMethods := []struct {
						label       string
						detail      string
						description string
						params      string
					}{
						{"size", "Get dictionary size", "Returns the number of key-value pairs in the dictionary", "||"},
						{"clear", "Clear all entries", "Removes all entries from the dictionary", "||"},
						{"has", "Check if key exists", "Returns true if the key exists in the dictionary", "|key|"},
						{"has_all", "Check if all keys exist", "Returns true if all keys in the array exist", "|keys_array|"},
						{"keys", "Get all keys", "Returns an array of all dictionary keys", "||"},
						{"values", "Get all values", "Returns an array of all dictionary values", "||"},
						{"sort", "Sort by keys", "Returns a new dictionary sorted by keys", "||"},
						{"stable_sort", "Stable sort by keys", "Returns a new dictionary with stable sort by keys", "||"},
						{"merge", "Merge dictionaries", "Merges another dictionary into this one", "|other_dict|"},
					}
					
					for _, method := range dictMethods {
						if prefix == "" || strings.HasPrefix(method.label, prefix) {
							items = append(items, protocol.CompletionItem{
								Label:         method.label,
								Kind:          protocol.CompletionItemKindMethod,
								Detail:        method.detail,
								Documentation: method.description,
								InsertText:    method.label + method.params,
							})
						}
					}
					
					// Return early with method completions
					result := protocol.CompletionList{
						IsIncomplete: false,
						Items:        items,
					}
					return reply(ctx, result, nil)
				}
			}
		}
	}

	// Default: provide string methods for any dot completion (fallback)
	if isDotCompletion {
		// Check if the identifier before the dot is a string
		// For now, we'll provide string methods for any dot completion
		// TODO: Improve type inference to check actual variable types
		
		stringMethods := []struct {
			label       string
			detail      string
			description string
			params      string
		}{
			{"length", "Get string length", "Returns the number of characters in the string", "||"},
			{"upper", "Convert to uppercase", "Returns the string in uppercase", "||"},
			{"lower", "Convert to lowercase", "Returns the string in lowercase", "||"},
			{"replace", "Replace substring", "Replaces occurrences of a substring with another", "|old, new|"},
			{"contains", "Check if contains substring", "Returns true if the string contains the substring", "|substring|"},
			{"camel_case", "Convert to camelCase", "Converts the string to camelCase", "||"},
			{"snake_case", "Convert to snake_case", "Converts the string to snake_case", "||"},
			{"pascal_case", "Convert to PascalCase", "Converts the string to PascalCase", "||"},
			{"kebab_case", "Convert to kebab-case", "Converts the string to kebab-case", "||"},
			{"match", "Match regex pattern", "Tests if the string matches a regular expression", "|pattern|"},
			{"split", "Split string", "Splits the string by a delimiter", "|delimiter|"},
			{"count", "Count occurrences", "Counts occurrences of a character or substring", "|substring|"},
			{"lpad", "Left pad string", "Pads the string on the left to a specified length", "|length, char|"},
			{"rpad", "Right pad string", "Pads the string on the right to a specified length", "|length, char|"},
			{"pad", "Pad string both sides", "Pads the string on both sides to a specified length", "|length, char|"},
			{"strip", "Trim whitespace", "Removes leading and trailing whitespace", "||"},
			{"get_file", "Get filename from path", "Extracts the filename from a file path", "||"},
		}
		
		for _, method := range stringMethods {
			if prefix == "" || strings.HasPrefix(method.label, prefix) {
				items = append(items, protocol.CompletionItem{
					Label:         method.label,
					Kind:          protocol.CompletionItemKindMethod,
					Detail:        method.detail,
					Documentation: method.description,
					InsertText:    method.label + method.params,
				})
			}
		}
		
		// Return early with method completions
		result := protocol.CompletionList{
			IsIncomplete: false,
			Items:        items,
		}
		return reply(ctx, result, nil)
	}

	// Add keyword completions (only if not dot completion)
	keywords := []string{
		"if", "else", "elseif", "anif", "then",
		"loop", "in", "to", "do",
		"func", "return",
		"switch", "on",
		"when",
		"import", "program",
		"ahoy",
		"is", "not", "and", "or",
		"break", "skip",
		"true", "false",
		"enum", "struct", "type",
		"int", "float", "string", "bool", "dict", "vector2", "color",
	}

	// Add keyword completions (only if not dot completion)
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
