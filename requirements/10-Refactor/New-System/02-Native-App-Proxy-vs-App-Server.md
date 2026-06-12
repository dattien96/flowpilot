# 02 - Native App Plus Proxy vs Controlled App-Server

## Decision Context

There are two realistic ways to improve the current experience.

1. Keep the native provider client as the main chat UI and expose FlowPilot as
   MCP tools, hooks, or proxy services.
2. Build FlowPilot Controlled Workflow Mode using provider adapters, with Codex
   app-server as the Codex adapter.

Both can coexist, but they do not provide the same guarantees.

## Decision Criteria

The comparison should be judged against FlowPilot's core requirements:

- prompt optimization must happen before the provider sees the turn
- workflow run and step state must be authoritative
- provider events must be captured for run logs
- final response must be persisted
- artifacts and summaries must be generated after turns
- Supabase RAG indexing must be reliable
- MCP proxy and approval policy must remain FlowPilot-owned
- the coding UX must support fast interaction, file navigation, and approvals

Native clients score well on interaction quality. Controlled app-server mode
scores better on automation guarantees.

## Option A: Native App (Codex App Client) + FlowPilot Proxy as MCP/hook

In this mode the user keeps chatting in the native provider client.

```text
Codex App / Claude Code / Gemini CLI
        |
        | MCP tools + hooks where supported
        v
FlowPilot Runner Proxy
```

FlowPilot can expose tools through MCP:

- fetch workflow context
- fetch optimized prompts
- save artifacts
- call Google Drive proxy tools
- trigger summaries or sync when hooks are available

### Benefits

- best native provider UX
- low-friction adoption
- user keeps existing provider habits
- less UI work for FlowPilot
- useful for convenience and assistive workflows

### Limitations

- FlowPilot cannot reliably force the model to call a tool before final answer
- FlowPilot may not see the exact prompt before the provider sees it
- FlowPilot may not see the exact final response
- workflow step enforcement is weaker because the provider client owns the turn
- dangerous command approvals remain provider-owned
- artifact and RAG persistence become best-effort unless hooks are strong
- failure diagnosis is split between native client and FlowPilot

Native App Plus Proxy is a good convenience mode. It is not the reliable
automation path.

### Best Use Cases

Native App Plus Proxy is still useful for:

- users who prefer native provider clients
- quick exploratory tasks
- low-risk assistive workflows
- asking the model to fetch FlowPilot context through MCP
- saving selected artifacts through FlowPilot tools
- gradual adoption before the controlled client is ready

### Failure Modes

This mode becomes risky when FlowPilot needs guarantees:

- the model ignores the FlowPilot MCP tool
- the model answers before saving artifacts
- the provider client truncates or hides useful event details
- hooks fail or are unavailable
- final answer capture is incomplete
- provider approval happens outside FlowPilot audit
- workflow step state diverges from the actual conversation

Those risks are acceptable for convenience mode, not for the core automation
path.

## Option B: FlowPilot Controlled Mode With Codex App-Server

In this mode FlowPilot owns the workflow turn and calls provider adapters.

```text
Interactive Client UX
        |
        v
FlowPilot Runner Runtime
        |
        v
Provider Runtime Gateway
        |
        +-- Codex app-server adapter
        +-- Claude adapter
        +-- Gemini adapter
```

For Codex, FlowPilot starts or connects to:

```bash
codex app-server --listen stdio://
```

Then the Codex adapter talks to app-server through a structured protocol instead
of treating Codex as a one-shot subprocess.

### Benefits

- guaranteed prompt optimization before Codex sees the prompt
- structured streaming events
- exact final response capture
- tool, file, command, and approval events can be mapped to FlowPilot events
- stronger resume and retry semantics
- stronger workflow step boundaries
- better run log and failure diagnosis
- reliable artifact and RAG finalization after turn completion
- enables product-quality rendering in a desktop or IDE client

