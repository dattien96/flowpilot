package runner

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/knowledge"
	"flowpilot-runner/internal/promptpacker"
)

// ContextSourceKnowledgeFlow — CP-66 P-2 (Task-374): the living-knowledge
// context source. Fetch resolves the turn's RetrievalLocus to distilled
// execution-flow sections (P-1 index.json + markdown) and packs ~500 tokens
// of macro-level flow context. Pure file reads at Fetch (no subprocess, no
// vector search): Deterministic() is true per the SD-22 D-2 invariant.
//
// Opt-in like conventions/lsp.diagnostics: registered so flows can resolve
// it, but NOT in defaultContextSourceIDs — flows that never opt in keep
// byte-identical output (CP-66 zero-conflict constraint).
const ContextSourceKnowledgeFlow ContextSourceID = "knowledge.flow"

// knowledgeFlowOmittedMissing is the Omitted reason when the workspace was
// never bootstrapped (a normal state, not an error — CP-66 §8 fallback).
const knowledgeFlowOmittedMissing = "knowledge_base_missing"

// knowledgeFlowSectionCap is the ~500-token Fetch budget; single sections
// are P-1-capped at knowledge.MaxSectionTokens (1.000).
const knowledgeFlowSectionCap = 500

type knowledgeFlowSource struct{ priority int }

func (s *knowledgeFlowSource) ID() string          { return string(ContextSourceKnowledgeFlow) }
func (s *knowledgeFlowSource) Priority() int       { return s.priority }
func (s *knowledgeFlowSource) Deterministic() bool { return true }

func (s *knowledgeFlowSource) Fetch(_ context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	workspace := strings.TrimSpace(hints.Workspace)
	if workspace == "" {
		return section, nil
	}
	idx, err := knowledge.LoadIndex(workspace)
	if err != nil {
		section.Omitted = []string{knowledgeFlowOmittedMissing}
		return section, nil
	}
	section.SourceRef = filepath.Join(knowledge.KnowledgeDir(workspace), "index.json")
	locus := buildRetrievalLocus(workspace, hints.WorkflowRunID, hints.UserPrompt,
		append(append([]string{}, hints.ChangedPaths...), hints.ExplicitSourcePaths...))
	matched := matchKnowledgeFlows(idx, locus)
	if len(matched) == 0 {
		return section, nil // quiet empty: locus simply names no distilled flow
	}
	bodies := readKnowledgeSections(workspace, idx, matched)
	picked, skipped := selectFlowSections(bodies, knowledgeFlowSectionCap)
	if len(picked) == 0 {
		return section, nil
	}
	for _, id := range skipped {
		section.Warnings = append(section.Warnings, "knowledge.flow section over budget, skipped: "+id)
	}
	section.Body = "## Context — Execution Flow Knowledge (distilled, ~500 tokens)\n\n" + strings.Join(picked, "\n")
	return section, nil
}

// matchedFlowSection is one resolved flow with its on-disk section text.
type matchedFlowSection struct {
	id     string
	tokens int
	body   string
}

// matchKnowledgeFlows resolves locus paths/symbols to indexed flow ids,
// sorted for determinism. Paths match directly (slash-normalized); symbols
// match exact keys or qualified↔simple suffix pairs in BOTH directions, so
// `Checkout` meets `shop/cart.Checkout` and vice versa (same suffix rule as
// the oracle baseline matcher).
func matchKnowledgeFlows(idx *knowledge.KnowledgeIndex, locus featurecatalog.RetrievalLocus) []string {
	matched := make(map[string]bool)
	for _, p := range locus.Paths {
		p = filepath.ToSlash(strings.TrimSpace(p))
		for _, id := range idx.Paths[p] {
			matched[id] = true
		}
	}
	for _, sym := range locus.Symbols {
		sym = strings.TrimSpace(sym)
		if sym == "" {
			continue
		}
		for key, ids := range idx.Symbols {
			if !knowledgeSymbolMatches(key, sym) {
				continue
			}
			for _, id := range ids {
				matched[id] = true
			}
		}
	}
	out := make([]string, 0, len(matched))
	for id := range matched {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// knowledgeSymbolMatches reports an exact or suffix-segment symbol match in
// either direction: index key `Checkout` meets locus `cart.Checkout`, and an
// index uid `Method:shop/cart/service.go:Checkout` meets locus `Checkout`.
func knowledgeSymbolMatches(indexKey, locusSym string) bool {
	if indexKey == locusSym {
		return true
	}
	for _, sep := range []string{".", "/", ":"} {
		if strings.HasSuffix(indexKey, sep+locusSym) || strings.HasSuffix(locusSym, sep+indexKey) {
			return true
		}
	}
	return false
}

// readKnowledgeSections loads the on-disk section bodies for matched flows
// (single-file and Q-1 sharded layouts alike, via the index file field).
// Flows whose section cannot be read are dropped quietly — a torn shard must
// degrade, never fail the Fetch.
func readKnowledgeSections(workspace string, idx *knowledge.KnowledgeIndex, matched []string) []matchedFlowSection {
	dir := knowledge.KnowledgeDir(workspace)
	cache := make(map[string]string)
	var out []matchedFlowSection
	for _, id := range matched {
		entry, ok := idx.Flows[id]
		if !ok || entry.Heading == "" {
			continue
		}
		doc, ok := cache[entry.File]
		if !ok {
			data, err := os.ReadFile(filepath.Join(dir, entry.File))
			if err != nil {
				continue
			}
			doc = string(data)
			cache[entry.File] = doc
		}
		body := knowledge.ExtractFlowSection(doc, entry.Heading)
		if strings.TrimSpace(body) == "" {
			continue
		}
		out = append(out, matchedFlowSection{id: id, tokens: promptpacker.EstimateTokens(body), body: body})
	}
	return out
}

// selectFlowSections greedily packs whole sections (sorted input order) until
// the cap; sections are never cut mid-way. A single section over the P-1
// 1.000-token contract is skipped with its id returned for the warning. When
// nothing fits, the smallest match is still returned — some grounded context
// beats a silent empty slot (worst case stays under the P-1 per-section cap).
func selectFlowSections(sections []matchedFlowSection, cap int) (picked []string, skipped []string) {
	running := 0
	for _, s := range sections {
		if s.tokens > knowledge.MaxSectionTokens {
			skipped = append(skipped, s.id)
			continue
		}
		if running+s.tokens <= cap {
			picked = append(picked, s.body)
			running += s.tokens
		}
	}
	if len(picked) == 0 && len(sections) > 0 {
		smallest := sections[0]
		for _, s := range sections[1:] {
			if s.tokens < smallest.tokens {
				smallest = s
			}
		}
		if smallest.tokens <= knowledge.MaxSectionTokens {
			picked = append(picked, smallest.body)
		}
	}
	return picked, skipped
}
