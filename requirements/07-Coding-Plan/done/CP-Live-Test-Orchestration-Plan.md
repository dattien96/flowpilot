# CP-Live-Test-Orchestration-Plan — Parallel Live Verification Of Important CPs

## Metadata

- Document ID: `CP-LIVE-TEST-ORCHESTRATION-PLAN`
- Title: `CP Live Test Orchestration Plan — HTTP/Log + Computer-Use Verification`
- Phase: `07-Coding-Plan` (verification plan, not a feature CP)
- Status: `draft` (awaiting operator approval before any execution)
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-10-13`
- Last Updated: `2026-10-13`
- Parent Documents: `CP-Test-Progress-Tracking.md` (this directory)
- Child Documents: per-agent `RESULT.md` fragments under `~/fp-beds/lt-evidence/cp<NN>/` (generated at execution time)
- Related Documents: all `CP-XX-Test-Steps.md` listed in §2
- Replaces: `none`
- Tags: `live-test, verification, orchestration, sub-agent, worktree`

## AI Quick View

### Summary

Re-verify ~27 important CPs on branch `cp_live_test` HEAD with **real live runs** — real runner processes, real provider turns, real HTTP API, real logs — plus a serialized computer-use pass for UI-only scenarios. Each CP gets its own git worktree, its own runner port, its own test bed, and its own dedicated sub-agent. Results merge back into `CP-Test-Progress-Tracking.md` by the orchestrator only.

### Current Ask

Approve this plan (or amend scope/order) before any runner is started or sub-agent is spawned. Confirm the two open decisions in §9 (sub-agent model path; Devin auth pre-requisite).

### Key Decisions

- One worktree + one runner port (`19200 + CP number`) + one bed + one unique `projectId` per CP → zero port collisions, zero `dispatch.lock` contention.
- HTTP/log scenarios are the primary evidence; UI-only scenarios go to a single serialized computer-use wave.
- Sub-agents **test and record only** — no production edits, no test edits (safe-fix contract). Found bugs are written into the result fragment, never fixed in the test pass.
- Turn provider defaults to `devin` (`swe-2-max` if the ACP catalog exposes it, else `swe-2-high`); grok/opencode-targeted scenarios use `grok-4.5-low` per operator rule.

### Constraints

- `gate-sandbox` (`~/Desktop/BE/gate-sandbox`) is **EPERM-blocked** on this Mac (macOS TCC). All beds live under `~/fp-beds/lt-cp<NN>/`. Never use `/tmp` beds (symlink safety check).
- `devin auth login` is **not** done on this machine → `devin -p` headless is unavailable and CP-70 §J summarizer stays BLOCKED; sub-agents therefore run via the orchestrator's built-in `run_subagent` (SWE-2 Max), per operator decision 2026-10-13. The runner-side Devin ACP provider uses PKCE `authenticate{devin-browser}` which is independent and completes silently if the browser is already signed into devin.ai.
- Desktop app requires real Supabase sign-in (no demo mode) — UI scenarios that need Desktop must run on the configured project or be marked BLOCKED.

### Open Questions

- `Q-1` RESOLVED (2026-10-13): sub-agents = orchestrator's built-in `run_subagent` (`subagent_general` profile, SWE-2 Max). No `devin auth login` needed for the driver path.
- `Q-2`: Is the browser on this Mac signed into devin.ai (silent ACP PKCE)? Verified in preflight W-0; if not, operator completes one browser sign-in.
- `Q-3`: Exact live model ids — `devin/swe-2-max` existence and `grok-4.5-low` spelling resolved from each provider's live catalog during preflight; agents record the actual id used.

### Source Refs

- `CP-Test-Progress-Tracking.md` (prior live evidence and open items)
- `apps/local-runner/internal/cli/root.go` (`--port`, `FLOWPILOT_RUNNER_PORT`, default 4317)
- `apps/local-runner/internal/runner/dispatch_store_local.go` (per-project `dispatch.lock` under runner cwd `.flowpilot/chats/<projectId>/`)
- `justfile` (`runner-dev`, `chat-dev`, `chat-print`, `desktop-dev-runner`)

---

## 1. Goal

Every listed CP ends with an explicit per-scenario status — `LIVE PASS`, `PARTIAL`, `BLOCKED`, `FAILED`, or `AWAITING USER (UI/env)` — backed by run IDs, ports, log lines, and artifact paths recorded in `CP-Test-Progress-Tracking.md`. No scenario is marked done on automated tests alone when a live scenario is specified.

## 2. Input Documents

| Group | Docs |
|---|---|
| Tracking | `done/CP-Test-Progress-Tracking.md` |
| Core test-step docs | `CP-23`, `CP-43`, `CP-48`, `CP-49`, `CP-54`, `CP-55`, `CP-58`, `CP-59`, `CP-60`, `CP-61`, `CP-62`, `CP-63`, `CP-64`, `CP-65`, `CP-66` `-Test-Steps.md` (all in `done/`); `CP-67-Test-Steps.md` (in `todo/`, §11 live runbook) |
| Core CP docs (no test-steps file) | `CP-34-Init-tool.md` §11, `CP-35` §7/§10, `CP-37` §7, `CP-41` §11, `CP-42` §7/§10, `CP-44` §11, `CP-50` §7, `CP-51` §10 + `CP-51-PhaseAB-Timeline-And-Verification-Log.md` §3 |
| Provider docs | `CP-46` §7/§10, `CP-57-Test-Steps.md`, `CP-70-Test-Steps.md` (R1–R10 matrix) |

## 3. Environment Preflight (findings 2026-10-13)

| Item | State |
|---|---|
| Go / Node | go1.24.5, node v22.23.2 — OK |
| Provider CLIs | devin 3000.10.31, grok 1.0.40, opencode 1.18.30, claude, codex, gemini — all installed |
| Grok auth | `~/.grok/auth.json` present — live-proven (runs 888315/908843 on this Mac) |
| Devin REPL auth | **NOT logged in** — blocks `devin -p` headless + CP-70 §J summarizer only |
| Devin ACP auth | PKCE `devin-browser`; silent if browser signed in — verify once in W-0 |
| Opencode auth | unknown — verify in preflight (`opencode auth list` / first turn) |
| `gate-sandbox` | EPERM — unusable; use `~/fp-beds/` |
| Bed seed | `~/fp-beds/cp64m2` = full gate-sandbox clone (calc/format/stringutil/textutil pkgs, `change-audit/FEATURE-KEYS.md`, requirements/, git history, `.gitnexus` index, installed skill dirs) |
| Runner port config | `FLOWPILOT_RUNNER_PORT` env and `--port` flag both supported |
| Dispatch lock | `<runner-cwd>/.flowpilot/chats/<projectId>/dispatch.lock` — isolated automatically by per-CP worktree + unique projectId |
| Desktop | needs Supabase sign-in (configured: projectRef `ipgxvrxrhfhaeskfvctr`); UI wave reuses one desktop instance |

## 4. Isolation Architecture (parallel-safety contract)

Per CP `<NN>` (zero-padded CP number), each agent MUST use exactly its assignment:

| Resource | Value pattern |
|---|---|
| Worktree | `git worktree add /Users/tiendat/Desktop/flowpilot/flowpilot-lt-cp<NN> cp_live_test` |
| Runner port | `19200 + NN` (CP-23→19223 … CP-70→19270). Pre-flight: `lsof -i :<port>` must be empty; export `FLOWPILOT_RUNNER_PORT=<port>` AND pass `--port <port>` |
| projectId | `lt-cp<NN>` (never reuse another CP's id — the dispatch lock and `.flowpilot/chats/` are keyed on it) |
| Test bed | `~/fp-beds/lt-cp<NN>/` — cloned per §5; absolute canonical path, never `/tmp` |
| Runner state | `<worktree>/.flowpilot/` (auto-isolated); runner stdout → `~/fp-beds/lt-evidence/cp<NN>/runner.log` |
| Flow diag | `FLOWPILOT_FLOW_DIAG_DIR=<bed>/.flowpilot/logs/features/agent-flow-engine` |
| Evidence dir | `~/fp-beds/lt-evidence/cp<NN>/` — result fragment `RESULT.md`, curl transcripts, ndjson excerpts, screenshots |
| Cleanup | agent kills only its own runner PID (recorded at start); never `pkill flowpilot` |

Shared HTTP harness (all agents):

```bash
BASE=http://127.0.0.1:$PORT
POST /client/workflow-runs            {projectId:"lt-cp<NN>", providerKey, model, chatMode, workingMode, cwd:<bed>, yoloMode}
POST /client/workflow-runs/{id}/turns {stepId, prompt, flowRef?, subMode?, changeType?}
GET  /client/workflow-runs/{id}        # state + steps
GET  /admin/workflow-runs/{id}/events  # event stream
POST /client/workflow-runs/{id}/resume           # after runner restart
POST /client/workflow-runs/{id}/agent-loop/continue   # parked/blocked flows
POST /client/workflow-runs/{id}/agent-loop/amend {paths:[...]}              # contract amend
POST /client/questions/{qId}/answer    # ask_user / approval cards
GET  /health
```

Turn body field is `prompt` (not `input`). On a parked flow: `continue` resumes, `done` settles the whole run terminal.

## 5. Test Bed Recipes

| Bed | Recipe | Used by |
|---|---|---|
| `full` | `rsync -a --exclude=.flowpilot --exclude=.grok --exclude=.opencode ~/fp-beds/cp64m2/ ~/fp-beds/lt-cp<NN>/` (keeps `.git` history + FEATURE-KEYS + `.gitnexus`) | context/gate/flow CPs (23,35,37,41,43,44,48,49,50,51,54,55,58,61,62,64,65,66,67) |
| `clean` | fresh `git init` + minimal go module (`go.mod`, `calc.go`, `calc_test.go`) + `change-audit/FEATURE-KEYS.md`; let `/init` install skills | CP-34, CP-68-style init checks, CP-60 vibe bed |
| `bug-seeded` | `full` + deliberate bug (e.g. `Square(n)=n+n`) + BUG-XXX doc | CP-64 |
| `nonexistent/escape` | paths that don't exist or escape boundary | boundary cases |

`gate_mode`: beds default `enforce` via `<bed>/.flowpilot/settings/gate-config.json`; warn-mode scenarios write the file and restart the runner.

## 6. Sub-Agent Contract

Each CP gets ONE dedicated sub-agent with this exact mandate:

1. Create/enter its worktree; `cd apps/local-runner && go build ./cmd/flowpilot`.
2. Run the CP's §"Automated — run first" suite; record pass/fail counts. **Pre-existing failures → record and continue; never fix, never edit tests.**
3. Start runner on its assigned port with required env (`FLOWPILOT_DEVIN_AGENT=1` when the scenario uses devin turns).
4. Execute its CP's live scenario table (§8) via HTTP + log/file assertions.
5. Write `~/fp-beds/lt-evidence/cp<NN>/RESULT.md`: per-scenario `{id, status, runIds, port, provider/model, evidence paths, log lines}` + a `BUGS FOUND` list.
6. Kill its runner; leave the worktree clean (no commits, no edits).
7. Orchestrator (main agent) merges `RESULT.md` into `CP-Test-Progress-Tracking.md` — agents never edit the tracking file or repo docs.

Safe-fix rules binding all agents: no production edits, no old-test edits, no flag flips outside the assigned worktree/bed, no killing foreign PIDs, no shared `projectId`.

## 7. Execution Waves

| Wave | CPs | Mode | Turn provider/model |
|---|---|---|---|
| W-0 Preflight | (orchestrator) — build check, devin ACP smoke turn on scratch bed, port sanity, bed seed script | sequential | devin probe |
| W-1 | CP-34, CP-35+37, CP-44+50, CP-23 | parallel ≤4 | devin/swe-2-max |
| W-2 | CP-41, CP-42, CP-43, CP-48+49, CP-54 | parallel ≤4 | devin |
| W-3 | CP-51, CP-55, CP-58, CP-60 | parallel ≤4 (CP-60 is long) | devin |
| W-4 | CP-59, CP-61, CP-62, CP-63 | parallel ≤4 | devin (CP-59 legs incl. grok/opencode) |
| W-5 | CP-64, CP-65, CP-66, CP-67 | parallel ≤4 | devin (CP-65 candidates mixed devin+grok) |
| W-6 Providers | CP-46, CP-57, CP-70 | parallel ≤3 | grok-4.5-low / opencode / devin swe-2-max |
| W-7 UI | serialized computer-use pass on one shared desktop+TUI worktree (port 19300) | sequential | per scenario |

Waves are sequential across groups for machine sanity; agents inside a wave run concurrently. Any agent may finish early; orchestrator starts the next wave when slots free.

## 8. Per-CP Live Scenario Matrix

Statuses reference prior evidence already in the tracking doc; the job is **fresh live evidence on `cp_live_test` HEAD**, plus closing still-open items.

### W-1 — Foundations

**CP-23 (port 19223, bed `full`)** — runtime intelligence
- `L-23-1` Budget packer: chat turn with wide file-read prompt → log `[prompt-pack]` / `prompt_context_audit` with truncation fields.
- `L-23-2` Drift ladder: engineer repeated failing-test turns → score events `drift_score_calculated`, system-note inject at 30–59, `narrow_context` at 60–79, `drift_pause_required` exactly once at ≥80 + run `blocked`/`blockReason=drift`; then `agent-loop/continue` resumes.
- `L-23-3` Vibe non-pause: same ladder with `workingMode=vibe` → score ≥80 but no pause event.
- `L-23-4` Skillpack presence: `.agents/skills/` + `.claude/skills/` populated on bed (post-init).
- `L-23-UI` drift card render → W-7.

**CP-34 (19234, bed `clean`)** — init tool
- `L-34-1` `POST /client/projects` bind → background init → bed contains `.flowpilot/tooling.json`, `engine-init.json`, `ledger/feature_history.ndjson`, `catalog/features.ndjson`, `.claude/skills/*/SKILL.md`, `.agents/skills/*/SKILL.md`.
- `L-34-2` Engine status endpoint returns global tooling rows (gitnexus/rtk/node) + project section.
- `L-34-3` Negative: break `gitnexus` on PATH → tooling row `missing`, init still success/partial (degrade, not block).
- `L-34-4` Boundary: `workingDirectory` escape → 400 `working_directory_outside_boundary`; nonexistent dir → soft-skip.
- `L-34-UI` Settings→Engine page → W-7 (needs signed-in Desktop).

**CP-35+37 (19235/19237, shared bed `full` — two agents, two projectIds)** — context engine + prompt continuity
- `L-35-1` Feature resolution: prompt "improve calc-core" → `featureConfidence: verified`; prompt log shows "Prior work" newest-last.
- `L-35-2` Gate: code change with no CA note → `r-ca` reprompt then block; write CA → pass.
- `L-35-3` Oracle: break a pre-existing test → `regression_test_broke` block; edit pre-existing test → flagged.
- `L-37-1` Feature-history + CA "why" injection visible in prompt artifact (`run-*-turns.ndjson` / prompt files).
- `L-37-2` Feature-key gate reprompt on unknown key → graceful degrade (no crash, reprompt once).
- `L-37-3` Prior-discussion injection + pivot: seed `chat_summary.ndjson`, ask unrelated → sticky resolution holds; explicit pivot switches feature.
- `L-37-4` Cross-feature mixing guard: prompt mentions calc-core + calc-format → no contamination (D/G tests).

**CP-44+50 (19244/19250, bed `full`)** — pluggable context sources + completion
- `L-44-1` Default flow unchanged: normal turn → prompt contains standard sections only.
- `L-44-2` Explicit source subset via flow override → only listed sections render.
- `L-44-3` Unknown source id → run rejected fast with validation error.
- `L-50-1` Head-first order: rag-harness run → coder prompt leads `## Canonical state` before `### Change History`/history; single occurrence (no BUG-268 double).
- `L-50-2` `source.excerpt`: dirty workspace file → excerpt block present, ≤cap; clean tree → absent.
- `L-50-3` `change.contract`: after a declared contract, next-step prompt carries `## Change Contract` block exactly once.

