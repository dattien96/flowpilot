package runner

import (
	"log"
	"os"
	"strconv"
	"path/filepath"
	"strings"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// CP-67 P-2/P-2b (Task-379 / Task-383): runner-side half of the
// Contract-First Scaffold TDD contract. The rule engine stays pure — the
// helpers here own the I/O: reading the written files, dispatching the
// language adapters (Go native / React via node / Kotlin + C++ LSP-anchored),
// caching extractions, and writing the single-bump scaffold lock.

// scaffoldBodyCache caches per-content extractions across gate evaluations.
var scaffoldBodyCache = flowgate.NewStubBodyCache()

// scaffoldStaticBodyViolations runs the B-11 static whitelist over the
// production (non-test) files written in a scaffold turn. Returns whether
// any body sits outside the whitelist plus the "symbol:line" rows for the
// reprompt. Fail-open: a file the adapters cannot parse yields NO violation
// (Unverified), never a block — but the cache still records it so the
// evidence trail shows coverage.
func (s *InteractiveService) scaffoldStaticBodyViolations(cwd string, written []string) (bool, []string) {
	var offenders []string
	seen := map[string]bool{}
	for _, rel := range written {
		lang := flowgate.LangForPath(rel)
		if lang == "" || flowgate.IsTestFile(rel) {
			continue
		}
		abs := filepath.Join(cwd, filepath.FromSlash(rel))
		src, err := os.ReadFile(abs)
		if err != nil {
			continue // unreadable → Unverified, fail-open
		}
		if syms := scaffoldBodyCache.Get(rel, src); syms == nil {
			syms, extractErr := flowgate.ExtractSymbolInfos(lang, rel, src, s.lspSymbolSourceFor(lang))
			if extractErr != nil {
				log.Printf("[gate] scaffold body check: extract %s (%s) failed: %v — body left unverified", rel, lang, extractErr)
				continue
			}
			scaffoldBodyCache.Put(rel, src, syms)
			syms = scaffoldBodyCache.Get(rel, src)
		}
		syms := scaffoldBodyCache.Get(rel, src)
		for _, v := range flowgate.ValidateStubBodies(lang, rel, src, s.lspSymbolSourceFor(lang), syms) {
			if v.Line <= 0 {
				offenders = append(offenders, v.Symbol)
			} else {
				offenders = append(offenders, v.Symbol+":"+strconv.Itoa(v.Line))
			}
			seen[v.Symbol] = true
		}
	}
	return len(offenders) > 0, offenders
}

// lspSymbolSourceFor returns the LSP-backed SymbolSource for a language, or
// nil when no live client exists (Kotlin/C++ then degrade to the anchored
// regex pass with Unverified bodies — fail-open, per CP-67 Q-1/R-6).
func (s *InteractiveService) lspSymbolSourceFor(lang string) flowgate.SymbolSource {
	// The diagnostics-oriented LSP integration (CP-63) does not expose a
	// documentSymbol session at gate time yet; the adapter degrades cleanly.
	// Wiring a live clangd/kotlin-language-server session here is the
	// follow-up noted in the CA.
	return nil
}

// recordScaffoldArtifactsLock is the scaffold pass path (B-8.3): lock the
// scaffold's test files read-only AND snapshot SignatureHash + LockedSignatures
// in ONE version bump on the coder step's frozen contract. Never fails a turn
// — a missing contract or I/O error is logged and the turn completes (same
// defense-in-depth posture as recordReproduceTestLock).
func (s *InteractiveService) recordScaffoldArtifactsLock(cwd, parentRunID string, written []string) {
	if strings.TrimSpace(cwd) == "" || strings.TrimSpace(parentRunID) == "" {
		return
	}
	testPaths := scaffoldTestFiles(written)
	prodSignatures, lockedSignatures := s.scaffoldSignatureSnapshot(cwd, written)
	if len(testPaths) == 0 && len(prodSignatures) == 0 {
		return
	}
	writerID := s.flowWriterNodeIDForRun(parentRunID)
	if writerID == "" {
		log.Printf("[gate] scaffold: no agent.code/scaffold writer node in run %q; skipping artifact lock", parentRunID)
		return
	}
	store, err := changecontract.NewFrozenStore(cwd)
	if err != nil {
		log.Printf("[gate] scaffold: frozen store open failed: %v", err)
		return
	}
	existing, ok, err := store.GetFrozenForStep(parentRunID, writerID)
	if err != nil {
		log.Printf("[gate] scaffold: frozen contract lookup failed: %v", err)
		return
	}
	if !ok {
		log.Printf("[gate] scaffold: no active frozen contract for step %q; skipping lock", writerID)
		return
	}
	rec, err := changecontract.LockScaffoldArtifacts(store, existing, testPaths, prodSignatures, lockedSignatures, time.Now().UTC())
	if err != nil {
		log.Printf("[gate] scaffold: lock artifacts for step %q failed: %v", writerID, err)
		return
	}
	log.Printf("[gate] scaffold: locked %v read-only + pinned signature hash %s for coder step %q (contract v%d)",
		testPaths, shortHash(prodSignatures), writerID, rec.Version)
}

// scaffoldTestFiles returns the written test files (workspace-relative).
func scaffoldTestFiles(written []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range written {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] || !flowgate.IsTestFile(p) {
			continue
		}
		seen[p] = true
		out = append(out, filepath.ToSlash(p))
	}
	return out
}

