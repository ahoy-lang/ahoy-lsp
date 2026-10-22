package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ahoy"

	"go.lsp.dev/uri"
)

// contextSource is a parsed file whose imports still need resolving. The
// compiler resolves imports recursively across the whole program; the LSP
// needs the same view so names declared by an imported engine (or by a
// headerless library file such as tools/godot_lib.ahoy) are not reported as
// undefined.
type contextSource struct {
	path string
	ast  *ahoy.ASTNode
}

// contextFileData is a parsed imported file, cached by path and validated by
// modification time and size. ASTs and symbol tables are immutable after
// construction, so sharing them across edits and documents is safe.
type contextFileData struct {
	modTime time.Time
	size    int64
	content string
	ast     *ahoy.ASTNode
	symbols *SymbolTable
}

// documentPath returns the absolute, cleaned filesystem path for a document
// URI. The URI may be relative ("file://editor/main.ahoy"), so filepath.Abs is
// applied before the path is used as a map key.
func documentPath(doc *Document) (string, error) {
	if doc == nil {
		return "", fmt.Errorf("nil document")
	}
	path := strings.TrimPrefix(string(doc.URI), "file://")
	if path == "" {
		return "", fmt.Errorf("empty document URI")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// parseContextFile reads and parses one file for import resolution. Parsing is
// best-effort: a broken imported file must not take down the language server,
// and its own diagnostics are not published through the importing document.
func parseContextFile(path string) (content string, ast *ahoy.ASTNode, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			content = string(data)
			ast = nil
			err = fmt.Errorf("parse panic in %s: %v", path, r)
		}
	}()

	tokens := ahoy.Tokenize(string(data))
	// Imported files only contribute declarations to the importing document's
	// program context. Their own semantic diagnostics are not published, so a
	// syntax-only parse is enough and avoids generating suggestions for every
	// name the imported file cannot resolve in isolation.
	ast, _ = ahoy.ParseSyntaxWithPath(tokens, path)
	if ast == nil {
		return string(data), nil, fmt.Errorf("no AST produced for %s", path)
	}
	return string(data), ast, nil
}

// loadContextFile returns a cached parse for an imported file, re-reading it
// only when its mtime or size changed. The current document is never loaded
// through this cache: it has unsaved editor content and is parsed separately.
func (s *Server) loadContextFile(path string) (*contextFileData, error) {
	path = filepath.Clean(path)
	before, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	s.contextMu.Lock()
	if s.contextCache != nil {
		if cached, ok := s.contextCache[path]; ok &&
			cached.size == before.Size() && cached.modTime.Equal(before.ModTime()) {
			s.contextMu.Unlock()
			return cached, nil
		}
	}
	s.contextMu.Unlock()

	content, ast, err := parseContextFile(path)
	if err != nil {
		return nil, err
	}
	after, statErr := os.Stat(path)
	if statErr != nil {
		after = before
	}
	fileURI := uri.URI("file://" + path)
	data := &contextFileData{
		modTime: after.ModTime(),
		size:    after.Size(),
		content: content,
		ast:     ast,
		symbols: BuildSymbolTableWithFile(ast, string(fileURI)),
	}

	s.contextMu.Lock()
	if s.contextCache == nil {
		s.contextCache = make(map[string]*contextFileData)
	}
	s.contextCache[path] = data
	s.contextMu.Unlock()
	return data, nil
}

// resolveContextImport resolves an import exactly the way the compiler does
// for the common cases: absolute paths stay absolute, and relative paths are
// resolved against the importing file. The package manager also falls back to
// the program's current directory for bare (non-./) paths, so try that too.
func resolveContextImport(importPath, importingFile, programFile string) (string, bool) {
	if strings.HasSuffix(importPath, ".h") {
		return "", false
	}

	var candidates []string
	if filepath.IsAbs(importPath) {
		candidates = append(candidates, importPath)
	} else {
		candidates = append(candidates, filepath.Join(filepath.Dir(importingFile), importPath))
		if filepath.Dir(importingFile) != filepath.Dir(programFile) {
			candidates = append(candidates, filepath.Join(filepath.Dir(programFile), importPath))
		}
	}

	for _, candidate := range candidates {
		cleaned := filepath.Clean(candidate)
		if _, err := os.Stat(cleaned); err == nil {
			return cleaned, true
		}
	}
	return "", false
}

