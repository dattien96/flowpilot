# CA-214: Mirror Sub-Agent Approvals To The Hub Stream

## Summary

Fixed `BUG-177`: sub-agent approvals only surfaced on the main/hub view when that specific sub-agent was the focused run, so two reviewers requesting approval at once left nothing to approve on main. The runner now mirrors a child's `permission_required` onto the hub run's stream (which the desktop already ingests), and the desktop dedups approvals by `approvalId`.

## What Changed

- `apps/local-runner/internal/runner/interactive_service.go`: `turnBridge.RequestApproval` now also emits the `permission_required` onto the parent/hub run's event stream when `parentRunID != ""` (same `approvalId`/details). Resolution remains global by `approvalId`.
- `apps/desktop-flowpilot/src/state/timelineReducer.ts`: the `permission_required` reducer is now idempotent by `approvalId` — skips if that approval is already pending or already in the timeline — so the hub mirror plus a concurrently-focused child stream never create duplicate cards / `pendingApprovals` entries (also hardens replay-from-seq-0).
- `apps/local-runner/internal/runner/flow_step_runtime_test.go`: added `TestChildApprovalMirroredToHubStream`.

## Verification

- New mirror test passes; `go test -run 'Approval|Flow|Workflow|Orchestrat|Cohort|Coder|Reviewer|Spawn|Child|Progress|StepRuntime|Advance|Resolve|Hub|Review|Loop'` → 389 passed (the only failures are the pre-existing codex-resume tests needing a real codex binary). `go build` clean; desktop `npm run typecheck` clean.
- Desktop `vitest` cannot run in this environment (pre-existing `require is not defined in ES module scope` config error that blocks the test files from loading) — flagged in `BUG-177` (`V-4`); a live multi-reviewer approval on the main view should be confirmed by the user.

## Notes

- The desktop's always-on orchestration stream already routes non-graph/bus events through `applyEvent`, so the mirror surfaces on main with no new subscription — only the emit + dedup were needed.
- The identical single-stream gap for `user_question_required` (sub-agent `ask_user`) is intentionally left for a follow-up; the same mirror+dedup pattern applies.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-177
change_type: bugfix
summary: Mirror a sub-agent's permission_required onto the hub run's event stream so concurrent cohort approvals surface and are actionable on the main view, with approvalId dedup on the desktop
# --->8---
