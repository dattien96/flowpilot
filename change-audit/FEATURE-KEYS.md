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
- kill-review — SP-05 Kill-Review closed-claim contract, portable kill-review skill pack, and KR-* review reports
- cli-tui — terminal Bubble Tea client (`flowpilot chat`): thin `/client/*` UI for normal chat + flow/step, slash controls, statusline, agent focus (CP-56)
- vibe-mode — Vibe working mode: SS ingest from raw requirement, TDD-first per-sprint loop, r-requirement user-only gate + 2-owner debate cohort (cap 5) (SS-18/SD-24)
- runtime-intelligence — context budget packer, wrong-way drift detection, mistake-to-skill learning (CP-23)

- zcode-parity — CP-62 harness hardening from ZCode lessons: gate precedence, verdict schema, structured escalation card, node isolation posture, context profiles, sprint handoff, conventions source

- sprint-handoff — CP-62 P-6 inter-sprint handoff artifact (sprint_handoff.v1) written from verified state at the boundary, injected into the next sprint's entry prompt, enriched with card choices + weakened tests (Task-342/346)
- node-isolation — CP-62 P-4 per-node posture enforcement (read_only/verdict_only) with gated provider modes so the bridge matrix is reachable end-to-end (Task-340/349)
- skill-catalog — CP-62 P-5 catalog tier: skills enter prompts as name+description+path pointers (never full bodies), provider harness triggers natively / agents Read on demand (Task-347)
- decision-card-ui — CP-62 P-3 card rendering: user_decision_card_requested event rendered as an interactive escalation card in desktop app + TUI, option id answered through the chat prompt channel (Task-345)
- drift-pause — CP-23 ladder pause leg: dev-mode drift ≥80 parks the run (BlockReason `drift`) + additive `drift_pause_required` event, resume via the parked-run continue channel — no dedicated confirm backend; vibe keeps owner-debate routing (Task-348)
- release-automation — GoReleaser, Homebrew tap, binary distribution, and release automation
- lsp-runtime — CP-63 IDE-grade LSP runtime: JSON-RPC client, server lifecycle, platform registry, diagnostics hook, context source, Kotlin/Gradle fallback
- reproduce-first-gate — CP-64 Reproduce-first TDD gate (r-reproduce): enforce executable red test before code mutation in bug and refactor flows
- tournament-harness — CP-65 Multi-candidate tournament harness & PDR escalation fallback for hard bugs and stalled debates
- living-knowledge-base — CP-66 Living knowledge base & execution flow context source (knowledge.flow) distilled from GitNexus and LSP
- contract-first-tdd — CP-67 Contract-First Scaffold TDD, AST Signature Lock (r-signature-lock), and batch renegotiation via Main Agent
- skill-anchored-init — CP-68 Skill-Anchored Project Scaffolding & AI-Guided Init Engine
- run-worktree — SS-23/SD-27/CP-71 opt-in per-run git worktree isolation: start toggle, run binding, patch-based merge-back card, conflict evidence, recovery & GC
- attention-queue — cross-run pending-action inbox: desktop Attention Queue aggregating waiting runs via a singleton observer store (Task-404)
- desktop-ui-consistency — desktop design-token revamp: spacing/elevation/radius/motion scales, icon unification, token guardrails (Task-405)