### W-2 — Harness family

**CP-41 (19241, bed `full`)** — RAG harness
- `L-41-1` Happy path: `flowRef:"rag-harness"` run → `flow_context_package` event, `[FlowPilot flow context package]` sentinel in implement prompt, `flow_validation_result` exit 0, `flow_audit_draft` `status:ready`, no auto-commit.
- `L-41-2` Retry: `test_command` fails once then passes → `flow_validation_retry` attempt 1, retry prompt has `## Validation Failure — Retry 1/3`, then pass.
- `L-41-3` Max retries: always-fail command → attempts 1/2/3 → `failed_validation_max_retries`, no 4th, clear failure surface.
- `L-41-4` Env error: nonexistent binary → `skipped_env_error`, `retryAttempt` stays 0.
- `L-41-5` Audit blocked: remove feature key + lock FEATURE-KEYS read-only → `blocked_missing_feature_key`, empty commitMessage/ledgerBlock.

**CP-42 (19242, bed `full`)** — flow-pack refactor
- `L-42-1` Built-in mirror: `GET` workflows/flows list shows built-in RAG/Review-Loop badges; delete mirror row → next `flowRef` start recreates it.
- `L-42-2` `flowRef` start auto-spawns entry node (not inert); hub gets `flow-start-wait` note.
- `L-42-3` `submit_review_outcome` absent on normal chat run; present only on hub turns.
- `L-42-4` Unknown `flowRef` → typed 4xx, no spawn.
- `L-42-5` Runner restart after context production → typed artifacts restored (resume).

