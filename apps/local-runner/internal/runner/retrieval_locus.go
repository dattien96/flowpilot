package runner

import (
	"path/filepath"
	"sort"
	"strings"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/flowgate"
)

// isConcreteCodeTarget reports whether p names a specific code file that a
// path-overlap test — or a code-graph query — can actually resolve.
//
// This is the shared normalization Task-259 T-3 also needs. Contracts in the
// wild are mostly *inferred*, and inference records top-level directory buckets
// ("apps", "internal") rather than files. Handing those to an overlap test
// matches nearly every commit in the repo, which would recreate exactly the
// dilution CP-54 exists to remove; handing them to a symbol-oriented tool
// resolves nothing at all. Globs and doc files fail the same way.
func isConcreteCodeTarget(p string) bool {
	p = strings.TrimSpace(p)
	switch {
	case p == "":
		return false
	case strings.HasPrefix(p, "-"):
		// Never let a target be read as a CLI flag by a downstream tool runner.
		return false
	case strings.ContainsAny(p, "*?["):
		return false
	case filepath.Ext(p) == "":
		// A bare directory bucket, not a file.
		return false
	case flowgate.IsDocOrAuditFile(p):
		return false
	}
	return true
}

// buildRetrievalLocus assembles the code region this turn is touching, merging
// three sources in descending order of confidence:
//
//  1. the run's declared change.contract — the AI stating what it intends to
//     touch, which is the only source available *before* any edit exists;
//  2. the uncommitted diff — what has actually been touched so far;
//  3. paths named in the prompt — what the operator pointed at.
//
// Sources 2 and 3 reuse the Task-246 producers that source.excerpt already
// relies on, so path parsing, separator normalization and doc filtering stay in
// one place rather than being reimplemented here.
//
// Everything is non-fatal (CP-54 QĐ-7): a missing contract, a workspace that is
// not a git repo, or an empty prompt yields an empty locus, which is the signal
// for callers to keep their current recency ordering.
//
// Nothing consumes this yet — wiring happens in CP-54 P-4.
func buildRetrievalLocus(workspace, runID, prompt string) featurecatalog.RetrievalLocus {
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

	// Sort last: the source order above only decides which duplicate wins, while
	// consumers treat the locus as a set. Sorting makes the result reproducible
	// run to run, the same determinism rule BUG-266 established for history.
	sort.Strings(locus.Paths)
	return locus
}

// dedupeSortedSymbols normalizes declared symbols. In practice this returns nil
// today because nothing populates Contract.DeclaredSymbols (BUG-323 Q-2); it is
// here so the path exists once a symbol source does.
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