// addContextFile stores a parsed imported/sibling file and queues its imports
// for the caller to walk.
func (s *Server) addContextFile(doc *Document, path string, data *contextFileData, isImport bool, loaded map[string]bool, queue *[]contextSource) *PackageFile {
	if data == nil || data.ast == nil {
		return nil
	}
	path = filepath.Clean(path)
	if loaded[path] {
		return nil
	}
	loaded[path] = true

	fileURI := uri.URI("file://" + path)
	pkgFile := &PackageFile{
		URI:      fileURI,
		Content:  data.content,
		AST:      data.ast,
		Symbols:  data.symbols,
		IsImport: isImport,
	}
	doc.PackageFiles[fileURI] = pkgFile
	*queue = append(*queue, contextSource{path: path, ast: data.ast})
	return pkgFile
}

// loadImportedProgramFile loads a .ahoy import. The compiler's
// LoadPackageFromFile treats a file with a program declaration as the entry
// point of its directory's package, so load the same-program siblings too.
func (s *Server) loadImportedProgramFile(doc *Document, path string, loaded map[string]bool, queue *[]contextSource) {
	data, err := s.loadContextFile(path)
	if err != nil {
		debugLog.Printf("Skipping imported file %s: %v", path, err)
		return
	}

	programName := extractProgramName(data.ast)
	s.addContextFile(doc, path, data, true, loaded, queue)
	if programName == "" {
		return
	}

	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ahoy") {
			continue
		}
		siblingPath := filepath.Clean(filepath.Join(dir, entry.Name()))
		if siblingPath == filepath.Clean(path) || loaded[siblingPath] {
			continue
		}
		siblingData, err := s.loadContextFile(siblingPath)
		if err != nil || extractProgramName(siblingData.ast) != programName {
			continue
		}
		s.addContextFile(doc, siblingPath, siblingData, true, loaded, queue)
	}
}

// loadImportedDirectory loads a directory import. A normal package directory
// has exactly one program declaration; include that program's files. If the
// directory is heterogeneous (or headerless), include everything rather than
// hiding names the importing file may legitimately use.
func (s *Server) loadImportedDirectory(doc *Document, dir string, loaded map[string]bool, queue *[]contextSource) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		debugLog.Printf("Error reading imported directory %s: %v", dir, err)
		return
	}

	type candidate struct {
		path    string
		data    *contextFileData
		program string
	}
	var candidates []candidate
	programGroups := map[string]int{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ahoy") {
			continue
		}
		path := filepath.Clean(filepath.Join(dir, entry.Name()))
		if loaded[path] {
			continue
		}
		data, err := s.loadContextFile(path)
		if err != nil {
			continue
		}
		program := extractProgramName(data.ast)
		candidates = append(candidates, candidate{path: path, data: data, program: program})
		if program != "" {
			programGroups[program]++
		}
	}

	chosenProgram := ""
	includeAll := len(programGroups) != 1
	if includeAll {
		// Multiple programs or only headerless files.
		chosenProgram = ""
	} else {
		for name := range programGroups {
			chosenProgram = name
		}
	}

	for _, candidate := range candidates {
		if !includeAll && candidate.program != chosenProgram {
			continue
		}
		s.addContextFile(doc, candidate.path, candidate.data, true, loaded, queue)
	}
}