**CP-43 (19243, bed `full`)** — change contract + canonical head
- `L-43-1` Declared contract → `contracts.ndjson` `confidence:"declared"` + declared_paths.
- `L-43-2` No contract → exactly one `r-contract` reprompt → `inferred` entry; turn not blocked.
- `L-43-3` Scope drift (enforce): writer touches file outside `declared_paths` → block `WAITING_USER_APPROVAL`; `POST agent-loop/amend {paths:[...]}` → frozen contract v2 supersedes v1 → resume → pass.
- `L-43-4` Pending canonical: mid-flow `pending_canonical.ndjson` staged; terminal done → finalized exactly once into `canonical/<feature>.json`.
- `L-43-4b` Restart durability: kill runner between Stage() and finalize → restart → resume → finalize still writes exactly once (no double Head write, no lost pending record).
- `L-43-5` Head ordering + rejected decisions render in next-run prompt (`## Canonical state` with `do NOT re-attempt`).

**CP-48+49 (19248/19249, bed `full`)** — doc conformance + SS-lock
- `L-48-1` Seed malformed docs → `/standardize`-class scan reports violations.
- `L-48-2` Auto-fix pass repairs them; diffs recorded.
- `L-48-3` Malformed Task doc (no `## 6. Acceptance Check`) → AC coverage skipped gracefully.
- `L-49-1` `/standardize` extraction writes SS-locked docs; `ss-lock` gate entries in log.
- `L-49-2` Hard-ceiling: sprint handoff records only verified state (choice/tamper have real sources).

