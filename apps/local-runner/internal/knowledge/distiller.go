// Package knowledge implements CP-66 P-1 (Task-373): the Knowledge Distiller
// Engine. It converts raw GitNexus execution-flow data (macro: processes +
// member symbols) and optional LSP workspace symbols (micro) into a Living
// Knowledge Base: three fixed-layout Markdown docs plus an index.json lookup
// plane under .flowpilot/knowledge/.
//
// The package never imports runner (runner imports knowledge — the only
// allowed direction); token estimates reuse promptpacker.EstimateTokens so
// P-1 section sizes and P-2 Fetch caps speak the same unit. Distill is fully
// deterministic for identical inputs (all maps sorted) so incremental updates
// can byte-compare untouched sections.
package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"flowpilot-runner/internal/promptpacker"
	"flowpilot-runner/internal/structure"
)

// SchemaVersion bumps whenever the index.json contract changes; a mismatch
// triggers a full rebuild instead of an incremental merge (writer.go).
const SchemaVersion = 1

// MaxSectionTokens is the hard per-flow-section budget (CP-66 P-2 key
// decision: every block < 1.000 tokens). Sections render shrink-to-fit:
// symbol/file lists halve until the estimate fits — re-distilled shorter,
// never cut mid-section.
const MaxSectionTokens = 1000

// MaxFlowsSingleFile implements Q-1: past 500 flows execution-flows.md shards
// into flows/<domain>.md + a manifest; index.json stays the only lookup plane.
const MaxFlowsSingleFile = 500

// DefaultProcessLimit bounds one background distill scan.
const DefaultProcessLimit = 60

// DefaultModelLimit bounds the data-model candidate scan.
const DefaultModelLimit = 80

// ProcessLister supplies GitNexus execution flows + model candidates. The
// production adapter lives in runner (gitnexusProcessLister over
// structure.Processes/ModelCandidates); tests use scripted fakes. A nil error
// with empty slices is a valid "index has no processes" answer, not a failure.
type ProcessLister interface {
	ListProcesses(ctx context.Context) ([]structure.FlowSummary, error)
	ListModelCandidates(ctx context.Context) ([]structure.ModelInfo, error)
}

// SymbolLister supplies micro-level workspace symbols (LSP-backed in
// production). Optional: Distill accepts nil and degrades to GitNexus-only
// output (Task-373 AC-4) — never an error.
type SymbolLister interface {
	ListWorkspaceSymbols(ctx context.Context, workspace string) ([]SymbolInfo, error)
}

// SymbolInfo is one workspace symbol for the data-models cross-reference.
type SymbolInfo struct {
	Kind string
	Path string
	Name string
}

// FlowIndexEntry points at one rendered flow section.
type FlowIndexEntry struct {
	File    string `json:"file"`
	Heading string `json:"heading"`
	Tokens  int    `json:"tokens"`
}

// KnowledgeIndex is the P-1↔P-2 contract: path/symbol → flow ids → section
// locations. All lists sorted + deduped for deterministic re-renders.
type KnowledgeIndex struct {
	SchemaVersion int                       `json:"schemaVersion"`
	Flows         map[string]FlowIndexEntry `json:"flows"`
	Paths         map[string][]string       `json:"paths"`
	Symbols       map[string][]string       `json:"symbols"`
}

// RenderedFlow is one distilled flow section with its placement.
type RenderedFlow struct {
	ID            string
	Domain        string
	File          string
	Heading       string
	Body          string
	TokenEstimate int
}

// KnowledgeBase is the in-memory distill result writer.go persists.
type KnowledgeBase struct {
	Overview string
	Flows    []RenderedFlow
	Models   string
	Index    KnowledgeIndex
}

// RepoFacts grounds system-overview.md in the actual repo.
type RepoFacts struct {
	Module      string
	TopDirs     []string
	CoreLibs    []string
	FlowCount   int
	ByType      map[string]int
	SymbolTotal int
	ModelCount  int
}

