---
id: CA-934b
title: Devin adapter — rejected model request not persisted as applied (BUG-451)
type: BugFix
feature: ai-providers
date: 2026-09-23
status: done
---

## Context

BUG-451: on resume, the Devin adapter requested a model via ACP
(`session/update` configOption). When ACP rejected it (e.g.
`Invalid value 'swe-2-max' for config option 'model'`) the adapter still
persisted the *requested* model into the session record as if it had been
applied — model truth diverged from what the provider actually ran, and the
wire log was the only place the rejection was visible.

## Change

`internal/runner/devin_adapter.go` (+334 lines incl. tests' supporting code):

- Track the model value actually accepted/observed from ACP separately from
  the requested value; track rejected requests.
- If no applied model was observed (rejected or never echoed), the session
  record persists a typed `unknown` marker (`devin/…`) rather than claiming
  the rejected request — restart/resume no longer treats a refused model as
  active.
- When ACP does confirm an applied model, that value remains authoritative.

## Tests (added only)

- `internal/runner/bug451_452_model_truth_test.go` — BUG-451 tests drive a
  fake ACP peer that rejects the requested model and assert the persisted
  record carries the typed-unknown marker, not `devin/swe-2-max` (red before:
  `record persisted the REJECTED requested model`); accepted-model path still
  persists the real applied value.

## Result

- `go test -count=1 -run 'Bug451|Bug452'` — green.
- Live note: `devin acp` handshake timed out on this machine during the wave;
  the rejection wire shape is pinned from the cp46/cp70 captures and the fake
  peer replays it exactly. Change is Devin-adapter-scoped; other providers
  unaffected.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-451
change_type: bugfix
summary: Devin resume no longer persists a rejected model request as applied — typed unknown marker recorded when ACP confirms no applied model
# --->8---
