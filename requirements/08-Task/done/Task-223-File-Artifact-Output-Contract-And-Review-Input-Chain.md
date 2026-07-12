# Task-223: File Artifact Output Contract And Review Input Chain

## Metadata

- Document ID: `Task-223`
- Title: `File Artifact Output Contract And Review Input Chain`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-11`
- Last Updated: `2026-07-11`
- Parent Documents: [CP-45: Generic Artifact Types And Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md)
- Child Documents: `None`
- Related Documents: [Task-202: File Artifact Type Proof Of Generality](Task-202-File-Artifact-Type-Proof-Of-Generality.md), [Task-205: Builtin Artifact Flow](Task-205-Builtin-Artifact-Flow-Context-Coding-Review-Synthesis.md), [Task-222: Artifact-Only Step UX](Task-222-Artifact-Only-Step-UX-And-File-Artifact-Semantics-Copy.md)
- Replaces: Task-202 product semantics as "read-only inject only" — Task-202 remains the read/inject substrate proof
- Tags: `artifact, file-artifact, write-contract, flow-gate, review-chain, cp-45`

## AI Quick View

### Summary

- `file_artifact.v1` paths designate deliverables: **OUTPUT** = write contract (force create/update), **INPUT** = read contract (inject excerpts).
- Coder/agent.delegate prompts list required output paths; flow-gate rule `r-artifact-output` reprompts when paths are missing on disk after the turn.
- Reviewer (and other delegate) spawns receive INPUT file artifacts via generalized inject (not only context→implement hop).

### Key Decisions

- `T-1` Same instance can be OUTPUT on coder and INPUT on review (cross-step typed file I/O).
- `T-2` Enforce via existing flow-gate reprompt family (`r-artifact-output`), checking filesystem existence (not only `WrittenPaths`).
- `T-3` Task-202 read/inject of pre-existing files remains valid secondary INPUT use.

### Source Refs

- `implementation_plan.md` (Codex Phase-1 Task-223)
- `artifact_type_registry.go`, `gate_hook.go`, `flow_executor.go`, `flowgate/evaluate.go`

## 1. Goal

Ship the owner write→read chain: designate path → force coder write → review reads same file.

## 2. Definition of Done

- [x] `DOD-1` Required OUTPUT paths extracted from node bindings.
- [x] `DOD-2` Coder prompt includes Required file outputs section.
- [x] `DOD-3` `r-artifact-output` reprompts on missing paths.
- [x] `DOD-4` Reviewer advance injects INPUT file artifacts.
- [x] `DOD-5` UI copy describes OUTPUT write / INPUT read.
- [x] `DOD-6` Targeted unit tests pass.

## 3. Completion Notes

- result: `done` 2026-07-11 Phase-1 MVP.
- Residual: mtime/content-change proof (pre-existing file without rewrite), broader UI timeline for required paths, per-slot type metadata — Phase 2.