### W-3 — Heavy flows

**CP-51 (19251, bed `full`)** — durable dispatch. Highest bug history; verify on devin this time.
- `L-51-1` Crash mid-turn: kill -9 runner during a live turn → restart → `POST resume` → dispatch record reconciles; no lost/duplicate turn (inspect `dispatch.ndjson` states + revisions).
- `L-51-2` Stop mid-flow: Stop with children in-flight → children cancelled, parent terminal, no ghost RUNNING; `stop gen` advanced.
- `L-51-3` Post-Stop follow-up: ordinary prompt on stopped run → admitted, answered, persisted across restart (BUG-307/308 regression watch).
- `L-51-4` Post-done follow-up: after loop `done`, new prompt → admitted, no `409 flow_stopped`, transcript order correct after restart (BUG-302/305/306).
- `L-51-5` Gate reprompt ×2 same child: durable idempotency key increments (`…0001`→`…0002`), no replay of completed turn (BUG-301/run-23820 class).
- `L-51-6` Operator surfaces: forced `uncertain`/`repair_required` → attention/operator endpoints list + resolve atomically.
- `L-51-7` Restart mid-flow state restore (A5 regression): kill runner while a flow child/synthesis is in-flight → restart → `POST resume` → step-transitions + agent cards + timeline restore from durable rows; synthesis not fake-done/fake-cancelled; hub's never-sent first prompt reappears (BUG-300 watch); `run-*-turns.ndjson` before/after compared line-by-line.

