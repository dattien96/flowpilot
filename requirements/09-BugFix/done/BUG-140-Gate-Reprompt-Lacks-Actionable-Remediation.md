# BUG-140: Gate Reprompt Lacks Actionable Remediation

## Metadata

- Document ID: `BUG-140`
- Title: `Gate Reprompt Lacks Actionable Remediation`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: none
- Related Documents: [BUG-139](./BUG-139-r-bug-Action-Should-Be-Reprompt-Not-Block.md), [CA-126](../../../change-audit/CA-126-fix-gate-block-ui-and-r-bug-action.md)
- Replaces: none
- Tags: `context-regression-engine, flow-gate, reprompt, r-bug, r-ca`

## AI Quick View

### Summary

- After BUG-139 made `r-bug` a `reprompt`, the gate started auto-reprompting the AI on a missing BugFix doc. But the reprompt prompt sent to the AI was `result.Message` — the terse symptom string (`"Flow gate: bug fix detected but no bugfix doc found"`).
- This is not actionable: the AI does not know **what** to do. Observed in E2E-11 — the AI tried to edit the `change-audit/CA-*.md` note instead of creating a `BUG-NNN.md` file, looping until the ≤2 attempt cap with no resolution.
- Fix: the reprompt now sends explicit, per-rule remediation steps that name the exact file to create (`requirements/09-BugFix/done/BUG-<NNN>.md` for `r-bug`, `change-audit/CA-<NNN>.md` for `r-ca`) and explicitly tells the AI NOT to edit the change-audit note to satisfy `r-bug`.

### Current Ask

- Replace the terse reprompt prompt with actionable, file-level remediation instructions, while keeping the inline desktop card text terse.

### Key Decisions

- `D-1` Add `flowgate.RepromptPrompt(EnforceResult)` that builds the AI-facing prompt from the reprompt-action violations; `gate_hook.go` sends this instead of `result.Message`.
- `D-2` The desktop inline ⚠ card keeps using `result.Message` (terse symptom) — only the AI-facing reprompt prompt gets the verbose guidance.
- `D-3` `RepromptPrompt` falls back to `result.Message` when no reprompt-action rule is present (defensive; the reprompt branch only runs when at least one is).

### Constraints

- Must not change the resolved action or severity ordering (`Enforce` is untouched aside from the added helper).
- Guidance is emitted only for `Action == "reprompt"` rules — block/warn rules never reach the reprompt branch.

### Open Questions

- Q-1: Should the remediation text be templated from the rule's `RequiredOutput` field rather than a hardcoded switch on `Trigger`? Deferred — the switch is clearer for v1's two reprompt rules.

### Source Refs

- Code: `apps/local-runner/internal/flowgate/enforce.go` (`RepromptPrompt`, `remediationFor`), `apps/local-runner/internal/runner/gate_hook.go` (reprompt branch).
- Tests: `apps/local-runner/internal/flowgate/flowgate_test.go` (`TestRepromptPrompt*`).

## 1. Issue Summary

When the flow gate resolves to `reprompt` (e.g. `r-bug` after BUG-139), the AI was reprompted with only the symptom message. Without instructions, the AI could not self-correct: in E2E-11 it edited the change-audit note instead of creating the required BugFix document, and exhausted its reprompt attempts without adding the file.

## 2. Parent Links

- impacted coding plan: CP-35 (E2E-11)
- impacted tech design: SD-20 §3 (reprompt UX), D-3
- impacted system spec: SS-14 (AC-11 force required outputs)

## 3. Environment and Reproduction

- environment: Desktop app + runner, enforce gate mode, flowpilot repo
- reproduction steps: Trigger r-bug (final message says "bug fix", no `BUG-` file added). Observe the reprompt turn: the AI edits the CA note / rephrases prose instead of creating a BugFix doc.
- frequency: 100% when r-bug (or r-ca) reprompts

## 4. Expected vs Actual

- expected: The reprompt names the exact file the AI must create and the AI adds `requirements/09-BugFix/done/BUG-<NNN>.md`, after which the gate passes.
- actual: The reprompt is a symptom string; the AI guesses, edits the wrong file, and the gate keeps firing until the attempt cap.

## 5. Impact

- users affected: Any user triggering an auto-remediable gate rule (r-bug, r-ca)
- workflows affected: Context & Regression Engine (CP-35)
- severity: High — the auto-remediation feature does not actually remediate without it

## 6. Root Cause

- hypothesis: The reprompt prompt was the UI message, not an instruction
- confirmed cause: `gate_hook.go` passed `result.Message` as the `Prompt` of the reprompt turn. `result.Message` is built by `Enforce` for the inline card (`"Flow gate: <detail>"`), which describes the problem, not the fix.
- evidence: E2E-11 screenshot — the AI repeatedly edits `change-audit/CA-2026-06-24.md` trying to neutralize the "bug fix" wording instead of creating a BUG document.

## 7. Fix Strategy

- `F-1` Add `RepromptPrompt(result EnforceResult) string` in `enforce.go`: iterate `result.Violations`, and for each `Action == "reprompt"` rule append a concrete instruction from `remediationFor`.
- `F-2` `remediationFor` maps `bug_fixed` → create `requirements/09-BugFix/done/BUG-<NNN>.md` per `FORMAT-REFERENCE-BUGFIX.md` (and "do NOT edit the change-audit note"); `code_changed` → create `change-audit/CA-<NNN>.md`; default → the raw detail.
- `F-3` `gate_hook.go` reprompt branch sends `flowgate.RepromptPrompt(result)` instead of `result.Message`. The emitted `flow_gate_violation` event keeps `result.Message` for the inline card.

## 8. Validation

- `V-1` `TestRepromptPromptIsActionable` — r-bug reprompt names the BUG file path, the format reference, and the "do NOT edit the change-audit note" guard. ✓
- `V-2` `TestRepromptPromptCoversBothMissingDocs` — when r-ca + r-bug both fire, the prompt names both files. ✓
- `V-3` `TestRepromptPromptFallsBackToMessage` — non-reprompt result falls back to `result.Message`. ✓
- `V-4` Manual E2E-11 after runner rebuild: AI creates the BUG doc and the gate passes. ⏳ (requires runner rebuild)

## 9. Regression Guard

- tests: `flowgate_test.go` `TestRepromptPrompt*` (3 cases).
- alerts: none
- audit checks: Confirm the inline desktop card still shows the terse `result.Message`, not the verbose prompt.

## 10. Follow-Up Document Updates

- CP-35 E2E-11: updated to describe the reprompt-with-actionable-steps behavior.
- SD-20 §3 reprompt row: behavior unchanged at the semantic level (still "reprompt"); the actionable-prompt detail is an implementation note recorded here.
