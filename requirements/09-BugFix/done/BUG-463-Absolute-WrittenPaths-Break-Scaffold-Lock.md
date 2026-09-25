# BUG-463: Absolute provider WrittenPaths break scaffold signature pin and read-only lock storage

- status: done
- found: live run-37268 sprint-2 (vibe snake, Task-4)
- fixed_by: CA-960
- tests: internal/runner/bug463_abs_written_paths_test.go (2 tests)

## Symptom (live)

Sprint-2's scaffold-architect (run-41607, devin) wrote real stubs + tests,
the gate ran red as designed, and the artifact lock fired — but logged:

```
[gate] scaffold: locked [/Users/tiendat/fp-beds/full/snake/input_test.go
 ...] read-only + pinned signature hash  for coder step "coder" (contract v4)
```

Note the **empty signature hash** and the **absolute paths** in the lock
list. The v4 contract record stored `read_only_paths` absolute and carried
no `locked_signatures`/`signature_hash` at all — so the BUG-462 coder gate
found no contract evidence and parked again (`vibe_tdd_missing` 16:37:14).

## Root cause

Devin's `EventFileChanged` for this turn reported absolute paths (sprint-1's
scaffold turn had reported relative — provider output shape varies turn to
turn). Two downstream breaks:

1. `scaffoldSignatureSnapshot` does `filepath.Join(cwd, abs)` which yields
   `<cwd>/<abs>` — a path that never exists — so every read failed and the
   pinned hash/signatures came back empty. Silent: no log line, no error.
2. `recordScaffoldArtifactsLock` calls `LockScaffoldArtifacts` (store-level)
   directly, bypassing `LockScaffoldArtifactsForStep` — the wrapper that
   performs BUG-388's abs→rel normalization. Absolute paths were stored into
   `ReadOnlyPaths`, so the approval-bridge deny-list (which matches
   workspace-relative writes) would never have fired — the locked test
   files were not actually protected.

## Fix

- `path_keys.go`: new `workspaceRelPath`/`workspaceRelPaths` — relativize
  absolute paths under the workspace to slash form; out-of-workspace and
  already-relative paths pass through unchanged.
- `interactive_service.go` `finalizeInputLocked`: normalize
  `EventFileChanged` paths at ingestion so `ChangedFiles`/`WrittenPaths`
  are workspace-relative for every downstream consumer (gate, audit,
  durable records — which must survive device switch anyway).
- `scaffold_gate.go` `recordScaffoldArtifactsLock`: relativize `written`
  defensively before snapshot+lock, covering carried `pendingPaths` from
  pre-fix records.

## Regression coverage

- `TestBUG463_FinalizeRelativizesAbsoluteChangedFiles` — abs-under-ws →
  rel; already-relative unchanged; abs-outside-ws preserved verbatim.
- `TestBUG463_ScaffoldLockRelativizesAndPinsSignatures` — abs `written`
  still pins LockedSignatures + stores relative ReadOnlyPaths.
