package changecontract

import (
	"path/filepath"
	"sort"
	"strings"
)

// GitNexusImpactTargets returns deduplicated, sorted symbol names suitable for
// `gitnexus impact` queries from a Change Contract. DeclaredSymbols pass
// through unchanged; each concrete declared code file is mapped to an exported
// identifier derived from its basename (BUG-323 Q-2 / Task-259 T-3). Directory
// buckets, globs, doc/audit paths, and flag-like strings are skipped.
func GitNexusImpactTargets(c Contract) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" || strings.HasPrefix(t, "-") {
			return
		}
		if _, dup := seen[t]; dup {
			return
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	for _, sym := range c.DeclaredSymbols {
		add(sym)
	}
	for _, p := range c.DeclaredPaths {
		p = filepath.ToSlash(strings.TrimSpace(p))
		if !IsConcreteCodeTarget(p) {
			continue
		}
		if sym := pathBasenameToGitNexusSymbol(p); sym != "" {
			add(sym)
		}
	}
	sort.Strings(out)
	return out
}

// GitNexusQueryTargetsForPath returns impact query targets for one out-of-scope
// diff path. GitNexus resolves symbols, not repo paths, so the derived symbol
// is tried first; the raw path is retained as a second fallback so legacy test
// doubles keyed by file path keep working (additive-tests-only).
func GitNexusQueryTargetsForPath(path string) []string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	if path == "" {
		return nil
	}
	var out []string
	if sym := pathBasenameToGitNexusSymbol(path); sym != "" {
		out = append(out, sym)
	}
	if len(out) == 0 || out[0] != path {
		out = append(out, path)
	}
	return out
}

// DerivedSymbolFromPath maps a concrete code file path to the same best-effort
// exported symbol name GitNexusImpactTargets uses (CP-54 P-6 symbol-overlap).
func DerivedSymbolFromPath(path string) string {
	return pathBasenameToGitNexusSymbol(path)
}

// pathBasenameToGitNexusSymbol maps a concrete code file path to a best-effort
// exported symbol name (e.g. gate_hook.go → GateHook). This is a deterministic
// heuristic — not AST-aware — but matches how operators name primary types.
func pathBasenameToGitNexusSymbol(path string) string {
	base := filepath.Base(filepath.ToSlash(path))
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		return ""
	}
	return snakeOrKebabToExportedIdent(stem)
}

func snakeOrKebabToExportedIdent(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' })
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		runes := []rune(p)
		b.WriteString(strings.ToUpper(string(runes[0])))
		if len(runes) > 1 {
			b.WriteString(strings.ToLower(string(runes[1:])))
		}
	}
	return b.String()
}
