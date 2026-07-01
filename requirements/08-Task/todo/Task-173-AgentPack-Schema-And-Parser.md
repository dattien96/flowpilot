# Task-173: AgentPack Schema And Parser

## Metadata

- Document ID: `Task-173`
- Title: `AgentPack Schema And Parser`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-01`
- Last Updated: `2026-07-01`
- Parent Documents: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- Child Documents: `Task-174`, `Task-175`, `Task-176`
- Related Documents: `CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration`, `CP-41-RAG-Harness-Flow-Mode`
- Replaces: `N/A`
- Tags: `agent-flow-engine, agentpack, generic-flow, local-runner`

## AI Quick View

### Summary

- Introduce a typed `agentpack` schema layer for the internal flow pack created by CP-42.
- Load and validate `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` plus referenced flow, agent, tool, prompt, and context files.
- This task is intentionally parser-only: it must not change runtime behavior yet.

### Current Ask

- Build the Go parser, validator, and tests that make the pack data trustworthy before any runner integration.

### Key Decisions

- `T-1` Pack data is configuration, not enforcement. Go still owns execution guarantees.
- `T-2` Missing references, duplicate IDs, invalid `selectableIn`, invalid `chatBaseline`, and malformed flow definitions must fail fast.
- `T-3` The parser must preserve unknown future fields where practical so pack schema can evolve.

### Constraints

- Do not route runtime execution through the parser in this task.
- Do not remove existing hardcoded review-loop or CP-41 behavior yet.
- Keep schema names aligned with `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml`.

### Open Questions

- Final schema version compatibility policy can remain `v1 only` for this task.
- Whether pack files are embedded with `go:embed` or read from disk can be decided in Task-174, but parser APIs should support both.

### Source Refs

- `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml`
- `apps/local-runner/internal/agentpack/flow-pack/README.md`

## 1. Goal

Create a reusable Go parser and validator for FlowPilot agent packs so later tasks can load built-in agents, flows, tool faces, context contracts, and prompt templates without hardcoding step names or behavior in runner files.

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- tech design: `requirements/06-System-Tech-Design/done/SD-19-Agent-Orchestration-Runtime.md`
- system spec: `requirements/05-System-Specs/done/SS-16-Agent-Orchestration.md`
- specific upstream ids: `CP-36`, `CP-41`, `CP-42`

## 3. Trigger

CP-36 and CP-41 define generic node/edge/policy primitives, but the current implementation still carries hardcoded flow-specific behavior. Before removing hardcodes, the runner needs a reliable typed source of built-in pack definitions.

## 4. Exact Change

- `T-1` Create `apps/local-runner/internal/agentpack` Go package.
- `T-2` Add typed structs for manifest, flow metadata, agent refs, tool refs, context refs, prompt refs, and compatibility fields.
- `T-3` Add flow YAML structs for nodes, edges, policies, declared faces, behavior refs, context refs, prompt template refs, and built-in mirror metadata.
- `T-4` Add parser API:
  - `LoadManifestFS(fs.FS, root string) (Manifest, error)`
  - `LoadFlowFS(fs.FS, path string) (FlowPackDefinition, error)`
  - `ValidatePack(fs.FS, Manifest) error`
- `T-5` Validate that all manifest file references exist.
- `T-6` Validate that all flow IDs are unique inside the pack.
- `T-7` Validate built-in metadata:
  - `editable=false` for internal built-in flows.
  - `chatBaseline=true` flow cannot be selectable in Chat picker unless explicitly allowed.
  - `selectableIn` only accepts `chat` or `flow`.
- `T-8` Validate node and edge references:
  - every edge `from` and `to` must reference a declared node or allowed terminal pseudo-node.
  - every behavior/tool/context/prompt ref must resolve to a manifest entry.
- `T-9` Add unit tests with the current example pack and table-driven invalid cases.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/**/*.go`
  - `apps/local-runner/internal/agentpack/**/*_test.go`
  - `apps/local-runner/internal/agentpack/flow-pack/**/*`
- modules:
  - local runner internal package only
- routes:
  - none
- tables:
  - none

## 6. Acceptance Check

- Parser loads the CP-42 example pack without errors.
- Invalid manifest reference fails with actionable error.
- Duplicate flow ID fails.
- Edge to unknown node fails.
- Invalid `selectableIn` fails.
- `go test ./apps/local-runner/internal/agentpack/...` passes.
- No runtime behavior changes in Chat Mode or Flow Mode.

## 7. Out of Scope

- Supabase mirror sync.
- Agent catalog integration.
- UI changes.
- Removing `isPlanStepType`, `isCodingStepType`, or review-loop hardcodes.

## 8. Completion Notes

- result: implemented
- follow-ups: `Task-174`, `Task-175`, `Task-176`
- upstream docs updated: [CP-42](../../../07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) progress notes and [CA-147](../../../change-audit/CA-147-agent-flow-pack-and-generic-node-behavior-refactor.md)
