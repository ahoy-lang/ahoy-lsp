package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ahoy"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func loadContextDoc(t *testing.T, path string) (*Server, *Document) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tokens := ahoy.Tokenize(string(content))
	ast, errs := ahoy.ParseSyntaxWithPath(tokens, path)
	if ast == nil {
		t.Fatalf("parse produced no AST: %v", errs)
	}
	doc := &Document{
		URI:     uri.URI("file://" + path),
		Content: string(content),
		Lines:   strings.Split(string(content), "\n"),
		Tokens:  tokens,
		AST:     ast,
		Errors:  errs,
	}
	doc.ProgramName = extractProgramName(ast)
	doc.SymbolTable = BuildSymbolTableWithFile(ast, string(doc.URI))
	doc.CHeaders, doc.CHeaderGlobal = extractCHeaderInfoWithURI(ast, doc.URI)

	s := &Server{documents: make(map[uri.URI]*Document)}
	s.loadPackageFiles(doc)
	return s, doc
}

// loadServerDoc mirrors handleDidOpen's setup, including parameter/return
// inference, so diagnostics match what Zed actually receives.
func loadServerDoc(t *testing.T, path string) (*Server, *Document) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tokens := ahoy.Tokenize(string(content))
	ast, errs := ahoy.ParseSyntaxWithPath(tokens, path)
	doc := &Document{
		URI:     uri.URI("file://" + path),
		Content: string(content),
		Lines:   strings.Split(string(content), "\n"),
		Tokens:  tokens,
		AST:     ast,
		Errors:  errs,
	}
	doc.ProgramName = extractProgramName(ast)
	doc.CHeaders, doc.CHeaderGlobal = extractCHeaderInfoWithURI(ast, doc.URI)
	doc.InferredParams = inferParameterTypes(ast, doc)
	inferredReturns := resolveAllInferReturnTypes(ast, doc)
	doc.SymbolTable = BuildSymbolTableWithFile(ast, string(doc.URI), doc.InferredParams)
	doc.SymbolTable.InferredReturns = inferredReturns

	s := &Server{documents: make(map[uri.URI]*Document)}
	if doc.ProgramName != "" {
		s.loadPackageFiles(doc)
	}
	s.revalidateWithProgramNames(doc)
	return s, doc
}

// The real Bell editor must produce no error diagnostics in the same checks
// Zed runs. This is the regression guard for the 13k false errors caused by
// not following ../engine and ../tools/godot_lib.ahoy.
func TestBellEditorHasNoFalseErrors(t *testing.T) {
	path := filepath.Join("..", "bell", "editor", "main.ahoy")
	if _, err := os.Stat(path); err != nil {
		t.Skip("bell editor not present")
	}
	_, doc := loadServerDoc(t, path)

	var errors []string
	for _, diag := range collectDiagnostics(doc) {
		if diag.Severity != protocol.DiagnosticSeverityError {
			continue
		}
		errors = append(errors, fmt.Sprintf("line %d: %s", diag.Range.Start.Line+1, diag.Message))
	}
	if len(errors) > 0 {
		limit := len(errors)
		if limit > 40 {
			limit = 40
		}
		t.Errorf("Bell editor has %d error diagnostics; first %d:\n%s", len(errors), limit, strings.Join(errors[:limit], "\n"))
	}
}

