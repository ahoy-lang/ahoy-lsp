package main

import (
	"context"
	"encoding/json"
	"fmt"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (s *Server) handleHover(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.HoverParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	debugLog.Printf("Hover request at line %d, char %d", params.Position.Line, params.Position.Character)

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil || doc.SymbolTable == nil {
		return reply(ctx, nil, nil)
	}

	// Safety check: prevent processing huge files
	if len(doc.Content) > 1000000 {
		return reply(ctx, nil, nil)
	}

	// Validate position bounds
	if int(params.Position.Line) < 0 || int(params.Position.Character) < 0 {
		return reply(ctx, nil, nil)
	}

	// Get the word at the cursor position
	word := getWordAtPosition(doc, int(params.Position.Line), int(params.Position.Character))
	if word == "" {
		return reply(ctx, nil, nil)
	}

	debugLog.Printf("Hover word: %s", word)

	// Look up the symbol
	symbol := doc.SymbolTable.Lookup(word)
	if symbol == nil {
		// Check if it's a keyword
		if hoverText := getKeywordHover(word); hoverText != "" {
			hover := protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: hoverText,
				},
			}
			return reply(ctx, hover, nil)
		}
		return reply(ctx, nil, nil)
	}

	// Build hover content
	hoverText := buildHoverText(symbol)

	hover := protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: hoverText,
		},
		Range: &protocol.Range{
			Start: protocol.Position{
				Line:      uint32(symbol.Line - 1),
				Character: uint32(symbol.Column),
			},
			End: protocol.Position{
				Line:      uint32(symbol.Line - 1),
				Character: uint32(symbol.Column + len(symbol.Name)),
			},
		},
	}

	return reply(ctx, hover, nil)
}

func buildHoverText(symbol *Symbol) string {
	var text string

	switch symbol.Kind {
	case SymbolKindVariable:
		text = fmt.Sprintf("```ahoy\n%s: %s\n```\n\n", symbol.Name, symbol.Type)
		text += fmt.Sprintf("**Variable** `%s`\n\n", symbol.Name)
		if symbol.Type != "" {
			text += fmt.Sprintf("Type: `%s`\n\n", symbol.Type)
		}
		text += fmt.Sprintf("Defined at line %d", symbol.Line)

	case SymbolKindFunction:
		text = fmt.Sprintf("```ahoy\nfunc %s", symbol.Name)
		if symbol.Type != "" {
			text += fmt.Sprintf(" -> %s", symbol.Type)
		}
		text += "\n```\n\n"
		text += fmt.Sprintf("**Function** `%s`\n\n", symbol.Name)
		if symbol.Type != "" {
			text += fmt.Sprintf("Returns: `%s`\n\n", symbol.Type)
		}
		text += fmt.Sprintf("Defined at line %d", symbol.Line)

	case SymbolKindParameter:
		text = fmt.Sprintf("```ahoy\n%s: %s\n```\n\n", symbol.Name, symbol.Type)
		text += fmt.Sprintf("**Parameter** `%s`\n\n", symbol.Name)
		if symbol.Type != "" {
			text += fmt.Sprintf("Type: `%s`", symbol.Type)
		}

	case SymbolKindEnum:
		text = fmt.Sprintf("```ahoy\n%s enum\n```\n\n", symbol.Name)
		text += fmt.Sprintf("**Enum** `%s`\n\n", symbol.Name)
		text += fmt.Sprintf("Defined at line %d", symbol.Line)

	case SymbolKindEnumValue:
		text = fmt.Sprintf("```ahoy\n%s\n```\n\n", symbol.Name)
		text += fmt.Sprintf("**Enum Value** `%s`\n\n", symbol.Name)
		if symbol.Type != "" {
			text += fmt.Sprintf("From enum: `%s`\n\n", symbol.Type)
		}
		text += fmt.Sprintf("Defined at line %d", symbol.Line)

	case SymbolKindStruct:
		text = fmt.Sprintf("```ahoy\n%s struct\n```\n\n", symbol.Name)
		text += fmt.Sprintf("**Struct** `%s`\n\n", symbol.Name)
		text += fmt.Sprintf("Defined at line %d", symbol.Line)

	case SymbolKindStructField:
		text = fmt.Sprintf("```ahoy\n%s: %s\n```\n\n", symbol.Name, symbol.Type)
		text += fmt.Sprintf("**Field** `%s`\n\n", symbol.Name)
		if symbol.Type != "" {
			text += fmt.Sprintf("Type: `%s`", symbol.Type)
		}

	case SymbolKindConstant:
		text = fmt.Sprintf("```ahoy\n%s :: %s\n```\n\n", symbol.Name, symbol.Type)
		text += fmt.Sprintf("**Constant** `%s`\n\n", symbol.Name)
		if symbol.Type != "" {
			text += fmt.Sprintf("Type: `%s`\n\n", symbol.Type)
		}
		text += fmt.Sprintf("Defined at line %d", symbol.Line)

	default:
		text = fmt.Sprintf("**%s**\n\nDefined at line %d", symbol.Name, symbol.Line)
	}

	return text
}

