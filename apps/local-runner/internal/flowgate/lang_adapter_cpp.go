package flowgate

import (
	"fmt"
	"regexp"
	"strings"
)

// CP-67 P-2b (Task-383 B-11): C/C++ adapter.
//
// Primary path: clangd LSP-anchored extraction via the injected SymbolSource
// (R-6 mitigation — the default build keeps CGO_ENABLED=0, so the exact
// tree-sitter-c/cpp parser is a documented follow-up behind the `treesitter`
// build tag once the go-tree-sitter dependency is wired). Without LSP the
// adapter degrades to a structured-regex signature pass with Unverified
// bodies (fail-open).
//
// C/C++ specifics this adapter owns:
//   - declaration (.h) vs definition (.c/.cpp) dedupe: the caller dedupes the
//     canonical signature list across the declared file set, so a prototype
//     plus its definition count as ONE symbol (TestCppDeclDefDedupe).
//   - macro hiding spots: a stub whose body is a single macro call is fine
//     ONLY while the macro body (inside the declared file set) is itself a
//     stub statement; a multi-statement macro definition is flagged.
//   - `#include` lines are preprocessor text, never symbols — adding an
//     include never moves the SignatureHash.

var cppStubThrowRe = regexp.MustCompile(`throw\s+std::(runtime_error|logic_error|invalid_argument)\s*\(\s*"[^"]*(?:not[\s_.-]?implemented)[^"]*"\s*\)`)

// cppStubReturnRe matches a single return of a zero sentinel.
var cppStubReturnRe = regexp.MustCompile(`return\s+(nullptr|NULL|0(\.0+f?|\.0)?|-1|false|\{\})\s*;`)

// cppAssertStubRe matches assert(0 && "not implemented").
var cppAssertStubRe = regexp.MustCompile(`assert\s*\(\s*0\s*&&\s*"[^"]*(?:not[\s_.-]?implemented)[^"]*"\s*\)`)

// cppBodyShape: a single canonical stub statement, else NonStub.
func cppBodyShape(body string) BodyShape {
	b := strings.TrimSpace(body)
	if b == "" {
		return BodyUnverified
	}
	// Strip the signature header up to the first `{`.
	bodyPart := b
	if idx := strings.Index(b, "{"); idx >= 0 {
		bodyPart = b[idx:]
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(bodyPart, "{"), "}"))
	if inner == "" {
		return BodyUnverified
	}
	// Drop a single trailing semicolon normalization happens inside matchers.
	if cppStubThrowRe.MatchString(inner) {
		return BodyStubThrow
	}
	if cppAssertStubRe.MatchString(inner) {
		return BodyStubThrow
	}
	if cppStubReturnRe.MatchString(inner) && !strings.Contains(strings.TrimPrefix(inner, "return"), "return") {
		return BodyStubReturnZero
	}
	return BodyNonStub
}

// cppMacroDefRe matches `#define NAME(args...) body` with the body captured.
var cppMacroDefRe = regexp.MustCompile(`(?m)^\s*#\s*define\s+(\w+)\s*(\([^)]*\))?\s+(.+)$`)

// cppHidingSpotViolations flags multi-statement macro definitions (logic
// hidden in a macro the stub "calls") and computed global/static
// initializers.
func cppHidingSpotViolations(filename string, src []byte, lsp SymbolSource, syms []SymbolInfo) []BodyViolation {
	if len(src) == 0 {
		return nil
	}
	text := string(src)
	var out []BodyViolation
	for _, m := range cppMacroDefRe.FindAllStringSubmatch(text, -1) {
		body := strings.TrimSpace(stripLineComment(m[3], ""))
		// Continuation lines (\) are invisible here; a multi-line macro body
		// leaves the final physical line non-terminated, which the statement
		// shape check below treats as suspicious only when it carries logic.
		if isCppSingleStubStatement(body) {
			continue
		}
		line := lineOfOffset(text, strings.Index(text, m[0]))
		out = append(out, BodyViolation{
			Symbol: "macro " + m[1],
			Line:   line,
			Reason: "macro definition carries multi-statement logic — stub files must not hide implementation in macros",
		})
	}
	// Computed global initializers: `static X y = f(...)` at file scope.
	globalInitRe := regexp.MustCompile(`(?m)^\s*(?:static\s+)?(?:const\s+)?[\w:<>,\s\*&]+\s+(\w+)\s*=\s*[A-Za-z_]\w*\s*\(`)
	for _, m := range globalInitRe.FindAllStringSubmatch(text, -1) {
		line := lineOfOffset(text, strings.Index(text, m[0]))
		out = append(out, BodyViolation{
			Symbol: m[1],
			Line:   line,
			Reason: "global initializer calls a function — stub files allow zero sentinels only",
		})
	}
	return out
}

// isCppSingleStubStatement reports whether one physical statement matches the
// stub whitelist (throw / assert(0) / zero-sentinel return).
func isCppSingleStubStatement(stmt string) bool {
	if cppStubThrowRe.MatchString(stmt) || cppAssertStubRe.MatchString(stmt) || cppStubReturnRe.MatchString(stmt) {
		return true
	}
	return false
}

// ExtractCppFileSignatures is the convenience the gate hook calls.
func ExtractCppFileSignatures(filename string, src []byte, lsp SymbolSource) ([]string, error) {
	return ExtractCanonicalSignatures("cpp", filename, src, lsp)
}

// extractCppSymbols is ExtractSymbolInfos's cpp dispatch target.
func extractCppSymbols(filename string, src []byte, lsp SymbolSource) ([]SymbolInfo, error) {
	if lsp != nil {
		if syms, ok := anchoredViaLSP("cpp", filename, src, lsp); ok {
			return syms, nil
		}
	}
	return anchoredRegexSymbols("cpp", filename, src), nil
}

// DedupeDeclAndDef collapses a symbol list where a header prototype and its
// definition share a canonical name+params shape, keeping the DEFINITION (the
// row with a verified body). TestCppDeclDefDedupeSignatureHashStable covers it.
func DedupeDeclAndDef(syms []SymbolInfo) []SymbolInfo {
	byKey := make(map[string]int, len(syms))
	out := make([]SymbolInfo, 0, len(syms))
	for _, s := range syms {
		key := cppCanonicalKey(s)
		if idx, ok := byKey[key]; ok {
			// Prefer the row with a concrete body shape over a bare prototype.
			if out[idx].BodyShape == BodyUnverified && s.BodyShape != BodyUnverified {
				out[idx] = s
			}
			continue
		}
		byKey[key] = len(out)
		out = append(out, s)
	}
	return out
}

// cppCanonicalKey normalizes a symbol for decl-vs-def matching: name +
// parameter-type sketch, whitespace-insensitive.
func cppCanonicalKey(s SymbolInfo) string {
	sig := sanitizeSigText(s.Signature)
	sig = regexp.MustCompile(`\s+`).ReplaceAllString(sig, " ")
	return strings.ToLower(sig)
}

var _ = fmt.Sprintf
