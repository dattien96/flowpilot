# CA-1157 — BUG-625: parked/overlay flags on step-runtime projection + timeline split

Live trigger: run-174243 during the Task-033 owner debate. The BUG-582
union projection (active overlay nodes + parked sprint nodes) rendered 15
steps in the task timeline — the four debate rows numbered 12-15 inline —
and the sprint's `synthesis` hub.inline node kept its RUNNING stamp, so the
task rail showed two "running" steps (`tdd` gated, `synthesis` hub-busy)
while the sprint was actually suspended.

## Fix

`workflowStepRuntimeView` gains two additive flags (`omitempty`, no wire
break for older clients):

- `parked` — the row's node is in `rs.vibeParkedNodes` while an overlay
  holds the run: suspended sprint work; its last stamp is not in-flight.
- `overlay` — the row's node is in the mounted overlay graph (visible and
  not parked): engine-internal remediation, not a task-flow step.

Both derive from the same `vibeParkedNodes` membership the BUG-582 union
filter already tracks — no new state source, durable rows untouched.

Desktop (`FlowTimelineSidebar` + `FlowStepTimeline` + styles):

- Overlay rows leave the numbered rail entirely — progress counts task
  steps only ("N/11 steps") and a `debate overlay active` chip appears in
  the header so the suspension is legible, not silent.
- Parked rows dim (`fti-parked`, no pulse) and a parked non-terminal row's
  label reads "parked" — kills the fake `synthesis running` read.
- Parked rows can no longer take the current-step highlight;
  `activeWorkflowStep` is consulted over non-parked rows only.
- The liveness tick still counts overlay RUNNING rows — the debate is
  real work on the run's stream; only the task rail is filtered.

## Tests

`bug625_step_overlay_flags_test.go` (additive, BUG-582 harness):
- `TestBUG625_MountedOverlayFlagsRows` — parked sprint rows flagged
  `parked`, debate rows flagged `overlay`, never both.
- `TestBUG625_PostRestoreClearsFlags` — post-restore rows carry no flags.
