# CA-836 — Task-333: /standardize command, reverse-doc drafts, non-bypassable SS-Lock gate

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-333
change_type: feature
summary: add /standardize [scope] (docscan conformance branch + GitNexus-evidenced reverse-doc branch) with SS draft skeleton, non-bypassable SS-Lock user-confirm gate, and CP-48 handoff
# --->8---

## Why

CP-49: brownfield projects need SS/SD/CP docs built from real evidence, with business intent owned by humans. Code says What, never Why — SS drafts stay skeletons (`TODO: human intent needed`) until a human approves at the SS-Lock gate; only then do drafts publish and hand off to the CP-48 docscan machinery (CA-834).

## Change

- `runner/standardize_cmd.go` (new): `ExecuteStandardize` — no scope = full project; existing docs → docscan conformance scan/autofix; missing docs → reverse-doc; partial docs → mixed mode; `StandardizeResult{Mode, ScanReport, DraftSSPath, DraftSDPath, RunID, Status}` (RunID/mode "mixed" are additive deviations needed for client resume and the §10 mixed-mode test).
- `runner/reverse_doc.go` (new): `CollectEvidence` via GitNexus CLI (timeout-guarded exec) with graceful fallback to static dir scan + Go AST + recent git log; `GenerateDraftSD` emits per-claim `(evidence: symbol/commit)` citations with TODO where evidence is missing; SS skeleton marks Goal/Problem/Non-Goals/User Stories/Acceptance Criteria/Business Rules/Open Questions with `TODO: human intent needed` and never authors intent. Design decision (reviewer-noted): draft generation is deterministic Go templating, 0 LLM — stronger anti-hallucination + provider-agnostic than the Code Guide's "AI subagent" sketch.
- `runner/ss_lock_gate.go` (new): one-shot mutex-guarded gate; pause emits CP-60-style user-confirm event; confirm = only resume path (merges user edits, publishes docs, runs docscan autofix handoff); reject = deletes drafts, publishes nothing. Non-bypassability verified by reviewer across all runner entry points: synthetic run invisible to `s.runs` (404 on resume/amend/gate/spawn paths), `ssLockTurnFence` 409s `POST .../turns` while parked.
- `runner/interactive_handlers.go`: 3 routes (`POST /client/standardize`, `POST /client/workflow-runs/{runId}/confirm`, `GET /client/workflow-runs/{runId}/ss-lock`) + turn fence.
- `tui/app`: `/standardize` slash command + result rendering (`standardize.go`).
- Read-only guarantee: target source never modified; writes only draft `todo/` docs, all tests pinned to `t.TempDir()`.

## Tests

`task333_standardize_test.go`: all 9 §10 signatures exact-name + 3 bonus (static fallback, SS-never-invents-intent, gate lifecycle/fence reset) — 12/12 pass. Full runner suite failures verified pre-existing at HEAD (13 tui failures byte-identical HEAD vs working tree; runner env failures per CA-835).

## Providers

Case 1 agnostic: deterministic Go + subprocess GitNexus; draft generation has no LLM call — identical behavior across Claude/Codex/Grok.

## Prior claims intact

CA-833/CA-835 (r-dod gates), CA-834 (docscan), CA-695/CA-442/CA-441 — untouched. CP-49 closed: doc moved to `07-Coding-Plan/done/` with all DOD items checked.
