package main

import (
	"strings"

	"ahoy"
)

// Symbol represents a symbol in the code (variable, function, type, etc.)
type Symbol struct {
	Name      string
	Kind      SymbolKind
	Type      string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
	// Don't store Definition node or Scope to prevent memory leaks - AST can't be GC'd
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
			Name:   funcName,
			Kind:   SymbolKindFunction,
			Type:   node.DataType,
			Line:   node.Line,
			Column: 0,
		}
		st.AddSymbol(symbol)

		// Enter function scope
		st.EnterScope()
		st.CurrentScope.StartLine = node.Line

		// Add parameters
		if len(node.Children) > 0 {
			params := node.Children[0]
			if params != nil {
				for i := 0; i < len(params.Children); i += 2 {
					if i < len(params.Children) {
						paramName := params.Children[i].Value
						paramType := ""
						if i+1 < len(params.Children) {
							paramType = params.Children[i+1].Value
						}

						paramSymbol := &Symbol{
							Name:   paramName,
							Kind:   SymbolKindParameter,
							Type:   paramType,
							Line:   params.Children[i].Line,
							Column: 0,
						}
						st.AddSymbol(paramSymbol)
					}
				}
			}
		}

		// Walk function body
		if len(node.Children) > 1 {
			st.walkNode(node.Children[1], depth+1)
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
		st.AddSymbol(symbol)

		// Walk the value expression
		if len(node.Children) > 0 {
			st.walkNode(node.Children[0], depth+1)
		}

	case ahoy.NODE_ENUM_DECLARATION:
		enumName := node.Value
		symbol := &Symbol{
			Name:   enumName,
			Kind:   SymbolKindEnum,
			Type:   "enum",
			Line:   node.Line,
			Column: 0,
		}
		st.AddSymbol(symbol)

		// Add enum values
		for _, child := range node.Children {
			if child.Type == ahoy.NODE_IDENTIFIER {
				valueSymbol := &Symbol{
					Name:   child.Value,
					Kind:   SymbolKindEnumValue,
					Type:   enumName,
					Line:   child.Line,
					Column: 0,
				}
				st.AddSymbol(valueSymbol)
			}
		}

	case ahoy.NODE_STRUCT_DECLARATION:
		structName := node.Value
		symbol := &Symbol{
			Name:   structName,
			Kind:   SymbolKindStruct,
			Type:   "struct",
			Line:   node.Line,
			Column: 0,
		}
		st.AddSymbol(symbol)

		// Add struct fields
		for i := 0; i < len(node.Children); i += 2 {
			if i < len(node.Children) {
				fieldName := node.Children[i].Value
				fieldType := ""
				if i+1 < len(node.Children) {
					fieldType = node.Children[i+1].Value
				}

				fieldSymbol := &Symbol{
					Name:   fieldName,
					Kind:   SymbolKindStructField,
					Type:   fieldType,
					Line:   node.Children[i].Line,
					Column: 0,
				}
				st.AddSymbol(fieldSymbol)
			}
		}

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

		// For loops with variables
		if node.Type == ahoy.NODE_FOR_IN_ARRAY_LOOP && len(node.Children) > 0 {
			// Add loop variable
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

		// Walk children
		for _, child := range node.Children {
			st.walkNode(child, depth+1)
		}

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
		return "dict"
	case ahoy.NODE_IDENTIFIER:
		// Look up the identifier
		if sym := st.Lookup(node.Value); sym != nil {
			return sym.Type
		}
	case ahoy.NODE_CALL:
		// Try to look up function return type
		if sym := st.Lookup(node.Value); sym != nil {
			return sym.Type
		}
	}

	return ""
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
