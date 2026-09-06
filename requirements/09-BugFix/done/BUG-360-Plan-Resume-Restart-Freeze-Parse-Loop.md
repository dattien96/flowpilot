# BUG-360: plan-phase resume after restart dies in freeze strict-parse loop

## Metadata

- Document ID: `BUG-360`
- Title: `plan-phase resume after restart dies in freeze strict-parse loop`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-06`
- Last Updated: `2026-09-06`
- Feature Keys: `agent-flow-engine`
- Parent Documents: [Task-325](../../08-Task/done/Task-325-Conditional-Plan-Approval-Park.md)
- Child Documents: `none`
- Related Documents: [BUG-357](../../09-BugFix/done/BUG-357-Flow-Doc-Writer-Khong-Ghi-Artifact-Instance.md)
- Replaces: `none`
- Tags: `freeze, restart-durability, planner-draft, escalate-loop`

## AI Quick View

### Summary

- Symptom (live run-584646, 2026-09-06): after reopen + approve from a plan_approval park (pre-restart), the flow advanced to `preflight_contract_freeze` and parked `escalate`: "invalid planner proposal: changecontract: strict preflight draft parse: invalid character 'c' looking for beginning of value". Every Retry re-escalates "(no progress since last continue)" — dead-end loop, no button helps except Stop.
- Root cause (reasoned, code-conformant): the planner draft lives only in the transient `preflight_contract_plan` child turn result. Post-restart the child is gone, `findPlannerResultForFreeze` finds nothing, and freeze strict-parses whatever prose fallback remains (starts with 'c') → fail. The park/approve machinery itself worked (LoopState + topology rehydrated, advance dispatched).
- Impact: any plan-phase resume after runner restart (approve OR feedback) funnels into this dead end. In-session approve is unaffected (proven separately).

### Current Ask

- Decide fix direction (see below), then implement + test. Capture-only at filing.

### Key Decisions

- D-1: Filed open without code change (same capture pattern as BUG-357 at filing).
- D-2: Approve-path live verify moves to a fresh no-restart run; run-584646 served its purpose (park trigger) and should be Stopped.

### Constraints

- additive-tests-only / oracle-rule as always.
- Must not weaken freeze strictness for the normal path (fail-closed stays).

### Open Questions

- Q-1: RESOLVED against (b)-as-specced — the frozen input is the preflight (scout) JSON, not the plan doc, so doc-derivation would change the contract source. Implemented instead: durable scout-draft cache (capture at settle → parent session → jsonb blob → reconstruct → fallback read). Same durability property, correct layer.
- Q-2: Same amnesia class may affect other child-turn-result consumers post-restart (audit?). Scope check during fix.

### Source Refs

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:1641` (`findPlannerResultForFreeze`), `:1730-1741` (fallback), `:1680`+ (`runContractFreezeNode`)
- Live: run-584646 transcript (approve → freeze escalate ×N with "no progress" suffix); `plan_synthesis` step shows CANCELED (park-cancelled approving turn)

## Evidence

- Screenshot 2026-09-06: `blocked: escalate`, reason verbatim above, `preflight_contract_freeze WAITING_USER_APPROVAL`, Retry→error toast, repeats with "(no progress since last continue)".
- run-584646 history: parked plan_approval pre-restart (writer round ≥1, Task-910 revised plan) → reopened via `/open` (card restored, durability OK) → Retry/approve advanced past the plan gate (no re-park) → freeze escalate loop.

## Root Cause

Planner draft = transient child-turn result; restart amnesia leaves freeze with prose fallback that strict parse rejects. Park/LoopState/topology durability all held — only the draft didn't survive.

## Fix direction (proposed, NOT implemented)

- Prefer (b): freeze derives draft from the on-disk plan doc (`plan_md` binding path, resolved via BUG-357-style lookup) when the child result is absent; keep strictness when a draft IS present.
- Tests: approve-after-restart with dead planner child → freeze proceeds from doc (new file); existing freeze/restart suites green.

## Completion Notes (implemented 2026-09-06, CA-754)

- Implemented as scout-draft CACHE, not plan-doc derivation: the frozen input is the preflight (scout) JSON draft, not the Task/BUG doc — deriving from the doc would change the contract source. Cache written at child settle (parse-gated, same `ParsePreflightDraft` freeze uses), carried on the parent via session snapshot + `session_runtime` jsonb (no migration), restored on reconstruct, read as last fallback in `findPlannerResultForFreeze` (live children first).
- Files: `interactiveRun.preflightDraftResult` + `cachePreflightDraftLocked` hook in `settleFlowChildTurnCompletedLocked` (now in `plan_approval_park.go`); `ProviderSessionState.PreflightDraftResult` + `sessionStateOf` + blob `preflight_draft_result` + `applySessionRuntimeBlob` + reconstruct mapping; stash fallback in `findPlannerResultForFreeze`.
- Tests (8, `bug360_preflight_draft_durable_test.go`, race-clean): parse-gated capture table, scout-prose/empty clears + dirty returns + settle-driven wiring, fallback order (live > stash > ""), e2e freeze-from-stash without scout child, fail-closed without any draft, restart round-trip (snapshot → local file store → disk reload → reconstruct + blob leg), clear-survives-restart (fresh-store reloads `""`).
- R1: full runner suite failure set == baseline churn (19≅19; `TestRun12613` full-suite-only flake passes isolated ×2 on both trees + together with new tests; `TestGrokSpawn…` appeared on a baseline run instead) — zero consistent new failures, zero old-test edits. agentpack/flowgate green; vet clean; gofmt new-lines clean (4 flagged files are pre-existing churn).
- Review round 1 (sub-agent, FAIL→fixed): C1 local-file NDJSON disk leg dropped the field (test only hit memory) — fixed with record field + both mappings + disk-reload test; I1 crash window between settle-cache and next parent persist — fixed with `go persistParentSession` on cache write; I2 stale stash on scout re-run failure — scout-labeled prose now clears (fail-closed over stale-freeze); I3 tests masked C1 + didn't prove settle wiring — fixed with disk-reload leg + settle-driven test; M2 store trimmed, M3 json tag added. M1 (any-child caching) kept deliberately — mirrors the existing fallback second loop.
- Review round 2 (sub-agent, FAIL→fixed): turn-2 caught I2-clear-not-durable — scout-prose clear returned `false` so settle never persisted it (RAM-only; restart resurrected the stale draft), and empty scout output didn't clear even RAM. Fixed: helper returns `true` on any stash mutation (write, scout prose clear, empty scout clear); no-op clears stay `false`; settle hook unchanged. Tests: extended clear tests + `TestBug360ScoutClearSurvivesRestart` (fresh-store reloads `""`) + `rec2` assert. R1: 8/8 race-clean; full suite 19 = 18 baseline + 1 stash-proven flake (`TestFlowCodingPrompt…`, fails 2/3 `-count=3` on clean tree).
- R2: orchestrator/persist path, zero adapter branches → provider-agnostic (no provider switch in touched code; freeze fixtures already matrix across providers in run201295).
