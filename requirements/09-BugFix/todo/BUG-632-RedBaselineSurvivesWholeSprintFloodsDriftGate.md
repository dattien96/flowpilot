# BUG-632 — Red `test_baseline` captured during TDD-stub phase persists for the entire dirty sprint → `gate_blind` warn flood feeds the drift gate

- **ID:** BUG-632
- **Severity:** Medium — non-blocking (downgraded to warn under
  contract-first, CP-67) but it floods every code-touching turn of the
  sprint and feeds the drift scorer that mounts owner debates
- **Status:** open
- **Found:** run-174243, 2026-10-03 — `.flowpilot/guard/test_baseline.json`
  held `suite_passed:false, green_tests:[]` captured 20:34 (Task-032's
  red-stub phase); every later turn emitted `gate_blind red_at_capture`
  while the actual suite was 13/13 green

## Symptom

`RefreshBaselineIfStaleContext` semantics:

```go
if dirty          { return bl }      // mid-edit: keep prior truth
if bl.SuitePassed { return bl }      // green baseline: frozen
return CaptureBaselineContext(...)   // red baseline + clean HEAD: recapture
```

A baseline captured *red* is only ever recaptured when the tree is
**clean AND HEAD moved**. But a sprint's working tree is dirty by
construction from scaffold until the audit commit — so a red-stub
baseline persists for the entire sprint, emitting `red_at_capture` on
every production-code turn regardless of the suite's real state.

## Defect

The "mid-edit keeps prior truth" rule treats a *TDD-intentional* red
the same as a *regression* red, and the "dirty ⇒ keep" rule has no
freshness bound — minutes or hours of subsequent green suite runs are
invisible to the gate. The warn-level output then flows into drift
scoring each turn, contributing to the debate-mount cascade seen on
run-174243.

## Expected fix direction

- Recapture (or mark superseded) a red baseline when direct suite
  evidence exists — e.g. a `gate` suite run inside the same turn that
  passed (the run already shells the suite; a passing result should
  update `green_tests`/`suite_passed`, not just emit warn).
- Alternatively scope baseline freshness to the leg/contract: a red
  baseline minted under tdd-contract X must not be inherited as truth
  by coder-contract Y.
- Keep the fail-closed posture for *unknown/unverifiable* baselines —
  this is about not mislabelling verified-green as red.