// Distill builds the knowledge base for repoDir. lister must be non-nil;
// syms may be nil (GitNexus-only degrade). Deterministic for identical
// inputs.
func Distill(ctx context.Context, repoDir string, lister ProcessLister, syms SymbolLister) (*KnowledgeBase, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	flows, err := lister.ListProcesses(ctx)
	if err != nil {
		return nil, err
	}
	models, err := lister.ListModelCandidates(ctx)
	if err != nil {
		return nil, err
	}
	var extraSyms []SymbolInfo
	if syms != nil {
		// LSP absence/error degrades silently — GitNexus data stands alone.
		if got, serr := syms.ListWorkspaceSymbols(ctx, repoDir); serr == nil {
			extraSyms = got
		}
	}
	facts := collectRepoFacts(repoDir, flows, models)
	rendered := renderFlows(flows)
	kb := &KnowledgeBase{
		Overview: renderOverview(facts),
		Flows:    rendered,
		Models:   renderModels(models, extraSyms),
	}
	kb.Index = buildIndex(flows, rendered)
	return kb, nil
}

// collectRepoFacts reads go.mod (module + top requires) and top-level dirs.
// Best-effort: any unreadable input yields empty fields, never an error.
func collectRepoFacts(repoDir string, flows []structure.FlowSummary, models []structure.ModelInfo) RepoFacts {
	facts := RepoFacts{ByType: make(map[string]int)}
	if data, err := os.ReadFile(filepath.Join(repoDir, "go.mod")); err == nil {
		facts.Module, facts.CoreLibs = parseGoMod(data)
	}
	if entries, err := os.ReadDir(repoDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
				continue
			}
			facts.TopDirs = append(facts.TopDirs, name)
		}
		sort.Strings(facts.TopDirs)
		if len(facts.TopDirs) > 12 {
			facts.TopDirs = facts.TopDirs[:12]
		}
	}
	// Fallback module name: repo dir base (non-Go repos still get a title).
	if facts.Module == "" {
		facts.Module = filepath.Base(filepath.Clean(repoDir))
	}
	facts.FlowCount = len(flows)
	for _, f := range flows {
		t := strings.TrimSpace(f.ProcessType)
		if t == "" {
			t = "general"
		}
		facts.ByType[t]++
		facts.SymbolTotal += len(f.Symbols)
	}
	facts.ModelCount = len(models)
	return facts
}

// parseGoMod extracts the module path and up to 8 direct require paths.
func parseGoMod(data []byte) (module string, libs []string) {
	inRequire := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			module = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			continue
		}
		if strings.HasPrefix(line, "require (") {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}
		if strings.HasPrefix(line, "require ") && !strings.HasSuffix(line, "(") {
			if f := strings.Fields(strings.TrimPrefix(line, "require ")); len(f) > 0 {
				libs = append(libs, f[0])
			}
			continue
		}
		if inRequire {
			if f := strings.Fields(line); len(f) > 0 && !strings.HasPrefix(f[0], "//") {
				libs = append(libs, f[0])
			}
		}
	}
	sort.Strings(libs)
	if len(libs) > 8 {
		libs = libs[:8]
	}
	return module, libs
}

