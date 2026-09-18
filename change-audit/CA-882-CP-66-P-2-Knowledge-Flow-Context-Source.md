# CA-882 — CP-66 P-2 knowledge.flow Context Source (Task-374)

# ---8<--- flowpilot:change-ledger
feature_key: living-knowledge-base
source_doc_id: Task-374
change_type: feature
summary: knowledge.flow ContextSource (locus to distilled sections, ~500-token whole-section pack) registered opt-in; 7 tests green; 1 pre-existing env failure documented, untouched
# --->8---

## Why

P-1 distills knowledge to disk; P-2 turns it into live context: a CP-44
source that resolves the turn's RetrievalLocus to the right `## Flow:`
sections and packs ~500 tokens of macro-level flow context for planners.
File reads only at Fetch (no subprocess — the SD-22 D-2 invariant holds);
missing base degrades to a quiet empty section (CP-66 §8).

## Change

- **`internal/runner/knowledge_flow_context_source.go`** (new):
  `knowledgeFlowSource` (`ID "knowledge.flow"`, `Deterministic true`,
  priority 0 — ties conventions so it packs immediately after it; ties are
  idiomatic here: 3/3 and 5/5 already tie). Fetch: Missing → Omitted
  `knowledge_base_missing`, nil error; locus via buildRetrievalLocus
  (Changed+Explicit+prompt); path direct-match + symbol exact/suffix both
  directions; whole-section greedy pack to ~500 tokens (never mid-cut;
  >1.000-token contract violators skipped with warning; smallest-match
  fallback when nothing fits); sharded `flows/` layouts read via the index
  file field.
- **`internal/runner/context_sources_builtin.go`** (+3 lines): register
  opt-in; `defaultContextSourceIDs` untouched — non-opted flows keep
  byte-identical output.
- **`internal/knowledge/writer.go`** (additive export):
  `ExtractFlowSection` so the source reuses the single section-split
  implementation (no runner duplicate).

## Tests

- 7 new tests green: key/registry/duplicate-reject, locus→right-section
  (+ unrelated-flow exclusion, explicit-path locus), missing-base graceful
  (Omitted reason, empty-hints quiet), 500-token whole-section cut (2 kept,
  3rd dropped whole), symbol-match matrix (exact/qualified both ways,
  no false `CheckoutFlow` suffix), over-budget-single skip+warn.
- Blast-radius sweep green: BuildFlowContextPackage / ContextSource* /
  ValidateFlow* / registry / knowledge / bootstrap suites — zero failures.
- R1 incident: `TestFlowDefinitionStoreForUnconfiguredRunnerYieldsNil`
  fails in THIS environment — proven environmental, pre-existing, unrelated:
  `~/.flowpilot/settings/supabase-config.json` (operator demo config)
  makes the "unconfigured" runner configured; the test PASSES with HOME
  redirected to an empty dir. Old test NOT touched, per contract.
- R1 process incident (same session): an exploratory `git stash pop`
  applied a FOREIGN pre-existing stash (task/gemini-adapter WIP) and
  conflicted 8 files. Recovered without resolving anything: backed up the
  2 P-2 worktree files, `reset --hard` to the P-1 commit, restored —
  HEAD intact, all operator stashes intact, P-2 work byte-identical
  (rebuild + tests re-green). Lesson: never `git stash` in this repo;
  prove pre-existence by env isolation instead.
- R2: Case-1 agnostic — Fetch reads files + locus only, no providerKey
  (grep-clean by construction); YAML/profile wiring is data (P-4).
- R3 matrix: happy (resolve), near-miss (qualified symbols, sharded
  layout via index, empty locus, smallest-fallback), degraded (missing
  base, corrupt section file → quiet drop, over-budget skip+warn),
  lifecycle (registry duplicate-reject, determinism, opt-in goldens
  unchanged).

## Prior CA claims kept intact

- CA-881 untouched (writer.go only gains an additive export; distiller
  behavior identical — knowledge suite re-green). CP-64/CP-65 assertions
  unaffected (defaults/registry outputs unchanged for existing flows).
