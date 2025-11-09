package main

import (
	"fmt"
	"strconv"
	"strings"

	"ahoy"
)

// Symbol represents a symbol in the code (variable, function, type, etc.)
type Symbol struct {
	Name       string
	Kind       SymbolKind
	Type       string
	Line       int
	Column     int
	EndLine    int
	EndColumn  int
	Fields     map[string]*StructField // For struct types, stores fields and nested types
	Parameters []ParameterInfo         // For functions, stores parameter information
	// Don't store Definition node or Scope to prevent memory leaks - AST can't be GC'd
}

// StructField represents a field in a struct (can be a regular field or nested type)
type StructField struct {
	Name         string
	Type         string
	Fields       map[string]*StructField // For nested types
	DefaultValue string                  // String representation of default value
}

type SymbolKind int

const (
	SymbolKindVariable SymbolKind = iota
	SymbolKindFunction
	SymbolKindParameter
	SymbolKindEnum
	SymbolKindEnumValue
	SymbolKindStruct
	SymbolKindStructField
	SymbolKindConstant
	SymbolKindCFunction  // C function from imported header
	SymbolKindCEnum      // C enum from imported header
	SymbolKindCDefine    // C #define from imported header
)

// Scope represents a lexical scope
type Scope struct {
	Parent    *Scope
	Symbols   map[string]*Symbol
	Children  []*Scope
	StartLine int
	EndLine   int
}

func NewScope(parent *Scope) *Scope {
	return &Scope{
		Parent:  parent,
		Symbols: make(map[string]*Symbol),
	}
}

func (s *Scope) AddSymbol(symbol *Symbol) {
	s.Symbols[symbol.Name] = symbol
	// Don't set symbol.Scope to avoid circular references
}

func (s *Scope) Lookup(name string) *Symbol {
	// Look in current scope
	if sym, ok := s.Symbols[name]; ok {
		return sym
	}
	// Look in parent scope
	if s.Parent != nil {
		return s.Parent.Lookup(name)
	}
	return nil
}

func (s *Scope) LookupLocal(name string) *Symbol {
	return s.Symbols[name]
}

// SymbolTable manages all symbols in a document
type SymbolTable struct {
	GlobalScope  *Scope
	CurrentScope *Scope
	AST          *ahoy.ASTNode // Store AST root for function lookups
	Doc          *Document     // Store document for type inference
}

func NewSymbolTable() *SymbolTable {
	global := NewScope(nil)
	return &SymbolTable{
		GlobalScope:  global,
		CurrentScope: global,
	}
}

// Clear breaks circular references to help garbage collection
func (st *SymbolTable) Clear() {
	if st.GlobalScope != nil {
		st.clearScope(st.GlobalScope)
	}
	st.GlobalScope = nil
	st.CurrentScope = nil
}

func (st *SymbolTable) clearScope(scope *Scope) {
	if scope == nil {
		return
	}

	// Clear symbols map
	for k := range scope.Symbols {
		delete(scope.Symbols, k)
	}
	scope.Symbols = nil

	// Recursively clear children
	for _, child := range scope.Children {
		st.clearScope(child)
	}
	scope.Children = nil
	scope.Parent = nil
}

func (st *SymbolTable) EnterScope() {
	newScope := NewScope(st.CurrentScope)
	st.CurrentScope.Children = append(st.CurrentScope.Children, newScope)
	st.CurrentScope = newScope
}

func (st *SymbolTable) ExitScope() {
	if st.CurrentScope.Parent != nil {
		st.CurrentScope = st.CurrentScope.Parent
	}
}

func (st *SymbolTable) AddSymbol(symbol *Symbol) {
	st.CurrentScope.AddSymbol(symbol)
}

func (st *SymbolTable) Lookup(name string) *Symbol {
	return st.CurrentScope.Lookup(name)
}

func (st *SymbolTable) FindSymbolAtPosition(line, column int) *Symbol {
	return st.findSymbolInScope(st.GlobalScope, line, column)
}

