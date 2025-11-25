package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ahoy"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (s *Server) handleCompletion(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.CompletionParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		debugLog.Printf("Failed to unmarshal params in completion.go: %v", err)
		return reply(ctx, nil, nil)
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

	// FIRST: Check if we're typing after a C header namespace prefix (e.g., "rl.")
	// This must be checked BEFORE general dot completion
	if params.Position.Character > 0 {
		checkPos := int(params.Position.Character) - len(prefix) - 1
		if checkPos >= 0 && checkPos < len(currentLine) && currentLine[checkPos] == '.' {
			// Found a dot, check if what's before it is a C header namespace
			identEnd := checkPos - 1
			identStart := identEnd
			for identStart >= 0 && (isIdentifierChar(rune(currentLine[identStart])) || currentLine[identStart] == '_') {
				identStart--
			}
			identStart++
			if identStart <= identEnd {
				possibleNamespace := currentLine[identStart : identEnd+1]

				// Check if this is a C header namespace
				if headerInfo, ok := doc.CHeaders[possibleNamespace]; ok {
					// Yes! Provide C header namespace completions
					items := []protocol.CompletionItem{}

					// Add namespace functions (snake_case)
					for funcName, cFunc := range headerInfo.Functions {
						snakeName := ahoy.PascalToSnake(funcName)
						if prefix == "" || strings.HasPrefix(snakeName, prefix) {
							paramList := ""
							for i, param := range cFunc.Parameters {
								if i > 0 {
									paramList += ", "
								}
								if param.Name != "" {
									paramList += param.Name
								} else {
									paramList += param.Type
								}
							}

							items = append(items, protocol.CompletionItem{
								Label:  snakeName,
								Kind:   protocol.CompletionItemKindFunction,
								Detail: fmt.Sprintf("%s -> %s", paramList, cFunc.ReturnType),
							})
						}
					}

					// Add namespace enums
					for enumName := range headerInfo.Enums {
						if prefix == "" || strings.HasPrefix(enumName, prefix) {
							items = append(items, protocol.CompletionItem{
								Label:  enumName,
								Kind:   protocol.CompletionItemKindEnumMember,
								Detail: "C enum",
							})
						}
					}

					// Add namespace defines
					for defineName := range headerInfo.Defines {
						if prefix == "" || strings.HasPrefix(defineName, prefix) {
							items = append(items, protocol.CompletionItem{
								Label:  defineName,
								Kind:   protocol.CompletionItemKindConstant,
								Detail: "C define",
							})
						}
					}

					// Add namespace structs (keep exact C name, case-insensitive matching)
					for structName := range headerInfo.Structs {
						if prefix == "" || strings.HasPrefix(strings.ToLower(structName), strings.ToLower(prefix)) {
							items = append(items, protocol.CompletionItem{
								Label:  structName,
								Kind:   protocol.CompletionItemKindStruct,
								Detail: fmt.Sprintf("C struct %s", structName),
							})
						}
					}

					// Return early with namespace completions
					result := protocol.CompletionList{
						IsIncomplete: false,
						Items:        items,
					}
					return reply(ctx, result, nil)
				}

				// Check if this is a C enum type (e.g., ConfigFlags.)
				// Check in global header
				if doc.CHeaderGlobal != nil {
					if cEnum, ok := doc.CHeaderGlobal.Enums[possibleNamespace]; ok {
						// Yes! Provide enum member completions
						items := []protocol.CompletionItem{}
						for valueName, value := range cEnum.Values {
							if prefix == "" || strings.HasPrefix(valueName, prefix) {
								items = append(items, protocol.CompletionItem{
									Label:  valueName,
									Kind:   protocol.CompletionItemKindEnumMember,
									Detail: fmt.Sprintf("= %d", value),
								})
							}
						}
						result := protocol.CompletionList{
							IsIncomplete: false,
							Items:        items,
						}
						return reply(ctx, result, nil)
					}
				}

				// Check in namespaced headers
				for _, headerInfo := range doc.CHeaders {
					if cEnum, ok := headerInfo.Enums[possibleNamespace]; ok {
						items := []protocol.CompletionItem{}
						for valueName, value := range cEnum.Values {
							if prefix == "" || strings.HasPrefix(valueName, prefix) {
								items = append(items, protocol.CompletionItem{
									Label:  valueName,
									Kind:   protocol.CompletionItemKindEnumMember,
									Detail: fmt.Sprintf("= %d", value),
								})
							}
						}
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
			} else if identEnd >= 0 && currentLine[identEnd] == '>' {
				// Check if it's a dict literal <...>
				angleCount := 1
				identStart = identEnd - 1
				for identStart >= 0 && angleCount > 0 {
					if currentLine[identStart] == '>' {
						angleCount++
					} else if currentLine[identStart] == '<' {
						angleCount--
					}
					identStart--
				}
				identStart++ // Move back to the '<'
				if identStart >= 0 && currentLine[identStart] == '<' {
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

		// Get symbol table to look up the type
		// Use PackageSymbols if available (for multi-file packages), otherwise use regular SymbolTable
		var symbolTable *SymbolTable
		if doc.PackageSymbols != nil {
			symbolTable = doc.PackageSymbols
		} else if doc.SymbolTable != nil {
			symbolTable = doc.SymbolTable
		} else if doc.AST != nil {
			symbolTable = BuildSymbolTable(doc.AST)
			defer symbolTable.Clear()
		}

		if symbolTable != nil {
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

				// Check if it's an enum type
				if sym.Kind == SymbolKindEnum {
					// Add enum member completions
					for memberName, field := range sym.Fields {
						if prefix == "" || strings.HasPrefix(memberName, prefix) {
							// field.Type contains the enum value (or "auto")
							detailText := fmt.Sprintf("enum %s", beforePrefix)
							if field.Type != "" && field.Type != "auto" {
								detailText = fmt.Sprintf("%s:%s %s", memberName, field.Type, detailText)
							} else {
								detailText = fmt.Sprintf("%s %s", memberName, detailText)
							}

							items = append(items, protocol.CompletionItem{
								Label:  memberName,
								Kind:   protocol.CompletionItemKindEnumMember,
								Detail: detailText,
							})
						}
					}
					return reply(ctx, protocol.CompletionList{IsIncomplete: false, Items: items}, nil)
				}

				// Check if it's an object literal type
				if sym.Type == "object" && sym.Fields != nil && len(sym.Fields) > 0 {
					// Add object literal property completions
					for fieldName, field := range sym.Fields {
						if prefix == "" || strings.HasPrefix(fieldName, prefix) {
							items = append(items, protocol.CompletionItem{
								Label:  fieldName,
								Kind:   protocol.CompletionItemKindProperty,
								Detail: field.Type,
							})
						}
					}

					// Return early with object literal property completions
					result := protocol.CompletionList{
						IsIncomplete: false,
						Items:        items,
					}
					return reply(ctx, result, nil)
				}

				// Check if it's a struct type (only after checking built-in types and object literals)
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
	// Use PackageSymbols if available (for multi-file packages), otherwise use regular SymbolTable
	var symbolTable *SymbolTable
	if doc.PackageSymbols != nil {
		symbolTable = doc.PackageSymbols
	} else if doc.SymbolTable != nil {
		symbolTable = doc.SymbolTable
	} else if doc.AST != nil {
		symbolTable = BuildSymbolTable(doc.AST)
		defer symbolTable.Clear()
	}

	if symbolTable != nil {
		// Add user-defined functions
		for _, sym := range symbolTable.GlobalScope.Symbols {
			if sym.Kind == SymbolKindFunction {
				if prefix == "" || strings.HasPrefix(sym.Name, prefix) {
					// Build function signature: name(param:type, ...) -> returnType
					params := []string{}
					for _, param := range sym.Parameters {
						paramStr := param.Name
						if param.Type != "" && param.Type != "generic" {
							paramStr += ":" + param.Type
						}
						params = append(params, paramStr)
					}

					detail := sym.Name + "(" + strings.Join(params, ", ") + ")"
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

		// Add built-in functions
		builtinFuncs := map[string]string{
			"read_json":           "(filename:string) -> (AhoyJSON*, error:string)",
			"write_json":          "(filename:string, json:AhoyJSON*) -> error:string",
			"ahoy_json_string":    "(json:AhoyJSON*) -> string",
			"ahoy_json_int":       "(json:AhoyJSON*) -> int",
			"ahoy_json_number":    "(json:AhoyJSON*) -> float",
			"ahoy_json_bool":      "(json:AhoyJSON*) -> bool",
			"ahoy_json_get":       "(json:AhoyJSON*, key:string) -> AhoyJSON*",
			"ahoy_json_get_index": "(json:AhoyJSON*, index:int) -> AhoyJSON*",
			"print":               "(value) -> void",
			"log":                 "(message, file_path:string) -> void",
			"panic":               "(error) -> void",
			"sprintf":             "(format:string, ...) -> string",
		}
		for funcName, signature := range builtinFuncs {
			if prefix == "" || strings.HasPrefix(funcName, prefix) {
				items = append(items, protocol.CompletionItem{
					Label:  funcName,
					Kind:   protocol.CompletionItemKindFunction,
					Detail: signature,
				})
			}
		}

		// Add variables in scope (including local variables)
		visibleSymbols := symbolTable.GetVisibleSymbols(int(params.Position.Line) + 1)
		debugLog.Printf("Completion: Found %d visible symbols at line %d, prefix='%s'", len(visibleSymbols), int(params.Position.Line)+1, prefix)
		for _, sym := range visibleSymbols {
			if sym.Kind == SymbolKindVariable {
				if prefix == "" || strings.HasPrefix(sym.Name, prefix) {
					debugLog.Printf("Completion: Adding variable '%s' (type: %s)", sym.Name, sym.Type)
					items = append(items, protocol.CompletionItem{
						Label:  sym.Name,
						Kind:   protocol.CompletionItemKindVariable,
						Detail: sym.Type,
					})
				}
			}
		}

		// Add constants in scope
		for _, sym := range visibleSymbols {
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
		for _, sym := range visibleSymbols {
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

		// Add type aliases
		for _, sym := range symbolTable.GlobalScope.Symbols {
			if sym.Kind == SymbolKindAlias {
				if prefix == "" || strings.HasPrefix(sym.Name, prefix) {
					items = append(items, protocol.CompletionItem{
						Label:  sym.Name,
						Kind:   protocol.CompletionItemKindClass,
						Detail: fmt.Sprintf("alias -> %s", sym.Type),
					})
				}
			}
		}

		// Add union types
		for _, sym := range symbolTable.GlobalScope.Symbols {
			if sym.Kind == SymbolKindUnion {
				if prefix == "" || strings.HasPrefix(sym.Name, prefix) {
					items = append(items, protocol.CompletionItem{
						Label:  sym.Name,
						Kind:   protocol.CompletionItemKindClass,
						Detail: fmt.Sprintf("union (%s)", strings.ReplaceAll(sym.Type, "|", ", ")),
					})
				}
			}
		}
	}

	// Add C header functions and enums
	if doc.CHeaderGlobal != nil {
		// Add global C functions (snake_case names)
		for funcName, cFunc := range doc.CHeaderGlobal.Functions {
			snakeName := ahoy.PascalToSnake(funcName)
			if prefix == "" || strings.HasPrefix(snakeName, prefix) {
				// Build parameter list
				paramList := ""
				for i, param := range cFunc.Parameters {
					if i > 0 {
						paramList += ", "
					}
					if param.Name != "" {
						paramList += param.Name
					} else {
						paramList += param.Type
					}
				}

				items = append(items, protocol.CompletionItem{
					Label:  snakeName,
					Kind:   protocol.CompletionItemKindFunction,
					Detail: fmt.Sprintf("%s -> %s", paramList, cFunc.ReturnType),
				})
			}
		}

		// Add global C enum VALUES (not enum names)
		debugLog.Printf("Completion: Checking C enum values, have %d enums, prefix='%s'", len(doc.CHeaderGlobal.Enums), prefix)
		for enumName, enum := range doc.CHeaderGlobal.Enums {
			debugLog.Printf("Completion: Processing enum %s with %d values", enumName, len(enum.Values))
			for valueName, value := range enum.Values {
				if prefix == "" || strings.HasPrefix(valueName, prefix) {
					debugLog.Printf("Completion: Adding enum value %s = %d", valueName, value)
					items = append(items, protocol.CompletionItem{
						Label:  valueName,
						Kind:   protocol.CompletionItemKindEnumMember,
						Detail: fmt.Sprintf("C enum value = %d", value),
					})
				}
			}
		}

		// Add global C defines
		for defineName := range doc.CHeaderGlobal.Defines {
			if prefix == "" || strings.HasPrefix(defineName, prefix) {
				items = append(items, protocol.CompletionItem{
					Label:  defineName,
					Kind:   protocol.CompletionItemKindConstant,
					Detail: "C define",
				})
			}
		}

		// Add global C structs (keep exact C name, case-insensitive matching)
		for structName := range doc.CHeaderGlobal.Structs {
			if prefix == "" || strings.HasPrefix(strings.ToLower(structName), strings.ToLower(prefix)) {
				items = append(items, protocol.CompletionItem{
					Label:  structName,
					Kind:   protocol.CompletionItemKindStruct,
					Detail: fmt.Sprintf("C struct %s", structName),
				})
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
		{"shuffle", "Shuffle array", "Randomly shuffles the array in place", "||"},
		{"pick", "Pick random element", "Returns a random element from the array", "||"},
		{"contains", "Check if contains", "Returns true if array contains element", "|element|"},
		{"has", "Check if has element", "Returns true if array has the element", "|element|"},
		{"sum", "Sum elements", "Returns the sum of all numeric elements", "||"},
		{"find", "Find element", "Returns index of element or -1", "|element|"},
		{"filter", "Filter array", "Returns new array with elements matching condition", "|condition|"},
		{"map", "Map array", "Returns new array with transformed elements", "|transform|"},
		{"join", "Join to string", "Joins array elements into a string", "|separator|"},
		{"slice", "Get subarray", "Returns a portion of the array", "|start, end|"},
		{"fill", "Fill array", "Creates array filled with value. Example: [].fill|-1, 4|", "|value, count|"},
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
		{"remove", "Remove key", "Removes a key-value pair from the dictionary", "|key|"},
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
