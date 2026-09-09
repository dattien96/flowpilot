# CA-799 — chat-kind vibe reconstruct restores flow steps

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Reconstruct of RunKind=chat with ActiveFlowNodes restores the flow timeline; sprint overlay writes chatFlowRef
# --->8---

## Why

Live `/open run-220036`: statusline `vibe-cp-ingest`, sidebar `(no steps)`, transcript only the original prompt + "Opened flow …". Vibe is chat-mode + flow. Reconstruct seeded synthetic `chat-<run>` because `RunKind==chat`, wiping tdd/coder rows. Sprint start did not update `chatFlowRef`, so chrome stayed on the ingest overlay.

## Change

- `reconstructRunInternal`: if `activeFlowNodes` exist, rebuild flow steps (same as workflow). Synthetic `chat-*` only when there are no flow nodes.
- `startResolvedFlowFromNode` writes `chatFlowRef` to the overlayed flow.
- `inferPackFlowRefFromNodes` on reconstruct: sprint nodes win over a stale persisted `vibe-cp-ingest` ref (live run-220036 chrome).

## Tests

- chat + nodes → tdd/coder, no `chat-*` (Claude/Codex/Grok)
- chat + empty nodes → synthetic `chat-*` (plain chat; Claude/Codex/Grok)
- sprint overlay writes `chatFlowRef` (Codex `newTestServer`; Claude/Grok `createRun` controlled runtime not implemented — startResolvedFlow itself has no provider branch)
- reconstruct with sprint nodes + stale `vibe-cp-ingest` ref → `vibe-sprint` (Claude/Codex/Grok)
- infer: sprint / cp-ingest / ingest / debate / empty fallback

Old BUG-178 reconstruct tests untouched.

## Providers

Agnostic Case 1: step-seed branch and `chatFlowRef=` take no `providerKey` and do not switch on Claude/Codex/Grok. Tests still table all three.

## Will not undo

CA-798 coder resume. CA-791 ingest join. BUG-170 workflow reconstruct. BUG-178 step timeline. Plain-chat synthetic `chat-*` seed.
