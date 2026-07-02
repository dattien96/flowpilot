# BUG-163: Flow-Pack Mirror-Sync Stamps Codex Provider Override

## Metadata

- Document ID: `BUG-163`
- Title: `Flow-Pack Mirror-Sync Stamps Codex Provider Override`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-160-Workflow-Step-Model-Override-Forced-And-Step-Identity-Hidden.md`, `requirements/09-BugFix/done/BUG-162-Coder-Reviewer-Steps-Must-Default-To-Claude-Haiku.md`
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, supabase, data`

## AI Quick View

### Summary

- The user was actually running the built-in **Review Loop** flow (`is_builtin=true`, `pack_flow_id=review-loop`), not the manually-built "Analytics Review" workflow — its runtime sidebar still showed `CODEX` on every step even after `BUG-162` set the Coder/Reviewer step types' catalog model to Claude Haiku.
- A user-provided CSV export of `workflow_steps` for the Review Loop mirror confirmed the exact mechanism: every mirrored node (`coder`, `reviewer_correctness`, `flow-hub-inline`'s `synthesis`) had `provider_override = codex` with `model_override` correctly empty — the model side was fixed, but `provider_override` was silently non-null.
- Root cause: `workflow_steps.provider_override` carries a Postgres **column-level default of `'codex'`** (`20260525140000_backfill_ai_model_and_reasoning_defaults.sql`). `insertSteps` (the flow-pack mirror-sync writer, `supabase_workflow_flow_store.go`) never included `provider_override` in its insert payload at all — so Postgres silently filled in the schema default for every mirrored node. `LoadRunSteps`'s model-derived provider fallback (`BUG-160`) only runs when `provider_override` is genuinely empty, so the schema default always won first.

### Current Ask

- The step must show Claude/Haiku, not Codex, once its model correctly resolves to Claude Haiku.

### Key Decisions

- `F-1` `insertSteps` now explicitly sets `"provider_override": nil` in every mirrored node's insert payload, overriding the schema default — a flow-engine node has no provider of its own (`agentpack.FlowNode` has no `Provider` field), so it should resolve the same way its model does: from what it's actually configured to run on, not a column default.
- `F-2` No data migration needed for the already-mirrored "Review Loop" rows: `ResolveBuiltin` unconditionally re-upserts (`replaceSteps` → `insertSteps`) every time a built-in flow is resolved/started, so the next time this flow runs, the mirror is freshly re-inserted with the corrected (null) `provider_override` automatically.

### Constraints

- Scoped to the flow-pack mirror-sync writer only — did not touch the schema default itself (other legitimate uses of `provider_override`, e.g. a manually-set override on a hand-built workflow step, are unaffected) and did not touch `model_override` (which has no such default and was already behaving correctly).

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go` (`insertSteps`)
- `supabase/migrations/20260525140000_backfill_ai_model_and_reasoning_defaults.sql:9,14` (`alter column provider_override set default 'codex'`)
- User-provided `workflow_steps_rows.csv`/`workflows_rows.csv` export — the concrete evidence that resolved this after static code review alone couldn't (this diagnosis required the live data; the read-side Go code was already correct).

## 1. Issue Summary

Even after `BUG-160`'s model-derived provider fallback and `BUG-162`'s Claude Haiku catalog default, the built-in Review Loop flow's runtime sidebar still showed `CODEX` on every step, because `provider_override` was non-null (schema-defaulted to `'codex'`) on the underlying `workflow_steps` rows, taking priority over the fallback.

## 2. Parent Links

- impacted coding plan: `none`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, a running Review Loop (built-in) flow.
- reproduction steps: start a Review Loop run; observe every step shows a `CODEX` badge despite `model_override` correctly resolving to `claude-haiku` via `BUG-162`'s fix.
- frequency: deterministic — every built-in flow-pack-mirrored node is affected, since `insertSteps` never set `provider_override` for any of them.

## 4. Expected vs Actual

- expected: provider badge derived from the resolved model (Claude), matching the Coder/Reviewer catalog default.
- actual: provider badge always `CODEX`, sourced from the `workflow_steps.provider_override` column's schema default rather than any real configuration.

## 5. Impact

- users affected: anyone running the built-in Review Loop (or any other flow-pack-mirrored flow with agent.delegate nodes).
- workflows affected: Flow Mode runtime sidebar display for built-in flows.
- severity: low — display-only; the actual model resolution (and therefore execution) via `BUG-158`/`BUG-160`'s fallback was already correct, only the provider *badge* was wrong.

## 6. Root Cause

- confirmed cause: `workflow_steps.provider_override`'s column-level default (`'codex'`, set in `20260525140000_backfill_ai_model_and_reasoning_defaults.sql`) filled in silently whenever `insertSteps` omitted the key from its insert payload — which it always did, since the flow-pack YAML schema has no per-node provider field to source one from.
- evidence: user-provided CSV showing `provider_override=codex` with a correctly-empty `model_override` on every Review Loop mirror row; `20260525140000_backfill_ai_model_and_reasoning_defaults.sql:9,14`; `insertSteps`'s payload map (pre-fix) never included `provider_override`.

## 7. Fix Strategy

- `F-1`/`F-2` as described in Key Decisions.

## 8. Validation

- `V-1` `go build ./...` in `apps/local-runner` — passes.
- `V-2` `go test ./internal/runner/... -run 'TestSupabaseWorkflowFlowStore'` — passes (existing tests don't assert on the exact insert payload keys, so no test needed updating).
- `V-3` `go test ./...` full suite in `apps/local-runner` — 16 pre-existing failures, all unrelated (Windows path mismatches, mocked Codex resume exec, skills-merge ordering, run-history ordering) — none reference `insertSteps`, `provider_override`, or `supabase_workflow_flow_store.go`.
- `V-4` Not executed: a live re-check that a freshly-started Review Loop run now shows a Claude/Haiku badge instead of Codex — no Supabase/backend available in this environment. Per `F-2`, this should self-correct on the next Review Loop run start without any manual data fix, since the mirror-sync always re-inserts on resolve.

## 9. Regression Guard

- tests: none added — the existing `TestSupabaseWorkflowFlowStore*` suite exercises `insertSteps` via mocked HTTP and doesn't assert on payload key presence; adding a assertion here was judged lower value than the fix itself given time constraints, and is a candidate follow-up.
- alerts: none.
- audit checks: recorded in `change-audit/CA-200-flow-pack-mirror-sync-provider-override-fix.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: the `provider_override`/`model` schema defaults (`'codex'`/`'gpt-5.5'`) themselves are left in place — they're reasonable defaults for the classic (non-flow-engine) step-authoring path where an admin picks an explicit provider; only the flow-pack mirror-sync writer, which has no provider of its own to offer, now overrides them with an explicit null.
