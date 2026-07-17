# CA-341: CP-51 Task-248..256 status audit correction (done → in_progress)

## Scope

Audit of all CP-51 tasks (248–258) against 4 criteria each (Code Guide followed, tests ≥ skeletons, Acceptance Check satisfied, DOD marked). Found 9 tasks marked `done` in metadata that are not actually complete. No code changed — documentation/traceability correction only, so the ledger stops overstating completion.

## Findings (per-task reality)

- **248** PARTIAL — core state machine + store + local NDJSON landed & unit-tested; doc §5/acceptance still assume dropped Supabase dispatch backend; 2 defined tests missing (1 real: lease+Stop atomicity).
- **249** NOT DONE — prep/claim/linearize/Accepted/Terminal seams wired, but durable Stop path (`requestRunStopV2`) has 0 callers → root Stop fence inert in prod; T-6 RAM clear not removed; 2/19 tests.
- **250** NOT DONE — reconcile/attach engine (T-4) + resume-boot wiring (T-6) absent; scanner never invoked; ~0/32 tests.
- **251** NOT DONE — SettleDriver is an unwired stub with placeholder effects; legacy checkpoint dance intact; 0/11 tests.
- **252** PARTIAL — security fix (DOD-I4) done; durable provenance (T-3/T-4) is a non-functional stub (never stamped/persisted/restored); 2/7 tests.
- **253** PARTIAL — versioned-blob/fail-closed logic + store repair ops exist, but not wired into prod Supabase load path (nil-dispatch legacy loader); ~5/17 tests.
- **254** PARTIAL — retention done; T-1 authority inversion (`FindActiveByOuterIntent`) is dead code; 2/4 tests.
- **255** NOT DONE — DOD suite owner; real subprocess-kill crash harness absent, crash cells use the explicitly-rejected in-process rebuild; 10/12 named suites missing; 1/25 skeletons.
- **256** PARTIAL — backend endpoints + card exist but use forbidden `/dispatch` root namespace, no inspect endpoint, no durable attention events, UI card incomplete; 0/5 named tests.
- **257** DONE (scoped: Codex/Grok/Claude; Gemini deferred).
- **258** DONE (local NDJSON authority + Drive sync + Supabase DROP migration; 5/5 acceptance).

CP-51 §10.1 acceptance ledger: 2/57 rows ✅. CP-51 is NOT done by its own finish-line definition.

## Changes

- Task-248..256: metadata `Status` `done` → `in_progress` with inline reason; `Last Updated` → 2026-07-17; §8 Completion Notes rewritten with an **Audit Gap (2026-07-17)** block listing the exact remaining work per task.
- Task-249: fixed traceability drift — 7 places said "Claude/Gemini are V2-disabled" (stale after Claude was enabled today); now "Codex/Grok/Claude three-outcome, Gemini V2-disabled". Updated `CE-CL/GEM` → `CE-GEM` reference.
- Task-257: `CE-CL/GEM` references split to `CE-CL` / `CE-GEM`.
- No `apps/` code changed in this correction.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: docs
summary: Correct CP-51 Task-248..256 status from premature done to in_progress with per-task audit-gap notes; fix Task-249 Claude/Gemini V2 drift
# --->8---