func (st *SymbolTable) findSymbolInScope(scope *Scope, line, column int) *Symbol {
	if scope == nil {
		return nil
	}

	// Check symbols in current scope
	for _, sym := range scope.Symbols {
		if sym.Line == line && sym.Column <= column && column < sym.Column+len(sym.Name) {
			return sym
		}
	}

	// Check child scopes (limit depth to prevent stack overflow)
	for _, child := range scope.Children {
		if child != nil && line >= child.StartLine && line <= child.EndLine {
			if sym := st.findSymbolInScope(child, line, column); sym != nil {
				return sym
			}
		}
	}

	return nil
}

// BuildSymbolTable walks the AST and builds the symbol table
func BuildSymbolTable(ast *ahoy.ASTNode) *SymbolTable {
	if ast == nil {
		return NewSymbolTable()
	}

	st := NewSymbolTable()
	st.AST = ast // Store AST for lookups
	st.walkNode(ast, 0)
	return st
}

func (st *SymbolTable) walkNode(node *ahoy.ASTNode, depth int) {
	if node == nil {
		return
	}

	// Prevent excessive recursion depth to avoid stack overflow and memory issues
	if depth > 1000 {
		debugLog.Printf("WARNING: Maximum recursion depth reached at depth %d", depth)
		return
	}

	// Prevent cycles - check if we have too many children
	if len(node.Children) > 1000 {
		debugLog.Printf("WARNING: Node has too many children: %d", len(node.Children))
		return
	}

	switch node.Type {
	case ahoy.NODE_PROGRAM:
		for _, child := range node.Children {
			st.walkNode(child, depth+1)
		}

	case ahoy.NODE_FUNCTION:
		// Add function to symbol table
		funcName := node.Value
		symbol := &Symbol{
			Name:       funcName,
			Kind:       SymbolKindFunction,
			Type:       node.DataType,
			Line:       node.Line,
			Column:     0,
			Parameters: []ParameterInfo{},
		}
		
		// Collect parameters
		if len(node.Children) > 0 {
			params := node.Children[0]
			if params != nil && params.Type == ahoy.NODE_BLOCK {
				for _, paramNode := range params.Children {
					if paramNode.Type == ahoy.NODE_IDENTIFIER {
						symbol.Parameters = append(symbol.Parameters, ParameterInfo{
							Name:       paramNode.Value,
							Type:       paramNode.DataType,
							HasDefault: paramNode.DefaultValue != nil,
						})
					}
				}
			}
		}
		
		st.AddSymbol(symbol)

		// Enter function scope
		st.EnterScope()
		st.CurrentScope.StartLine = node.Line

		// Add parameters as symbols in function scope
		if len(node.Children) > 0 {
			params := node.Children[0]
			if params != nil && params.Type == ahoy.NODE_BLOCK {
				// Each parameter is a NODE_IDENTIFIER with Value=name and DataType=type
				for _, paramNode := range params.Children {
					if paramNode.Type == ahoy.NODE_IDENTIFIER {
						paramSymbol := &Symbol{
							Name:   paramNode.Value,
							Kind:   SymbolKindParameter,
							Type:   paramNode.DataType,
							Line:   paramNode.Line,
							Column: 0,
						}
						st.AddSymbol(paramSymbol)
					}
				}
			}
		}

		// Walk function body
		if len(node.Children) > 1 {
			bodyNode := node.Children[1]
			st.walkNode(bodyNode, depth+1)
			
			// Try to set end line from body
			if bodyNode != nil && bodyNode.Line > 0 {
				st.CurrentScope.EndLine = findLastLine(bodyNode)
			}
		}

		st.ExitScope()

	case ahoy.NODE_VARIABLE_DECLARATION, ahoy.NODE_ASSIGNMENT:
		varName := node.Value
		varType := node.DataType

		// Try to infer type from value if not specified
		if varType == "" && len(node.Children) > 0 {
			varType = st.inferType(node.Children[0])
		}

		symbol := &Symbol{
			Name:   varName,
			Kind:   SymbolKindVariable,
			Type:   varType,
			Line:   node.Line,
			Column: 0,
		}
		
		// Extract object literal properties if the value is an object literal
		if len(node.Children) > 0 && node.Children[0].Type == ahoy.NODE_OBJECT_LITERAL {
			symbol.Fields = st.extractObjectLiteralFields(node.Children[0])
		}
		
		st.AddSymbol(symbol)

		// Walk the value expression
		if len(node.Children) > 0 {
			st.walkNode(node.Children[0], depth+1)
		}

	case ahoy.NODE_TUPLE_ASSIGNMENT:
		// Handle tuple assignments like: a, b: func|args|
		// Children[0] = leftSide (block with identifiers)
		// Children[1] = rightSide (block with function call or expressions)
		if len(node.Children) >= 2 {
			leftSide := node.Children[0]
			rightSide := node.Children[1]
			
			// Always register the variables on the left side
			// The parser will handle validation, so we just need to track them
			for _, leftChild := range leftSide.Children {
				if leftChild.Type == ahoy.NODE_IDENTIFIER {
					varName := leftChild.Value
					varType := "any" // Default type
					
					// Try to infer type from right side if it's a function call
					if len(rightSide.Children) == 1 && rightSide.Children[0].Type == ahoy.NODE_CALL {
						callNode := rightSide.Children[0]
						funcName := callNode.Value
						
						// Look up function to get return types
						funcSymbol := st.Lookup(funcName)
						if funcSymbol != nil {
							returnTypeStr := funcSymbol.Type
							
							// If function has infer/generic return type, actually infer it
							if returnTypeStr == "infer" || returnTypeStr == "" || returnTypeStr == "generic" {
								// Find the function node in the AST
								var funcNode *ahoy.ASTNode
								var findFunc func(*ahoy.ASTNode)
								findFunc = func(n *ahoy.ASTNode) {
									if n == nil {
										return
									}
									if n.Type == ahoy.NODE_FUNCTION && n.Value == funcName {
										funcNode = n
										return
									}
									for _, child := range n.Children {
										findFunc(child)
									}
								}
								if st.AST != nil {
									findFunc(st.AST)
								}
								
								// Infer argument types from the call
								var argTypes []string
								for _, arg := range callNode.Children {
									argType := inferExpressionType(arg, st.Doc)
									argTypes = append(argTypes, argType)
								}
								
								// Infer actual return types by tracing through function
								if funcNode != nil && st.Doc != nil {
									inferredTypes := inferFunctionReturnTypes(funcNode, argTypes, st.Doc)
									if len(inferredTypes) > 0 {
										returnTypeStr = strings.Join(inferredTypes, ",")
									}
								}
							}
							
							// Parse and assign return types
							if returnTypeStr != "" && returnTypeStr != "void" {
								returnTypes := strings.Split(returnTypeStr, ",")
								
								// Find which position this variable is at
								varIndex := -1
								for i, child := range leftSide.Children {
									if child == leftChild {
										varIndex = i
										break
									}
								}
								
								// Use the corresponding return type if available
								if varIndex >= 0 && varIndex < len(returnTypes) {
									varType = strings.TrimSpace(returnTypes[varIndex])
								}
							}
						}
					}
					
					symbol := &Symbol{
						Name:   varName,
						Kind:   SymbolKindVariable,
						Type:   varType,
						Line:   leftChild.Line,
						Column: 0,
					}
					st.AddSymbol(symbol)
				}
			}
			
			// Walk right side to process the function call
			st.walkNode(rightSide, depth+1)
		}

	case ahoy.NODE_ENUM_DECLARATION:
		enumName := node.Value
		symbol := &Symbol{
			Name:   enumName,
			Kind:   SymbolKindEnum,
			Type:   "enum",
			Line:   node.Line,
			Column: 0,
			Fields: make(map[string]*StructField),
		}

		// Add enum values as symbols AND track as fields for completion
		nextAutoValue := 0
		for _, child := range node.Children {
			if child.Type == ahoy.NODE_IDENTIFIER {
				memberName := child.Value
				memberValue := child.DataType
				
				// Calculate actual value
				actualValue := ""
				if memberValue == "" {
					// Auto-increment value
					actualValue = fmt.Sprintf("%d", nextAutoValue)
					nextAutoValue++
				} else {
					// Custom value
					actualValue = memberValue
					// Try to parse for next auto value
					if val, err := strconv.Atoi(memberValue); err == nil {
						nextAutoValue = val + 1
					}
				}
				
				// Add as field for completion
				symbol.Fields[memberName] = &StructField{
					Name: memberName,
					Type: actualValue,
				}
				
				// Add as enum value symbol
				valueSymbol := &Symbol{
					Name:   memberName,
					Kind:   SymbolKindEnumValue,
					Type:   enumName,
					Line:   child.Line,
					Column: 0,
				}
				st.AddSymbol(valueSymbol)
			}
		}
		st.AddSymbol(symbol)

	case ahoy.NODE_STRUCT_DECLARATION:
		structName := node.Value
		symbol := &Symbol{
			Name:   structName,
			Kind:   SymbolKindStruct,
			Type:   "struct",
			Line:   node.Line,
			Column: 0,
			Fields: make(map[string]*StructField),
		}

		// Parse struct fields
		for _, child := range node.Children {
			if child.Type == ahoy.NODE_IDENTIFIER {
				// Regular field
				fieldName := child.Value
				fieldType := child.DataType
				defaultVal := ""
				if child.DefaultValue != nil {
					defaultVal = formatDefaultValue(child.DefaultValue)
				}
				symbol.Fields[fieldName] = &StructField{
					Name:         fieldName,
					Type:         fieldType,
					DefaultValue: defaultVal,
				}
			} else if child.Type == ahoy.NODE_TYPE {
				// Nested type (e.g., "type smoke_particle:")
				typeName := child.Value
				nestedField := &StructField{
					Name:   typeName,
					Type:   structName + "." + typeName, // Full type name
					Fields: make(map[string]*StructField),
				}

				// Add fields from nested type
				for _, nestedChild := range child.Children {
					if nestedChild.Type == ahoy.NODE_IDENTIFIER {
						defaultVal := ""
						if nestedChild.DefaultValue != nil {
							defaultVal = formatDefaultValue(nestedChild.DefaultValue)
						}
						nestedField.Fields[nestedChild.Value] = &StructField{
							Name:         nestedChild.Value,
							Type:         nestedChild.DataType,
							DefaultValue: defaultVal,
						}
					}
				}

				// Store the nested type as a field
				symbol.Fields[typeName] = nestedField

				// Also create a separate symbol for the nested type
				// First, copy parent struct's non-nested fields for inheritance
				inheritedFields := make(map[string]*StructField)
				for parentFieldName, parentField := range symbol.Fields {
					if parentField.Fields == nil {
						// Copy regular field from parent
						inheritedFields[parentFieldName] = &StructField{
							Name:         parentField.Name,
							Type:         parentField.Type,
							DefaultValue: parentField.DefaultValue,
						}
					}
				}
				// Add nested type's own fields
				for nestedFieldName, nestedFieldObj := range nestedField.Fields {
					inheritedFields[nestedFieldName] = nestedFieldObj
				}
				
				nestedSymbol := &Symbol{
					Name:   structName + "." + typeName,
					Kind:   SymbolKindStruct,
					Type:   "struct",
					Line:   child.Line,
					Column: 0,
					Fields: inheritedFields,
				}
				st.AddSymbol(nestedSymbol)
				
				// ALSO add symbol with just the nested type name for easier lookup
				nestedSymbolShort := &Symbol{
					Name:   typeName,
					Kind:   SymbolKindStruct,
					Type:   "struct",
					Line:   child.Line,
					Column: 0,
					Fields: inheritedFields,
				}
				st.AddSymbol(nestedSymbolShort)
			}
		}

		st.AddSymbol(symbol)

	case ahoy.NODE_CONSTANT_DECLARATION:
		constName := node.Value
		constType := node.DataType

		if constType == "" && len(node.Children) > 0 {
			constType = st.inferType(node.Children[0])
		}

		symbol := &Symbol{
			Name:   constName,
			Kind:   SymbolKindConstant,
			Type:   constType,
			Line:   node.Line,
			Column: 0,
		}
		st.AddSymbol(symbol)

	case ahoy.NODE_IF_STATEMENT, ahoy.NODE_WHILE_LOOP, ahoy.NODE_FOR_LOOP,
		ahoy.NODE_FOR_RANGE_LOOP, ahoy.NODE_FOR_COUNT_LOOP,
		ahoy.NODE_FOR_IN_ARRAY_LOOP, ahoy.NODE_FOR_IN_DICT_LOOP:
		// Enter new scope for block
		st.EnterScope()
		st.CurrentScope.StartLine = node.Line

		// For loops with loop variables, add them to the scope
		switch node.Type {
		case ahoy.NODE_FOR_IN_ARRAY_LOOP:
			// loop element in array
			if len(node.Children) > 0 {
				loopVar := node.Children[0]
				if loopVar.Type == ahoy.NODE_IDENTIFIER {
					symbol := &Symbol{
						Name:   loopVar.Value,
						Kind:   SymbolKindVariable,
						Type:   "any", // Could be inferred from array type
						Line:   loopVar.Line,
						Column: 0,
					}
					st.AddSymbol(symbol)
				}
			}

		case ahoy.NODE_FOR_IN_DICT_LOOP:
			// loop key,value in dict
			if len(node.Children) >= 2 {
				keyVar := node.Children[0]
				valueVar := node.Children[1]
				if keyVar.Type == ahoy.NODE_IDENTIFIER {
					symbol := &Symbol{
						Name:   keyVar.Value,
						Kind:   SymbolKindVariable,
						Type:   "string",
						Line:   keyVar.Line,
						Column: 0,
					}
					st.AddSymbol(symbol)
				}
				if valueVar.Type == ahoy.NODE_IDENTIFIER {
					symbol := &Symbol{
						Name:   valueVar.Value,
						Kind:   SymbolKindVariable,
						Type:   "any",
						Line:   valueVar.Line,
						Column: 0,
					}
					st.AddSymbol(symbol)
				}
			}

		case ahoy.NODE_FOR_RANGE_LOOP:
			// loop i:start to end OR loop i to end
			if len(node.Children) >= 4 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
				loopVar := node.Children[0]
				symbol := &Symbol{
					Name:   loopVar.Value,
					Kind:   SymbolKindVariable,
					Type:   "int",
					Line:   loopVar.Line,
					Column: 0,
				}
				st.AddSymbol(symbol)
			}

		case ahoy.NODE_WHILE_LOOP:
			// loop i:start till condition OR loop i till condition
			if len(node.Children) >= 3 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
				loopVar := node.Children[0]
				symbol := &Symbol{
					Name:   loopVar.Value,
					Kind:   SymbolKindVariable,
					Type:   "int",
					Line:   loopVar.Line,
					Column: 0,
				}
				st.AddSymbol(symbol)
			}

		case ahoy.NODE_FOR_COUNT_LOOP:
			// loop i:start: OR loop i do
			if len(node.Children) >= 2 && node.Children[0].Type == ahoy.NODE_IDENTIFIER {
				loopVar := node.Children[0]
				symbol := &Symbol{
					Name:   loopVar.Value,
					Kind:   SymbolKindVariable,
					Type:   "int",
					Line:   loopVar.Line,
					Column: 0,
				}
				st.AddSymbol(symbol)
			}
		}

		// Walk children
		for _, child := range node.Children {
			st.walkNode(child, depth+1)
		}

		// Set the end line for this scope before exiting
		st.CurrentScope.EndLine = findLastLine(node)
		st.ExitScope()

	case ahoy.NODE_BLOCK:
		for _, child := range node.Children {
			st.walkNode(child, depth+1)
		}

	default:
		// Walk all children for other node types
		for _, child := range node.Children {
			st.walkNode(child, depth+1)
		}
	}
}

