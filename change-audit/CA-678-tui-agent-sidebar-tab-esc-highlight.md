# CA-678 — TUI: Tab only for buttons; agent sidebar highlight + /agents + Esc; stable list

## Context

User confirmed Tab loop should only cycle buttons (action ring / picker / posture),
not agent views. The sidebar step rows still rendered clickable chips `[open]` /
`[back]` despite mouse being off; switching agents should be keyboard-only via
`/agents`. Agent view must offer Esc back to main, and the transient `/agents N
active` spam on every `agent_graph_updated` should be removed. The `/agents`
pick list flickered because `orderAgentsMainFirst` kept poll arrival order.

## Decision

- **Tab** no longer cycles `focusedAgentIdx` / `cmdFocusAgent`. Tab → picker
  (slash/file/skill/agent) → action ring (`handleActionRingKey` Right) →
  posture. Shift-Tab → picker backward → action ring Left. `cycleFocusedAgent`
  stays but is no longer reachable via Tab; `/agents <name>` is the only agent
  switch.
- **Sidebar step rows**: remove `styleStepAgentAction("[open]")` and the
  `[back]` header chip. The focused child row now renders `▸` (ascii `>`) +
  `styleStatusAgent` (teal `#2dd4bf`) so the selected agent is obvious without a
  clickable chip. `stepsSectionTitle` is a plain `styleGate("steps")`.
- **Esc** while `viewingChild()` calls `restoreMainTranscript()` (no click
  needed). Read-only banner changed to `/agent main or Esc`; `/agents` dump
  changed to `/agents <name> to open transcript · Esc returns to main`.
- **Spam**: removed `if m.agentsFocus { addMessage("[agents] N active") }` in the
  `agent_graph_updated` handler.
- **Flicker**: `orderAgentsMainFirst` sorts the non-main rest STABLY by
  `AgentName` (case-insensitive) then `RunID`. Both the picker and the dump now
  keep a deterministic order across hydrate reorders.

## Consequences

- `handleKey` Tab/Shift-Tab, `handleActionRingKey`, `flowStepsPanelLinesMax`,
  `stepsSectionTitle`, ` Esc` handler, `/agents` banner, `orderAgentsMainFirst`.
- Six test files updated to assert the new UX (no `[open]`/`[back]`,
  teal marker instead, Tab does not cycle agent, Esc returns main). New
  regression file `tui_agent_sidebar_new_ux_test.go` covers Tab no-cycle / picker
  / highlight / no-chips / Esc / banner / stable order.
- BUG-328 contract preserved (mouse tracking off); Tab/Esc are keyboard-only.

## Alternatives Considered

- Keeping Tab → cycle agent via index: rejected — user wants Tab for buttons only.
- Keeping `[open]` as a keyboard `o` on F2: retained for `f2_step_picker.go`
  (F2 `o` still calls `cmdFocusAgent`); only the rendered chip token is removed.

## Verification

- `go build ./internal/tui/app` — OK.
- `go test ./internal/tui/app -count=1` — `ok   flowpilot-runner/internal/tui/app  7.026s`.
- New tests: `TestTab_DoesNotCycleAgentView_NewUX`, `TestTab_PickerAgentsStillWorks`,
  `TestSidebar_HighlightsSelectedAgent`, `TestSidebar_NoOpenBackChips`,
  `TestEsc_FromAgentView_ReturnsMain`, `TestAgentView_BannerMentionsEsc`,
  `TestAgentsDump_MentionsEsc`, `TestAgentsList_NoFlickerAcrossHydrateReorder`.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-311
change_type: refactor
summary: Tab cycles buttons only (not agents); agent sidebar teal highlight with ▸ marker, no [open]/[back] chips; /agents + Esc for agent switch; stable agents list
# --->8---