func getKeywordHover(keyword string) string {
	keywordDocs := map[string]string{
		"if":      "**if** - Conditional statement\n\nSyntax: `if condition then ... end`",
		"else":    "**else** - Alternative branch in conditional\n\nSyntax: `if condition then ... else ... end`",
		"elseif":  "**elseif** - Additional condition in if statement\n\nSyntax: `if cond1 then ... elseif cond2 then ... end`",
		"anif":    "**anif** - Alternative to elseif\n\nSyntax: `if cond1 then ... anif cond2 then ... end`",
		"then":    "**then** - Begins the body of a conditional or loop",
		"do":      "**do** - Begins the body of a loop or function",
		"end":     "**end** - Closes a block (if, loop, func, etc.)",
		"loop":    "**loop** - Loop statement\n\nSyntax:\n- `loop condition do ... end`\n- `loop i:start to end do ... end`\n- `loop element in array do ... end`",
		"in":      "**in** - Used in for-in loops\n\nSyntax: `loop element in array do ... end`",
		"to":      "**to** - Range operator in loops\n\nSyntax: `loop i:1 to 10 do ... end`",
		"func":    "**func** - Function definition\n\nSyntax: `func name param1 type1 param2 type2 do ... end`",
		"return":  "**return** - Return from function\n\nSyntax: `return value`",
		"break":   "**break** - Exit from loop",
		"skip":    "**skip** - Continue to next loop iteration (like continue)",
		"switch":  "**switch** - Switch statement\n\nSyntax: `switch value on case1 do ... case2 do ... end`",
		"on":      "**on** - Used in switch statements",
		"when":    "**when** - Compile-time conditional\n\nSyntax: `when CONDITION do ... end`",
		"import":  "**import** - Import external library\n\nSyntax: `import \"library.h\"`",
		"ahoy":    "**ahoy** - Print statement (shorthand for print)\n\nSyntax: `ahoy \"Hello!\"`",
		"is":      "**is** - Equality operator (==)\n\nSyntax: `if x is 5 then ... end`",
		"not":     "**not** - Logical NOT operator (!)\n\nSyntax: `if not condition then ... end`",
		"and":     "**and** - Logical AND operator (&&)\n\nSyntax: `if cond1 and cond2 then ... end`",
		"or":      "**or** - Logical OR operator (||)\n\nSyntax: `if cond1 or cond2 then ... end`",
		"true":    "**true** - Boolean true value",
		"false":   "**false** - Boolean false value",
		"enum":    "**enum** - Enumeration definition\n\nSyntax: `name enum: VALUE1 VALUE2 VALUE3 end`",
		"struct":  "**struct** - Structure definition\n\nSyntax: `name struct: field1 type1 field2 type2 end`",
		"type":    "**type** - Type alias",
		"int":     "**int** - Integer type",
		"float":   "**float** - Floating-point number type",
		"string":  "**string** - String type",
		"bool":    "**bool** - Boolean type",
		"dict":    "**dict** - Dictionary/map type",
		"plus":    "**plus** - Addition operator (+)\n\nSyntax: `result: a plus b`",
		"minus":   "**minus** - Subtraction operator (-)\n\nSyntax: `result: a minus b`",
		"times":   "**times** - Multiplication operator (*)\n\nSyntax: `result: a times b`",
		"div":     "**div** - Division operator (/)\n\nSyntax: `result: a div b`",
		"mod":     "**mod** - Modulo operator (%)\n\nSyntax: `result: a mod b`",
		"lesser":  "**lesser** - Less than operator (<)\n\nSyntax: `if a lesser b then ... end`",
		"greater": "**greater** - Greater than operator (>)\n\nSyntax: `if a greater b then ... end`",
	}

	if doc, ok := keywordDocs[keyword]; ok {
		return doc
	}
	return ""
}