func formatDefaultValue(node *ahoy.ASTNode) string {
	if node == nil {
		return ""
	}
	
	switch node.Type {
	case ahoy.NODE_NUMBER:
		return node.Value
	case ahoy.NODE_STRING:
		return `"` + node.Value + `"`
	case ahoy.NODE_BOOLEAN:
		return node.Value
	case ahoy.NODE_OBJECT_LITERAL:
		// Format object literal as string
		if len(node.Children) == 2 && node.DataType == "vector2" {
			// Simple vector2: <x,y>
			return "<" + node.Children[0].Value + "," + node.Children[1].Value + ">"
		}
		// Full object literal
		result := "<"
		for i, prop := range node.Children {
			if i > 0 {
				result += ","
			}
			if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
				result += prop.Value + ":"
				if len(prop.Children) > 0 {
					result += formatDefaultValue(prop.Children[0])
				}
			}
		}
		result += ">"
		return result
	default:
		return ""
	}
}

func (st *SymbolTable) inferType(node *ahoy.ASTNode) string {
	if node == nil {
		return ""
	}

	switch node.Type {
	case ahoy.NODE_NUMBER:
		if strings.Contains(node.Value, ".") {
			return "float"
		}
		return "int"
	case ahoy.NODE_STRING:
		return "string"
	case ahoy.NODE_BOOLEAN:
		return "bool"
	case ahoy.NODE_ARRAY_LITERAL:
		return "array"
	case ahoy.NODE_DICT_LITERAL:
		// Check if this is a struct initialization by looking for type annotation
		if node.DataType != "" {
			return node.DataType
		}
		return "dict"
	case ahoy.NODE_OBJECT_LITERAL:
		// Check if it has a type (struct initialization) or explicit DataType
		if node.Value != "" {
			// Struct initialization: name : type<...>
			return node.Value
		}
		if node.DataType == "vector2" {
			return "vector2"
		}
		// Plain object literal
		return "object"
	case ahoy.NODE_IDENTIFIER:
		// Look up the identifier
		if sym := st.Lookup(node.Value); sym != nil {
			return sym.Type
		}
	case ahoy.NODE_CALL:
		// Try to look up function return type and actually infer it if needed
		funcName := node.Value
		if sym := st.Lookup(funcName); sym != nil {
			returnType := sym.Type
			
			debugLog.Printf("DEBUG inferType: Function %s has return type: %s", funcName, returnType)
			
			// If function has infer/generic return type, actually infer it
			if returnType == "infer" || returnType == "" || returnType == "generic" || strings.Contains(returnType, "generic") {
				debugLog.Printf("DEBUG inferType: Need to infer for %s", funcName)
				
				// Find the function node in the AST
				var funcNode *ahoy.ASTNode
				var findFunc func(*ahoy.ASTNode)
				findFunc = func(n *ahoy.ASTNode) {
					if n == nil {
						return
					}
					if n.Type == ahoy.NODE_FUNCTION && n.Value == funcName {
						funcNode = n
						return
					}
					for _, child := range n.Children {
						findFunc(child)
					}
				}
				if st.AST != nil {
					findFunc(st.AST)
				}
				
				if funcNode != nil {
					debugLog.Printf("DEBUG inferType: Found function node for %s", funcName)
				}
				
				// Infer argument types from the call
				var argTypes []string
				for _, arg := range node.Children {
					argType := inferExpressionType(arg, st.Doc)
					argTypes = append(argTypes, argType)
				}
				
				debugLog.Printf("DEBUG inferType: Argument types: %v", argTypes)
				
				// Infer actual return types by tracing through function
				if funcNode != nil && st.Doc != nil {
					inferredTypes := inferFunctionReturnTypes(funcNode, argTypes, st.Doc)
					debugLog.Printf("DEBUG inferType: Inferred types: %v", inferredTypes)
					if len(inferredTypes) > 0 {
						result := strings.Join(inferredTypes, ",")
						debugLog.Printf("DEBUG inferType: Returning inferred type: %s", result)
						return result
					}
				}
			}
			
			debugLog.Printf("DEBUG inferType: Returning original type: %s", returnType)
			return returnType
		}
	}

	return ""
}