// scaffoldSignatureSnapshot extracts the canonical signature strings over the
// production files written this turn (deduped for C/C++ decl-vs-def) and
// hashes them. The hash is empty when nothing extractable was written.
func (s *InteractiveService) scaffoldSignatureSnapshot(cwd string, written []string) (hash string, signatures []string) {
	var all []string
	seen := map[string]bool{}
	for _, rel := range written {
		lang := flowgate.LangForPath(rel)
		if lang == "" || flowgate.IsTestFile(rel) {
			continue
		}
		src, err := os.ReadFile(filepath.Join(cwd, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		syms, err := flowgate.ExtractSymbolInfos(lang, rel, src, s.lspSymbolSourceFor(lang))
		if err != nil {
			continue
		}
		if normalizeLangForDedupe(lang) {
			syms = flowgate.DedupeDeclAndDef(syms)
		}
		for _, sig := range flowgate.CanonicalStrings(syms) {
			if !seen[sig] {
				seen[sig] = true
				all = append(all, sig)
			}
		}
	}
	if len(all) == 0 {
		return "", nil
	}
	return flowgate.CanonicalSignatureHash(all), all
}

func normalizeLangForDedupe(lang string) bool {
	return flowgate.LangForPath("x.cpp") == lang || flowgate.LangForPath("x.c") == lang
}

func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// snapshotCoderSignatureHash recomputes the canonical signature hash over the
// coder turn's declared scope (+ anything newly written) and returns it with
// the drifted declarations (locked minus current / vice versa). Empty hash
// when extraction is impossible — the lock rule treats empty as no-signal.
func (s *InteractiveService) snapshotCoderSignatureHash(cwd string, rec changecontract.FrozenContractRecord, written []string) (hash string, drift []string) {
	paths := append([]string(nil), rec.DeclaredPaths...)
	seen := map[string]bool{}
	for _, p := range paths {
		seen[p] = true
	}
	for _, p := range written {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	var all []string
	for _, rel := range paths {
		lang := flowgate.LangForPath(rel)
		if lang == "" || flowgate.IsTestFile(rel) || isMarkdownPath(rel) {
			continue
		}
		src, err := os.ReadFile(filepath.Join(cwd, filepath.FromSlash(rel)))
		if err != nil {
			continue // declared but gone → its locked signature disappears from the diff naturally
		}
		syms, err := flowgate.ExtractSymbolInfos(lang, rel, src, s.lspSymbolSourceFor(lang))
		if err != nil {
			continue
		}
		if normalizeLangForDedupe(lang) {
			syms = flowgate.DedupeDeclAndDef(syms)
		}
		all = append(all, flowgate.CanonicalStrings(syms)...)
	}
	if len(all) == 0 {
		return "", nil
	}
	hash = flowgate.CanonicalSignatureHash(all)
	if hash != rec.SignatureHash {
		locked := map[string]bool{}
		for _, sig := range rec.LockedSignatures {
			locked[sig] = true
		}
		current := map[string]bool{}
		for _, sig := range all {
			current[sig] = true
		}
		for _, sig := range rec.LockedSignatures {
			if !current[sig] {
				drift = append(drift, "removed: "+sig)
			}
		}
		for _, sig := range all {
			if !locked[sig] {
				drift = append(drift, "added: "+sig)
			}
		}
	}
	return hash, drift
}

func isMarkdownPath(p string) bool {
	return strings.HasSuffix(strings.ToLower(p), ".md")
}

// coderRenegotiatingForRun peeks (non-destructively) at the buffered coder
// batch for the run's current step — the ONLY legal r-signature-lock bypass.
// The synthesis_negotiation hub consumes the buffer when it mediates.
func (s *InteractiveService) coderRenegotiatingForRun(parentRunID string) bool {
	return len(s.snapshotCoderBatchSignatures(parentRunID)) > 0
}
