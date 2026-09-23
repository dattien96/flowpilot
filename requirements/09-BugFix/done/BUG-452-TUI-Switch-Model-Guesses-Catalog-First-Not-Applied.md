---
id: BUG-452
title: TUI provider switch shows first catalog model instead of applied seed model
status: done
version: 1
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-430, CA-926]
---

## AI Quick View
- **What**: A provider switch without explicit model can show a plausible but incorrect model in the footer.
- **Why**: The switch response echoes an empty request model and TUI replaces it with catalog element zero, unrelated to the session's applied model.
- **Key constraint**: Show the actual resolved/applied model, or an honest unknown state, never infer it from catalog order.

## 1. Metadata
- Document ID: `BUG-452`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `cli-tui`
- Parent Documents: [BUG-430](../done/BUG-430-TUI-Polish-Continue-Softlock-Stale-Foreign-Model-Paste-Token.md), [CA-926](../../../change-audit/CA-926-tui-post-switch-stream-flow-catalog-and-composer-fixes.md)

## 2. Symptom and Impact
`switchChatProvider` passes `req.Model` unchanged to createRun and also returns it verbatim in `chatSwitchResponse.Model` (`internal/runner/chat_switch.go:275-291,326`). A new chat run with blank model keeps `resolvedModel=""` (`interactive_handlers.go:786-787`) and the seed `TurnRequest` inherits that empty value (`interactive_service.go:7768-7777`); e.g. Devin's `devinModelForTurn` then omits the model set and the ACP session uses its own `currentValue` (`devin_adapter.go:630-667,836-839`). But on empty response, `applyChatSwitched` sets TUI `m.model = modelsForProvider(...)[0]` (`internal/tui/app/chat_switch.go:322-335`). Catalog ordering is not the ACP session's currentValue, so CA-926/BUG-430(b) can replace a stale *foreign* model with a false *same-provider* model. Existing `bug430_model_paste_test.go:19-41` seeds a catalog whose first element is intentionally the expected model, masking this case. Severity: **medium** for UI metadata truthfulness and subsequent per-turn model overrides.

## 3. Reproduction / Evidence
Create a provider catalog with first model A, session/new config `currentValue=B`, switch without explicit model, observe TUI shows A while seed runs B. This is a deterministic path through the code; **no fresh live session or red assertion run during this review**. The earlier live test footer showed a Devin model but did not check that it matched the applied ACP model.

## 4. Acceptance and Verification
Return/display the actual resolved/applied seed model (or unknown until observed); avoid blindly using catalog index zero. Add additive API/TUI E2E matrix covering empty response, model order differing from ACP currentValue, provider-session rejection, explicit target model and reconnection, then verify live with eligible providers. Preserve existing test semantics.

## 5. Resolution (2026-09-23, CA-933)

- Runner `chat_switch.go` returns the resolved leg model
  (`newLeg.modelName`, fallback `defaultModelForProvider`) instead of echoing
  the request; TUI `chat_switch.go` prefers `TargetModel`/resolved model
  before catalog order — empty response no longer displays `catalog[0]`.
- Tests: `TestBug452_SwitchResponseReturnsResolvedLegModel` (runner),
  `TestBug452_EmptyResponsePrefersRequestedOverCatalogOrder` (tui/app) —
  green.