func (st *SymbolTable) GetStructFields(typeName string) map[string]*StructField {
	// Look up the struct type
	sym := st.Lookup(typeName)
	if sym == nil || sym.Kind != SymbolKindStruct {
		return nil
	}

	// Return all fields from this struct and its parent
	allFields := make(map[string]*StructField)

	// Check if this is a nested type (contains a dot)
	if strings.Contains(typeName, ".") {
		// e.g., "particle.smoke_particle"
		parts := strings.Split(typeName, ".")
		if len(parts) == 2 {
			parentName := parts[0]
			childName := parts[1]

			// Get parent struct fields
			parentSym := st.Lookup(parentName)
			if parentSym != nil && parentSym.Kind == SymbolKindStruct {
				// Add parent fields
				for name, field := range parentSym.Fields {
					// Skip nested type declarations
					if !strings.Contains(field.Type, ".") {
						allFields[name] = field
					}
				}
			}

			// Get child type fields
			if childField, exists := parentSym.Fields[childName]; exists {
				for name, field := range childField.Fields {
					allFields[name] = field
				}
			}
		}
	} else {
		// Regular struct, return all fields
		for name, field := range sym.Fields {
			allFields[name] = field
		}
	}

	return allFields
}

// GetAllSymbols returns all symbols in the table (for outline/symbol list)
func (st *SymbolTable) GetAllSymbols() []*Symbol {
	symbols := []*Symbol{}
	st.collectSymbols(st.GlobalScope, &symbols)
	return symbols
}

