# CA-656 — clone UUIDs defer chat-orchestration validation to family gate

## Scope
Go-only, one call site. Starting a vibe-rooted clone via a direct
`flowRef` turn failed with 400 `invalid_flow_ref` (`flowRef %q is not a
valid built-in orchestration option`) at `handleStartTurn` admission:
`SkipChatOrchestrationCheck` is id-based, so a clone UUID fell into
`validateChatOrchestrationSelection`, whose option list only contains
canonical built-in refs.

## Fix
- `vibe_clone_gate.go`: new `vibeCloneRootPackID` (GetByRef ancestry walk
  with the CA-1078 first-hop guard — mirrors never inherit).
- `interactive_handlers.go` turn admission: when the ref is non-empty, not
  skipped as a pack builtin, and has no clone root, behavior is
  byte-identical (validate as before — the hidden review-loop 400 pin
  still holds). A mirror-rooted clone UUID skips the chat-option check
  and defers to the family gate below (CA-652/653), which authorizes it.
  Store reads happen only on that path; empty flowRefs cost nothing.

## Tests
- `vibe_clone_gate_test.go` (+`TestVibeCloneRootPackID`): RED (undefined)
  → GREEN.
- Orchestration/gate/CA-1078 sweep green, incl. the review-loop 400 pin
  and both laundering pins.
- `detect-changes`: LOW (handleStartTurn only, 0 processes).
