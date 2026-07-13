package changecontract

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
)

// requirementsDocDirs lists every requirements/ subfolder a doc stem (a
// featurecatalog.DocRefs entry, e.g. "SS-14-Code-Context-And-Regression-Safety")
// can live under. Mirrors the source directories featurecatalog's own
// doc-ref loader globs — see catalog.go's loadDocRefs.
var requirementsDocDirs = []string{
	"05-System-Specs",
	"06-System-Tech-Design",
	filepath.Join("07-Coding-Plan", "done"),
	filepath.Join("07-Coding-Plan", "todo"),
	filepath.Join("07-Coding-Plan", "inprogress"),
	filepath.Join("08-Task", "done"),
	filepath.Join("08-Task", "todo"),
	filepath.Join("08-Task", "inprogress"),
	filepath.Join("09-BugFix", "done"),
	filepath.Join("09-BugFix", "todo"),
}

// resolveDocRefPath finds the real file for a DocRefs stem. DocRefs entries
// are full filename stems (extension stripped), not doc-id prefixes or
// paths, so this is a direct existence check per candidate directory rather
// than a glob/prefix search.
func resolveDocRefPath(workspace, stem string) string {
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return ""
	}
	for _, dir := range requirementsDocDirs {
		candidate := filepath.Join(workspace, "requirements", dir, stem+".md")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

// hashGoverningDocs resolves and hashes each doc-ref stem. A doc that cannot
// be found or read gets an empty hash recorded rather than being omitted or
// panicking (F-1) — SpecDrifted then correctly sees "was X, now missing" as
// a drift rather than silently ignoring it.
func hashGoverningDocs(workspace string, docIDs []string) map[string]string {
	if len(docIDs) == 0 {
		return nil
	}
	hashes := make(map[string]string, len(docIDs))
	for _, id := range docIDs {
		path := resolveDocRefPath(workspace, id)
		if path == "" {
			hashes[id] = ""
			continue
		}
		hash, err := HashDoc(path)
		if err != nil {
			hashes[id] = ""
			continue
		}
		hashes[id] = hash
	}
	return hashes
}

// BuildHead mints a fresh CanonicalHead for featureKey (Task-186 T-3):
//   - governing docs come from featurecatalog.DocRefs (catalog may be nil);
//   - the behavior statement backfills from the newest changeledger entry
//     when history exists;
//   - "birth": when there is no history at all, the statement is minted from
//     birthContract's own declared/inferred Intent instead (birthContract may
//     be nil, e.g. when backfilling ahead of any turn — the Head is then
//     created with an empty statement, flagged low-confidence via spec_less
//     only if it also has no governing docs).
//
// spec_confidence/status is spec_less when there are no governing docs
// (AC-2), else spec_backed/current.
func BuildHead(workspace, featureKey string, ledger *changeledger.Ledger, catalog *featurecatalog.Catalog, birthContract *Contract) CanonicalHead {
	h := CanonicalHead{
		FeatureKey: featureKey,
		UpdatedAt:  time.Now().UTC(),
	}

	if catalog != nil {
		if feat, ok := catalog.Get(featureKey); ok {
			h.GoverningDocIDs = append([]string(nil), feat.DocRefs...)
		}
	}
	h.GoverningDocHashes = hashGoverningDocs(workspace, h.GoverningDocIDs)

	if ledger != nil {
		if entry, ok := ledger.LatestEntry(featureKey); ok {
			h.HeadCommit = entry.CommitHash
			h.BehaviorStatement = behaviorStatementFromEntry(entry)
		}
	}

	if h.BehaviorStatement == "" && birthContract != nil {
		h.BehaviorStatement = birthContract.Intent
	}

	if len(h.GoverningDocIDs) == 0 {
		h.SpecConfidence = SpecConfidenceSpecLess
		h.Status = HeadStatusSpecLess
	} else {
		h.SpecConfidence = SpecConfidenceSpecBacked
		h.Status = HeadStatusCurrent
	}

	h.IntentSignature = ComputeSignature(h)
	return h
}

// behaviorStatementFromEntry prefers a CA excerpt (the "why") over the bare
// commit summary (the "what") — matches D-3's reasoning for leading with the
// richest available truth.
func behaviorStatementFromEntry(e changeledger.Entry) string {
	if strings.TrimSpace(e.CAExcerpt) != "" {
		return e.CAExcerpt
	}
	return e.Summary
}
