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

## Keys (seed — extend as features are catalogued)

- chat-ui — desktop chat workspace, input, history, slash commands
- agent-spawn — child agent spawning and orchestration
- workflow-runtime — workflow run engine, steps, sessions
- ai-providers — provider adapters (Claude, Codex) and account config
- google-drive — Drive connection, artifact and chat sync
- supabase-config — Supabase connection and runtime config
- context-regression-engine — SD-17 context + regression engine (Plane C, gate, resolver)
- project-nav — desktop project navigator sidebar: project selector, history panel, remote chats
- cross-provider-handoff — cross-provider chat handoff: provider-neutral transcript transfer + summary-based hybrid
