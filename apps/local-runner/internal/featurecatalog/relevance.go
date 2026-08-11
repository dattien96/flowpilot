package featurecatalog

import (
	"log"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"flowpilot-runner/internal/changeledger"
)

// HistoryRelevance is one changeledger.Entry's computed rank signal against a
// RetrievalLocus (CP-55 P-6). PathOverlap is the exact count of distinct
// entry.ChangedPaths that also appear in locus.Paths. SymbolOverlap counts
// distinct changed paths whose repo-relative path or basename-derived exported
// symbol matches locus.Symbols (CP-54 P-6 — shares GitNexusImpactTargets
// heuristic with Task-259). CommitUnix is
// entry.CommittedAt parsed to a Unix timestamp — 0 (the Unix epoch) if
// CommittedAt does not parse, so an entry with an unparseable timestamp still
// sorts deterministically rather than erroring. Note 0 is not a true sentinel
// minimum: a genuine pre-1970 timestamp parses to a negative value and would
// sort older still. Commit timestamps are never pre-1970 in real use, so 0
// functions as a floor by convention, not by construction.
//
// CommitUnix also deliberately differs from changeledger.GetFeatureHistory's
// own ordering, which compares CommittedAt as a raw RFC3339 *string* — that
// comparison agrees with Unix-time comparison for same-timezone timestamps,
// but a timestamp carrying a non-UTC offset (e.g. "...+09:00") can rank
// differently under string comparison than under the instant it represents.
// P-7 (feature.history wiring) must not assume the two orderings coincide.
type HistoryRelevance struct {
	PathOverlap   int
	SymbolOverlap int
	CommitUnix    int64
	CommitHash    string
}

// RankedHistoryEntry pairs an Entry with its computed HistoryRelevance and
// final 0-based Rank after RankHistoryEntries orders it. Entry is a shallow
// copy of the input: scalar fields are independent, but slice fields (e.g.
// ChangedPaths) still alias the caller's backing array — callers must not
// mutate them in place.
type RankedHistoryEntry struct {
	Entry     changeledger.Entry
	Relevance HistoryRelevance
	Rank      int
}

// canonicalPathForOverlap normalizes p to the form path-overlap comparison
// treats as canonical: forward slashes, no leading "./", no trailing
// separator, whitespace trimmed. Matching is otherwise exact-string and
// case-sensitive (matching git's own path semantics — case-folding would
// introduce false matches on a case-sensitive checkout). Producers of
// changeledger.Entry.ChangedPaths and RetrievalLocus.Paths are each expected
// to already emit paths in this form, but their normalization has drifted in
// practice (a locus path built from a user prompt can retain a leading
// "./", and not every ChangedPaths producer applies the same normalization
// pass) — canonicalizing again here, on both sides, at comparison time is
// cheap insurance against a silent zero-overlap score from a shape mismatch
// neither side's own tests would catch.
func canonicalPathForOverlap(p string) string {
	p = filepath.ToSlash(strings.TrimSpace(p))
	if p == "" {
		return ""
	}
	return path.Clean(p)
}

// buildLocusPathSet returns the canonicalized set of locus.Paths, deduped.
func buildLocusPathSet(locus RetrievalLocus) map[string]struct{} {
	set := make(map[string]struct{}, len(locus.Paths))
	for _, p := range locus.Paths {
		if cp := canonicalPathForOverlap(p); cp != "" {
			set[cp] = struct{}{}
		}
	}
	return set
}

// buildLocusSymbolSet returns the trimmed set of locus.Symbols, deduped.
func buildLocusSymbolSet(locus RetrievalLocus) map[string]struct{} {
	set := make(map[string]struct{}, len(locus.Symbols))
	for _, s := range locus.Symbols {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		set[s] = struct{}{}
	}
	return set
}

// derivedSymbolFromPath mirrors changecontract.DerivedSymbolFromPath without
// importing changecontract (featurecatalog ← changecontract import cycle).
func derivedSymbolFromPath(path string) string {
	base := filepath.Base(filepath.ToSlash(path))
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		return ""
	}
	parts := strings.FieldsFunc(stem, func(r rune) bool { return r == '_' || r == '-' })
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