**CP-55 (19255, bed `full`)** — flow-first preflight contract
- `L-55-1` task-harness/rag-harness: `preflight_contract_plan` → `preflight_contract_freeze` → `frozen_contracts.ndjson` v1 with base_sha; writer steps pinned to declared paths.
- `L-55-2` Amend flow: drift block → amend → v2 supersedes; flow resumes.
- `L-55-3` Freeze idempotent on re-drive (no re-fire planner-readonly on prior scaffold output — CP-67 F-6 class).

**CP-58 (19258, bed `full`)** — bug/task/cp harness family
- `L-58-1` `task-harness` happy path end-to-end via HTTP.
- `L-58-2` Plan review loop (loop 1) and code review loop (loop 2) run as separate loops.
- `L-58-3` `cp-harness` slice-only run; `cp-harness-smoke` (13 nodes, opt-in).
- `L-58-4` Regression canary: legacy flows unchanged.

**CP-60 (19260, bed `clean`)** — vibe flows
- `L-60-1` Vibe mode gate: TUI/HTTP vibe session on clean bed → mode gate enforces vibe workflow.
- `L-60-2` Snake MVP build (the live DoD): vibe run produces playable snake per CP-60 §5 acceptance (files, build/run command green).
- `L-60-3` Fail-closed probes per §6.
- Long-running; agent may run while other waves proceed.

