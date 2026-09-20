package flowgate

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"regexp"
	"strings"
)

// CP-67 P-2b (Task-383, B-11): static stub-body whitelist. The scaffold gate's
// detect signal 3 — deterministic, independent of the suite's red/green
// outcome, closing the "wrote real (or half-right) logic but the suite stays
// red" hole. Shape-based, never string-based: bodies must MATCH one of the
// canonical stub statement shapes; message wording is normalized away.

// notImplementedRe matches the canonical not-implemented wording family across
// languages ("not implemented", "notimplemented", "not_implemented").
var notImplementedRe = regexp.MustCompile(`(?i)not[\s_.-]?implemented`)

// BodyViolation names the symbol whose body sits outside the scaffold
// whitelist, with the 1-based line the reprompt points at.
type BodyViolation struct {
	Symbol string
	Line   int
	Reason string
}

// ValidateStubBodies checks every symbol's body against the per-language
// whitelist and returns the violations (empty = scaffold bodies are clean).
// Pure: bytes in, violations out.
func ValidateStubBodies(lang, filename string, src []byte, lsp SymbolSource, syms []SymbolInfo) []BodyViolation {
	var out []BodyViolation
	for _, s := range syms {
		if s.BodyShape == BodyNonStub {
			out = append(out, BodyViolation{
				Symbol: s.Name,
				Line:   s.Line,
				Reason: fmt.Sprintf("body outside the %s stub whitelist (shape %s)", normalizeLang(lang), s.BodyShape),
			})
		}
	}
	// Language-specific hiding spots beyond the per-symbol shapes (Go package
	// vars/init are folded into goPackageLevelShapes already; the anchored
	// languages get their own checks inside their extractors).
	switch normalizeLang(lang) {
	case "kotlin":
		out = append(out, kotlinHidingSpotViolations(filename, src, lsp, syms)...)
	case "cpp":
		out = append(out, cppHidingSpotViolations(filename, src, lsp, syms)...)
	}
	return out
}

// --- Go body shapes (exact, statement-level) --------------------------------

// goBodyShape classifies a function body against the Go stub whitelist:
// exactly ONE statement, either a panic(string literal) or a return whose
// operands are all zero-sentinel / not-implemented error expressions.
func goBodyShape(body *ast.BlockStmt) BodyShape {
	if body == nil {
		return BodyStubTODO // declaration without a body (assembly etc.)
	}
	if len(body.List) == 0 {
		return BodyNonStub // empty body silently "passes" — a stub must state itself
	}
	if len(body.List) != 1 {
		return BodyNonStub
	}
	switch stmt := body.List[0].(type) {
	case *ast.ExprStmt:
		call, ok := stmt.X.(*ast.CallExpr)
		if !ok {
			return BodyNonStub
		}
		if isGoPanic(call) && len(call.Args) == 1 && isNotImplementedString(call.Args[0]) {
			return BodyStubThrow
		}
		return BodyNonStub
	case *ast.ReturnStmt:
		if goReturnIsStub(stmt) {
			return BodyStubReturnZero
		}
		return BodyNonStub
	default:
		return BodyNonStub
	}
}

func isGoPanic(call *ast.CallExpr) bool {
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "panic"
}

// stubOperandRe matches the identifier family FlowPilot scaffold prompts
// generate for sentinel errors (ErrNotImplemented, errNotImplemented...).
var stubOperandRe = regexp.MustCompile(`(?i)^(err[a-z_]*)?not[_]?implemented$`)

// goReturnIsStub reports whether every return operand is a zero sentinel:
// nil, a zero literal, an errors.New/fmt.Errorf not-implemented literal, or
// an Err*NotImplemented sentinel identifier.
func goReturnIsStub(stmt *ast.ReturnStmt) bool {
	if len(stmt.Results) == 0 {
		return true // bare return in a stub-shaped zero-return function
	}
	for _, r := range stmt.Results {
		if !goExprIsZeroSentinel(r) {
			return false
		}
	}
	return true
}

func goExprIsZeroSentinel(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.Ident:
		if v.Name == "nil" || v.Name == "false" {
			return true
		}
		return stubOperandRe.MatchString(v.Name)
	case *ast.BasicLit:
		switch v.Kind {
		case token.INT, token.FLOAT:
			return isZeroNumber(v.Value)
		case token.STRING:
			return v.Value == `""` || v.Value == "``"
		}
		return false
	case *ast.CallExpr:
		// errors.New("...") / fmt.Errorf("...") with a not-implemented message.
		sel, ok := v.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return false
		}
		isErrors := pkg.Name == "errors" && sel.Sel.Name == "New"
		isFmt := pkg.Name == "fmt" && sel.Sel.Name == "Errorf"
		if !isErrors && !isFmt {
			return false
		}
		return len(v.Args) >= 1 && isNotImplementedString(v.Args[0])
	default:
		return false
	}
}

