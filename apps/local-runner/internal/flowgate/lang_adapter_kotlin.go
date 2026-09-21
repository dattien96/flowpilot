package flowgate

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// CP-67 P-2b (Task-383 B-11): Kotlin adapter. LSP-anchored extraction —
// documentSymbol ranges anchor the declaration; body text is sliced from the
// caller-provided bytes and matched against the whitelist (near-exact, no
// full-file regex ambiguity). Without an LSP source the adapter degrades to a
// structured-regex signature pass with Unverified bodies (fail-open).

// ExtractKotlinSymbols is ExtractSymbolInfos's kotlin dispatch target.
func extractAnchoredSymbols(lang, filename string, src []byte, lsp SymbolSource) ([]SymbolInfo, error) {
	if lsp != nil {
		if syms, ok := anchoredViaLSP(lang, filename, src, lsp); ok {
			return syms, nil
		}
	}
	return anchoredRegexSymbols(lang, filename, src), nil
}

func anchoredViaLSP(lang, filename string, src []byte, lsp SymbolSource) ([]SymbolInfo, bool) {
	docs, err := lsp.DocumentSymbols(filename)
	if err != nil || len(docs) == 0 {
		return nil, false
	}
	syms := make([]SymbolInfo, 0, len(docs))
	for _, d := range docs {
		sig := d.Detail
		if sig == "" {
			sig = anchoredSignatureLine(src, d.StartLine)
		}
		if sig == "" {
			continue
		}
		body := trimAnchoredBody(src, d)
		syms = append(syms, SymbolInfo{
			Name:      d.Name,
			Kind:      d.Kind,
			Signature: sanitizeSigText(sig),
			BodyShape: anchoredBodyShape(lang, body, d.Kind),
			Line:      d.StartLine,
		})
	}
	if len(syms) == 0 {
		return nil, false
	}
	// Hiding-spot sweep (init blocks, property initializers, default params).
	syms = append(syms, anchoredHidingSpots(lang, filename, src, syms)...)
	return syms, true
}

// anchoredSignatureLine returns the declaration header text on a 1-based line.
func anchoredSignatureLine(src []byte, line int) string {
	if len(src) == 0 || line < 1 {
		return ""
	}
	lines := strings.Split(string(src), "\n")
	if line > len(lines) {
		return ""
	}
	return strings.TrimSpace(stripLineComment(lines[line-1], ""))
}

// anchoredBodyShape matches an anchored body against the per-language
// whitelist. Interface/struct/class declarations carry no executable body.
func anchoredBodyShape(lang, body, kind string) BodyShape {
	switch kind {
	case "interface", "struct", "class":
		return BodyStubTODO
	}
	switch normalizeLang(lang) {
	case "kotlin":
		return kotlinBodyShape(body)
	default:
		return cppBodyShape(body)
	}
}

var kotlinTODORe = regexp.MustCompile(`=\s*TODO\s*\(`)
var kotlinThrowRe = regexp.MustCompile(`throw\s+NotImplementedError\s*\(`)
var kotlinErrorFnRe = regexp.MustCompile(`\berror\s*\(\s*"[^"]*(?:not[\s_.-]?implemented)[^"]*"\s*\)`)

// kotlinBodyShape: expression `= TODO(...)` or a single throw/error statement.
func kotlinBodyShape(body string) BodyShape {
	b := strings.TrimSpace(body)
	if b == "" {
		return BodyUnverified
	}
	// Strip the signature header up to the first `{` or `=` body anchor.
	bodyPart := b
	if idx := strings.Index(b, "{"); idx >= 0 {
		bodyPart = b[idx:]
	} else if idx := strings.Index(b, "="); idx >= 0 {
		bodyPart = b[idx:]
	}
	if kotlinTODORe.MatchString(bodyPart) {
		return BodyStubTODO
	}
	if kotlinThrowRe.MatchString(bodyPart) || kotlinErrorFnRe.MatchString(bodyPart) {
		return BodyStubThrow
	}
	// A `= <expr>` body that is not TODO is real logic.
	return BodyNonStub
}

// kotlinInitBlockRe finds `init { ... }` blocks; content beyond whitespace
// and comments is a hiding-spot violation.
var kotlinInitBlockRe = regexp.MustCompile(`(?m)^\s*init\s*\{([^}]*)\}`)

// kotlinPropInitRe finds property declarations with computed initializers —
// any initializer that is not a literal / TODO is a hiding spot.
var kotlinPropInitRe = regexp.MustCompile(`(?m)^\s*(?:private\s+|internal\s+|public\s+)?(?:val|var)\s+(\w+)\s*(?::\s*[\w.<>?]+)?\s*=\s*(.+)$`)

