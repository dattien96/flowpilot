package flowgate

import (
	"crypto/sha256"
	"path/filepath"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

// CP-67 P-2/P-2b (Task-379 / Task-383): canonical AST signature extraction.
//
// Purity contract: the core functions here take the file CONTENT as bytes (and
// an optional LSP-backed SymbolSource for non-Go languages) — flowgate never
// touches the disk itself, the same caller-computed contract as the
// ReproduceExpected signals. The runner's gate hook reads each declared file
// once and feeds it in.

// BodyShape classifies a symbol's body against the CP-67 §3.2 stub whitelist
// (B-11). Signature hashing ignores it entirely (B-8.1: signature-only), but
// the scaffold gate uses it as detect signal 3.
type BodyShape int

const (
	// BodyStubTODO marks an expression-body TODO stub (Kotlin `= TODO(...)`).
	BodyStubTODO BodyShape = iota
	// BodyStubThrow marks a single `throw ...("not implemented")` body.
	BodyStubThrow
	// BodyStubReturnZero marks a single return of a zero sentinel / not-
	// implemented error (Go `return nil, errors.New("not implemented")`).
	BodyStubReturnZero
	// BodyNonStub marks any body outside the whitelist — real logic hidden in
	// a stub, multi-statement bodies, or user-function calls.
	BodyNonStub
	// BodyUnverified marks a symbol the extractor could not verify (parser
	// unavailable, dialect mismatch) — the gate fails OPEN on it with
	// evidence instead of blocking the workflow.
	BodyUnverified
)

// String renders the shape for gate evidence lines.
func (b BodyShape) String() string {
	switch b {
	case BodyStubTODO:
		return "stub_todo"
	case BodyStubThrow:
		return "stub_throw"
	case BodyStubReturnZero:
		return "stub_return_zero"
	case BodyNonStub:
		return "non_stub"
	default:
		return "unverified"
	}
}

// SymbolInfo is one extracted declaration: the canonical signature string the
// hash consumes, plus the body shape and line the scaffold gate reports.
type SymbolInfo struct {
	Name      string
	Kind      string // function | method | interface | struct | class
	Signature string
	BodyShape BodyShape
	Line      int
}

// DocumentSymbol is the runner-side LSP shape (textDocument/documentSymbol
// result reduced to what anchored extraction needs). flowgate cannot import
// the runner's lsp_client, so the runner adapts its results into this.
type DocumentSymbol struct {
	Name      string
	Kind      string // "function" | "method" | "interface" | "struct" | "class"
	Detail    string // signature text when the server provides it
	StartLine int    // 1-based, inclusive
	EndLine   int    // 1-based, inclusive (declaration end, body end when present)
}

// SymbolSource is the LSP-backed symbol lookup the runner injects for
// languages flowgate cannot parse natively (Kotlin, C/C++ without the
// treesitter tag). Nil/unavailable means Unverified, never a gate block.
type SymbolSource interface {
	DocumentSymbols(filePath string) ([]DocumentSymbol, error)
}

// ExtractSymbolInfos parses src (the file's bytes) into symbol declarations
// for lang. dispatch:
//
//   - go:    native go/ast walk (exact, statement-level body shapes)
//   - react: node subprocess via the extract-ts.mjs helper (exact)
//   - kotlin/cpp: LSP-anchored via src (SymbolSource) + body-text shape match
//
// src (bytes) may be nil for react (the subprocess reads the file itself);
// lsp may be nil, in which case non-Go languages come back Unverified.
func ExtractSymbolInfos(lang, filename string, src []byte, lsp SymbolSource) ([]SymbolInfo, error) {
	switch normalizeLang(lang) {
	case "go":
		return extractGoSymbols(filename, src)
	case "react":
		return extractReactSymbols(filename, src)
	case "kotlin":
		return extractAnchoredSymbols("kotlin", filename, src, lsp)
	case "cpp":
		return extractCppSymbols(filename, src, lsp)
	default:
		return nil, fmt.Errorf("ast_signatures: unsupported language %q", lang)
	}
}

// ExtractCanonicalSignatures returns the sorted canonical signature strings
// for the file — the P-2 shape the SignatureHash snapshot consumes.
func ExtractCanonicalSignatures(lang, filename string, src []byte, lsp SymbolSource) ([]string, error) {
	syms, err := ExtractSymbolInfos(lang, filename, src, lsp)
	if err != nil {
		return nil, err
	}
	return CanonicalStrings(syms), nil
}

// CanonicalStrings reduces symbol infos to their canonical signature strings
// in sorted order — the stable canonical form every consumer (hash, lock
// snapshot, drift diff) shares.
func CanonicalStrings(syms []SymbolInfo) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.Signature)
	}
	sort.Strings(out)
	return out
}