// scoreAgainstLocusSet is ScoreHistoryEntry's implementation, taking an
// already-built locus path set so RankHistoryEntries can build it once and
// reuse it across every entry instead of once per entry.
func scoreAgainstLocusSet(entry changeledger.Entry, locusSet map[string]struct{}, locusSymbols map[string]struct{}) HistoryRelevance {
	rel := HistoryRelevance{CommitHash: entry.CommitHash}
	if t, err := time.Parse(time.RFC3339, entry.CommittedAt); err == nil {
		rel.CommitUnix = t.Unix()
	}
	if len(entry.ChangedPaths) == 0 || (len(locusSet) == 0 && len(locusSymbols) == 0) {
		return rel
	}

	seenPaths := make(map[string]struct{}, len(entry.ChangedPaths))
	pathOverlap := 0
	symbolOverlap := 0
	for _, p := range entry.ChangedPaths {
		cp := canonicalPathForOverlap(p)
		if cp == "" {
			continue
		}
		if _, dup := seenPaths[cp]; !dup {
			seenPaths[cp] = struct{}{}
			if _, ok := locusSet[cp]; ok {
				pathOverlap++
			}
		}
		if len(locusSymbols) == 0 {
			continue
		}
		if _, ok := locusSymbols[cp]; ok {
			symbolOverlap++
			continue
		}
		if sym := derivedSymbolFromPath(cp); sym != "" {
			if _, ok := locusSymbols[sym]; ok {
				symbolOverlap++
			}
		}
	}
	rel.PathOverlap = pathOverlap
	rel.SymbolOverlap = symbolOverlap
	return rel
}

// ScoreHistoryEntry computes entry's HistoryRelevance against locus.
//
// PathOverlap counts the overlap between entry.ChangedPaths and locus.Paths
// after both sides are independently canonicalized (see
// canonicalPathForOverlap) — matching is exact-string and case-sensitive, not
// fuzzy, prefix, or substring matching. A path repeated within
// entry.ChangedPaths (after canonicalization) is only ever counted once, so a
// duplicate cannot inflate the score; the same holds for a duplicated
// locus.Paths entry, since it collapses into the same set. A nil/empty
// entry.ChangedPaths or an empty locus.Paths both yield PathOverlap 0. When
// locus.Symbols is populated, SymbolOverlap counts distinct changed paths that
// match a locus symbol exactly or via derivedSymbolFromPath.
func ScoreHistoryEntry(entry changeledger.Entry, locus RetrievalLocus) HistoryRelevance {
	return scoreAgainstLocusSet(entry, buildLocusPathSet(locus), buildLocusSymbolSet(locus))
}

// RankHistoryEntries scores every entry against locus and returns them in
// deterministic rank order:
//
//  1. higher PathOverlap first;
//  2. higher SymbolOverlap first;
//  3. newer CommitUnix first;
//  4. lexicographically smaller CommitHash first, as a final, always-decisive
//     tie-break.
//
// This mirrors changeledger.GetFeatureHistory's own CommitHash tie-break
// discipline (BUG-266): the result never depends on Go's randomized map
// iteration order, and never on the input slice's own order provided
// CommitHash is unique across entries — which changeledger.Ledger guarantees
// by construction (it is keyed by commit hash), but which this function does
// not itself enforce for an arbitrary caller-supplied slice. entries is only
// ever read, never mutated in place; nil is returned for a nil/empty input,
// matching changeledger.GetFeatureHistory's own empty-result convention.
func RankHistoryEntries(entries []changeledger.Entry, locus RetrievalLocus) []RankedHistoryEntry {
	if len(entries) == 0 {
		return nil
	}
	locusSet := buildLocusPathSet(locus)
	locusSymbols := buildLocusSymbolSet(locus)
	ranked := make([]RankedHistoryEntry, len(entries))
	for i, e := range entries {
		ranked[i] = RankedHistoryEntry{Entry: e, Relevance: scoreAgainstLocusSet(e, locusSet, locusSymbols)}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Relevance.PathOverlap != ranked[j].Relevance.PathOverlap {
			return ranked[i].Relevance.PathOverlap > ranked[j].Relevance.PathOverlap
		}
		if ranked[i].Relevance.SymbolOverlap != ranked[j].Relevance.SymbolOverlap {
			return ranked[i].Relevance.SymbolOverlap > ranked[j].Relevance.SymbolOverlap
		}
		if ranked[i].Relevance.CommitUnix != ranked[j].Relevance.CommitUnix {
			return ranked[i].Relevance.CommitUnix > ranked[j].Relevance.CommitUnix
		}
		return ranked[i].Relevance.CommitHash < ranked[j].Relevance.CommitHash
	})
	for i := range ranked {
		ranked[i].Rank = i
	}
	return ranked
}

// HistoryRankingConfig tunes when SelectHistoryEntries activates ranking
// (CP-55 P-7) instead of falling back to plain recency.
type HistoryRankingConfig struct {
	// ActivationThreshold is the candidate count above which ranking
	// activates — at or below it, recency alone is assumed to already be a
	// good enough ordering (there is no dilution problem yet), so
	// SelectHistoryEntries falls back rather than pay the ranking cost.
	ActivationThreshold int
	// Limit caps how many entries a ranked selection returns.
	Limit int
}

