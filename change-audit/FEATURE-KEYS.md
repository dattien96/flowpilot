# Feature Key Registry

Source of truth for stable `feature_key` values used by:

- the change-audit `flowpilot:change-ledger` block (`SS-13 §13`)
- the commit-history ledger (`SD-17 §3.2`, Task-096)
- the Feature Catalog, which seeds from this file (`SD-17 §3.3`, Task-097)

## Rules

- Keys are kebab-case, one per line: `key — short description`.
- When writing a change-audit note (or commit), pick an existing key. If none fits, append a new line here first, then use it.
- Keep keys stable: do not rename silently. To retire a key, mark it `superseded by <new-key>` rather than deleting.
- One feature per key; a note that spans features uses its dominant key.

## Keys (catalogued from the change-audit history — extend as new features land)

- chat-ui — desktop chat workspace: input bar, slash commands, markdown rendering, skill-picker UI, mode borders, send/stop control
- chat-history — chat persistence, resume, replay, delete, and restore across restarts (incl. Codex rollout files)
- project-nav — desktop project navigator: project selector, run-history popover, registry detail parity, sync-progress cues
- agent-spawn — child agent spawning, agent panel, lifecycle, and parent/child cascade
- workflow-runtime — workflow run engine: steps, sessions, runs UI, orchestrator bridge, provider-event persistence
- artifacts — artifact system: generation, sync, catalog, preview/viewer
- ai-providers — provider adapters (Claude, Codex, Gemini): auth/refresh, model aliases, usage-limit handling, account config
- mcp-tools — MCP servers/tools: permission & approval flow, ask_user, elicitation, project MCP context
- skill-injection — skill content injection, skill packs, prompt-composition order
- yolo-policy — YOLO auto-approve configuration and runtime policy (workflow-level + single-step)
- google-drive — Google Drive connection, artifact/chat sync, and restore
- supabase-config — Supabase connection, RLS policies, runtime config, schema constraints
- terminal-session — terminal/process lifecycle, dev-stack controls, thinking-stream rendering
- context-regression-engine — SD-17 context + regression engine (Plane C): gate, resolver, requirements scaffold, flow rules
- cross-provider-handoff — cross-provider chat handoff: provider-neutral transcript transfer + summary-based hybrid
- token-usage
- agent-flow-engine — generic flow vocabulary (FlowNode/FlowEdge/FlowPolicy), review-loop runtime (cap, cohort barrier, auto-reinvoke), submit_review_outcome tool, synthesizer builtin, orchestration board (CP-36)
- change-contract — change contract, scope-drift detection, canonical head, and intent signature (CP-43)