// CanonicalSignatureHash sorts the signatures, joins them, and hashes the
// canonical text with SHA256 (Task-379 T-4). Deterministic for the same
// declaration set regardless of source order. Signature-only: bodies are
// excluded by construction, so every add/remove/modify of a declaration moves
// the hash (B-8.1) while body-only edits never do.
func CanonicalSignatureHash(signatures []string) string {
	sorted := append([]string(nil), signatures...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])
}

func normalizeLang(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "go", "golang":
		return "go"
	case "react", "typescript", "ts", "tsx", "javascript", "js", "jsx":
		return "react"
	case "kotlin", "kt":
		return "kotlin"
	case "cpp", "c++", "c":
		return "cpp"
	default:
		return strings.ToLower(strings.TrimSpace(lang))
	}
}

// --- Go (native go/ast, exact) ----------------------------------------------

// extractGoSymbols walks the parsed AST and collects FuncDecls (methods keep
// their receiver), interface method sets, and struct type declarations. The
// canonical signature renders WITHOUT the body — the body only feeds
// BodyShape.
func extractGoSymbols(filename string, src []byte) ([]SymbolInfo, error) {
	fset := token.NewFileSet()
	mode := parser.ParseComments // comments never leak into signatures; needed for accurate offsets
	file, err := parser.ParseFile(fset, filename, src, mode)
	if err != nil {
		return nil, fmt.Errorf("go parse %s: %w", filename, err)
	}
	out := make([]SymbolInfo, 0, 16)

	// Package-level var initializers and init() are the Go hiding spots the
	// B-11 whitelist has to cover: logic smuggled there never shows up in a
	// function body.
	out = append(out, goPackageLevelShapes(file)...)

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			out = append(out, goFuncInfo(fset, d))
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				switch t := ts.Type.(type) {
				case *ast.InterfaceType:
					out = append(out, SymbolInfo{
						Name:      ts.Name.Name,
						Kind:      "interface",
						Signature: goInterfaceSignature(ts.Name.Name, t),
						BodyShape: bodyShapeOfInterface(t),
						Line:      fset.Position(ts.Pos()).Line,
					})
				case *ast.StructType:
					out = append(out, SymbolInfo{
						Name:      ts.Name.Name,
						Kind:      "struct",
						Signature: goStructSignature(ts.Name.Name, t),
						BodyShape: BodyStubTODO, // field lists carry no executable body
						Line:      fset.Position(ts.Pos()).Line,
					})
				}
			}
		}
	}
	return out, nil
}

func goFuncInfo(fset *token.FileSet, d *ast.FuncDecl) SymbolInfo {
	sig := renderGoFuncSignature(d)
	kind := "function"
	if d.Recv != nil {
		kind = "method"
	}
	return SymbolInfo{
		Name:      d.Name.Name,
		Kind:      kind,
		Signature: sig,
		BodyShape: goBodyShape(d.Body),
		Line:      fset.Position(d.Pos()).Line,
	}
}

