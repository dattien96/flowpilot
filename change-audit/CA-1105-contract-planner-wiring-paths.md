# CA-1105 — contract-planner prompt: enumerate build/test wiring in declared_paths

## What
`flow-pack/agents/contract-planner.md` now explicitly instructs the planner
to include the non-source files a change needs to build and verify —
build/wiring files (`build.gradle.kts`, `CMakeLists.txt`, `pom.xml`), new or
updated test files, registration/manifest edits — not just implementation
files.

## Why
Live run-38799 (PrivateVault CP-02): the preflight contract for Task-021
declared only `dod_shredder.h`/`.cpp`. The coder's legitimate work —
`core/vault-core/build.gradle.kts`, `CMakeLists.txt`, the host test file —
landed outside the frozen contract and the scope-drift gate flagged it,
forcing a manual `/amend` mid-flight. The prompt said "name every file" but
implied source-only; wiring/test files are the usual blind spot.

## Guarantees kept
Prompt-only change to pack data (go:embed). No runtime behavior change; the
frozen-contract enforcement and amend path are untouched. Non-file scopes
still go through `/amend`, not silent widening.
