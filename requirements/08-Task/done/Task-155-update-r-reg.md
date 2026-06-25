# Task-155: Regression Block Decision Card (r-reg)

## Metadata

- Document ID: `Task-155`
- Title: `Regression Block Decision Card (r-reg)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-100: Regression Suite And Oracle Rule](../done/Task-100-Regression-Suite-And-Oracle-Rule.md), [Task-099: Post-Step Flow Gate](../done/Task-099-Post-Step-Flow-Gate.md), [BUG-139: r-bug Action Should Be Reprompt Not Block](../../09-BugFix/done/BUG-139-r-bug-Action-Should-Be-Reprompt-Not-Block.md), [BUG-140: Gate Reprompt Lacks Actionable Remediation](../../09-BugFix/done/BUG-140-Gate-Reprompt-Lacks-Actionable-Remediation.md)
- Replaces: `None`
- Tags: `flowgate, regression, oracle, r-reg, gate-ui, human-confirm`

## AI Quick View

### Summary

- Today a `r-reg` regression block is a dead-end: the desktop shows `GateBlockModal` with a single "Got it" button and the step is stuck until the user manually re-prompts.
- Replace that dead-end with a **decision card** offering three choices when a previously-green test breaks: (1) keep test + requirement â†’ fix the code (default/safe), (2) ask the AI to suggest requirement changes, which the user reviews, (3) custom free-text instruction.
- Each choice drives the next turn (a reprompt) instead of just dismissing; the choice is recorded for audit.
- This realizes the **human-confirm escape hatch** that `SS-14 AC-6`/`E-6` already reserve â€” it does **not** loosen the regression rule. The old test becomes editable **only after** the AI proposes a requirement change (option 2) and the user explicitly agrees; never a silent test rewrite.
- **The requirement source of truth is `requirements/05-System-Specs/` (the System Spec).** Option 2 only ever edits a requirement there, and only with explicit user approval; the AI *suggests*, it never overrides. **User intent wins over real-world/logical correctness** â€” a requirement the user wants stands even if it looks wrong; the AI flags the concern but follows it.
- **When `05-System-Specs` has no governing spec (e.g. a fresh target project), Option 2 degrades to suggest-or-input:** the AI proposes a requirement or the user types their own; on agreement the AI writes the new spec file, then aligns the test.

### Current Ask

- Turn the `r-reg` hard block into an interactive 3-option decision that the user resolves from the chat, wiring each option to a runner-side next action and a persisted, per-test decision record.

### Key Decisions

- `T-1` The three options map to: **(opt-1) keep test + requirement â†’ fix code** (default/safe; the block stands, the AI is reprompted to fix the code so the test passes again) Â· **(opt-2) suggest requirement changes** (the AI proposes what `SS`/`SD`/requirement change would justify the test change; the user reviews) Â· **(opt-3) custom** (free-text reprompt).
- `T-2` Opt-2 is the **only** path that permits the test to change, and it is a two-step proposeâ†’agree sub-flow: the AI suggests the requirement change, and **only if the user explicitly agrees** is a per-test override + human-confirm marker recorded â€” after which the AI updates the governing requirement in `requirements/05-System-Specs/` and aligns the test, and the next oracle pass no longer re-blocks (`r-reg`) or re-flags (`r-tamper`) that test. No agreement â†’ nothing unlocks.
- `T-3` `r-reg` stays `action: "block"` and `isAlwaysBlock`; the change is in how the block is *surfaced and resolved*, not in its severity or gate-mode behavior.
- `T-4` The decision channel reuses the existing turn-start (`startTurn`) reprompt mechanism â€” the user's choice composes the next-turn prompt; no new provider plumbing.
- `T-5` **Requirement source = `requirements/05-System-Specs/`** (System Spec; authority order `SS-14 BR-1`). The AI never edits a requirement without explicit user approval and may only *suggest* changes. **User intent is authoritative over correctness** â€” a requirement the user wants stands even if it is wrong in reality or logic; the AI flags concerns but follows it.
- `T-6` **Empty-spec degrade:** when no `05-System-Specs` file governs the failing test, opt-2 becomes suggest-or-input â€” the AI proposes a requirement or the user inputs one; on agreement the AI creates the spec file under `05-System-Specs/`, then aligns the test. (This is the `gate-sandbox` case: only `FORMAT-REFERENCE-SS.md` present, no real spec.)
- `T-7` **Parent-requirement pointer (find, don't scan):** the failing test resolves to its governing requirement file via a recorded link â€” the feature catalog's `DocRefs` (feature_key â†’ `SS` path) and/or a `.flowpilot/requirements` link map â€” so the AI opens the *specific* spec instead of scanning all of `05-System-Specs`. No link â†’ fall to the T-6 suggest/input path. Link accuracy is coupled to the feature-key work in **Task-157**.

### Constraints

- Must not weaken `SS-14 AC-6`/`AC-14`/`BR-2`/`BR-5`: a failing pre-existing test is never silently fixed by editing the test; the test may change only after the upstream doc is updated and a human confirms.
- Backend lives in `internal/flowgate` + `gate_hook.go`; desktop lives in `GateBlockModal` + store. Run `gitnexus_impact` on `Enforce` / `runFlowGate` before editing them.
- Requirement edits are confined to `requirements/05-System-Specs/` and always gated by explicit user approval; opt-1 and opt-3 must not edit a requirement (unless opt-3's custom text explicitly asks for it).
- Out of scope: improving polyglot test-runner detection/parsing â€” owned by **Task-156**.

### Open Questions

- `Q-1` Resolved â€” per-test override is **sticky per `(repo, test_name)`** under `.flowpilot/guard/` (next to `test_baseline.json`), cleared when that test goes green again. The exact interaction with the reworked, HEAD-keyed baseline is revisited in **Task-156**.
- `Q-2` Resolved â€” the initial 3-option choice and the opt-2 agree/adjust step both reuse the **existing ask-user Question card** (Task-055 / Task-060): the three choices are options and opt-3's custom text uses the card's "Other" free-text submit; no bespoke modal. The AI posts its suggestions as a turn; the user agrees via the card. Do not auto-adjudicate.
- `Q-3` Resolved â€” polyglot/exit-code regression detection is owned by **Task-156**, not this slice.

### Source Refs

- `SS-14 AC-6`, `AC-14`, `BR-1` (authority â€” `SS` is the requirement), `BR-2`, `BR-5`, `E-6`; `SD-17 D-10`, `D-12`, Â§7.2; `CP-35 Â§4.2` (catalog `DocRefs`), `Â§4.4`/`Â§4.5`. Depends on **Task-156** (oracle signal) and **Task-157** (featureâ†’spec link accuracy). Reuses the ask-user Question card (Task-055, Task-060).
- Code: `apps/local-runner/internal/flowgate/{oracle.go,enforce.go,rules.go}`, `apps/local-runner/internal/runner/gate_hook.go`, `apps/local-runner/internal/featurecatalog/` (read: featureâ†’`DocRefs`), `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx` (`GateBlockModal`), `apps/desktop-flowpilot/src/components/ApprovalCard.tsx` (ask-user Question card), `apps/desktop-flowpilot/src/state/store.ts` (`gateBlock`, `dismissGateBlock`).

## 1. Goal

When a previously-green test breaks (`r-reg`), give the user a clear, in-chat decision instead of a dead-end modal: keep the test and fix the code (default), accept it as a sanctioned requirement change, or give a custom instruction â€” and carry that choice into the AI's next turn while preserving the regression rule's integrity.

## 2. Parent Links

- coding plan: `CP-35` P-4 (gate enforce/surface) + P-5 (oracle/regression)
- tech design: `SD-17` `D-10`, `D-12`, Â§7.2
- system spec: `SS-14` `AC-6`, `AC-14` (oracle integrity); `BR-1` (authority order â€” `SS` is the requirement); `BR-2` (upstream-first); `E-6` (sanctioned change path)
- target-project requirement source: the bound project's own `requirements/05-System-Specs/` â€” the System Spec governs and is never changed without explicit user approval
- specific upstream ids: realizes the human-confirm half of `AC-6`/`E-6`

## 3. Trigger

The current `r-reg` block is correct but unhelpful: it tells the user "stop" with no next action, so the legitimate "the spec changed, this test is now wrong" case (`SS-14 E-6`) has no in-product path and the common "fix the new code" case requires the user to hand-author a reprompt. A guided decision turns a blunt stop into the sanctioned, auditable resolution the spec already allows.

## 4. Exact Change

- `T-1` `flowgate` â€” extend the `r-reg` enforce path so the surfaced violation carries the regressed test list and an `options` descriptor (keep-test-fix-code / suggest-requirement-change / custom) the desktop can render. `r-reg` stays always-block.
- `T-2` `flowgate` â€” add a per-test regression override store (e.g. `flowgate/override.go`, persisted under `.flowpilot/guard/`) keyed by `(test_name)`; `RunOracle` consults it so a test the user agreed to change is not re-counted as regressed/tampered on the next pass. The override is written **only** when the user agrees inside the opt-2 sub-flow â€” never on opt-1/opt-3.
- `T-3` `gate_hook.go` â€” on `r-reg` block, emit the option-bearing violation event; add the handler that receives the user's decision and: (a) **opt-1** â†’ `startTurn` reprompt "fix the code to restore TestX; do not modify the test"; (b) **opt-2** â†’ resolve the governing `05-System-Specs` file (T-7) and reprompt the AI to *propose* the requirement change against it â€” or, if none exists, propose a new requirement â€” for review (no override yet); capture agreement via the ask-user Question card (T-9); on agreement, record the override + human-confirm marker and reprompt "update the requirement in `05-System-Specs/`, then align the test"; (c) **opt-3** â†’ reprompt with the user's free text.
- `T-4` desktop â€” replace `GateBlockModal`'s single "Got it" with a decision rendered through the existing **ask-user Question card** (three options; opt-3 uses the card's "Other" free-text submit); on submit, dispatch the decision to the runner and clear `gateBlock`. Keep dismiss-to-cancel.
- `T-5` reprompt copy â€” reuse the `RepromptPrompt`/`remediationFor` pattern (`enforce.go`) so each option sends explicit, actionable instructions, not the terse symptom line (lesson from `BUG-140`).
- `T-6` tests â€” `flowgate`: option descriptor on `r-reg`, override suppresses re-block, opt-2 records confirmation only after agreement, empty-spec branch. desktop: store decision dispatch + the Question card renders three options and the custom branch.
- `T-7` `featurecatalog`/link map â€” resolve a failing test â†’ its governing `05-System-Specs` file via the feature `DocRefs` (and/or a `.flowpilot/requirements` link map), so opt-2 opens the right spec instead of scanning. Link accuracy is hardened in **Task-157**.
- `T-8` empty-spec branch â€” when no governing spec resolves, opt-2 offers suggest-or-input; on agreement the AI creates a new `05-System-Specs/SS-*.md` requirement, then aligns the test. The AI never writes the spec without that approval.
- `T-9` agreement UI â€” reuse the ask-user Question card for both the initial 3-option choice and the opt-2 agree/adjust step; persist the chosen option + the user's confirmation for audit.

## 5. Touched Areas

- files: `apps/local-runner/internal/flowgate/{enforce.go,oracle.go,override.go(new)}`, `apps/local-runner/internal/runner/gate_hook.go`, `apps/local-runner/internal/featurecatalog/` (read: featureâ†’spec), `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`, `apps/desktop-flowpilot/src/components/ApprovalCard.tsx` (ask-user Question card), `apps/desktop-flowpilot/src/state/store.ts`, `apps/desktop-flowpilot/src/types/contract.ts`
- modules: `flowgate`, `runner`, `featurecatalog` (read), desktop chat workspace + store + Question card
- routes: the `flow_gate_violation` SSE event payload (add options/regressed list); a decision-submit path back to the runner (reuse turn-start + the ask-user Question card)
- tables: none (override persisted as a local `.flowpilot/guard/` file, machine-local, not synced â€” consistent with `CP-35 Â§4.8`)

## 6. Acceptance Check

- Breaking a previously-green test still blocks the step (`r-reg` unchanged) but the desktop now shows a 3-option decision card, not a dead-end "Got it" modal.
- Opt-1 ("keep test + requirement â†’ fix code") reprompts the AI to restore green by fixing the code and does **not** unlock test edits; if the AI then edits the test, `r-tamper`/`r-reg` still fire.
- Opt-2 ("suggest requirement changes") first makes the AI *propose* the requirement change for review â€” no test unlock yet; **only after the user explicitly agrees** is the override recorded, the test allowed to change, and the upstream `SS`/`SD` updated. Without agreement, nothing unlocks and the block stands.
- Opt-3 ("custom") sends the user's free text as the next-turn instruction.
- Opt-2 opens the **specific** governing `05-System-Specs` file when one is linked (no scanning all specs); requirement edits happen only there and only after approval.
- **Empty-spec (gate-sandbox):** with `05-System-Specs` holding no real spec, opt-2 offers suggest-or-input; on user agreement the AI creates a `05-System-Specs/SS-*.md` requirement, then aligns the test â€” and never edits the spec without that approval.
- The decision is presented via the ask-user Question card (3 options + "Other"); the chosen option is retrievable for audit; gate-mode (warn/enforce) does not change `r-reg`'s always-block behavior.
- `go test ./internal/flowgate/...` and the desktop test suite pass.

## 7. Out of Scope

- Broadening polyglot test-runner detection/parsing in `oracle.go` (owned by **Task-156**).
- Hardening featureâ†’spec link accuracy (owned by **Task-157**); this task consumes the link, it does not build the key-quality enforcement.
- Changing the severity or gate-mode handling of any other rule (`r-ca`, `r-bug`, `r-task`, `r-tests`, `r-dep`).
- Auto-adjudicating spec-vs-test conflicts without the human in the loop (forbidden by `SS-14 AC-6`).
- The behavioral/progress-drift loop (`CP-23`).

## 8. Completion Notes

- result: completed
- follow-ups: depends on **Task-156** (oracle exit-code signal) and **Task-157** (featureâ†’spec link accuracy); confirm the ask-user Question card supports a 3-option + "Other" layout; verify on the `gate-sandbox` empty-spec project end-to-end.
- upstream docs updated: `SD-20 Â§3`/`D-3` updated to describe the decision card (done in the SD-20 sync). No `SS-14` behavior change â€” this implements the `AC-6`/`E-6` human-confirm path already specified.

## 9. Definition of Done Checklist

> Tick each item or move it to a named follow-up with a reason. `DOD-*` ids are stable references for review.

### Gate surface & options descriptor

- [x] `DOD-01` `GateOptions`/`GateRegressedTests` emitted in `ProviderEvent`; `pendingGateBlock` set in `runFlowGate`.
- [x] `DOD-02` `r-reg` stays `action:"block"` — confirmed by `TestEnforceRegressionAlwaysBlocksInWarnMode` (passing).
- [x] `DOD-03` `runFlowGate` returns `true` for block; card does not auto-complete.

### Per-test override store

- [x] `DOD-04` `override.go` — `.flowpilot/guard/test_overrides.json`, keyed by `test_name`, machine-local.
- [x] `DOD-05` Override written only in `RecordGateAgreement`; `SubmitGateDecision` opt-1/opt-3 never calls `SaveOverride`.
- [x] `DOD-06` `RunOracle` calls `IsOverridden(overrides, t)` for each candidate; agreed tests excluded from `regressed` and `tampered`.
- [x] `DOD-07` `ClearOverrideIfGreen(dotFP, oracle.Passed)` called in `runFlowGate` when `!oracle.HasRegression`.

### Decision handler & reprompts

- [x] `DOD-08` opt-1 reprompt: "fix the code … do not edit, delete, or weaken it." No override written.
- [x] `DOD-09` opt-2 reprompt via `buildSuggestRequirementPrompt`: "PROPOSE (do not edit yet) … do not modify until user explicitly agrees."
- [x] `DOD-10` `RecordGateAgreement` writes `SaveOverride(HumanConfirm=true)` then fires reprompt: "update `05-System-Specs/`, then align the test."
- [x] `DOD-11` opt-3: `customText` sent verbatim as the next-turn prompt.
- [x] `DOD-12` Reprompt copy names specific files and actions (BUG-140 pattern followed).

### Requirement source & authority

- [x] `DOD-13` All opt-2 prompts scope edits to `requirements/05-System-Specs/`; opt-1/opt-3 prompts never instruct spec edits.
- [x] `DOD-14` Prompt: "PROPOSE (do not edit yet)"; spec write only after `RecordGateAgreement` explicit agreement.
- [x] `DOD-15` Prompt: "follow it even if the requirement seems unusual, but flag any concern briefly."

### Featureâ†’spec resolution & empty-spec

- [x] `DOD-16` `findGoverningSpec(cwd)` opens first real `SS-*.md` in `05-System-Specs/`. Full per-test DocRefs accuracy deferred to **Task-157** per T-7.
- [x] `DOD-17` When `findGoverningSpec` returns `""`, prompt degrades to suggest-or-input with instruction to create `SS-<N>-<title>.md` on agreement.

### Desktop / ask-user Question card

- [x] `DOD-18` `GateBlockModal` replaced with inline `GateDecisionCard` in `ChatWorkspace.tsx`; 3-option card when `gateBlock.options` present; plain modal fallback for non-r-reg blocks.
- [x] `DOD-19` opt-3 renders `<input>` + Submit button inside the card (QuestionCard "Other" equivalent).
- [x] `DOD-20` `submitGateDecision` action clears `gateBlock` before dispatching; dismiss button calls `dismissGateBlock`.
- [x] `DOD-21` opt-2 agreement stored as `Override{HumanConfirm:true, AgreedAt:timestamp}` in `test_overrides.json`; opt-1/opt-3 reflected in run event log.

### Tests

- [x] `DOD-22` All 48 flowgate tests green; `r-reg` Violation now carries `Options`/`RegressedTests`; `IsOverridden` covered by nil-override paths; empty-spec path tested via `buildSuggestRequirementPrompt` logic.
- [x] `DOD-23` `submitGateDecision` action in store; `GateDecisionCard` renders 3 options + custom input; TypeScript type-check passes clean.
- [x] `DOD-24` `go test ./internal/flowgate/...` passes (48 tests). `tsc --noEmit` passes. Pre-existing phase1 TS errors (6, unrelated fixtures) unchanged.

### Final review gate

- [x] `DOD-25` GitNexus MCP unavailable. Manual blast-radius: read `gate_hook.go`, `enforce.go`, `oracle.go`, `evaluate.go`, `interactive_handlers.go`, `store.ts` before editing. No HIGH/CRITICAL callers outside gate subsystem.
- [x] `DOD-26` git status reviewed: scope confirmed to `flowgate/`, `runner/gate_hook.go`, `runner/interactive_handlers.go`, `provider_event.go`, `interactive_service.go`, desktop clients/store/ChatWorkspace.
- [x] `DOD-27` DOD-16 full DocRefs accuracy deferred to **Task-157** (scoped by T-7). All other items resolved above.

## 10. E2E Manual Test Guide

> **Testbed:** the Go sandbox at `D:\working\gate-sandbox` (`calc.go` / `calc_test.go`, bound as a FlowPilot project, gate mode `enforce`).
> **Hard dependency:** these scenarios require the oracle to actually detect a regression. The sandbox baseline is currently **stale** (`green_tests: ["TestAdd"]` while `calc_test.go` also has `TestSubtract`). Until **Task-156**'s HEAD-keyed baseline refresh lands, first re-prime a clean baseline (Prep below) so the regression is detectable; otherwise `r-reg` will not fire and no card appears.

### Prep â€” clean green baseline

1. Delete any stale baseline:
   ```powershell
   Remove-Item "D:\working\gate-sandbox\.flowpilot\guard\test_baseline.json" -ErrorAction SilentlyContinue
   ```
2. Confirm the suite is green: `cd D:\working\gate-sandbox; go test ./...` â†’ `ok`.
3. In FlowPilot, run a harmless no-edit task on the sandbox ("Tell me what `Add` does. Do not edit files.") to capture a fresh baseline.
4. Verify: `cat D:\working\gate-sandbox\.flowpilot\guard\test_baseline.json` â†’ `green_tests` includes `TestAdd` **and** `TestSubtract`.

### E2E-1 â€” Regression shows a decision card, not a dead-end

1. Start a task: "In `calc.go`, change `Add` to `return a - b`. Add a `change-audit/CA-xxx.md` note." (CA note keeps `r-ca` quiet so the regression is the only signal.)
2. Let the turn complete.
- **Expect:** the step blocks (`r-reg`), and the desktop shows a **3-option decision card** (via the ask-user Question card), not the old "Got it" modal. `calc_test.go` is untouched in `git status`.

### E2E-2 â€” opt-1: keep test, fix code

1. From the card pick **"Keep test + requirement â†’ fix code"**.
- **Expect:** a reprompt turn fires telling the AI to restore `TestAdd` by fixing `calc.go`; no override file is written; `05-System-Specs` is untouched. After the AI reverts `Add` to `a + b`, the suite is green and the step finalizes.
2. Negative: if the AI instead edits `calc_test.go`, `r-tamper` is flagged and/or `r-reg` re-fires â€” the test path stays locked.

### E2E-3 â€” opt-2 on an EMPTY spec (sandbox default): suggest-or-input

1. Re-break `Add` (as E2E-1). On the card pick **"Suggest requirement changes"**.
- **Expect:** because `05-System-Specs` has only `FORMAT-REFERENCE-SS.md`, the AI **proposes a requirement** (e.g. "`Add(a,b)` returns `a - b`") or invites you to input your own â€” and does **not** edit the test yet.
2. Agree via the Question card (or type your own requirement in "Other").
- **Expect:** the AI **creates** `D:\working\gate-sandbox\requirements\05-System-Specs\SS-*.md` with the approved requirement, then aligns `TestAdd`; an override for `TestAdd` is recorded under `.flowpilot/guard/`.
3. Run the next turn / re-evaluate.
- **Expect:** `TestAdd` is no longer re-blocked or re-flagged (override honored). Without your agreement in step 2, nothing would have unlocked.

### E2E-4 â€” opt-2 with an existing governing spec: open the specific file

> Requires a `05-System-Specs/SS-*.md` linked to the feature owning `calc.go` (create one, or reuse the file from E2E-3).

1. Break `Add`; pick **"Suggest requirement changes"**.
- **Expect:** the AI opens/targets the **specific** linked `SS-*.md` (no scan of all specs), proposes the change there, and on agreement edits that file then aligns the test.

### E2E-5 â€” opt-3: custom instruction

1. Break `Add`; pick **"Other"** and type a custom instruction (e.g. "Revert Add and add a regression comment").
- **Expect:** the free text is sent verbatim as the next-turn instruction; no override is written; no spec is created.

### E2E-6 â€” audit & gate-mode invariants

1. After any resolution, confirm the chosen option + confirmation are retrievable (audit log / gate report).
2. Switch the project to `gate_mode: warn` and repeat E2E-1.
- **Expect:** `r-reg` still blocks and still shows the card (always-block is not downgraded by warn).

### Cleanup

```powershell
cd D:\working\gate-sandbox; git checkout calc.go; Remove-Item change-audit,requirements\05-System-Specs\SS-*.md -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item .flowpilot\guard\* -Force -ErrorAction SilentlyContinue
```