// renderOverview writes system-overview.md with the fixed TOC (tech stack,
// layer boundaries, core libraries, flow census).
func renderOverview(f RepoFacts) string {
	var b strings.Builder
	b.WriteString("# System Overview\n\n")
	b.WriteString("_Auto-distilled living knowledge (CP-66). Regenerated by the runner; do not hand-edit — the audit node rewrites stale sections._\n\n")
	b.WriteString("## Tech Stack\n\n")
	b.WriteString("- Module: `" + f.Module + "`\n")
	if len(f.CoreLibs) == 0 {
		b.WriteString("- Core libraries: (none detected)\n")
	} else {
		b.WriteString("- Core libraries:\n")
		for _, l := range f.CoreLibs {
			b.WriteString("  - `" + l + "`\n")
		}
	}
	b.WriteString("\n## Layer Boundaries\n\n")
	if len(f.TopDirs) == 0 {
		b.WriteString("(no top-level directories detected)\n")
	} else {
		for _, d := range f.TopDirs {
			b.WriteString("- `" + d + "`\n")
		}
	}
	b.WriteString("\n## Core Libraries\n\n")
	if len(f.CoreLibs) == 0 {
		b.WriteString("(none detected — see Tech Stack)\n")
	} else {
		for _, l := range f.CoreLibs {
			b.WriteString("- `" + l + "`\n")
		}
	}
	b.WriteString("\n## Flow Census\n\n")
	b.WriteString("- Execution flows indexed: " + itoa(f.FlowCount) + "\n")
	b.WriteString("- Flow symbols total: " + itoa(f.SymbolTotal) + "\n")
	b.WriteString("- Data models tracked: " + itoa(f.ModelCount) + "\n")
	if len(f.ByType) > 0 {
		types := make([]string, 0, len(f.ByType))
		for t := range f.ByType {
			types = append(types, t)
		}
		sort.Strings(types)
		b.WriteString("- By process type:\n")
		for _, t := range types {
			b.WriteString("  - " + t + ": " + itoa(f.ByType[t]) + "\n")
		}
	}
	return b.String()
}

// renderModels writes data-models.md: relevance-ranked entities (most-shared
// across flows first) plus any LSP workspace symbols as cross-reference.
func renderModels(models []structure.ModelInfo, extra []SymbolInfo) string {
	var b strings.Builder
	b.WriteString("# Data Models\n\n")
	b.WriteString("_Entities ranked by execution-flow participation (most-shared first)._ \n\n")
	if len(models) == 0 && len(extra) == 0 {
		b.WriteString("(no data models detected — the index exposes no struct/interface/class symbols yet)\n")
		return b.String()
	}
	for _, m := range models {
		b.WriteString("## Model: " + m.Kind + " " + m.Name + "\n\n")
		if m.Path != "" {
			b.WriteString("- Defined in: `" + m.Path + "`\n")
		}
		b.WriteString("- Participates in " + itoa(m.FlowCount) + " execution flow(s)\n\n")
	}
	if len(extra) > 0 {
		names := make([]SymbolInfo, 0, len(extra))
		names = append(names, extra...)
		sort.SliceStable(names, func(i, j int) bool {
			if names[i].Path != names[j].Path {
				return names[i].Path < names[j].Path
			}
			return names[i].Name < names[j].Name
		})
		if len(names) > 40 {
			names = names[:40]
		}
		b.WriteString("## Workspace Symbols (LSP cross-reference)\n\n")
		for _, s := range names {
			b.WriteString("- `" + s.Name + "` (" + s.Kind + ", " + s.Path + ")\n")
		}
	}
	return b.String()
}

// renderFlows renders one section per flow, shrink-to-fit under
// MaxSectionTokens, and assigns the output file (single vs Q-1 sharded).
func renderFlows(flows []structure.FlowSummary) []RenderedFlow {
	sharded := len(flows) > MaxFlowsSingleFile
	out := make([]RenderedFlow, 0, len(flows))
	for _, f := range flows {
		id := sanitizeHeadingID(f.ID)
		domain := strings.TrimSpace(f.ProcessType)
		if domain == "" {
			domain = "general"
		}
		heading := "## Flow: " + id
		file := "execution-flows.md"
		if sharded {
			file = "flows/" + sanitizePathSegment(domain) + ".md"
		}
		body := renderFlowSection(heading, f)
		out = append(out, RenderedFlow{
			ID: id, Domain: domain, File: file, Heading: heading,
			Body: body, TokenEstimate: promptpacker.EstimateTokens(body),
		})
	}
	return out
}

// renderFlowSection builds one flow section, halving symbol/file lists until
// the estimate fits MaxSectionTokens (re-distilled shorter, never mid-cut).
func renderFlowSection(heading string, f structure.FlowSummary) string {
	syms := f.Symbols
	files := flowFiles(f)
	for {
		body := buildFlowBody(heading, f, syms, files)
		if promptpacker.EstimateTokens(body) <= MaxSectionTokens {
			return body
		}
		if len(syms) > 4 {
			syms = syms[:len(syms)/2]
			continue
		}
		if len(files) > 2 {
			files = files[:len(files)/2]
			continue
		}
		// Floor: heading + purpose + meta only (always fits sane labels).
		return buildFlowBody(heading, f, nil, nil)
	}
}

