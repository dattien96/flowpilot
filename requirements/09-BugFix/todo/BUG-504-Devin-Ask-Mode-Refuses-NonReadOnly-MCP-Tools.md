# BUG-504 — Verdict-tool provisioning inconsistent across gated children: devin `ask` mode refuses non-readOnlyHint MCP tools; a grok reviewer session lacked `submit_review_outcome` entirely

## Status
FIXED — unit-verified + MCP-layer verified live, 2026-09-26, fixed build.

- Fix (CA-1009): (a) `submit_review_outcome` is now offered whenever the
  child's node posture is `verdict_only` OR the session's host flow
  declares any `verdict_only` node — owner_debate members and hub
  sessions were missed by the cohort mapping. (b) The runner-hosted
  interaction/verdict tools (`approve`, `ask_user`,
  `submit_review_outcome`, `vibe-requirement-outcome`) carry
  `annotations.readOnlyHint: true` — they mutate only FlowPilot's
  orchestration ledger, never the session environment — so devin's
  ask/accept-edits mode filter no longer walls them off upstream of the
  runner bridge. `spawn_agent` is deliberately NOT annotated (real side
  effect).
- Unit: `bug504_verdict_tool_exposure_test.go` (3 tests, green).
- Live re-verify: `tools/list` on a live devin/grok session's runner MCP
  endpoint shows `readOnlyHint: true` on approve/ask_user and none on
  spawn_agent. Verdict-only child offer proven at unit level; owner-debate
  end-to-end verdict landing pending the running vibe leg (run-23536).

## Live-found during
Full CP live-test rerun, 2026-09-26, build 8c95a5bb.

- `run-1` (task-harness, devin/swe-2-high hub, lt-full bed):
  `plan_reviewer` child (devin, session mode `accept-edits`/`ask`) completed
  its review but never called `submit_review_outcome` — the session refused
  to execute the MCP tool because it is not `readOnlyHint`-annotated.
  - The runner-side permission layer DID allow the call (`allow_once` —
    BUG-374's fix is working); the refusal happened **inside devin's own
    session mode filter**, upstream of our permission bridge.
  - Result: `plan_synthesis` parked on `missing machine verdict from plan
    reviewer(s): plan_reviewer`. Bare `continue` re-parked
    (`no progress since last continue`) — the park is unrecoverable because
    no verdict can ever arrive.
- Contrast (same build, same flow family): `run-6010`/`run-7276` grok
  `plan_reviewer` submitted `submit_review_outcome` normally and the flow
  advanced to freeze. `run-14071` (historical) also had grok review.
- Second mechanism on `run-3688` (tournament, 2026-09-25 ~23:52): a grok
  review-side child recorded in-band
  "`submit_review_outcome` is not exposed on the `flowpilot` MCP server on
  this turn (available tools: `approve`, `ask_user`, `spawn_agent`)" — the
  tool was *not registered at all* for that session shape, a different
  failure mode than devin's mode filter. The cohort note carried the verdict
  as prose, so the tournament survived it; a gated flow node depending on a
  machine verdict would not have.
- Working counter-example: `run-16950` vibe owner-debate synthesis turn
  (devin) DID call `submit_review_outcome` successfully
  ("Submitted the consolidated review outcome: changes_requested") — so the
  tool is correctly exposed on synthesis/hub sessions; the gap is specific
  to gated/reviewer child sessions.
- Third observation, `run-22241`/`turn-22554` (vibe-ingest, grok hub,
  2026-09-26): the owner-debate synthesis session also lacked
  `submit_review_outcome`. The model invented a file-drop fallback —
  "I'll look up `submit_review_outcome` and record `blocked`" → requested
  `Write /tmp/submit_review_outcome.json`. **No runner code reads that
  path** — it is a provider-invented convention with zero ingestion. The
  write also required interactive approval (appr-22618) which expired
  unanswered. Verdict lost silently; the run surfaced `completed` while
  the owner-debate loop stayed open (`loop_status=running`, round 0/5).
  See BUG-507 for the expiry/status consequence.

## Root cause

BUG-374's fix covered the **runner's** approval/permission layer (denied →
allow_once). It does not cover the **provider-side** tool filter: when a
devin session runs in `ask` mode, devin itself refuses to execute MCP tools
that lack `readOnlyHint`, regardless of the runner's allow decision. Gated
children (reviewer/arbiter roles that must call `submit_review_outcome`,
`submit_coder_outcome`, `flow_control`) are structurally unable to complete
their contract when they land on a devin `ask`-mode session.

## Impact

Any flow whose machine-verdict node lands on devin can permanently park:
task-harness `plan_reviewer`/`reviewer`, tournament arbiter paths, cp-harness
`cp_reviewer`. Currently avoided only by luck of cohort provider assignment
(historical green runs had reviewers on grok/codex/claude).

## Suggested fix directions (needs design pick)

1. Spawn verdict-bearing children in a session mode that permits the toolset
   (`accept-edits` apparently still refused — confirm which devin modes allow
   non-readOnly MCP calls, or run gated children with tools via a side
   channel).
2. Annotate FlowPilot's verdict tools with `readOnlyHint`? Wrong semantically
   (they mutate run state) — likely rejected by review.
3. Fallback seam: if a verdict child completes without a machine verdict, let
   the hub re-dispatch it on a different provider instead of dead-parking
   (bounded, provider-switch = new leg per §2).
4. Surface the refusal reason in the park card — today the operator sees only
   "missing machine verdict" and cannot tell permission-denied from
   provider-refused.

## Related
- BUG-374 (fixed): permission-layer denial of the same tool — sibling
  mechanism, different layer.
- Watch item (same family): codex gpt-5.4 hub ignoring
  `submit_review_outcome` — possible tool-name drift
  (`flowpilot_submit_review_outcome` vs `mcp__flowpilot__…`).
