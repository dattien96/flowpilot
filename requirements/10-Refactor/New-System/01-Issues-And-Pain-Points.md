# 01 - Issues With Current Code And Pain Points

## Situation

FlowPilot currently has real product value in the runner:

- workflow definitions and controlled step progression
- prompt optimization from markdown process documents
- artifact persistence
- summary generation
- Supabase RAG indexing
- project and workflow history
- MCP proxy policy, especially Google Drive proxy enforcement
- FlowPilot-owned configuration for provider and YOLO behavior

The problem is not that FlowPilot has no value. The problem is that the current
interactive coding experience is weaker than native provider clients such as
Codex App, Codex CLI, Claude Code, and Gemini CLI.

## Current Integration Pain

The current Codex integration behaves like command execution around a provider
process. FlowPilot can pass an optimized prompt and inspect results, but the
integration boundary is still too coarse.

The concrete root cause: the chat path runs the provider as a **one-shot
subprocess** (`ExecutePrompt` -> `cmd.Run()`,
`apps/local-runner/internal/runner/runner.go:852`) — spawn, wait, exit. There is
no lifecycle, no streaming, and no point at which FlowPilot can intercept an
approval. Every symptom below follows from this single fact. The chosen fix
(Codex app-server) is detailed in `02`–`05`.

Pain points:

- hard to reliably separate final assistant answer from logs and command output
- hard to render file paths cleanly
- hard to show native-feeling command approval prompts
- hard to capture structured tool, file, and command events
- hard to support low-latency streaming chat
- hard to resume provider sessions with strong semantics
- hard to diagnose failures because information is split across process output,
  generated files, and FlowPilot state
- hard to build `/` command and skill-selection UX on top of raw process output

### What The Current Boundary Looks Like

The current shape is effectively:

```text
FlowPilot Runner
        |
        | spawn command with optimized prompt
        v
Provider CLI Process
        |
        | stdout / stderr / files / exit code
        v
FlowPilot post-processing
```

This boundary is enough for simple prompt execution, but it is weak for an
interactive workflow product. It gives FlowPilot output after or during the
process, but not a stable provider lifecycle model.

The runner needs to know:

- when the provider turn starts
- what assistant message is being streamed
- which tool or command is running
- which file changed
- when the provider is waiting for approval
- what approval decisions are available
- when the final answer is complete
- whether the turn failed, was interrupted, or completed

With raw command execution, FlowPilot must infer many of these facts. That makes
the UI brittle and makes workflow state harder to trust.

### Observable Symptoms

The user-facing symptoms are:

- the chat response feels less polished than native Codex App
- the UI cannot reliably show approval prompts at the right moment
- command output and assistant response can feel mixed together
- long file paths make the response noisy
- file changes are not always presented as clickable, structured objects
- workflow state feels separate from the actual coding interaction
- resuming a run can depend on fragile provider CLI behavior
- debugging a failed run requires looking in several places

### Product Risk If We Keep This Shape

If FlowPilot keeps the current command-first integration as the main path, it
will remain difficult to compete with native clients on interaction quality.

The likely outcome:

- users prefer native Codex App for daily coding
- FlowPilot becomes a configuration and sidecar tool only
- workflow compliance stays best-effort
- artifact/RAG persistence depends on post-hoc reconstruction
- approval and security UX remains confusing
- provider-specific parsing grows over time and becomes hard to maintain

## Web Chat Pain

The React web app is useful for admin and configuration, but it is not the best
surface for real-time coding interaction.

Note: the streaming, approval, and raw-output problems come from the
**integration boundary** (the one-shot subprocess described above), not from the
web surface itself — and they are fixed by the app-server boundary regardless of
which client renders the chat. Moving the coding UX to an IDE is a **UX
improvement** (file navigation, path rendering, approvals next to the code), not
the fix for those boundary problems. The two changes are independent.

Current web limitations:

- weaker file navigation than an IDE
- slower perceived interaction for streaming coding work
- less natural place for command approvals
- less natural place for workflow/step selection during coding
- path rendering is noisy because the UI often shows full workspace paths
- difficult to compete with native provider clients for chat polish

The web app should not be forced to become a full native coding client.

### What The Web App Should Still Do Well

The current web app still has a strong role. It is better suited for:

- project onboarding
- provider setup
- workflow editing and review
- MCP/proxy configuration
- approval policy configuration
- run history and audit
- artifact browsing
- administrative actions that do not need low-latency editor integration

The issue is not that React web should disappear. The issue is that the
real-time coding UX should move closer to the IDE.

## Workflow Pain

FlowPilot has many workflows and workflow steps. The user needs a fast way to:

- select a project
- select a workflow
- select a workflow step
- see the active run and active step
- send a prompt for the selected step
- approve or deny provider actions
- inspect generated files and artifacts
- continue or resume from the same context

This interaction belongs close to the coding workspace.

### Workflow UX Requirements

The coding client needs to make workflow state visible without forcing the user
to leave the editor.

Minimum requirements:

- show active project/workspace
- show selected workflow
- show selected workflow step
- show whether a run is new, active, waiting for approval, failed, or complete
- allow start/resume from the selected step
- allow advancing to the next step when the runner permits it
- show provider activity for the current step
- show artifacts and changed files from the current step
- keep workflow state synchronized with the runner

The client should never make workflow-state decisions on its own. It should ask
the runner to start, resume, advance, or complete workflow steps.

## Approval Pain

Dangerous command approval is a core trust issue.

With command-output parsing, FlowPilot cannot provide a clean approval model.
The system needs structured provider approval events so the UI can show:

- command or tool name
- working directory
- reason
- affected workflow run
- affected workflow step
- available decisions
- final approval result

FlowPilot also needs to persist these approval decisions for audit.

### Approval Requirements

Approval handling needs to be explicit and auditable.

The system should capture:

- provider key
- workflow run id
- workflow step run id
- provider session id
- provider turn id when available
- command or tool requested
- working directory
- provider reason
- available decisions
- selected decision
- approver
- timestamp
- resulting provider event

Approval UI should support at least:

- approve once
- deny
- cancel or stop turn

If the provider supports broader scopes, the UI can later add:

- approve for session
- approve for workspace
- approve similar commands

Those broader scopes must be controlled by runner policy, not by UI-only logic.

## Response Rendering Pain

The response should feel like a product UI, not terminal output.

Expected rendering:

- final assistant answer separated from logs
- command output collapsed by default
- file changes rendered as file rows
- full paths hidden behind readable file names
- full path still available by tooltip, detail panel, copy action, or link
- tool and MCP calls rendered as activity rows
- errors shown as actionable status, not raw mixed stderr

Raw subprocess output makes this unreliable.

### Rendering Requirements

The response renderer should treat provider output as structured data.

Expected render groups:

- assistant message
- final answer
- command execution
- MCP/tool activity
- file changes
- approval requests and decisions
- errors
- artifacts and summaries

File rendering should prefer:

- display name: `workflow-start-runtime.ts`
- relative path: `apps/admin-web/src/features/.../workflow-start-runtime.ts`
- absolute path hidden unless needed
- click action to open file
- copy action for full path

This requires the runner to emit structured file and event metadata, not only
plain text.

## Why Returning Fully To Native Codex App Is Tempting

Native Codex App already solves many UX problems:

- polished chat input
- native approvals
- better streaming experience
- command and file activity display
- lower interaction friction

However, returning fully to native Codex App would weaken FlowPilot's reliable
workflow control unless FlowPilot becomes only a proxy or sidecar.

That is the central tension:

- native app gives better UX
- FlowPilot controlled runner gives better automation guarantees

The new system must keep FlowPilot's automation guarantees while fixing the UX
pain.

## Required Product Outcome

The new system should make FlowPilot feel closer to a native coding client while
preserving runner-owned automation.

Success means:

- the user can work primarily from the IDE during coding
- FlowPilot still owns workflow progression
- optimized prompts are always sent before the provider sees the turn
- approvals are structured and auditable
- final responses are cleanly captured
- files and artifacts are easy to navigate
- Admin Web remains useful for configuration and audit
- native provider clients remain optional convenience, not the reliable path

## Non-Goals

This refactor should not try to:

- replace Codex App feature-for-feature
- make React Admin Web the full coding IDE
- force Claude or Gemini through Codex app-server
- claim full dangerous-command safety without sandbox and policy work
- remove Native Assist Mode
- remove existing workflow history or artifact features