### W-4 — Continuity + parity-1

**CP-59 (19259, bed `full`)** — cross-provider chat SSOT
- `L-59-1` Switch matrix spot: grok→opencode and opencode→grok legs via `POST /client/chats/{chatId}/switch-provider` → new leg, envelope `includedTurnCount`, E-9 divider, identity check on target.
- `L-59-2` Same-provider → `handoff_same_provider` 409 → in-place, zero new leg.
- `L-59-3` Timeline: `GET /client/chats/{id}/timeline` → legs sorted, records deduped, dividers positioned.
- `L-59-4` Detached chat → first prompt reattaches via createRun (no `chat_no_active_leg`).
- `L-59-5` Devin participation check: devin as switch **target** if supported; `supportsHandoffSource(devin)=false` documented either way.
- `L-59-6` Restart mid-multi-leg chat (was skipped historically as "disruptive" — free in isolated worktree): ≥2-leg chat → kill runner → restart → reopen via `GET /client/chats/{id}/timeline` → replay identical (stable `chatSeq`, idempotent `E-9` dividers, no dup records), current leg resumes via `seedTranscriptFromDisk`, new turn lands on latest leg.
- `L-59-G` Drive sync (G1-G7) → BLOCKED without Drive creds — record as such, don't fake.

**CP-61 (19261, bed `full`)** — harness done-verdict gate
- `L-61-1` `task-harness` M-scenario: verdict gate enforces done criteria; log grep per §7.
- `L-61-2` `cp-harness` C-scenario.
- `L-61-3` Negative non-harness run unaffected.

**CP-62 (19262, bed `full`)** — ZCode harness parity
- `L-62-1` M-1 verdict schema + node isolation live on `task-harness`.
- `L-62-2` M-2 context profile + sprint handoff on `vibe-sprint` (fresh live run; prior pass was automated-only).
- `L-62-3` M-4 AC coverage on review-submit path (bridge enforcement, real flow this time — not parser-only).
- `L-62-UI` M-5 decision card + M-8 drift card → W-7.

**CP-63 (19263, bed `full`)** — LSP runtime
- `L-63-1` Go bed → `[lsp] lsp.start` + diagnostics in context/log.
- `L-63-2` Missing binary path → graceful degrade + `flowpilot doctor` reports missing.
- `L-63-3` Crash budget: force LSP crashes → session-wide disable; assert no respawn after budget (CA-889).

### W-5 — Parity-2 family

