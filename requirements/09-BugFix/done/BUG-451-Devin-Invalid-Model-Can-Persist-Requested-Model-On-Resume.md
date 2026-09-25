---
id: BUG-451
title: Devin resumed session can persist rejected requested model when currentValue is missing
status: done
version: 1
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-433, CA-916b]
---

## AI Quick View
- **What**: Model metadata can still claim a requested ID that Devin rejected on session/load.
- **Why**: A session/load without model `currentValue` leaves appliedModel unknown, yet recordSession writes requested model before config result.
- **Key constraint**: When actual model cannot be verified, never claim the rejected requested model ran.

## 1. Metadata
- Document ID: `BUG-451`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `agent-flow-engine`
- Parent Documents: [BUG-433](../done/BUG-433-Devin-Invalid-Model-Silent-Fallback-Metadata-Lies.md), [CA-916b](../../../change-audit/CA-916-Devin-ACP-Provider-Fixes-And-Durable-Reprompt-Rearm.md)

## 2. Symptom and Impact
`ensureSession` handles a config-only `session/load` response (including absent sessionId), but only seeds `appliedModel[sessionID]` if model `currentValue` is present (`internal/runner/devin_adapter.go:630-668`). It then calls `recordSession`, which writes `ModelName:req.ModelName` (`:689-705`). On invalid `session/set_config_option`, `applyDevinSessionConfig` emits a warning and leaves the applied model unchanged (`:718-739`). `recordAppliedSessionModel` returns without an upsert when `appliedModelFor` is empty (`:807-830`). Because `appliedModel` is cleared at the end of each turn (`:350-358`), a resumed load with no currentValue + rejected model continues with misleading requested-model metadata, the behavior BUG-433 claimed fixed. Severity: **medium**, provider identity/history false even with a visible warning.

## 3. Reproduction / Evidence
Stub `session/load` to return no model currentValue; reject the subsequent model set; inspect persisted `ProviderSessionRecord.ModelName` after the turn. Existing invalid-model test stubs session responses with `currentValue:"swe-2-high"` (`bug379_devin_model_fallback_test.go:15-34`) and does not cover missing-value resume. **Static branch evidence, no fresh Devin live verification performed.**

## 4. Acceptance and Verification
Add additive red test for resumed config-only load and rejected model, plus accepted model, restart/history and no-model-known cases; treat actual model as unknown/typed degradation rather than persist the rejected request. Check provider path classification (Devin-only) and retain Codex/Claude/Grok behavior.

## 5. Resolution (2026-09-23, CA-934b)

- `devin_adapter.go` tracks ACP-observed applied model separately from the
  request, and rejected requests explicitly. When no applied model is
  observed, the session record persists a typed unknown marker — a rejected
  request is never recorded as applied truth.
- Tests: `bug451_452_model_truth_test.go` — fake ACP peer rejecting
  `swe-2-max` → record carries the unknown marker (red before); accepted-model
  path persists the real value.
- Live note: `devin acp` handshake timed out on this machine; the rejection
  wire shape is pinned from cp46/cp70 captures and replayed by the fake peer.