### Limitations

- FlowPilot must own or build the client UX
- native Codex App is not the primary interface in Controlled Mode
- provider parity depends on adapter quality
- app-server improves approval UX, but security still requires sandbox,
  provider approval settings, and FlowPilot policy

### Best Use Cases

Controlled Mode is the right path for:

- workflow execution where every step matters
- optimized prompts that must be sent exactly
- artifact and RAG persistence
- approval-gated command execution
- run logs and audits
- repeatable process execution
- multi-provider abstraction
- future IDE extension UX

### Failure Modes To Design For

Controlled Mode also has risks:

- Codex app-server process crashes
- JSON-RPC stream disconnects
- provider approval request is not answered
- runner and extension lose connection
- finalizer fails after provider turn completes
- provider session id is lost
- user opens the same run from multiple clients

The runner must treat these as first-class states, not as generic command
failures.

## Current Command Call vs App-Server

The difference is not "command" versus "no command." Both can start Codex from a
command. The difference is the integration boundary.

| Capability           | Current Code = Command Call      | Codex App-Server Adapter - We will use this one |
| ----------------------| ----------------------------------| -------------------------------------------------|
| Process model        | One-shot subprocess              | Long-lived runtime/session                      |
| Integration boundary | stdout, stderr, exit code, files | structured protocol events                      |
| Prompt optimization  | possible at invocation           | guaranteed through turn start                   |
| Streaming            | parse provider output            | consume structured notifications                |
| Final response       | fragile if mixed with logs       | part of turn lifecycle                          |
| File/tool events     | best-effort parsing or diff      | direct event mapping                            |
| Dangerous approval   | hard to model cleanly            | structured approval request                     |
| Resume               | CLI/external-state dependent     | store provider thread/session ids               |
| UI rendering         | terminal-like output             | product components                              |

## Detailed Capability Comparison

| Capability | Native App Plus Proxy | Current Command Call | Controlled App-Server - We will use this one |
|---|---|---|---|
| Native chat polish | strongest | weak | depends on FlowPilot client |
| FlowPilot prompt ownership | weak | medium | strong |
| Workflow step ownership | weak | medium | strong |
| Final response capture | weak to medium | medium | strong |
| Tool event capture | provider/hook dependent | weak | strong |
| File event capture | provider/hook dependent | post-run diff | strong if mapped |
| Dangerous approval UX | provider-owned | weak | strong provider event path |
| FlowPilot security policy | only for FlowPilot tools | partial | strong where runner owns gate |
| Artifact/RAG finalization | best-effort | post-process | turn lifecycle based |
| Resume | native-provider owned | fragile | runner stores provider ids |
| Failure diagnosis | split | split | centralized event timeline |
| Multi-provider path | per-client setup | per-command adapter | provider gateway |
| Implementation cost | low to medium | already exists | medium to high |
| Product fit | sidecar | compatibility path | orchestration runtime |

## Mode Positioning

The product should name the modes clearly:

- **Native Assist Mode**: provider client remains the main UI; FlowPilot exposes
  MCP tools and hooks where possible.
- **Controlled Workflow Mode**: FlowPilot owns the turn lifecycle through
  provider adapters.

This avoids overpromising. Users can choose convenience or guarantees.

## Recommendation

Use both modes with explicit positioning:

- Native App Plus Proxy: convenience and assist mode
- Controlled App-Server Mode: reliable automation path

Codex app-server should be the first strong provider runtime adapter because it
directly solves the current pain around structured events, approvals, session
control, and rendering.

## Migration Decision

Do not delete the current command path immediately.

Recommended migration:

1. keep current command execution as a compatibility adapter
2. implement Provider Runtime Gateway interfaces
3. add Codex app-server adapter
4. move controlled runs to app-server
5. keep Native Assist Mode as optional
6. retire command execution only after app-server covers required workflows

This reduces risk while moving the product toward the stronger architecture.
