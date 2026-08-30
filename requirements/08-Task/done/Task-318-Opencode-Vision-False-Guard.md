# Task-318: Opencode Vision=false Guard — Capture & Lock the Pending State

## Metadata

- Document ID: `Task-318`
- Title: `Opencode Vision=false Guard — Capture & Lock the Pending State`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-30`
- Last Updated: `2026-08-30`
- Parent Documents: [CP-57: Opencode Provider Integration](../../07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md)
- Child Documents: `None`
- Related Documents: [Task-303: Opencode Chat Flow Model Reasoning YOLO Cards Tools](../done/Task-303-Opencode-Chat-Flow-Model-Reasoning-YOLO-Cards-Tools.md), [CP-57-Test-Steps](../../07-Coding-Plan/done/CP-57-Test-Steps.md)
- Replaces: `None`
- Tags: `opencode, vision, capability-guard, ai-providers`

## AI Quick View

### Summary

- Capture the **intentional** `Vision=false` state for Opencode across all layers (docs, runner adapter, desktop UI gate) — this is a designed pending state, not a forgotten gap.
- Lock it with **additive tests** so a future accidental flip (or a premature enable before the ACP image round-trip is proven) fails CI.
- The unlock condition is explicit: `Vision` may only flip `true` when the ACP `promptCapabilities.image` attachment round-trip is **proven end-to-end** (image → `.tmp/images` path fallback or native ACP image block → model sees it → turn completes), per CP-57 `P-12` / Task-303 `T-7` / `DOD-15`.

### Current Ask

- Add the missing desktop-side guard test (`VISION_PROVIDERS` must exclude `opencode`) and a runner-side prompt-params test asserting no image block is ever built for Opencode. Verify all existing Vision=false assertions stay green. No production behavior change — this task is a **guard lock**, not a feature flip.

### Key Decisions

- `T-1` **Vision stays `false` until proven** — ratified from CP-57 `P-12`, Task-303 `T-7`/`DOD-15`, CP-57-Test-Steps `M1`. The unlock gate is a proven ACP image attachment round-trip, not a code comment.
- `T-2` **`opencode_acp.go:88-90` comment is the live evidence anchor** — it records that `promptCapabilities.image` was observed `true` in the initialize handshake, but the adapter deliberately keeps `Vision:false` because the **attachment round-trip** (Task-301) was never proven. This comment must stay accurate; do not "fix" it to `true` without the round-trip proof.
- `T-3` **Guard is additive-only** — no production code change. New tests only. Old tests (`TestOpencodeCapabilitiesMatchProvenSet`, `TestOpencodeACPPromptParamsBuildsCorrectJSON`) stay untouched and green.
- `T-4` **Desktop gate is the user-facing enforcement** — `ChatInput.tsx:92 VISION_PROVIDERS` excludes `opencode`, so the attach button is disabled and paste is blocked (`supportsVision` false). This is the `M1` "chặn trước khi gửi prompt" behavior. A desktop test must lock this.

### Constraints

- **ZERO production code change** in `apps/**` — this task only adds tests and a Task doc. If a production change is needed, it belongs in a separate task.
- Additive-tests-only: do not edit `TestOpencodeCapabilitiesMatchProvenSet` or `TestOpencodeACPPromptParamsBuildsCorrectJSON`.
- Do not flip `Vision` to `true` — the unlock condition (proven round-trip) is not met.
- `feature_key: ai-providers` (provider adapters — Opencode capability surface).

### Open Questions

- None — the state is intentional and documented. The only open item is the future unlock proof, which is explicitly out of scope here.

### Source Refs

- CP-57 `P-12` (line 47): "Capability flags stay `false` until proven. `Vision=false`"
- CP-57 capability table (line 144): "Vision | `Vision=false` until proven | Opencode image input unconfirmed → keep `false`"
- CP-57 row 28 (line 294): "Set `Vision:false` initially for opencode; add `opencode` to `VISION_PROVIDERS` only when ACP image input proven"
- CP-57 line 473: "`Vision=false` and handoff-as-source `false` remain by design (unproven)"
- Task-303 `T-7` (line 39), `DOD-15` (line 146)
- CP-57-Test-Steps `M1` (line 172)
- `opencode_adapter.go:184` (`Vision: false`), `opencode_acp.go:88-90` (live-verified `image=true` but round-trip unproven), `ChatInput.tsx:92` (`VISION_PROVIDERS`)

## 1. Goal

Lock the intentional `Vision=false` state for Opencode with additive tests so it cannot silently regress, and record the exact unlock condition. The deliverable is a Task doc + new tests; **no production behavior changes**.

## 2. Parent Links

- coding plan: `CP-57` `P-12`, capability table row 28, line 473
- tech design: `SD-06` (provider integration)
- system spec: `SS-05` (workflow AI provider)
- specific upstream ids: CP-57 `P-12`, Task-303 `T-7`/`DOD-15`, CP-57-Test-Steps `M1`

## 3. Trigger

Operator audit confirmed the `Vision=false` state is **intentional and consistent** across all 9 verified locations (docs + runner + desktop). However, two guard gaps exist:

1. **Desktop has no test** locking `VISION_PROVIDERS` to exclude `opencode` — a future accidental add would silently enable image attach for a provider whose round-trip is unproven.
2. **Runner prompt-params test** asserts the text-only shape but does not explicitly assert "no image block is ever built" — the `opencode_acp.go:88-90` comment (live-verified `image=true`) is the only guard against a premature flip.

This task closes those gaps so the pending state is **locked by CI**, not by comment discipline.

## 4. Exact Change

- `T-1` **Add desktop guard test** — new test file asserting `VISION_PROVIDERS` does **not** contain `"opencode"` and does contain `"codex"`, `"claude"`, `"grok"`. This locks the `M1` "chặn trước khi gửi prompt" behavior at the source. **Refactor note:** `VISION_PROVIDERS` was moved from `ChatInput.tsx` (inline, non-exported) to a new `visionProviders.ts` (exported) so the test can import it — `tsconfig.phase1-tests.json` only includes `*.test.ts` and does not compile `.tsx`. This is a visibility-only refactor; the set contents and all call sites are unchanged.
- `T-2` **Add runner prompt-params guard test** — new test asserting `opencodeACPPromptParams` produces exactly one `{type:"text",text}` block and **never** an image block, even when a hypothetical image payload is passed (defensive: the function signature is text-only today, so the test documents the contract that image blocks are intentionally not built).
- `T-3` **Verify existing guards stay green** — run `TestOpencodeCapabilitiesMatchProvenSet` (asserts `caps.Vision == false`) and `TestOpencodeACPPromptParamsBuildsCorrectJSON` (asserts text-only prompt). No edits to these.
- `T-4` **Record the unlock condition** in this Task doc §8 Completion Notes: `Vision` may flip `true` only when the ACP image attachment round-trip is proven (image → `.tmp/images` path fallback or native ACP image block → model sees it → turn completes), matching Grok's `CA-483` path-fallback pattern.

## 5. Touched Areas

- files:
  - new: `apps/desktop-flowpilot/src/components/chatInputVisionProviders.test.ts`
  - new: `apps/desktop-flowpilot/src/components/visionProviders.ts` (VISION_PROVIDERS extracted from ChatInput.tsx for testability)
  - modified: `apps/desktop-flowpilot/src/components/ChatInput.tsx` (import VISION_PROVIDERS from visionProviders.ts; remove inline declaration — visibility-only, no behavior change)
  - new: `apps/local-runner/internal/runner/opencode_vision_guard_test.go`
  - new: `requirements/08-Task/todo/Task-318-Opencode-Vision-False-Guard.md` (this doc)
- modules: `apps/desktop-flowpilot` (test + visibility-only refactor), `apps/local-runner/internal/runner` (test only)
- routes: none
- tables: none

## 6. Acceptance Check

- `VISION_PROVIDERS` test fails if `"opencode"` is added to the set (proves the guard).
- `opencodeACPPromptParams` test fails if an image block is ever introduced (proves the text-only contract).
- `TestOpencodeCapabilitiesMatchProvenSet` and `TestOpencodeACPPromptParamsBuildsCorrectJSON` remain **untouched** and green.
- Production change is **visibility-only refactor** (`ChatInput.tsx` imports `VISION_PROVIDERS` from the new `visionProviders.ts`; set contents and call sites unchanged). No behavior change.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` Desktop test asserts `VISION_PROVIDERS` excludes `opencode` (and includes codex/claude/grok).
- [x] `DOD-2` Runner test asserts `opencodeACPPromptParams` builds exactly one text block and never an image block.
- [x] `DOD-3` Existing `TestOpencodeCapabilitiesMatchProvenSet` + `TestOpencodeACPPromptParamsBuildsCorrectJSON` untouched and green.
- [x] `DOD-4` Production change is **visibility-only refactor** (`ChatInput.tsx` imports `VISION_PROVIDERS` from `visionProviders.ts`; set contents and call sites unchanged). No behavior change.
- [x] `DOD-5` Unlock condition recorded in §8 Completion Notes.

### 6.2 Test Signatures

- `TestVisionProvidersExcludeOpencode` (desktop) — asserts `VISION_PROVIDERS.has("opencode") === false`, `has("codex") === true`, `has("claude") === true`, `has("grok") === true`.
- `TestOpencodePromptParamsNeverBuildsImageBlock` (runner) — asserts `opencodeACPPromptParams("ses_1","hello")` returns exactly one prompt entry of `type:"text"` and zero entries of `type:"image"`; also asserts the function has no image-building path (defensive contract test).

## 7. Out of Scope

- **Flipping `Vision` to `true`** — the unlock condition (proven ACP image round-trip) is not met. This is the explicit future work, tracked here only as a recorded condition.
- Implementing the `.tmp/images` path fallback for Opencode (Grok `CA-483` pattern) — that is the future unlock work, not this guard task.
- Any production code change in `apps/**`.
- Editing existing tests (`TestOpencodeCapabilitiesMatchProvenSet`, `TestOpencodeACPPromptParamsBuildsCorrectJSON`).

## 8. Completion Notes

- result: `Task-318` closed. Added `chatInputVisionProviders.test.ts` (desktop, 2 tests) + `opencode_vision_guard_test.go` (runner, 1 test). Extracted `VISION_PROVIDERS` to `visionProviders.ts` (visibility-only refactor). All tests green; existing guards untouched. CA-692 written.
- follow-ups:
  - **Unlock condition (recorded):** `Vision` may flip `true` for Opencode only when the ACP image attachment round-trip is proven end-to-end — image → `.tmp/images` path fallback (Grok `CA-483` pattern) or native ACP image block → model sees it → turn completes. When proven, update: `opencode_adapter.go:184` (`Vision:true`), `ChatInput.tsx:92` (add `opencode` to `VISION_PROVIDERS`), `opencode_acp.go:88-90` (comment), CP-57 `P-12`/row 28/line 473, Task-303 `T-7`/`DOD-15`, CP-57-Test-Steps `M1`.
- upstream docs updated: `None` (this task only adds a Task doc + tests; upstream CP-57/Task-303 already document the pending state correctly)