// loadPackageFiles builds the program context for doc: same-program sibling
// files plus every .ahoy import reachable from them, recursively. This mirrors
// the compiler's whole-program lint pass (see resolveImports and
// collectPackages in ahoy/source/package.go) closely enough that an editor sees
// the same names a build does.
func (s *Server) loadPackageFiles(doc *Document) {
	if doc == nil {
		return
	}

	// Clear the previous context before any early return. Otherwise a file
	// that just acquired a syntax error could keep the symbol table from its
	// last good parse and report diagnostics against a stale program.
	doc.PackageFiles = make(map[uri.URI]*PackageFile)
	doc.PackageSymbols = nil
	if doc.AST == nil {
		return
	}

	filePath, err := documentPath(doc)
	if err != nil {
		debugLog.Printf("Cannot resolve document path: %v", err)
		return
	}

	loaded := map[string]bool{filePath: true}
	queue := []contextSource{{path: filePath, ast: doc.AST}}

	// Same-directory files that declare the same program are part of this
	// package. They are not imports, so duplicate-definition checks still
	// apply to them.
	if doc.ProgramName != "" {
		dir := filepath.Dir(filePath)
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".ahoy") {
					continue
				}
				siblingPath := filepath.Clean(filepath.Join(dir, entry.Name()))
				if siblingPath == filePath || loaded[siblingPath] {
					continue
				}
				data, err := s.loadContextFile(siblingPath)
				if err != nil || extractProgramName(data.ast) != doc.ProgramName {
					continue
				}
				s.addContextFile(doc, siblingPath, data, false, loaded, &queue)
				debugLog.Printf("Loaded package file: %s", entry.Name())
			}
		}
	}

	// Walk imports from the document and every loaded file. Newly loaded
	// files append themselves to the queue, so nested imports resolve too.
	for len(queue) > 0 {
		source := queue[0]
		queue = queue[1:]
		if source.ast == nil {
			continue
		}
		for _, child := range source.ast.Children {
			if child.Type != ahoy.NODE_IMPORT_STATEMENT {
				continue
			}
			importPath := child.Value
			resolved, ok := resolveContextImport(importPath, source.path, filePath)
			if !ok {
				continue
			}
			info, err := os.Stat(resolved)
			if err != nil {
				continue
			}
			if info.IsDir() {
				s.loadImportedDirectory(doc, resolved, loaded, &queue)
			} else if strings.HasSuffix(resolved, ".ahoy") {
				s.loadImportedProgramFile(doc, resolved, loaded, &queue)
			}
		}
	}

	if len(doc.PackageFiles) == 0 {
		doc.PackageSymbols = nil
		return
	}

	// Merge the document's own symbols first so its definitions win over an
	// imported package that happens to declare the same name.
	doc.PackageSymbols = NewSymbolTable()
	if doc.SymbolTable != nil {
		mergeSymbolTable(doc.PackageSymbols.GlobalScope, doc.SymbolTable.GlobalScope)
	}
	for _, pkgFile := range doc.PackageFiles {
		if pkgFile.Symbols != nil {
			mergeSymbolTable(doc.PackageSymbols.GlobalScope, pkgFile.Symbols.GlobalScope)
		}
	}

	// C headers imported by package files are visible program-wide to the
	// compiler, so merge their symbols into the document as well.
	mergePackageCHeaders(doc)

	debugLog.Printf("Merged package symbols from %d files", len(doc.PackageFiles)+1)
}

// mergePackageCHeaders adds the C header symbols declared or imported by the
// program's other files to the current document's header view.
func mergePackageCHeaders(doc *Document) {
	if doc == nil {
		return
	}
	if doc.CHeaders == nil {
		doc.CHeaders = make(map[string]*ahoy.CHeaderInfo)
	}
	if doc.CHeaderGlobal == nil {
		doc.CHeaderGlobal = &ahoy.CHeaderInfo{
			Functions: make(map[string]*ahoy.CFunction),
			Enums:     make(map[string]*ahoy.CEnum),
			Defines:   make(map[string]*ahoy.CDefine),
			Structs:   make(map[string]*ahoy.CStruct),
			Typedefs:  make(map[string]*ahoy.CTypedef),
		}
	}

	for _, pkgFile := range doc.PackageFiles {
		if pkgFile.AST == nil {
			continue
		}
		headers, global := extractCHeaderInfoWithURI(pkgFile.AST, pkgFile.URI)
		for namespace, header := range headers {
			if _, exists := doc.CHeaders[namespace]; !exists {
				doc.CHeaders[namespace] = header
			}
		}
		if global == nil {
			continue
		}
		for name, fn := range global.Functions {
			if _, exists := doc.CHeaderGlobal.Functions[name]; !exists {
				doc.CHeaderGlobal.Functions[name] = fn
			}
		}
		for name, enum := range global.Enums {
			if _, exists := doc.CHeaderGlobal.Enums[name]; !exists {
				doc.CHeaderGlobal.Enums[name] = enum
			}
		}
		for name, def := range global.Defines {
			if _, exists := doc.CHeaderGlobal.Defines[name]; !exists {
				doc.CHeaderGlobal.Defines[name] = def
			}
		}
		for name, strct := range global.Structs {
			if _, exists := doc.CHeaderGlobal.Structs[name]; !exists {
				doc.CHeaderGlobal.Structs[name] = strct
			}
		}
		for name, td := range global.Typedefs {
			if _, exists := doc.CHeaderGlobal.Typedefs[name]; !exists {
				doc.CHeaderGlobal.Typedefs[name] = td
			}
		}
		if global.FuncPtrTypedefs != nil {
			if doc.CHeaderGlobal.FuncPtrTypedefs == nil {
				doc.CHeaderGlobal.FuncPtrTypedefs = make(map[string]*ahoy.CFuncPtrTypedef)
			}
			for name, td := range global.FuncPtrTypedefs {
				if _, exists := doc.CHeaderGlobal.FuncPtrTypedefs[name]; !exists {
					doc.CHeaderGlobal.FuncPtrTypedefs[name] = td
				}
			}
		}
	}
}
