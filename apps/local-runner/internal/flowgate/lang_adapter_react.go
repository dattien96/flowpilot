package flowgate

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CP-67 P-2b (Task-383 B-11): React/TS adapter. Exact extraction runs the
// workspace's own TypeScript compiler via a node subprocess (a React repo
// always has node + typescript — CP-67 Q-1). When node or the typescript
// package is unavailable, the adapter DEGRADES to a structured-regex
// signature pass with BodyUnverified bodies: signatures stay hashable, the
// body check fails open with evidence instead of blocking the workflow.

//go:embed scripts/extract-ts.mjs
var extractTSScript string

// reactExtractTimeout bounds the node subprocess — a hung toolchain must
// degrade, never stall the gate.
const reactExtractTimeout = 15 * time.Second

type reactSymbolJSON struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

type reactExtractJSON struct {
	Symbols []reactSymbolJSON `json:"symbols"`
}

// ExtractReactSymbols is ExtractSymbolInfos's react dispatch target. src may
// be nil (the subprocess reads the file itself); workspaceCwd anchors
// node_modules resolution for the typescript import.
func extractReactSymbols(filename string, src []byte) ([]SymbolInfo, error) {
	if syms, ok := reactViaNode(filename); ok {
		return syms, nil
	}
	// Degraded path: regex signatures, unverified bodies.
	return reactRegexSymbols(filename, src), nil
}

func reactViaNode(filename string) ([]SymbolInfo, bool) {
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, false
	}
	tmp, err := os.CreateTemp("", "flowpilot-extract-ts-*.mjs")
	if err != nil {
		return nil, false
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(extractTSScript); err != nil {
		tmp.Close()
		return nil, false
	}
	tmp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), reactExtractTimeout)
	defer cancel()
	// Dir stays empty on purpose: the adapter must resolve the workspace's
	// typescript package, so the caller sets the exec Dir via the workspace.
	cmd := exec.CommandContext(ctx, node, tmp.Name(), filename)
	cmd.Dir = reactWorkdir(filename)
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	var parsed reactExtractJSON
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, false
	}
	syms := make([]SymbolInfo, 0, len(parsed.Symbols))
	for _, s := range parsed.Symbols {
		shape := BodyUnverified // raw JSON carries no body text; bodies validate below
		syms = append(syms, SymbolInfo{
			Name:      s.Name,
			Kind:      s.Kind,
			Signature: s.Signature,
			BodyShape: shape,
			Line:      s.StartLine,
		})
	}
	// Refine body shapes from the file text via the anchored ranges.
	refineReactBodyShapes(filename, syms, parsed.Symbols)
	return syms, true
}

// reactWorkdir picks the directory whose node_modules should resolve: the
// nearest ancestor of the file that contains node_modules, else the file's
// directory.
func reactWorkdir(filename string) string {
	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "node_modules")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(filename)
		}
		dir = parent
	}
}

// refineReactBodyShapes slices each symbol's body text from the file using
// the anchored ranges and matches it against the TS/React whitelist.
func refineReactBodyShapes(filename string, syms []SymbolInfo, raw []reactSymbolJSON) {
	textBytes, err := os.ReadFile(filename)
	if err != nil {
		return // bodies stay Unverified (fail-open)
	}
	lines := strings.Split(string(textBytes), "\n")
	for i, s := range raw {
		if i >= len(syms) {
			break
		}
		body := sliceBodyText(lines, s.StartLine, s.EndLine)
		syms[i].BodyShape = reactBodyShape(body)
	}
}