// defaultHistoryRankingConfig is the CP-55 P-7 recommended configuration.
// Unexported and returned by value through DefaultHistoryRankingConfig so no
// caller can accidentally mutate a shared global ranking policy (CP-55 P-7
// review finding I-5 — an exported mutable var read at production call time
// is a global-state hazard: any package in the binary, including a test in
// an unrelated file, could reassign it and silently change ranking behavior
// process-wide).
var defaultHistoryRankingConfig = HistoryRankingConfig{
	ActivationThreshold: 30,
	Limit:               15,
}

// DefaultHistoryRankingConfig returns the CP-55 P-7 recommended
// configuration, by value — callers get an independent copy, never a
// reference to shared mutable state.
func DefaultHistoryRankingConfig() HistoryRankingConfig {
	return defaultHistoryRankingConfig
}

// HistorySelection is the result of SelectHistoryEntries: either the
// unranked fallback set (Ranked=false, Entries unchanged from the input,
// still in ascending-by-time order) or a ranked, capped subset (Ranked=true).
type HistorySelection struct {
	Entries        []changeledger.Entry
	Ranked         bool
	CandidateCount int
}

// SelectHistoryEntries chooses which of entries to surface for locus, per
// CP-55 P-7's selection invariants:
//
//   - Fallback (locus empty, or len(entries) <= config.ActivationThreshold):
//     entries is returned unchanged (Ranked=false) — the caller's existing
//     recency-based rendering keeps working byte-for-byte, since nothing
//     about the set or its order changed.
//   - Ranked (locus non-empty AND len(entries) > config.ActivationThreshold):
//     entries are ordered via RankHistoryEntries and capped at config.Limit
//     (or fewer, if there are fewer than Limit entries; a non-positive Limit
//     falls back to DefaultHistoryRankingConfig's own Limit rather than
//     returning every candidate uncapped — CP-55 P-7 review finding I-4).
//     The absolute newest entry (entries[len(entries)-1] —
//     GetFeatureHistory's own "newest = current truth" convention) is always
//     present in the result even if its locus score is low. If it would not
//     otherwise make the cut, it is inserted at the FRONT of the result
//     (displacing the lowest-ranked selected entry) rather than left at the
//     back — a "current truth" entry buried at the bottom of a
//     relevance-ordered list is exactly the wrong place for it (CP-55 P-7
//     review finding I-3's positioning note).
//
// entries is assumed sorted ascending by commit time (changeledger.Ledger's
// own GetFeatureHistory convention) — SelectHistoryEntries does not itself
// re-sort it for the fallback path, and identifies "the absolute newest
// entry" by trusting the last element, not by re-deriving it.
func SelectHistoryEntries(entries []changeledger.Entry, locus RetrievalLocus, config HistoryRankingConfig) HistorySelection {
	candidateCount := len(entries)
	if candidateCount == 0 || locus.IsEmpty() || candidateCount <= config.ActivationThreshold {
		return HistorySelection{Entries: entries, Ranked: false, CandidateCount: candidateCount}
	}

	ranked := RankHistoryEntries(entries, locus)
	limit := config.Limit
	if limit <= 0 {
		limit = defaultHistoryRankingConfig.Limit
	}
	if limit > len(ranked) {
		limit = len(ranked)
	}

	newest := entries[len(entries)-1]
	selected := make([]changeledger.Entry, 0, limit)
	newestIncluded := false
	for i := 0; i < limit; i++ {
		selected = append(selected, ranked[i].Entry)
		if ranked[i].Entry.CommitHash == newest.CommitHash {
			newestIncluded = true
		}
	}
	if !newestIncluded && len(selected) > 0 {
		selected = append([]changeledger.Entry{newest}, selected[:len(selected)-1]...)
	}

	return HistorySelection{Entries: selected, Ranked: true, CandidateCount: candidateCount}
}

// LogHistoryRankingSelection writes a deterministic audit line for each ranked
// candidate and whether it was selected (CP-54 P-6 / SS-14 AC-7).
func LogHistoryRankingSelection(featureKey string, locus RetrievalLocus, ranked []RankedHistoryEntry, selected []changeledger.Entry, limit int) {
	if len(ranked) == 0 {
		return
	}
	selectedSet := make(map[string]struct{}, len(selected))
	for _, e := range selected {
		selectedSet[e.CommitHash] = struct{}{}
	}
	for _, r := range ranked {
		_, picked := selectedSet[r.Entry.CommitHash]
		log.Printf(
			"[context-rank] feature=%s rank=%d commit=%s selected=%t path_overlap=%d symbol_overlap=%d locus_paths=%d locus_symbols=%d limit=%d",
			featureKey,
			r.Rank,
			r.Entry.CommitHash,
			picked,
			r.Relevance.PathOverlap,
			r.Relevance.SymbolOverlap,
			len(locus.Paths),
			len(locus.Symbols),
			limit,
		)
	}
}