func kotlinHidingSpotViolations(filename string, src []byte, lsp SymbolSource, syms []SymbolInfo) []BodyViolation {
	if len(src) == 0 {
		return nil
	}
	text := string(src)
	var out []BodyViolation
	for _, m := range kotlinInitBlockRe.FindAllStringSubmatch(text, -1) {
		inner := strings.TrimSpace(stripKotlinComments(m[1]))
		if inner != "" {
			line := lineOfOffset(text, strings.Index(text, m[0]))
			out = append(out, BodyViolation{Symbol: "init block", Line: line, Reason: "init block carries executable logic — stub files must keep init empty"})
		}
	}
	for _, m := range kotlinPropInitRe.FindAllStringSubmatch(text, -1) {
		init := strings.TrimSpace(m[2])
		if isKotlinStubInitializer(init) {
			continue
		}
		line := lineOfOffset(text, strings.Index(text, m[0]))
		out = append(out, BodyViolation{Symbol: m[1], Line: line, Reason: "property initializer computes a value — stub files allow literals and TODO() only"})
	}
	return out
}

// anchoredHidingSpots folds hiding-spot findings into Unverified... actually
// into NonStub symbol infos so the per-symbol violation path reports them.
func anchoredHidingSpots(lang, filename string, src []byte, syms []SymbolInfo) []SymbolInfo {
	var out []SymbolInfo
	var viols []BodyViolation
	switch normalizeLang(lang) {
	case "kotlin":
		viols = kotlinHidingSpotViolations(filename, src, nil, syms)
	default:
		viols = cppHidingSpotViolations(filename, src, nil, syms)
	}
	for _, v := range viols {
		out = append(out, SymbolInfo{
			Name: v.Symbol, Kind: "function",
			Signature: v.Symbol + " (hiding spot)",
			BodyShape: BodyNonStub, Line: v.Line,
		})
	}
	return out
}

func isKotlinStubInitializer(init string) bool {
	if init == "" {
		return true
	}
	if todoRe.MatchString(init) {
		return true
	}
	// String/char/number/boolean literals and simple collections of literals.
	if regexp.MustCompile(`^"[^"]*"$`).MatchString(init) ||
		regexp.MustCompile(`^'.'$`).MatchString(init) ||
		regexp.MustCompile(`^-?\d+(\.\d+)?[fL]?$`).MatchString(init) ||
		init == "true" || init == "false" || init == "null" {
		return true
	}
	return false
}

func stripKotlinComments(s string) string {
	s = regexp.MustCompile(`/\*.*?\*/`).ReplaceAllString(s, "")
	return regexp.MustCompile(`//.*`).ReplaceAllString(s, "")
}

func lineOfOffset(text string, off int) int {
	if off < 0 {
		return 0
	}
	return 1 + strings.Count(text[:off], "\n")
}

// ExtractKotlinFileSignatures is the convenience the gate hook calls.
func ExtractKotlinFileSignatures(filename string, src []byte, lsp SymbolSource) ([]string, error) {
	return ExtractCanonicalSignatures("kotlin", filename, src, lsp)
}

// anchoredRegexSymbols is the degraded pass for anchored languages: signature
// lines only (fun/function), bodies Unverified.
func anchoredRegexSymbols(lang, filename string, src []byte) []SymbolInfo {
	if len(src) == 0 {
		return nil
	}
	var re *regexp.Regexp
	switch normalizeLang(lang) {
	case "kotlin":
		re = regexp.MustCompile(`^\s*(?:override\s+|suspend\s+)*fun\s+(?:<[^>]+>\s+)?(\w+)\s*\(([^)]*)\)\s*(?::\s*([^{=\n]+))?`)
	default:
		re = regexp.MustCompile(`^\s*(?:[\w:*<>\[\]]+\s+)+?\**(\w+)\s*\(([^;{)]*)\)\s*(?:const)?\s*\{?\s*$`)
	}
	var out []SymbolInfo
	lines := strings.Split(string(src), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(stripLineComment(line, ""))
		m := re.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		kind := "function"
		if normalizeLang(lang) == "kotlin" && strings.Contains(trimmed, "class ") {
			kind = "class"
		}
		out = append(out, SymbolInfo{
			Name: m[1], Kind: kind,
			Signature: sanitizeSigText(trimmed),
			BodyShape: BodyUnverified, Line: i + 1,
		})
	}
	return out
}

// ensure os import stays used regardless of future edits.
var _ = os.Getenv
var _ = fmt.Sprintf