func isZeroNumber(lit string) bool {
	lit = strings.TrimSpace(lit)
	// Allow 0, 0.0, 0e0, and trailing-f zero forms; nothing else.
	for _, r := range lit {
		switch {
		case r == '0' || r == '.' || r == 'e' || r == 'E' || r == '+' || r == '-':
		case r == 'f' || r == 'i': // 0.0f-style suffixed forms
		default:
			return false
		}
	}
	return strings.Contains(lit, "0")
}

func isNotImplementedString(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	return notImplementedRe.MatchString(lit.Value)
}

// goPackageLevelShapes flags logic smuggled outside function bodies: a
// non-trivial package-level var initializer or a non-empty init().
func goPackageLevelShapes(file *ast.File) []SymbolInfo {
	var out []SymbolInfo
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == "init" && d.Recv == nil && d.Body != nil && len(d.Body.List) > 0 {
				out = append(out, SymbolInfo{
					Name: "init", Kind: "function", Signature: "func init()",
					BodyShape: BodyNonStub, Line: posLine(d.Pos()),
				})
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, v := range vs.Values {
					if goExprIsZeroSentinel(v) {
						continue
					}
					name := fmt.Sprintf("var %s", valueSpecName(vs, i))
					out = append(out, SymbolInfo{
						Name: name, Kind: "function", Signature: name + " (non-stub initializer)",
						BodyShape: BodyNonStub, Line: posLine(v.Pos()),
					})
				}
			}
		}
	}
	return out
}

func valueSpecName(vs *ast.ValueSpec, i int) string {
	if i < len(vs.Names) {
		return vs.Names[i].Name
	}
	return "?"
}

// posLine converts a token.Pos to a 1-based line without a FileSet — used
// only for package-level shapes where the exact line matters less than the
// symbol name; the caller-side fset pass fills precise lines when needed.
func posLine(p token.Pos) int { return int(p) }

// --- printer helper ----------------------------------------------------------

// printerFprint renders an AST node back to canonical Go text.
func printerFprint(b *strings.Builder, n ast.Node) error {
	fset := token.NewFileSet()
	return printer.Fprint(b, fset, n)
}

// --- anchored text shapes (Kotlin / C++ / React text bodies) ----------------

// trimAnchoredBody extracts the body text of an anchored declaration: the
// text from the line after the signature header through the declaration's end
// line, with comments and blank lines stripped. The LSP range anchors the
// slice, so string/comment content OUTSIDE the declaration never pollutes the
// shape match (the full-file regex problem B-8.4 avoids).
func trimAnchoredBody(src []byte, sym DocumentSymbol) string {
	text := string(src)
	lines := strings.Split(text, "\n")
	if sym.StartLine < 1 || sym.EndLine < sym.StartLine || sym.StartLine > len(lines) {
		return ""
	}
	end := sym.EndLine
	if end > len(lines) {
		end = len(lines)
	}
	start := sym.StartLine - 1 // 1-based → 0-based; keep the signature line
	var kept []string
	for i := start; i < end; i++ {
		line := strings.TrimSpace(stripLineComment(lines[i], normalizeFileLang(sym)))
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "/*") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, " ")
}

func normalizeFileLang(sym DocumentSymbol) string { return "" }

// stripLineComment removes a trailing // comment outside string literals —
// a quote-aware scan, not a blind split.
func stripLineComment(line string, _ string) string {
	inStr := false
	strCh := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == strCh {
				inStr = false
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = true
			strCh = c
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return line[:i]
			}
		}
	}
	return line
}

// textBodyShape matches a body text (signature line already included) against
// the anchored whitelist. Anything with control flow or multiple statements is
// NonStub; a single canonical stub statement matches its shape.
func textBodyShape(body string, stubThrowRe *regexp.Regexp, stubReturnRe *regexp.Regexp) BodyShape {
	b := strings.TrimSpace(body)
	if b == "" {
		return BodyUnverified
	}
	if stubThrowRe != nil && stubThrowRe.MatchString(b) {
		return BodyStubThrow
	}
	if stubReturnRe != nil && stubReturnRe.MatchString(b) {
		return BodyStubReturnZero
	}
	// TODO / NotImplementedError expression bodies.
	if notImplementedRe.MatchString(b) {
		if todoRe.MatchString(b) || notImplThrowRe.MatchString(b) {
			return BodyStubTODO
		}
		return BodyStubThrow
	}
	return BodyNonStub
}

var todoRe = regexp.MustCompile(`\bTODO\s*\(`)
var notImplThrowRe = regexp.MustCompile(`(?i)throw\s+new\s+[A-Za-z_.]*Error|NotImplementedError|std::(runtime|logic)_error`)
