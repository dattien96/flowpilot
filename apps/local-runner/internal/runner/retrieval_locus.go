package runner

import (
	"path/filepath"
	"sort"
	"strings"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/featurecatalog"
)

// isConcreteCodeTarget reports whether p names a specific code file that a
// path-overlap test — or a code-graph query — can actually resolve.
//
// CP-55 P-2: this is now a thin wrapper around changecontract.IsConcreteCodeTarget,
// the shared predicate CP-55's frozen-contract path normalization also uses,
// so "what counts as a concrete code target" cannot drift between the two
// call sites. Behavior (and every existing test above/below) is unchanged —
// only the implementation moved.
func isConcreteCodeTarget(p string) bool {
	return changecontract.IsConcreteCodeTarget(p)
}

// buildRetrievalLocus assembles the code region this turn is touching, merging
// four sources in descending order of confidence:
//
//  1. explicitPaths — paths a caller already resolved with higher confidence
//     than this function could derive on its own (CP-55 P-8: a Flow's own
//     FrozenContractRecord.DeclaredPaths, passed by featureHistorySource.Fetch
//     as hints.ExplicitSourcePaths — see below);
//  2. the run's declared change.contract — the AI stating what it intends to
//     touch, which is the only source available *before* any edit exists;
//  3. the uncommitted diff — what has actually been touched so far;
//  4. paths named in the prompt — what the operator pointed at.
//
// Sources 3 and 4 reuse the Task-246 producers that source.excerpt already
// relies on, so path parsing, separator normalization and doc filtering stay in
// one place rather than being reimplemented here.
//
// Everything is non-fatal (CP-54 QĐ-7): a missing contract, a workspace that is
// not a git repo, or an empty prompt yields an empty locus, which is the signal
// for callers to keep their current recency ordering.
//
// Consumed by featureHistorySource.Fetch (CP-55 P-7) to rank feature.history
// by code-locus overlap.
//
// CP-55 P-8: explicitPaths exists because source 2 above (the legacy
// changecontract.Store) is never populated for a Flow coder governed by a
// FROZEN contract — CP-55 P-4's own design keeps FrozenStore/FrozenContractRecord
// completely separate from the legacy Store/Contract path (an agent.code
// writer never calls prepareChangeContract at all). Without an explicit
// input, the very first coder context build for a freshly frozen contract
// (advanceFlowThroughFreezeChain's context.produce hop, before the coder has
// written anything or said anything path-bearing) would find an empty locus
// on all three of the other sources and silently rank by recency alone —
// exactly the case CP-55 P-8's "first coder context ranks feature history by
// current locus" guarantee needs to NOT degrade to. The caller already
// resolves this value for a different hint field (ExplicitSourcePaths, which
// feeds source.excerpt) — reusing it here is cheaper and more consistent
// than adding a second FrozenStore lookup keyed by a coder step id this
// function does not otherwise need.
func buildRetrievalLocus(workspace, runID, prompt string, explicitPaths []string) featurecatalog.RetrievalLocus {
	locus := featurecatalog.RetrievalLocus{RunID: strings.TrimSpace(runID)}

	seen := make(map[string]bool)
	add := func(paths []string) {
		for _, p := range paths {
			p = filepath.ToSlash(strings.TrimSpace(p))
			if !isConcreteCodeTarget(p) || seen[p] {
				continue
			}
			seen[p] = true
			locus.Paths = append(locus.Paths, p)
		}
	}

	add(explicitPaths)

	if locus.RunID != "" && strings.TrimSpace(workspace) != "" {
		if store, err := changecontract.OpenStoreReadOnly(workspace); err == nil && store != nil {
			if c, ok := store.GetLatestForRun(locus.RunID); ok {
				add(c.DeclaredPaths)
				locus.Symbols = dedupeSortedSymbols(c.DeclaredSymbols)
			}
		}
	}

	add(uncommittedChangedPaths(workspace))
	add(extractPromptSourcePaths(prompt))

	c := changecontract.Contract{
		DeclaredPaths:   locus.Paths,
		DeclaredSymbols: locus.Symbols,
	}
	locus.Symbols = changecontract.GitNexusImpactTargets(c)

	// Sort last: the source order above only decides which duplicate wins, while
	// consumers treat the locus as a set. Sorting makes the result reproducible
	// run to run, the same determinism rule BUG-266 established for history.
	sort.Strings(locus.Paths)
	return locus
}

// dedupeSortedSymbols normalizes declared symbols from the contract store.
func dedupeSortedSymbols(symbols []string) []string {
	if len(symbols) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(symbols))
	out := make([]string, 0, len(symbols))
	for _, s := range symbols {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}
