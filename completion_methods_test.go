package main

import (
	"strings"
	"testing"

	ahoy "ahoy"
	"go.lsp.dev/protocol"
)

const methodsCompletionProgram = `struct Point:
	x: float
	y: float
$

@ Point.make |x:float, y:float| Point:
	return Point{x: x, y: y}
$

@ (p Point) move |dx:float, dy:float| Point:
	p.x = p.x + dx
	p.y = p.y + dy
	return p
$

@ (p Point) sum || float:
	return p.x + p.y
$
`

func buildDocForMethods(t *testing.T, src string) *Document {
	t.Helper()
	ast, _ := ahoy.ParseLintWithPath(ahoy.Tokenize(src), "test.ahoy")
	return &Document{
		Content: src,
		Lines:   strings.Split(src, "\n"),
		AST:     ast,
	}
}

func completionLabels(items []protocol.CompletionItem) map[string]string {
	out := map[string]string{}
	for _, it := range items {
		out[it.Label] = it.Detail
	}
	return out
}

func TestMethodCompletionOnReceiver(t *testing.T) {
	doc := buildDocForMethods(t, methodsCompletionProgram)
	got := completionLabels(addUserMethods(nil, doc, "Point", "", false))
	if _, ok := got["move"]; !ok {
		t.Errorf("expected move in %v", got)
	}
	if _, ok := got["sum"]; !ok {
		t.Errorf("expected sum in %v", got)
	}
	if _, ok := got["make"]; ok {
		t.Errorf("associated make must not complete on a value, got %v", got)
	}
	if d := got["move"]; !strings.Contains(d, "Point.move(dx:float, dy:float)") {
		t.Errorf("unexpected detail %q", d)
	}
}

func TestMethodCompletionPrefixFilter(t *testing.T) {
	doc := buildDocForMethods(t, methodsCompletionProgram)
	got := completionLabels(addUserMethods(nil, doc, "Point", "mo", false))
	if len(got) != 1 {
		t.Fatalf("expected only move for prefix mo, got %v", got)
	}
}

func TestAssociatedCompletionOnType(t *testing.T) {
	doc := buildDocForMethods(t, methodsCompletionProgram)
	got := completionLabels(addUserMethods(nil, doc, "Point", "", true))
	if len(got) != 1 {
		t.Fatalf("expected only make for Point., got %v", got)
	}
	if d := got["make"]; !strings.Contains(d, "Point.make(x:float, y:float)") {
		t.Errorf("unexpected detail %q", d)
	}
}

func TestMethodCompletionDedupeBuiltin(t *testing.T) {
	doc := buildDocForMethods(t, methodsCompletionProgram)
	seed := []protocol.CompletionItem{{Label: "move", Kind: protocol.CompletionItemKindMethod}}
	got := completionLabels(addUserMethods(seed, doc, "Point", "", false))
	if len(got) != 2 { // move (seed) + sum
		t.Fatalf("expected seed move plus sum, got %v", got)
	}
}

func TestMethodCompletionUnknownType(t *testing.T) {
	doc := buildDocForMethods(t, methodsCompletionProgram)
	got := addUserMethods(nil, doc, "Nope", "", false)
	if len(got) != 0 {
		t.Fatalf("expected nothing for unknown type, got %v", got)
	}
}