// A program that imports a directory and a headerless library file must see
// the names they declare. This is the Bell editor layout in miniature:
// ../engine is a directory package, ../tools/godot_lib.ahoy is headerless.
func TestPackageContextFollowsImports(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "engine"), 0755); err != nil {
		t.Fatal(err)
	}

	mainSrc := `program p
import "./engine"
import "./lib.ahoy"

@ main :: || void:
	a: engine_fn|1|
	b: nested_fn|2|
	c: lib_fn||
	print|a, b, c|
$
`
	engineSrc := `program engine

@ engine_fn |x:int| int:
	return nested_fn|x|
$
`
	nestedSrc := `program engine

@ nested_fn |x:int| int:
	return x + 1
$
`
	libSrc := `? headerless library

@ lib_fn ::|| int:
	return 7
$
`
	otherSrc := `program unrelated

@ unrelated_fn ::|| int:
	return 99
$
`
	mainPath := filepath.Join(dir, "main.ahoy")
	for path, src := range map[string]string{
		mainPath: mainSrc,
		filepath.Join(dir, "engine", "engine.ahoy"): engineSrc,
		filepath.Join(dir, "engine", "nested.ahoy"): nestedSrc,
		filepath.Join(dir, "lib.ahoy"):              libSrc,
		filepath.Join(dir, "other.ahoy"):            otherSrc,
	} {
		if err := os.WriteFile(path, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}

	s, doc := loadContextDoc(t, mainPath)
	if doc.PackageSymbols == nil {
		t.Fatal("import context was not merged")
	}
	for _, name := range []string{"engine_fn", "nested_fn", "lib_fn"} {
		sym := doc.PackageSymbols.GlobalScope.Lookup(name)
		if sym == nil || sym.Kind != SymbolKindFunction {
			t.Errorf("imported function %q missing from package symbols: %+v", name, sym)
		}
	}
	if sym := doc.PackageSymbols.GlobalScope.Lookup("unrelated_fn"); sym != nil {
		t.Errorf("unrelated sibling program leaked into package symbols: %+v", sym)
	}

	if diags := checkUndefinedFunctions(doc); len(diags) != 0 {
		t.Errorf("imported functions reported undefined: %v", diags)
	}
	if diags := checkDuplicateFunctionDefinitions(doc); len(diags) != 0 {
		t.Errorf("imported functions reported as duplicate definitions: %v", diags)
	}

	// The same names are what revalidateWithProgramNames feeds the checker.
	s.revalidateWithProgramNames(doc)
	for _, err := range doc.Errors {
		if !err.IsWarning() && strings.Contains(err.Message, "not defined") {
			t.Errorf("semantic pass still reports an imported call: %s", err.Message)
		}
	}
}

// Import resolution must recurse: engine.ahoy imports ./nested/sub.ahoy, and
// the main file only imports ./engine.
func TestPackageContextFollowsNestedImports(t *testing.T) {
	dir := t.TempDir()
	engineDir := filepath.Join(dir, "engine")
	nestedDir := filepath.Join(engineDir, "nested")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		filepath.Join(dir, "main.ahoy"): `program p
import "./engine"

@ main :: || void:
	print|deep_fn|3||
$
`,
		filepath.Join(engineDir, "engine.ahoy"): `program engine
import "./nested/sub.ahoy"

@ engine_fn |x:int| int:
	return deep_fn|x|
$
`,
		filepath.Join(nestedDir, "sub.ahoy"): `? headerless helper

@ deep_fn |x:int| int:
	return x * 2
$
`,
	}
	for path, src := range files {
		if err := os.WriteFile(path, []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}

	_, doc := loadContextDoc(t, filepath.Join(dir, "main.ahoy"))
	if doc.PackageSymbols == nil {
		t.Fatal("nested import context was not merged")
	}
	if sym := doc.PackageSymbols.GlobalScope.Lookup("deep_fn"); sym == nil {
		t.Fatal("function from nested import missing from package symbols")
	}
}

// The document is parsed syntax-only first, so revalidateWithProgramNames must
// run the semantic checks even when there are no imports or siblings to supply
// names. Returning early there would leave a single-file program with syntax
// diagnostics only.
func TestRevalidateRunsWithoutPackageFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "single.ahoy")
	src := "program p\n\n@ main :: || void:\n\tx: missing_fn||\n$\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	tokens := ahoy.Tokenize(src)
	ast, _ := ahoy.ParseSyntaxWithPath(tokens, path)
	doc := &Document{
		URI:     uri.URI("file://" + path),
		Content: src,
		Tokens:  tokens,
		AST:     ast,
	}
	doc.ProgramName = extractProgramName(ast)
	doc.SymbolTable = BuildSymbolTableWithFile(ast, string(doc.URI))

	s := &Server{documents: make(map[uri.URI]*Document)}
	s.loadPackageFiles(doc)
	if len(doc.PackageFiles) != 0 {
		t.Fatalf("expected no package files for a single-file program, got %d", len(doc.PackageFiles))
	}

	s.revalidateWithProgramNames(doc)
	found := false
	for _, err := range doc.Errors {
		if strings.Contains(err.Message, "not defined") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("semantic checks did not run for a single-file program: %v", doc.Errors)
	}
}