func buildFlowBody(heading string, f structure.FlowSummary, syms []structure.FlowSymbol, files []string) string {
	var b strings.Builder
	b.WriteString(heading + "\n\n")
	purpose := strings.TrimSpace(f.Label)
	if len(purpose) > 400 {
		purpose = purpose[:400] + "…"
	}
	if purpose == "" {
		purpose = f.ID
	}
	b.WriteString("Purpose: " + purpose + "\n\n")
	meta := "Type: " + orDefault(f.ProcessType, "general")
	meta += " · Steps: " + itoa(f.StepCount)
	meta += " · Symbols: " + itoa(len(f.Symbols))
	b.WriteString(meta + "\n")
	if len(syms) == 0 && len(files) == 0 {
		return b.String() + "\n"
	}
	if len(syms) > 0 {
		b.WriteString("\n### Key symbols\n\n")
		for _, s := range syms {
			b.WriteString("- `" + s.Name + "`")
			detail := s.Kind
			if s.Path != "" {
				if detail != "" {
					detail += ", "
				}
				detail += s.Path
			}
			if detail != "" {
				b.WriteString(" (" + detail + ")")
			}
			b.WriteString("\n")
		}
	}
	if len(files) > 0 {
		b.WriteString("\n### Files\n\n")
		for _, p := range files {
			b.WriteString("- `" + p + "`\n")
		}
	}
	return b.String() + "\n"
}

// flowFiles returns the deduped, sorted repo-relative paths of a flow.
func flowFiles(f structure.FlowSummary) []string {
	seen := make(map[string]bool, len(f.Symbols))
	var out []string
	for _, s := range f.Symbols {
		p := filepath.ToSlash(strings.TrimSpace(s.Path))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// buildIndex assembles the P-1↔P-2 lookup plane: every flow entry plus
// reverse maps path → flows and symbol → flows (simple name AND full uid, so
// Fetch matches both `CheckoutService` and qualified mentions).
func buildIndex(flows []structure.FlowSummary, rendered []RenderedFlow) KnowledgeIndex {
	idx := KnowledgeIndex{
		SchemaVersion: SchemaVersion,
		Flows:         make(map[string]FlowIndexEntry, len(rendered)),
		Paths:         make(map[string][]string),
		Symbols:       make(map[string][]string),
	}
	addRef := func(m map[string][]string, key, flowID string) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		for _, have := range m[key] {
			if have == flowID {
				return
			}
		}
		m[key] = append(m[key], flowID)
	}
	byID := make(map[string][]structure.FlowSymbol, len(flows))
	for _, f := range flows {
		byID[sanitizeHeadingID(f.ID)] = f.Symbols
	}
	for _, r := range rendered {
		idx.Flows[r.ID] = FlowIndexEntry{File: r.File, Heading: r.Heading, Tokens: r.TokenEstimate}
		for _, s := range byID[r.ID] {
			if p := filepath.ToSlash(strings.TrimSpace(s.Path)); p != "" {
				addRef(idx.Paths, p, r.ID)
			}
			addRef(idx.Symbols, s.Name, r.ID)
			addRef(idx.Symbols, strings.TrimSpace(s.ID), r.ID)
		}
	}
	for _, m := range []map[string][]string{idx.Paths, idx.Symbols} {
		for k := range m {
			sort.Strings(m[k])
		}
	}
	return idx
}

func sanitizeHeadingID(id string) string {
	r := strings.ReplaceAll(strings.TrimSpace(id), "\n", " ")
	r = strings.ReplaceAll(r, "\r", " ")
	r = strings.ReplaceAll(r, "#", "-")
	if r == "" {
		r = "unnamed-flow"
	}
	return r
}

func sanitizePathSegment(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "general"
	}
	return out
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
