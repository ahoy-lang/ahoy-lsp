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
	beforePrefixType := "" // Track if we detected a literal type
	
	if params.Position.Character > 0 {
		// Look back from the prefix to find if there's a dot
		checkPos := int(params.Position.Character) - len(prefix) - 1
		if checkPos >= 0 && checkPos < len(currentLine) && currentLine[checkPos] == '.' {
			// We're after a dot, find what's before it
			identEnd := checkPos - 1
			identStart := identEnd
			
			// Check if it's a string literal (quoted)
			if identEnd >= 0 && (currentLine[identEnd] == '"' || currentLine[identEnd] == '\'') {
				quoteChar := currentLine[identEnd]
				identStart = identEnd - 1
				// Find the opening quote
				for identStart >= 0 && currentLine[identStart] != quoteChar {
					identStart--
				}
				if identStart >= 0 && currentLine[identStart] == quoteChar {
					beforePrefix = currentLine[identStart : identEnd+1]
					beforePrefixType = "string"
					isDotCompletion = true
				}
			} else if identEnd >= 0 && currentLine[identEnd] == ']' {
				// Check if it's an array literal [...]
				bracketCount := 1
				identStart = identEnd - 1
				for identStart >= 0 && bracketCount > 0 {
					if currentLine[identStart] == ']' {
						bracketCount++
					} else if currentLine[identStart] == '[' {
						bracketCount--
					}
					identStart--
				}
				identStart++ // Move back to the '['
				if identStart >= 0 && currentLine[identStart] == '[' {
					beforePrefix = currentLine[identStart : identEnd+1]
					beforePrefixType = "array"
					isDotCompletion = true
				}
			} else if identEnd >= 0 && currentLine[identEnd] == '}' {
				// Check if it's a dict literal {...}
				braceCount := 1
				identStart = identEnd - 1
				for identStart >= 0 && braceCount > 0 {
					if currentLine[identStart] == '}' {
						braceCount++
					} else if currentLine[identStart] == '{' {
						braceCount--
					}
					identStart--
				}
				identStart++ // Move back to the '{'
				if identStart >= 0 && currentLine[identStart] == '{' {
					beforePrefix = currentLine[identStart : identEnd+1]
					beforePrefixType = "dict"
					isDotCompletion = true
				}
			} else {
				// It's an identifier, extract it
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
	}
	
	// If we're after a dot, provide completions based on the type
	if isDotCompletion {
		// First check if we detected a literal type directly
		if beforePrefixType == "string" {
			// String literal methods
			items = addStringMethods(items, prefix)
			return reply(ctx, protocol.CompletionList{IsIncomplete: false, Items: items}, nil)
		} else if beforePrefixType == "array" {
			// Array literal methods
			items = addArrayMethods(items, prefix)
			return reply(ctx, protocol.CompletionList{IsIncomplete: false, Items: items}, nil)
		} else if beforePrefixType == "dict" {
			// Dict literal methods
			items = addDictMethods(items, prefix)
			return reply(ctx, protocol.CompletionList{IsIncomplete: false, Items: items}, nil)
		}
		
		// Build symbol table to look up the type
		if doc.AST != nil {
			symbolTable := BuildSymbolTable(doc.AST)
			defer symbolTable.Clear()
			
			// Look up the variable/identifier before the dot
			if sym := symbolTable.Lookup(beforePrefix); sym != nil {
				// Don't provide method completions for constants
				if sym.Kind == SymbolKindConstant {
					// Return empty completion list for constants
					return reply(ctx, protocol.CompletionList{IsIncomplete: false, Items: []protocol.CompletionItem{}}, nil)
				}
				
				// Check type-specific completions first
				// Check if it's a string type for string methods
				if sym.Type == "string" {
					items = addStringMethods(items, prefix)
					return reply(ctx, protocol.CompletionList{IsIncomplete: false, Items: items}, nil)
				}

				// Check if it's an array type for array methods
				if sym.Type == "array" {
					items = addArrayMethods(items, prefix)
					return reply(ctx, protocol.CompletionList{IsIncomplete: false, Items: items}, nil)
				}

				// Check if it's a dict type for dictionary methods
				if sym.Type == "dict" {
					items = addDictMethods(items, prefix)
					return reply(ctx, protocol.CompletionList{IsIncomplete: false, Items: items}, nil)
				}
				
				// Check if it's a struct type (only after checking built-in types)
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
			}
		}
	}

	// If dot completion but no type found, return empty (no fallback)
	if isDotCompletion {
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

	// Add function completions from symbol table
	if doc.AST != nil {
		symbolTable := BuildSymbolTable(doc.AST)
		defer symbolTable.Clear()

		// Add user-defined functions
		for _, sym := range symbolTable.GlobalScope.Symbols {
			if sym.Kind == SymbolKindFunction {
				if prefix == "" || strings.HasPrefix(sym.Name, prefix) {
					// Build function signature for detail
					detail := "func"
					if sym.Type != "" && sym.Type != "void" {
						detail += " -> " + sym.Type
					}

					items = append(items, protocol.CompletionItem{
						Label:  sym.Name,
						Kind:   protocol.CompletionItemKindFunction,
						Detail: detail,
					})
				}
			}
		}

		// Add variables in scope
		for _, sym := range symbolTable.GlobalScope.Symbols {
			if sym.Kind == SymbolKindVariable {
				if prefix == "" || strings.HasPrefix(sym.Name, prefix) {
					items = append(items, protocol.CompletionItem{
						Label:  sym.Name,
						Kind:   protocol.CompletionItemKindVariable,
						Detail: sym.Type,
					})
				}
			}
		}

		// Add constants in scope
		for _, sym := range symbolTable.GlobalScope.Symbols {
			if sym.Kind == SymbolKindConstant {
				if prefix == "" || strings.HasPrefix(sym.Name, prefix) {
					items = append(items, protocol.CompletionItem{
						Label:  sym.Name,
						Kind:   protocol.CompletionItemKindConstant,
						Detail: sym.Type,
					})
				}
			}
		}

		// Add enum values
		for _, sym := range symbolTable.GlobalScope.Symbols {
			if sym.Kind == SymbolKindEnumValue {
				if prefix == "" || strings.HasPrefix(sym.Name, prefix) {
					items = append(items, protocol.CompletionItem{
						Label:  sym.Name,
						Kind:   protocol.CompletionItemKindEnumMember,
						Detail: sym.Type, // enum type name
					})
				}
			}
		}
	}

	result := protocol.CompletionList{
		IsIncomplete: false,
		Items:        items,
	}

	return reply(ctx, result, nil)
}

func isIdentifierChar(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

// Helper function to add string methods to completion items
func addStringMethods(items []protocol.CompletionItem, prefix string) []protocol.CompletionItem {
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
	return items
}

// Helper function to add array methods to completion items
func addArrayMethods(items []protocol.CompletionItem, prefix string) []protocol.CompletionItem {
	arrayMethods := []struct {
		label       string
		detail      string
		description string
		params      string
	}{
		{"length", "Get array length", "Returns the number of elements in the array", "||"},
		{"push", "Add element", "Adds an element to the end of the array", "|element|"},
		{"pop", "Remove last element", "Removes and returns the last element", "||"},
		{"sort", "Sort array", "Sorts the array in place", "||"},
		{"reverse", "Reverse array", "Reverses the array in place", "||"},
		{"contains", "Check if contains", "Returns true if array contains element", "|element|"},
		{"find", "Find element", "Returns index of element or -1", "|element|"},
		{"filter", "Filter array", "Returns new array with elements matching condition", "|condition|"},
		{"map", "Map array", "Returns new array with transformed elements", "|transform|"},
		{"join", "Join to string", "Joins array elements into a string", "|separator|"},
		{"slice", "Get subarray", "Returns a portion of the array", "|start, end|"},
	}
	
	for _, method := range arrayMethods {
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
	return items
}

// Helper function to add dict methods to completion items
func addDictMethods(items []protocol.CompletionItem, prefix string) []protocol.CompletionItem {
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
	return items
}