**CP-64 (19264, bed `bug-seeded`)** — reproduce-first gate
- `L-64-1` Real bug → `bug-harness`: reproducer writes RED test → `r-reproduce` passes → `[gate] reproduce-first: locked` test file read-only → implement touches only production → validate GREEN → audit done.
- `L-64-2` False-alarm bug → green-on-arrival → fail-closed reprompt with correct wording; `implement` stays PENDING; production untouched.
- `L-64-3` Compile-error test → reprompt says "failed to compile" not "suite passed" (ClassifySuiteOutput `[setup failed]` regression watch).

**CP-65 (19265, bed `full`)** — tournament. Now genuinely multi-provider.
- `L-65-1` `tournament-harness` standalone: ≥2 candidates with **different providers** (devin + grok legs) → winner selected + merged clean.
- `L-65-2` Review-cap escalation → tournament auto-trigger (live, not just unit).
- `L-65-3` Tie → human decision card surfaces via API; operator decision resolves.
- `L-65-4` Retry ≤2 then stop (back-edge), resume parent after tournament.

**CP-66 (19266, bed `full`)** — living knowledge base
- `L-66-1` Bootstrap from real GitNexus on bed → knowledge artifacts written.
- `L-66-2` Planner receives `knowledge.flow` context at right locus; coder does not.
- `L-66-3` Audit hook updates differentially, non-blocking; empty/stale graph → graceful empty section.

**CP-67 (19267, beds `full` + `vibe`)** — contract-first scaffold TDD
- `L-67-A` Happy path on `task-harness` with **devin** (prior pass was grok-only; L-A codex/claude spots still pending): scaffold stubs + RED suite → contract lock v1 → coder body-only fill → signature lock holds → validate green → audit → `flow_run_complete_done`.
- `L-67-B` Gate rejections: real-logic scaffold → `r-scaffold-red` reprompt; all-green scaffold → reject; compile-broken → compile classification; coder edits locked test → ReadOnlyPaths reject; signature drift/additive function → `r-signature-lock`.
- `L-67-C` Renegotiation: coder submits `renegotiate_signatures` batch → record-only → `synthesis_negotiation` hub dispatch → round increments → cap 5 escalate; vibe-sprint parity run (was `pending`).
- `L-67-D` Restart mid-negotiation → graceful (in-memory batch loss is a documented limitation, verify no hang).

### W-6 — Providers

**CP-46 (19246)** — Grok (turns use `grok`/`grok-4.5-low`)
- Compact parity pass on this HEAD: `L-46-1` chat turn + streaming events; `L-46-2` YOLO-off approval round-trip (deny blocks); `L-46-3` `spawn_agent` child isolation + no orphan `devin acp`/`grok` processes; `L-46-4` mid-chat model switch same session; `L-46-5` resume after restart; `L-46-6` flow gate `r-ca` on grok turn; `L-46-7` usage/`modelContextWindow` fields.

**CP-57 (19257)** — Opencode (turns use `providerKey:"opencode"`)
- `L-57-1` smoke S1-S5; `L-57-2` E approval+YOLO; `L-57-3` F spawn_agent; `L-57-4` J summary; `L-57-5` K flow mode + gates (`flowRef` on opencode); `L-57-6` L MCP tools visible. Sections already PASSED 2026-08-30 get a spot re-check, not a full re-run.

**CP-70 (19270, bed `~/fp-beds/cp70-devin` reuse-or-clone)** — Devin, extra depth (turns use `devin`, model `swe-2-max` else `swe-2-high`)
- `L-70-R1..R10` full matrix re-run on macOS HEAD (chat+PKCE auth, file write, mid-chat model switch, approval deny+approve, resume after restart, flow gates, spawn_agent isolation, cancel, grok regression, usage).
- `L-70-NEW-1` Skills injection + posture scan/plan re-check on macOS.
- `L-70-NEW-2` Failure handling: kill `devin acp` mid-turn → turn terminal-fails clean, no infinite `running`; network flap if scriptable.
- `L-70-BLOCKED` J summarizer (`devin -p` needs `devin auth login`), I5/I6 Drive/cross-account — record BLOCKED with reason unless creds provided.

### W-7 — Computer-use / UI (serialized)

