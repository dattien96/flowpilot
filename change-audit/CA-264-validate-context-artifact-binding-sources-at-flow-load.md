# CA-264: Validate Context-Artifact Binding Sources At Flow Load

## Scope

Fixed BUG-269, found while manually re-running CP-44 E2E case 11.3 against the live desktop app: a `context_artifact.v1` artifact binding's `config_json.sources` was never validated against the `ContextSourceRegistry` at flow-load time, so an unknown id inside it silently degraded to a buried runtime warning instead of failing the flow-load fast — bypassing the exact "never silently run without it" guarantee CP-44/Task-194 already enforces for the two older context-source tiers.

## Changes

- `context_sources_builtin.go`: `ValidateFlowArtifactBindings` now also resolves every `context_artifact.v1` binding's `config_json.sources` entries against `DefaultContextSourceRegistry()`, failing with a clear error naming the flow, node, instance, and bad id on the first unregistered entry. The existing required/missing-instance check (SD-23 F-1) is untouched.
- `context_source_step_precedence_test.go`: +3 tests — rejects an unknown id inside a context_artifact binding, accepts a binding whose sources are all known, and confirms a file_artifact binding's unrelated `paths` config is never misread as a source list.

## Verification

- `go build ./...`, `go vet ./...` — clean.
- `go test ./internal/runner/ -run TestValidateFlowArtifactBindings` — all 6 tests (3 existing + 3 new) pass.
- `go test ./internal/...` — no new failures beyond the pre-existing, already-confirmed-unrelated environment-dependent flakes (Codex/Claude CLI resume, skills-merge, live provider tests).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-269
change_type: bugfix
summary: fail flow-load fast when a context_artifact.v1 binding's config_json.sources names an unregistered context source, instead of silently degrading to a runtime warning
# --->8---