// sliceBodyText extracts the body region: from the first { after the start
// line to the matching close on the end line, or the text after => for
// expression arrows. Quote-aware, comment-stripped.
func sliceBodyText(lines []string, startLine, endLine int) string {
	if startLine < 1 || endLine < startLine || startLine > len(lines) {
		return ""
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	var sb strings.Builder
	for i := startLine - 1; i < endLine; i++ {
		line := stripLineComment(lines[i], "")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		sb.WriteString(trimmed)
		sb.WriteString(" ")
	}
	return strings.TrimSpace(sb.String())
}

// reactStubThrow matches a single not-implemented throw (function or arrow).
var reactStubThrow = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+\w+\s*\([^)]*\)\s*\{\s*throw\s+new\s+([A-Za-z_.]*Error)\(\s*['"` + "`" + `]not[\s_.-]?implemented['"` + "`" + `]\s*\)\s*;?\s*\}$` +
	`|throw\s+new\s+([A-Za-z_.]*Error)\(\s*['"` + "`" + `]not[\s_.-]?implemented['"` + "`" + `]\s*\)\s*;?$`)

// reactTodoArrow matches `=> TODO()` / `= TODO("...")` expression bodies.
var reactTodoArrow = regexp.MustCompile(`=>\s*(?:async\s+)?\{?\s*(?:throw\s+new\s+Error\(\s*['"` + "`" + `]not[\s_.-]?implemented['"` + "`" + `]\s*\)|TODO\s*\(\s*['"` + "`" + `]?[^'"` + "`" + `)]*['"` + "`" + `]?\s*\))\s*;?\s*\}?$`)

// reactBodyShape classifies a sliced body against the TS/React whitelist.
// Control flow, JSX returns, hooks, and multi-statement bodies are NonStub.
func reactBodyShape(body string) BodyShape {
	b := strings.TrimSpace(body)
	if b == "" {
		return BodyUnverified
	}
	if reactStubThrow.MatchString(b) || reactTodoArrow.MatchString(b) {
		return BodyStubThrow
	}
	if notImplementedRe.MatchString(b) && todoRe.MatchString(b) {
		return BodyStubTODO
	}
	// A body that mentions not-implemented but in a larger statement is still
	// non-stub; the whitelist is shape-based, not substring-based.
	return BodyNonStub
}

// reactRegexSymbols is the degraded signature pass: top-level function and
// arrow declarations only, enough to keep the SignatureHash usable when node
// is missing. Bodies come back Unverified.
func reactRegexSymbols(filename string, src []byte) []SymbolInfo {
	if len(src) == 0 {
		return nil
	}
	var out []SymbolInfo
	lines := strings.Split(string(src), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(stripLineComment(line, ""))
		matched := ""
		if m := reactFuncRe.FindStringSubmatch(trimmed); m != nil {
			matched = m[1]
		} else if m := reactConstArrowRe.FindStringSubmatch(trimmed); m != nil {
			matched = m[1]
		}
		if matched == "" {
			continue
		}
		bodyEnd := braceBodyEnd(lines, i)
		body := sliceBodyText(lines, i+1, bodyEnd)
		out = append(out, SymbolInfo{
			Name:      matched,
			Kind:      "function",
			Signature: sanitizeSigText(trimmed),
			BodyShape: reactBodyShape(body),
			Line:      i + 1,
		})
	}
	return out
}

// braceBodyEnd returns the 1-based line where the brace opened on startIdx
// (0-based) balances back to zero, or startIdx+1 when the declaration never
// opens a block (expression bodies).
func braceBodyEnd(lines []string, startIdx int) int {
	depth := 0
	opened := false
	for i := startIdx; i < len(lines); i++ {
		line := stripLineComment(lines[i], "")
		for j := 0; j < len(line); j++ {
			switch line[j] {
			case '{':
				depth++
				opened = true
			case '}':
				depth--
				if opened && depth <= 0 {
					return i + 1
				}
			}
		}
		if !opened && strings.Contains(line, "=>") {
			// Expression arrow body: consume to end of the logical line.
			if semicolon := strings.Index(line, ";"); semicolon >= 0 {
				return i + 1
			}
		}
	}
	return startIdx + 1
}

var reactFuncRe = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+(\w+)`)
var reactConstArrowRe = regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s*)?\(`)

// sanitizeSigText normalizes whitespace in a regex-extracted signature.
func sanitizeSigText(s string) string {
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(s, " "))
}

// ExtractReactFileSignatures is the convenience the gate hook calls.
func ExtractReactFileSignatures(filename string, src []byte) ([]string, error) {
	return ExtractCanonicalSignatures("react", filename, src, nil)
}

// ensure fmt stays used when the degraded path is the only compiled path.
var _ = fmt.Sprintf
