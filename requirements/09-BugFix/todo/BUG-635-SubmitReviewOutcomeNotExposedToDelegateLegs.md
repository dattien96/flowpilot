# BUG-635 — `submit_review_outcome` not exposed through the MCP tool surface for delegate legs → completed work cannot report verdict, infinite re-drive + `zero_delta_progress` drift churn + escalate loops

- **ID:** BUG-635
- **Severity:** High — hit on every remediation leg in both live lanes
  (vibe-tasks run-306526, vibe-adopt run-297984); each leg re-drives 7+
  times re-confirming the same green verdict while the engine never
  receives it.
- **Status:** FIXED — CA-1236 (2026-10-08): every flow-driven delegate child (agent.code/agent.delegate/agent.scaffold) is offered submit_review_outcome; bridge still rejects settle from non-cohort legs
- **Found:** run-348382 (coder leg, CP-04 Task-042), 2026-10-06 —
  leg self-reported via question `q-353592`: suite GREEN
  (10 PASSED / 2 SKIPPED), commits landed (`e729743`, `b904107`),
  CA-083 mounted, but "submit_review_outcome không được expose qua MCP
  `flowpilot` cho leg này — mỗi lần re-drive đều xác nhận lại cùng một
  kết quả GREEN". Same signature seen earlier on CP-05's coder leg.

## Symptom

A delegate leg finishes its work but its declared completion face —
`submit_review_outcome` — is not present in its MCP tool surface. The leg
has no channel to deliver the verdict, so:

1. The leg finishes its turn with verdict only in prose.
2. Engine sees no outcome → treats the leg as unsettled → re-drives.
3. Re-driven leg finds nothing to do → reports `completed` again →
   `zero_delta_progress` drift gate fires → escalate → operator continue.
4. Cycle repeats until the leg itself escalates with a meta-question.

On run-348382 this ran 7+ re-drives before the leg surfaced the
question card explicitly naming the dispatch dead-end.

## Defect

The deterministic-pipeline contract requires every leg's outcome to
arrive through its declared tool face. `submit_review_outcome` is
declared for writer/coder-style legs (it backs `flow-control`
ReviewOutcomeInput on the hub side) but is not registered/exposed on the
delegate leg's MCP surface — so the leg literally cannot comply with the
protocol. The engine then interprets "no verdict" as "still working" and
re-drives a completed leg indefinitely, bounded only by escalate/round
caps that surface as noise, not as the real defect.

## Expected fix direction

- Audit the MCP tool registration for delegate legs: the leg that runs
  `agent.code`/`agent.delegate` behavior must have `submit_review_outcome`
  (or its mapped transition face) in its tool surface whenever its node
  contract expects a machine verdict.
- Fail loud when the tool face is missing at spawn: a leg whose contract
  requires a declared outcome face but lacks the tool should surface
  `missing_tool_face` immediately — not after N silent re-drives.
- The re-drive loop should detect the no-tool-face case and route to a
  requirement-class park naming the missing face, instead of escalating
  on `zero_delta_progress` (which blames the leg for the engine's gap).
