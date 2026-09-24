# CA-960 — BUG-463: normalize absolute provider file-change paths to workspace-relative

- type: bugfix
- bug: BUG-463 (found live run-37268 sprint-2 — empty signature pin +
  absolute ReadOnlyPaths from devin's absolute EventFileChanged paths)
- feature: agent-flow-engine
- follows: CA-959 (BUG-462 evidence gate — this is the missing half that
  made the live contract carry no signatures)

## Change

`internal/runner/path_keys.go`:

- New `workspaceRelPath`/`workspaceRelPaths`: absolute paths under the
  workspace → workspace-relative slash form; out-of-workspace and
  already-relative paths pass through verbatim.

`internal/runner/interactive_service.go`:

- `finalizeInputLocked` normalizes `EventFileChanged` paths at ingestion —
  `ChangedFiles`/`WrittenPaths` are now workspace-relative for all
  consumers (gate, audit draft, durable records) and survive device switch.

`internal/runner/scaffold_gate.go`:

- `recordScaffoldArtifactsLock` relativizes `written` before the signature
  snapshot and `LockScaffoldArtifacts` call — covers carried `pendingPaths`
  from pre-fix records and any caller that bypasses the finalize path.

`internal/runner/vibe_sprint.go` + `internal/runner/flow_executor.go`
(CA-959 extension):

- Contract TDD evidence now also counts `ReadOnlyPaths` on the coder
  record: a post-freeze test lock (scaffold or reproduce gate) is the
  runner's own attestation even when signature extraction came back empty.
  Bare v1 freeze still fails closed.

## Tests (additive only)

- `TestBUG463_FinalizeRelativizesAbsoluteChangedFiles`
- `TestBUG463_ScaffoldLockRelativizesAndPinsSignatures`
- `TestBUG462_ReadOnlyLockWithoutSignaturesSatisfiesGate` — the exact
  sprint-2 live shape.

## Provider parity

The path-shape variance IS the provider difference: devin emitted absolute
paths for the sprint-2 scaffold turn, relative for sprint-1. Normalizing at
ingestion makes codex/grok/devin converge — verified by unit tests; no
provider-specific code changed.