Single shared worktree `flowpilot-lt-ui` + runner :19300 + Desktop (`desktop-dev-runner`) + TUI (`chat-dev`). TUI input stalls are documented — every UI row keeps a raw-HTTP/log fallback evidence line, and the row is marked `UI-only` vs `backend-verified` explicitly. Scenarios:

| # | Case | CP source | Surface |
|---|------|-----------|---------|
| `UI-1` | Drift card render at dev-mode score ≥80 (shared surface) | CP-23 K2 / CP-62 M-8 | Desktop + TUI |
| `UI-2` | Decision card render + choice submit | CP-62 M-5; CP-65 M-3 tie | Desktop + TUI |
| `UI-3` | Devin provider: `/provider` `/model` pickers, Ctrl+C state restore, Settings Devin card/detect, Tab posture cycle | CP-70 S/A/B | TUI + Desktop |
| `UI-4` | Engine page: global tooling rows, project section, init/re-sync | CP-34 §11 | Desktop |
| `UI-5` | Account usage panel; scaffold UI paths | CP-70 P; CP-68 M-2/M-6 | Desktop |
| `UI-6` | TUI flow start: `/flow task-harness` smoke + vibe-mode entry (CP-60 bed) | CP-58 §S, CP-60 §4 | TUI |
| `UI-7` | TUI provider/model select + persistence across restart (grok + opencode + devin) | CP-57 B, CP-46 | TUI |
| `UI-8` | Cross-surface parity: same chatId timeline identical on TUI vs Desktop; E-9 dividers same positions | CP-59 I1 | TUI + Desktop |
| `UI-9` | Approval/gate card render + approve/deny round-trip | CP-51 A3-class, CP-46/CP-57 E | Desktop + TUI |
| `UI-10` | Blocked/repair attention card + operator resolve | CP-51 OP3, CP-64 fail-closed card | Desktop |

Any scenario needing Supabase sign-in that can't be completed → `AWAITING USER`; needs-2nd-account or Drive → `BLOCKED (env)`.

## 9. Operator Decisions (2026-10-13)

1. **Sub-agent model**: built-in `run_subagent` background agents (`subagent_general`, SWE-2 Max) — one per CP, no external auth needed.
2. **Turn provider (core CPs)**: `devin` with model `swe-2-max` when the ACP catalog exposes it (else `swe-2-high`, recorded verbatim); `FLOWPILOT_DEVIN_AGENT=1` at runner boot. First devin turn triggers PKCE `devin-browser` — silent if browser already signed into devin.ai; W-0 verifies before waves start.
3. **Concurrency**: ≤4 live agents per wave.
4. **Tracking writes**: agents emit `RESULT.md` fragments; orchestrator merges into `CP-Test-Progress-Tracking.md` after each agent finishes (append-only, historical evidence preserved).

## 10. Risks

- `R-1` Devin auth missing → all devin-turn scenarios degrade to grok fallback or BLOCKED; surfaced in W-0 before waves start.
- `R-2` 24 worktrees ≈ disk; each runner `go build` is cached but first build per worktree still takes minutes. Mitigation: build once in W-0 to warm `GOCACHE`.
- `R-3` Rate limits (xAI/Devin) under parallel load → agents retry once, then mark PARTIAL with the error verbatim.
- `R-4` Historical beds carry `.gitnexus`/ledger state that may skew context tests → beds cloned fresh per CP; LSP/dependence tests keep `.gitnexus`, init tests use `clean` beds.
- `R-5` TUI input stalls (documented) → UI wave keeps raw-HTTP fallback evidence and marks UI rows separately.

## 11. Definition of Done

- [ ] Every §8 row carries a fresh status + runId + evidence path in `CP-Test-Progress-Tracking.md` (or an explicit BLOCKED/AWAITING reason).
- [ ] Every discovered bug recorded as `BUG-LIVE-cp<NN>-<n>` with repro, logs, and owning CP — none fixed inside the test pass.
- [ ] All runners stopped; no worktree left dirty; no port left bound (`lsof -i :192xx` clean).
- [ ] Tracking file updated append-only; historical evidence untouched.
- [ ] Final summary to operator: per-CP PASS/PARTIAL/BLOCKED table + bug list + remaining UI items.
