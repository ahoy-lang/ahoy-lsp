package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ahoy"
	"go.lsp.dev/uri"
)

// Mirrors the shape of a real program: main is declared before test, and both
// declare an `x`. Hover inside test must describe test's x, not main's; and a
// variable assigned from a call must take the callee's explicit return type.
const scopedProgram = `program hotdemo

@ main :: || void:
	frames: 0
	x: 10
	y: test|9|
	print|x, y, frames|
$

@ test ::|n:int| int:
	y, x : 29, "test"
	print|x, y|
	return 29
$
`

func lineOf(t *testing.T, src, needle string) int {
	t.Helper()
	for i, line := range strings.Split(src, "\n") {
		if strings.Contains(line, needle) {
			return i + 1
		}
	}
	t.Fatalf("no line contains %q", needle)
	return 0
}

func buildTableFor(t *testing.T, src string) *SymbolTable {
	t.Helper()
	ast, _ := ahoy.ParseLintWithPath(ahoy.Tokenize(src), "test.ahoy")
	return BuildSymbolTableWithFile(ast, "file:///test.ahoy")
}

func TestHoverResolvesInsideFunctionScope(t *testing.T) {
	st := buildTableFor(t, scopedProgram)
	mainX := lineOf(t, scopedProgram, "x: 10")
	testX := lineOf(t, scopedProgram, "y, x :")

	sym := st.LookupAtPosition("x", testX, 6)
	if sym == nil {
		t.Fatalf("x not found at line %d", testX)
	}
	if sym.Line != testX {
		t.Errorf("x at line %d resolved to the declaration at line %d", testX, sym.Line)
	}

	sym = st.LookupAtPosition("x", mainX, 2)
	if sym == nil || sym.Line != mainX {
		t.Errorf("x at line %d did not resolve to its own declaration: %+v", mainX, sym)
	}
}

func TestMergedPackageTableKeepsScopes(t *testing.T) {
	st := buildTableFor(t, scopedProgram)
	merged := NewSymbolTable()
	mergeSymbolTable(merged.GlobalScope, st.GlobalScope)

	testX := lineOf(t, scopedProgram, "y, x :")
	sym := merged.LookupAtPosition("x", testX, 6)
	if sym == nil {
		t.Fatalf("x not found after merge at line %d", testX)
	}
	if sym.Line != testX {
		t.Errorf("after merge, x at line %d resolved to the declaration at line %d", testX, sym.Line)
	}
}

func TestVariableTypeUsesExplicitReturnType(t *testing.T) {
	st := buildTableFor(t, scopedProgram)
	yLine := lineOf(t, scopedProgram, "y: test|9|")

	sym := st.LookupAtPosition("y", yLine, 2)
	if sym == nil {
		t.Fatalf("y not found at line %d", yLine)
	}
	if sym.Type != "int" {
		t.Errorf("y should be int from test's explicit return type, got %q", sym.Type)
	}
}

// A file that calls a function declared in a sibling file of the same program
// must not be told the function is undefined - this is the path --validate
// takes, and what TestLSPDiagnostics exercises.
func TestPackageSymbolsResolveSiblingFunctions(t *testing.T) {
	dir := t.TempDir()
	mainSrc := "program example\n@ main :: || void:\n\tr: helper|1|\n\tprint|r|\n$\n"
	helperSrc := "program example\n@ helper ::|a:int| int:\n\treturn a\n$\n"
	mainPath := filepath.Join(dir, "program.ahoy")
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "program_extended.ahoy"), []byte(helperSrc), 0644); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	ast, _ := ahoy.ParseLintWithPath(ahoy.Tokenize(string(content)), mainPath)
	doc := &Document{
		URI:     uri.URI("file://" + mainPath),
		Content: string(content),
		AST:     ast,
	}
	doc.ProgramName = extractProgramName(ast)
	if doc.ProgramName != "example" {
		t.Fatalf("program name not found, got %q", doc.ProgramName)
	}
	doc.SymbolTable = BuildSymbolTableWithFile(ast, string(doc.URI))

	s := &Server{documents: make(map[uri.URI]*Document)}
	s.loadPackageFiles(doc)

	if doc.PackageSymbols == nil {
		t.Fatal("PackageSymbols was not built for a multi-file program")
	}
	sym := doc.PackageSymbols.GlobalScope.Lookup("helper")
	if sym == nil || sym.Kind != SymbolKindFunction {
		t.Fatalf("sibling function 'helper' not merged into PackageSymbols: %+v", sym)
	}

	if diags := checkUndefinedFunctions(doc); len(diags) != 0 {
		t.Errorf("sibling function reported undefined: %v", diags)
	}
}