// renderGoFuncSignature renders "func (r Recv) Name(params) results" without
// the body, with whitespace normalized (types.Print-style but canonical).
func renderGoFuncSignature(d *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("func ")
	if d.Recv != nil {
		b.WriteString("(")
		b.WriteString(renderGoField(d.Recv.List[0]))
		b.WriteString(") ")
	}
	b.WriteString(d.Name.Name)
	b.WriteString(renderGoFuncType(d.Type))
	return b.String()
}

func renderGoFuncType(t *ast.FuncType) string {
	var b strings.Builder
	b.WriteString("(")
	// Canonical form keeps parameter names when declared (CP-67-Test-Steps
	// §2.1: "func CreateUser(name string) (*User, error)").
	b.WriteString(renderGoFieldList(t.Params, true))
	b.WriteString(")")
	if t.Results != nil && len(t.Results.List) > 0 {
		b.WriteString(" ")
		hasNames := false
		for _, f := range t.Results.List {
			if len(f.Names) > 0 {
				hasNames = true
			}
		}
		if hasNames || len(t.Results.List) > 1 {
			b.WriteString("(")
			b.WriteString(renderGoFieldList(t.Results, true))
			b.WriteString(")")
		} else {
			b.WriteString(renderGoFieldList(t.Results, false))
		}
	}
	return b.String()
}

func renderGoFieldList(fl *ast.FieldList, keepNames bool) string {
	parts := make([]string, 0, len(fl.List))
	for _, f := range fl.List {
		if keepNames {
			parts = append(parts, renderGoField(f))
		} else {
			parts = append(parts, exprString(f.Type))
		}
	}
	return strings.Join(parts, ", ")
}

func renderGoField(f *ast.Field) string {
	names := make([]string, 0, len(f.Names))
	for _, n := range f.Names {
		names = append(names, n.Name)
	}
	if len(names) == 0 {
		return exprString(f.Type)
	}
	return strings.Join(names, ", ") + " " + exprString(f.Type)
}

// exprString renders an arbitrary Go expression/type text via the printer —
// deterministic for the same AST.
func exprString(e ast.Node) string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	if err := printerFprint(&b, e); err != nil {
		return ""
	}
	return b.String()
}

func goInterfaceSignature(name string, t *ast.InterfaceType) string {
	methods := make([]string, 0, 4)
	for _, f := range t.Methods.List {
		switch m := f.Type.(type) {
		case *ast.FuncType:
			// Interface method rows render WITHOUT the "func" keyword — the
			// CP-67-Test-Steps §2.1 canonical shape.
			fn := &ast.FuncDecl{Name: identOf(f.Names), Type: m}
			methods = append(methods, strings.TrimPrefix(renderGoFuncSignature(fn), "func "))
		default:
			// Embedded interface.
			methods = append(methods, exprString(f.Type))
		}
	}
	sort.Strings(methods)
	if len(methods) == 0 {
		return "type " + name + " interface"
	}
	return "type " + name + " interface { " + strings.Join(methods, "; ") + " }"
}

func identOf(names []*ast.Ident) *ast.Ident {
	if len(names) > 0 {
		return names[0]
	}
	return nil
}

func goStructSignature(name string, t *ast.StructType) string {
	fields := make([]string, 0, len(t.Fields.List))
	for _, f := range t.Fields.List {
		fields = append(fields, renderGoField(f))
	}
	if len(fields) == 0 {
		return "type " + name + " struct"
	}
	return "type " + name + " struct { " + strings.Join(fields, "; ") + " }"
}

func bodyShapeOfInterface(t *ast.InterfaceType) BodyShape {
	return BodyStubTODO // declarations carry no executable body
}

// LangForPath maps a workspace-relative path to the extractor language key —
// "" for files no adapter owns (docs, configs, unknown extensions). Test
// files keep their language so the scaffold signature snapshot can skip them
// explicitly.
func LangForPath(path string) string {
	switch strings.ToLower(strings.TrimSpace(filepath.Ext(path))) {
	case ".go":
		return "go"
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		return "react"
	case ".kt", ".kts":
		return "kotlin"
	case ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hh":
		return "cpp"
	default:
		return ""
	}
}