func (st *SymbolTable) collectSymbols(scope *Scope, symbols *[]*Symbol) {
	if scope == nil {
		return
	}

	// Prevent memory exhaustion from too many symbols
	if len(*symbols) > 1000 {
		return
	}

	for _, sym := range scope.Symbols {
		*symbols = append(*symbols, sym)
		// Early exit if we have enough symbols
		if len(*symbols) > 1000 {
			return
		}
	}
	for _, child := range scope.Children {
		st.collectSymbols(child, symbols)
	}
}

// FindReferences finds all references to a symbol
func (st *SymbolTable) FindReferences(symbolName string, ast *ahoy.ASTNode) []Position {
	positions := []Position{}
	st.findReferencesInNode(ast, symbolName, &positions, 0)
	return positions
}

type Position struct {
	Line   int
	Column int
}

func (st *SymbolTable) findReferencesInNode(node *ahoy.ASTNode, name string, positions *[]Position, depth int) {
	if node == nil {
		return
	}

	// Prevent unbounded recursion and memory issues
	if depth > 500 {
		return
	}

	// Limit number of results to prevent memory exhaustion
	if len(*positions) > 100 {
		return
	}

	if node.Type == ahoy.NODE_IDENTIFIER && node.Value == name {
		*positions = append(*positions, Position{
			Line:   node.Line,
			Column: 0,
		})
		// Early exit if we have enough references
		if len(*positions) > 100 {
			return
		}
	}

	// Limit child iteration
	if len(node.Children) > 1000 {
		return
	}

	for _, child := range node.Children {
		st.findReferencesInNode(child, name, positions, depth+1)
	}
}

// extractObjectLiteralFields extracts properties from an object literal node
func (st *SymbolTable) extractObjectLiteralFields(node *ahoy.ASTNode) map[string]*StructField {
	if node == nil || node.Type != ahoy.NODE_OBJECT_LITERAL {
		return nil
	}

	fields := make(map[string]*StructField)
	
	// Iterate through object properties
	for _, prop := range node.Children {
		if prop.Type == ahoy.NODE_OBJECT_PROPERTY {
			fieldName := prop.Value
			fieldType := ""
			
			// Try to infer type from the property value
			if len(prop.Children) > 0 {
				fieldType = st.inferType(prop.Children[0])
			}
			
			fields[fieldName] = &StructField{
				Name: fieldName,
				Type: fieldType,
			}
		}
	}
	
	return fields
}

// findLastLine recursively finds the highest line number in a node tree
func findLastLine(node *ahoy.ASTNode) int {
if node == nil {
return 0
}

maxLine := node.Line
for _, child := range node.Children {
childLine := findLastLine(child)
if childLine > maxLine {
maxLine = childLine
}
}

return maxLine
}
