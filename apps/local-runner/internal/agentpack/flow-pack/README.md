# FlowPilot Agent Pack Examples

This directory is a reference data shape for CP-42. It is not wired into the
runner yet.

The intent is to move built-in flow/template knowledge out of Go literals while
keeping runtime guarantees in Go handlers.

## Layout

- `manifest.yaml`: pack entrypoint and version metadata.
- `agents/`: markdown agent definitions compatible with the current
  `AgentDefinition` frontmatter parser.
- `flows/`: flow topology and node behavior declarations.
- `tools/`: declared tool faces that map domain-facing tool input to generic
  `flow_control`.
- `contexts/`: typed context artifact declarations.
- `prompts/`: render-only prompt templates.

## Rule

Markdown guides the model. Go enforces state.

Built-in flows are read-only templates. The app may mirror them into the
definition store for execution and allow users to clone them, but direct edits
belong in user-owned flow definitions.

Chat Mode applies the RAG/context harness as a baseline. Optional orchestration
templates, such as Review Loop, are selected separately from a built-in picker.
