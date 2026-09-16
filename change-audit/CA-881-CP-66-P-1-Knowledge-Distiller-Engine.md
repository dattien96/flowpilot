# CA-881 — CP-66 P-1 Knowledge Distiller Engine (Task-373)

# ---8<--- flowpilot:change-ledger
feature_key: living-knowledge-base
source_doc_id: Task-373
change_type: feature
summary: New internal/knowledge distiller+writer, structure.Processes cypher read path, background bootstrap on run creation; 13 tests green + live-index proof
# --->8---

## Why

P-1 is the knowledge-producing core every later CP-66 slice consumes: turn
GitNexus execution flows (macro) into three fixed-layout Markdown docs +
index.json under `.flowpilot/knowledge/`, distilled once per process in the
background, updated incrementally later. No CLI drill-down exists for
process members, so the read path goes through `gitnexus cypher`
(Process nodes + STEP edges + `Kind:path:name` symbol uids — probed live
before coding).

## Change

- **`internal/structure/processes.go`** (new, additive — gitnexus.go
  untouched): `Processes` (top flows by step count + batched member-symbol
  IN query, chunked at 100) and `ModelCandidates` (data-model kinds ranked
  by flow participation, ranked in Go). Only MATCH/RETURN/WHERE/IN dialect;
  GFM-table envelope parser (`{"markdown"}` / `{"error"}`); malformed uids
  degrade to name-only symbols, never dropped.
- **`internal/knowledge/distiller.go`** (new): `Distill` renders
  system-overview.md (fixed TOC: tech stack / layer boundaries / core
  libraries / flow census), execution-flows.md (`## Flow: <id>` sections),
  data-models.md, plus the index.json lookup plane (flow entries + reverse
  path→flows / symbol→flows maps, sorted for deterministic re-render).
  Sections shrink-to-fit under 1.000 tokens (halve lists, never mid-cut);
  >500 flows shard to `flows/<domain>.md` + manifest (Q-1). LSP lister is
  optional (nil = GitNexus-only degrade). Token unit is
  promptpacker.EstimateTokens — same counter P-2 Fetch will use.
- **`internal/knowledge/writer.go`** (new): `WriteFull` (atomic
  temp+rename per file, stale-layout cleanup), `IncrementalUpdate`
  (reverse-index lookup, body-for-body section swap, write-changed-only,
  new-flow append, corrupt/mismatched index → full rebuild, empty affected
  set → no redistill call at all), `Missing`/`LoadIndex` helpers.
- **`internal/runner/knowledge_bootstrap.go`** (new):
  `ensureKnowledgeBaseAsync` (once-per-process guard, Missing fast path,
  failure clears guard for retry) and `UpdateAsync` (sync + per-workspace
  mutex; callers go-background; missing base / no code paths / errors all
  log-only). Production `gitnexusProcessLister` adapter; thin
  `ensureKnowledgeBaseForWorkspace` / `updateKnowledgeForAudit` methods.
- **`internal/runner/interactive_handlers.go`** (1 line): fire
  `ensureKnowledgeBaseForWorkspace(in.Cwd)` in createRun next to
  ensureGitNexusIndexAsync.
- Deviation from Task-373 T-1 noted: processes surface lives in a NEW
  structure file instead of editing gitnexus.go (zero existing-symbol
  churn; same additive guarantee). LSP adapter deferred (AC-4 path is the
  default in v1).

## Tests

- knowledge: 9 tests (overview TOC, flow sections + <1.000-token +
  index contract, incremental rewrite with byte-identical untouched
  section + index sync, unrelated-path fast path, no-model/LSP degrade,
  empty-index base, oversized-section shrink, >500-flow sharding,
  corrupt-index rebuild) — all green.
- runner: 4 bootstrap tests (once-only + proven-background via
  block-until-release closure, bootstrapped fast path, per-workspace
  serialization with max-concurrency assert, missing-base skip) — green.
- Live proof (FLOWPILOT_KNOWLEDGE_LIVE=1, env-gated, never CI): distilled
  flowpilot itself — 10 flows, 21 path keys, 84 symbol keys, ~300-token
  sections with real purpose/symbols/files (e.g. proc_0
  ArtifactCloudStoragePanel → SafeGetEdgeFunctionUrl, 8/8 symbols+files).
- R1: no pre-existing test edited; full knowledge + touched runner tests
  green; interactive_handlers one-liner covered by existing createRun-path
  suites (re-run in P-4 sweep).
- R2: Case-1 agnostic — distiller/writer/bootstrap touch no providerKey
  (file + graph reads only); verified by construction + grep at closeout.
- R3 matrix: happy (2-flow distill), near-miss (empty index, oversized
  section, sharded layout, new-flow append), degraded (corrupt index,
  missing base, nil LSP, lister errors), lifecycle (once-only, serialize,
  fast paths).

## Prior CA claims kept intact

- New feature_key `living-knowledge-base` (registered in FEATURE-KEYS.md);
  no prior CA to undo. CP-64/CP-65 flows untouched (single additive hook
  line in run creation; registry/defaults/YAML unchanged until P-2/P-4).
