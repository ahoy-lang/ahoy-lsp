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

func (s *Server) handleHover(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.HoverParams
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return reply(ctx, nil, err)
	}

	debugLog.Printf("Hover request at line %d, char %d", params.Position.Line, params.Position.Character)

	doc := s.getDocument(params.TextDocument.URI)
	if doc == nil {
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

	// Check if it's a C function/enum/define
	if doc.CHeaderGlobal != nil {
		// Check C functions
		for cFuncName, cFunc := range doc.CHeaderGlobal.Functions {
			if ahoy.PascalToSnake(cFuncName) == word {
				paramList := ""
				for i, param := range cFunc.Parameters {
					if i > 0 {
						paramList += ", "
					}
					paramList += param.Type
					if param.Name != "" {
						paramList += " " + param.Name
					}
				}
				
				hoverText := fmt.Sprintf("```c\n%s %s(%s)\n```\n\n", cFunc.ReturnType, cFuncName, paramList)
				hoverText += fmt.Sprintf("**C Function** from imported header\n\n")
				hoverText += fmt.Sprintf("Call as: `%s|...|`", word)
				
				hover := protocol.Hover{
					Contents: protocol.MarkupContent{
						Kind:  protocol.Markdown,
						Value: hoverText,
					},
				}
				return reply(ctx, hover, nil)
			}
		}
		
		// Check C enum VALUES (not enum names)
		foundEnumValue := false
		var enumValueInfo struct {
			value    int
			enumName string
		}
		for enumName, cEnum := range doc.CHeaderGlobal.Enums {
			if value, ok := cEnum.Values[word]; ok {
				foundEnumValue = true
				enumValueInfo.value = value
				enumValueInfo.enumName = enumName
				break
			}
		}
		
		if foundEnumValue {
			hoverText := fmt.Sprintf("```c\n%s\n```\n\n", word)
			hoverText += fmt.Sprintf("**C Enum Value** from %s\n\n", enumValueInfo.enumName)
			hoverText += fmt.Sprintf("Value: `%d`", enumValueInfo.value)
			
			hover := protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: hoverText,
				},
			}
			return reply(ctx, hover, nil)
		}
		
		// Check C defines
		if cDefine, ok := doc.CHeaderGlobal.Defines[word]; ok {
			hoverText := fmt.Sprintf("```c\n#define %s %s\n```\n\n", word, cDefine.Value)
			hoverText += "**C Define** from imported header"
			
			hover := protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: hoverText,
				},
			}
			return reply(ctx, hover, nil)
		}
		
		// Check C structs (case-insensitive match)
		for structName, cStruct := range doc.CHeaderGlobal.Structs {
			if ahoy.ToLowerFirst(structName) == word || structName == word {
				hoverText := fmt.Sprintf("```c\ntypedef struct %s {\n", structName)
				for _, field := range cStruct.Fields {
					hoverText += fmt.Sprintf("    %s %s;\n", field.Type, field.Name)
				}
				hoverText += fmt.Sprintf("} %s;\n```\n\n", structName)
				hoverText += fmt.Sprintf("**C Struct** from imported header\n\n")
				hoverText += fmt.Sprintf("Use as: `%s<field1: val1, field2: val2>`", word)
				
				hover := protocol.Hover{
					Contents: protocol.MarkupContent{
						Kind:  protocol.Markdown,
						Value: hoverText,
					},
				}
				return reply(ctx, hover, nil)
			}
		}
	}
	
	// Check namespaced C headers
	for _, headerInfo := range doc.CHeaders {
		// Check functions
		for cFuncName, cFunc := range headerInfo.Functions {
			if ahoy.PascalToSnake(cFuncName) == word {
				paramList := ""
				for i, param := range cFunc.Parameters {
					if i > 0 {
						paramList += ", "
					}
					paramList += param.Type
					if param.Name != "" {
						paramList += " " + param.Name
					}
				}
				
				hoverText := fmt.Sprintf("```c\n%s %s(%s)\n```\n\n", cFunc.ReturnType, cFuncName, paramList)
				hoverText += "**C Function** from namespaced import\n\n"
				hoverText += fmt.Sprintf("Call as: `%s|...|`", word)
				
				hover := protocol.Hover{
					Contents: protocol.MarkupContent{
						Kind:  protocol.Markdown,
						Value: hoverText,
					},
				}
				return reply(ctx, hover, nil)
			}
		}
		
		// Check enums
		if cEnum, ok := headerInfo.Enums[word]; ok {
			hoverText := fmt.Sprintf("```c\n%s\n```\n\n", word)
			hoverText += "**C Enum** from namespaced import\n\n"
			if len(cEnum.Values) > 0 {
				hoverText += "Values: "
				first := true
				for name := range cEnum.Values {
					if !first {
						hoverText += ", "
					}
					hoverText += fmt.Sprintf("`%s`", name)
					first = false
				}
			}
			
			hover := protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: hoverText,
				},
			}
			return reply(ctx, hover, nil)
		}
		
		// Check defines
		if cDefine, ok := headerInfo.Defines[word]; ok {
			hoverText := fmt.Sprintf("```c\n#define %s %s\n```\n\n", word, cDefine.Value)
			hoverText += "**C Define** from namespaced import"
			
			hover := protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: hoverText,
				},
			}
			return reply(ctx, hover, nil)
		}
		
		// Check structs
		for structName, cStruct := range headerInfo.Structs {
			if ahoy.ToLowerFirst(structName) == word || structName == word {
				hoverText := fmt.Sprintf("```c\ntypedef struct %s {\n", structName)
				for _, field := range cStruct.Fields {
					hoverText += fmt.Sprintf("    %s %s;\n", field.Type, field.Name)
				}
				hoverText += fmt.Sprintf("} %s;\n```\n\n", structName)
				hoverText += "**C Struct** from namespaced import\n\n"
				hoverText += fmt.Sprintf("Use as: `%s<field1: val1, field2: val2>`", word)
				
				hover := protocol.Hover{
					Contents: protocol.MarkupContent{
						Kind:  protocol.Markdown,
						Value: hoverText,
					},
				}
				return reply(ctx, hover, nil)
			}
		}
	}
	
	// Check if it's a built-in function
	if hoverText := getBuiltinFunctionHover(word); hoverText != "" {
		hover := protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: hoverText,
			},
		}
		return reply(ctx, hover, nil)
	}
	
	// Check if it's a built-in method (array, dict, or string)
	if hoverText := getMethodHover(word); hoverText != "" {
		hover := protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.Markdown,
				Value: hoverText,
			},
		}
		return reply(ctx, hover, nil)
	}

	// Look up Ahoy symbol in the symbol table
	// Use PackageSymbols if available (for multi-file packages), otherwise use regular SymbolTable
	var symbolTable *SymbolTable
	if doc.PackageSymbols != nil {
		symbolTable = doc.PackageSymbols
	} else {
		symbolTable = doc.SymbolTable
	}
	
	if symbolTable != nil {
		// Use LookupAtPosition to find symbols in the correct scope (including function-local variables)
		symbol := symbolTable.LookupAtPosition(word, int(params.Position.Line)+1, int(params.Position.Character))
		debugLog.Printf("Hover: Looking for '%s' at line %d - found: %v", word, int(params.Position.Line)+1, symbol != nil)
		if symbol == nil {
			debugLog.Printf("Hover: Symbol '%s' not found in symbol table", word)
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

	return reply(ctx, nil, nil)
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
		// Build parameter list
		params := []string{}
		for _, param := range symbol.Parameters {
			paramStr := param.Name
			if param.Type != "" && param.Type != "generic" {
				paramStr += ":" + param.Type
			}
			params = append(params, paramStr)
		}
		
		// Build signature: @ name :: |params| returnType:
		returnType := symbol.Type
		if returnType == "" {
			returnType = "void"
		}
		
		text = fmt.Sprintf("```ahoy\n@ %s :: |%s| %s:\n```\n\n",
			symbol.Name,
			strings.Join(params, ", "),
			returnType)
		text += fmt.Sprintf("**Function** `%s`\n\n", symbol.Name)
		if len(symbol.Parameters) > 0 {
			text += "**Parameters:**\n"
			for _, param := range symbol.Parameters {
				text += fmt.Sprintf("- `%s`: %s\n", param.Name, param.Type)
			}
			text += "\n"
		}
		if returnType != "void" {
			text += fmt.Sprintf("**Returns:** `%s`\n\n", returnType)
		}
		text += fmt.Sprintf("Defined at line %d", symbol.Line)

	case SymbolKindParameter:
		text = fmt.Sprintf("```ahoy\n%s: %s\n```\n\n", symbol.Name, symbol.Type)
		text += fmt.Sprintf("**Parameter** `%s`\n\n", symbol.Name)
		if symbol.Type != "" {
			text += fmt.Sprintf("Type: `%s`", symbol.Type)
		}

	case SymbolKindAlias:
		text = fmt.Sprintf("```ahoy\nalias %s: %s\n```\n\n", symbol.Name, symbol.Type)
		text += fmt.Sprintf("**Type Alias** `%s`\n\n", symbol.Name)
		text += fmt.Sprintf("Aliased type: `%s`\n\n", symbol.Type)
		text += fmt.Sprintf("Defined at line %d", symbol.Line)

	case SymbolKindUnion:
		text = fmt.Sprintf("```ahoy\nunion %s: %s\n```\n\n", symbol.Name, strings.ReplaceAll(symbol.Type, "|", ", "))
		text += fmt.Sprintf("**Union Type** `%s`\n\n", symbol.Name)
		text += fmt.Sprintf("Accepts types: `%s`\n\n", strings.ReplaceAll(symbol.Type, "|", "`, `"))
		text += fmt.Sprintf("Defined at line %d", symbol.Line)

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
		text = fmt.Sprintf("```ahoy\nstruct %s:\n", symbol.Name)
		if len(symbol.Fields) > 0 {
			for fieldName, field := range symbol.Fields {
				if len(field.Fields) > 0 {
					// Nested type
					text += fmt.Sprintf("  type %s:\n", fieldName)
					for nestedFieldName, nestedField := range field.Fields {
						text += fmt.Sprintf("    %s: %s", nestedFieldName, nestedField.Type)
						if nestedField.DefaultValue != "" {
							text += fmt.Sprintf(" = %s", nestedField.DefaultValue)
						}
						text += "\n"
					}
				} else {
					// Regular field
					text += fmt.Sprintf("  %s: %s", fieldName, field.Type)
					if field.DefaultValue != "" {
						text += fmt.Sprintf(" = %s", field.DefaultValue)
					}
					text += "\n"
				}
			}
		}
		text += "```\n\n"
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

func getBuiltinFunctionHover(funcName string) string {
	builtinFunctions := map[string]string{
		"read_json": "**read_json** - Read and parse JSON file\n\nSyntax: `result, err : read_json|\"file.json\"|`\n\nReturns: (AhoyJSON*, error string)\n\nExample:\n```ahoy\nconfig, err : read_json|\"config.json\"|\nif err then\n    print|err|\n    return\n$\nversion : config.version\n```",
		"write_json": "**write_json** - Write JSON to file\n\nSyntax: `err : write_json|\"file.json\", json_obj|`\n\nReturns: error string or NULL\n\nExample:\n```ahoy\nerr : write_json|\"output.json\", my_json|\n```",
		"ahoy_json_string": "**ahoy_json_string** - Extract string value from JSON\n\nSyntax: `str : ahoy_json_string|json_node|`\n\nReturns: String value\n\nExample:\n```ahoy\nversion : config.version\nver_str : ahoy_json_string|version|\nprint|ver_str|\n```",
		"ahoy_json_int": "**ahoy_json_int** - Extract integer value from JSON\n\nSyntax: `num : ahoy_json_int|json_node|`\n\nReturns: Integer value\n\nExample:\n```ahoy\ncount : config.count\ncount_val : ahoy_json_int|count|\n```",
		"ahoy_json_number": "**ahoy_json_number** - Extract number value from JSON\n\nSyntax: `num : ahoy_json_number|json_node|`\n\nReturns: Double/float value",
		"ahoy_json_bool": "**ahoy_json_bool** - Extract boolean value from JSON\n\nSyntax: `flag : ahoy_json_bool|json_node|`\n\nReturns: Boolean (0 or 1)",
		"ahoy_json_get": "**ahoy_json_get** - Get property from JSON object\n\nSyntax: `prop : ahoy_json_get|json_obj, \"key\"|`\n\nNote: Usually used automatically with dot notation: `json.property`",
		"ahoy_json_get_index": "**ahoy_json_get_index** - Get element from JSON array\n\nSyntax: `elem : ahoy_json_get_index|json_arr, index|`\n\nNote: Usually used automatically with array access: `json.arr[0]`",
	}
	
	if doc, ok := builtinFunctions[funcName]; ok {
		return fmt.Sprintf("```ahoy\n%s\n```\n\n%s\n\n**Built-in Function**", funcName, doc)
	}
	
	return ""
}

func getMethodHover(method string) string {
	// Array methods
	arrayMethods := map[string]string{
		"push":    "**push** - Add element(s) to array\n\nSyntax: `arr.push|value|` or `arr.push|val1, val2, ...|`\n\nReturns: Modified array",
		"pop":     "**pop** - Remove and return last element\n\nSyntax: `arr.pop||`\n\nReturns: Removed element",
		"length":  "**length** - Get array length\n\nSyntax: `arr.length||`\n\nReturns: Integer",
		"sum":     "**sum** - Sum all numeric elements\n\nSyntax: `arr.sum||`\n\nReturns: Integer",
		"has":     "**has** - Check if array contains value\n\nSyntax: `arr.has|value|`\n\nReturns: Boolean (0 or 1)",
		"sort":    "**sort** - Sort array in ascending order\n\nSyntax: `arr.sort||`\n\nReturns: Modified array",
		"reverse": "**reverse** - Reverse array order\n\nSyntax: `arr.reverse||`\n\nReturns: Modified array",
		"shuffle": "**shuffle** - Randomly shuffle array\n\nSyntax: `arr.shuffle||`\n\nReturns: Modified array",
		"pick":    "**pick** - Get random element\n\nSyntax: `arr.pick||`\n\nReturns: Random element",
		"fill":    "**fill** - Fill array with value\n\nSyntax: `arr.fill|value, count|`\n\nExample: `[].fill|-1, 4|` creates `[-1, -1, -1, -1]`\n\nReturns: Filled array",
		"map":     "**map** - Transform each element\n\nSyntax: `arr.map|lambda|`\n\nExample: `[1,2,3].map|x => x * 2|`\n\nReturns: New array",
		"filter":  "**filter** - Keep elements matching condition\n\nSyntax: `arr.filter|lambda|`\n\nExample: `[1,2,3,4].filter|x => x > 2|`\n\nReturns: New array",
	}
	
	// Dictionary methods
	dictMethods := map[string]string{
		"size":        "**size** - Get number of key-value pairs\n\nSyntax: `dict.size||`\n\nReturns: Integer",
		"has":         "**has** - Check if key exists\n\nSyntax: `dict.has|key|`\n\nReturns: Boolean",
		"has_all":     "**has_all** - Check if all keys exist\n\nSyntax: `dict.has_all|key1, key2, ...|`\n\nReturns: Boolean",
		"keys":        "**keys** - Get array of all keys\n\nSyntax: `dict.keys||`\n\nReturns: Array of strings",
		"values":      "**values** - Get array of all values\n\nSyntax: `dict.values||`\n\nReturns: Array",
		"clear":       "**clear** - Remove all entries\n\nSyntax: `dict.clear||`\n\nReturns: Void",
		"remove":      "**remove** - Delete key-value pair\n\nSyntax: `dict.remove|key|`\n\nReturns: Void",
		"merge":       "**merge** - Combine with another dict\n\nSyntax: `dict1.merge|dict2|`\n\nReturns: Merged dictionary",
		"sort":        "**sort** - Sort dict by keys\n\nSyntax: `dict.sort||`\n\nReturns: Sorted dictionary",
		"stable_sort": "**stable_sort** - Stable sort by keys\n\nSyntax: `dict.stable_sort||`\n\nReturns: Sorted dictionary",
	}
	
	// String methods
	stringMethods := map[string]string{
		"length":      "**length** - Get string length\n\nSyntax: `str.length||`\n\nReturns: Integer",
		"upper":       "**upper** - Convert to uppercase\n\nSyntax: `str.upper||`\n\nReturns: String",
		"lower":       "**lower** - Convert to lowercase\n\nSyntax: `str.lower||`\n\nReturns: String",
		"replace":     "**replace** - Replace substring\n\nSyntax: `str.replace|old, new|`\n\nReturns: Modified string",
		"contains":    "**contains** - Check if contains substring\n\nSyntax: `str.contains|substr|`\n\nReturns: Boolean",
		"match":       "**match** - Check if matches pattern\n\nSyntax: `str.match|pattern|`\n\nReturns: Boolean",
		"split":       "**split** - Split into array\n\nSyntax: `str.split|delimiter|`\n\nReturns: Array of strings",
		"count":       "**count** - Count substring occurrences\n\nSyntax: `str.count|substr|`\n\nReturns: Integer",
		"camel_case":  "**camel_case** - Convert to camelCase\n\nSyntax: `str.camel_case||`\n\nReturns: String",
		"snake_case":  "**snake_case** - Convert to snake_case\n\nSyntax: `str.snake_case||`\n\nReturns: String",
		"pascal_case": "**pascal_case** - Convert to PascalCase\n\nSyntax: `str.pascal_case||`\n\nReturns: String",
		"kebab_case":  "**kebab_case** - Convert to kebab-case\n\nSyntax: `str.kebab_case||`\n\nReturns: String",
		"lpad":        "**lpad** - Pad left to width\n\nSyntax: `str.lpad|width, padchar|`\n\nReturns: Padded string",
		"rpad":        "**rpad** - Pad right to width\n\nSyntax: `str.rpad|width, padchar|`\n\nReturns: Padded string",
		"pad":         "**pad** - Pad both sides to width\n\nSyntax: `str.pad|width, padchar|`\n\nReturns: Padded string",
		"strip":       "**strip** - Remove leading/trailing whitespace\n\nSyntax: `str.strip||`\n\nReturns: Trimmed string",
		"get_file":    "**get_file** - Extract filename from path\n\nSyntax: `path.get_file||`\n\nReturns: Filename string",
	}
	
	// Check all method types
	if doc, ok := arrayMethods[method]; ok {
		return fmt.Sprintf("```ahoy\n%s\n```\n\n%s\n\n**Array Method**", method, doc)
	}
	if doc, ok := dictMethods[method]; ok {
		return fmt.Sprintf("```ahoy\n%s\n```\n\n%s\n\n**Dictionary Method**", method, doc)
	}
	if doc, ok := stringMethods[method]; ok {
		return fmt.Sprintf("```ahoy\n%s\n```\n\n%s\n\n**String Method**", method, doc)
	}
	
	return ""
}
