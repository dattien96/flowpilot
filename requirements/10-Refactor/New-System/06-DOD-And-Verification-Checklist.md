# 06 - Definition of Done & Verification Checklists

This document is the **acceptance gate** for the Codex app-server refactor. It maps
every pain point in `01` to a Definition-of-Done (DOD) criterion and how it is
verified, then lists the feature checklist (from `03`–`05`) and the test-case
checklist that must all pass before the feature is marked done.

Final decision in effect (see `05`): **one shared Codex app-server, multi-workspace
runner, `cwd`-per-thread; chat session = Codex thread; lean on Codex thread APIs;
persist only `(workspace, threadId)`.**

Legend: `[ ]` not done · `[~]` partial · `[x]` done.

> **Status (this pass).** Phases 1–7 are implemented in the Go runner
> (`apps/local-runner`) + the Electron desktop client (`apps/desktop-flowpilot`),
> with 365 package tests passing (20 pre-existing Google-Drive/MCP/sessions failures
> that need live credentials are unrelated to this refactor). The async dispatcher +
> adapter + mapper, YOLO SSOT + approval policy + finalizer, orchestration state
> machine + prompt builders + per-run locking, multi-workspace + account scoping, the
> live `codex app-server` process layer + registry swap, `ask_user` registration,
> Claude/Gemini placeholders, and capability enforcement are all built and tested
> (over scripted mock processes / in-memory pipes / mocked PostgREST, as noted
> per-item). **Irreducible external-verification deferrals** (recorded per Part D #5,
> cannot be done from this environment): a run against a **real `codex` binary**, a
> **live Supabase** round-trip + golden-parity fixture run, the **Admin Web Next.js**
> server-orchestration rip-out, and **signed desktop installers** (need a macOS
> runner + Apple/Windows signing secrets). These carry `[~]`/`[ ]` with a reason.

---

## Part A — Pain-Point → Resolution DOD (Traceability)

Each pain point (PP-xx) traces to `01`, to the mechanism that resolves it, and to
the verification item (T-xx in Part C) that proves it.

| ID | Pain point (`01`) | Resolved by | DOD criterion | Verify |
|----|-------------------|-------------|---------------|--------|
| PP-01 | Final answer mixed with logs/command output | normalized `message_completed` / `turn_completed` events | Final assistant answer is a distinct event, never concatenated with logs/tool output | T-07, T-12 |
| PP-02 | File paths not rendered cleanly | `file_changed` events with structured path | Changed files render as rows: display name + relative path, absolute hidden | T-10 |
| PP-03 | No native-feeling command approval prompts | `permission_required` event + approval bridge | A command approval is shown as a structured card before execution | T-08, T-09 |
| PP-04 | Tool/file/command events not captured | `tool_started` / `tool_completed` / `file_changed` mapping | Tool, MCP, and file events are captured as discrete normalized events | T-06, T-10 |
| PP-05 | No low-latency streaming chat | `turn/start` + `SendMessageWithCallback` streaming | Assistant text streams incrementally as `message_delta` | T-05 |
| PP-06 | Weak provider-session resume | Codex threads + `(workspace, threadId)` persistence | A prior chat resumes with full context via `thread/resume` after runner restart | T-13, T-15 |
| PP-07 | Failure diagnosis split across output/files/state | centralized normalized event log | A failed turn is diagnosable from one persisted event timeline | T-11, T-16 |
| PP-08 | Hard `/` command + skill-selection UX | client picker + `listSkills` + skill on turn | `/` opens a picker; selected skill is sent with the turn and recorded | T-17 |
| PP-09 | Noisy full workspace paths in UI | structured `file_changed` path metadata | Full absolute path is available on demand (tooltip/copy), not shown inline | T-10 |
| PP-10 | Workflow/step state not visible during coding | runner owns workflow state; client selectors | Active project/workflow/step + run status visible in the client | T-01, T-04 |
| PP-11 | Workflow state diverges from runner | runner is single source of truth | Client never computes workflow progression; it renders runner state | T-04 |
| PP-12 | No structured approval data | approval record (cmd, cwd, reason, run, step, decisions) | Approval request carries command, cwd, reason, run/step, available decisions | T-08 |
| PP-13 | Approval decisions not audited | persisted approval records | Every approval decision is persisted with approver + timestamp + outcome | T-09, T-16 |
| PP-14 | Response not grouped by type | normalized event union | Assistant / command / tool / file / approval / error render as distinct groups | T-07, T-12 |
| PP-15 | File rows not clickable / no copy path | client `FileEventRenderer` over `file_changed` | File row opens in editor; full path copyable | T-10 |
| PP-16 | Cannot work primarily from IDE | interactive client over runner APIs | A full coding turn can be driven from the client end to end | T-01–T-10 |
| PP-17 | Workflow progression not owned by FlowPilot | runner turn dispatch + step state | Provider cannot advance workflow state; only the runner does | T-04 |
| PP-18 | Optimized prompt not guaranteed | optimize before `turn/start` | The optimized prompt is what reaches Codex, every turn | T-03 |
| PP-19 | Approvals not structured/auditable | PP-12 + PP-13 combined | (covered by PP-12/PP-13) | T-08, T-09 |
| PP-20 | Final response not cleanly captured | `turn_completed.finalMessage` persisted | Final response is persisted exactly, separate from logs | T-12 |
| PP-21 | Files/artifacts hard to navigate | finalizer artifacts + file events | Artifacts and changed files are listable and openable | T-10, T-14 |
| PP-22 | Admin Web role unclear | Admin Web = config/audit only | Admin Web shows provider/session/event audit, not primary chat | T-18 |
| PP-23 | Native client treated as reliable path | Controlled Mode default; native = assist | Controlled app-server path is the default; native client optional | design gate |
| PP-24 | Many workspaces × many chats each | shared app-server + `cwd`-per-thread + `thread/list` | Two workspaces each list their own threads from one app-server | T-02, T-13 |
| PP-25 | Resume after PC off | `thread/resume` + local thread log (same machine) | After full restart, a stored thread resumes; in-flight turn is re-sent | T-15 |
| PP-26 | YOLO inconsistent (Codex always `="approve"`, rule only in proxy) | YOLO as SSOT drives both runner policy + Codex sandbox/approval per turn | One resolved YOLO value configures both layers; no standalone `="approve"` | T-21, T-22, T-23 |
| PP-27 | Dangerous **terminal** command not gated (only MCP) | `permission_required` for command-exec when YOLO=false | YOLO=false: a dangerous terminal command is gated and **deny blocks** it | T-22 |
| PP-28 | YOLO=true posture not visible/audited | explicit per-run YOLO + audit record | YOLO=true run is auditable as gating-disabled (explains absent approvals) | T-24 |
| PP-29 | Command execution not captured as a distinct event | `commandExecution` → `tool_started`/`tool_completed` with exit status | A command run maps to discrete command events with status, separate from the assistant message | T-26 |
| PP-30 | Retry semantics unverified (finalizer / failed turn) | finalizer-failure + `turn_failed(recoverable)` retry paths | Finalizer failure is retried without erasing the completed turn; a recoverable failed turn is re-sendable | T-27, T-28 |
| PP-31 | No structured "ask the user" interaction (confirm/options popup) | `ask_user` MCP tool (model-driven) **and** workflow-driven `user_question_required` (deterministic), via the user-interaction bridge | Both paths render an options card; selecting an option resumes the turn/step with the choice | T-29, T-30 |

**Coverage rule:** the refactor is not done until every PP-xx row is `[x]` and its
linked T-xx passes.

**Status:** every PP is resolved by code with a passing test, **except** those whose
linked test carries a recorded external-verification deferral — PP-06/PP-25 (T-13/T-15
live `thread/list`/resume against the real binary), PP-22 (T-18 Admin Web UI timeline),
PP-26/PP-27 (T-22/T-23 real-terminal deny + live mixed-YOLO threads), and PP-31 (T-29
live model `ask_user` call / T-40 live tool-result). These are recorded deferrals per
Part D #5, all gated on a real `codex` binary, live Supabase, or the Admin Web refactor.

---

## Part B — Feature Checklist (must be built)

Grounded in `05` work items (W1–W8) and `04` implementation order.

### Runner contract & shared types
- [x] Normalized `ProviderEvent` union defined as Go types (`provider_event.go`)
- [x] `ProviderRuntimeAdapter` contract in the runner; core imports no Codex-specific types
- [x] Provider registry with Codex implemented, Claude/Gemini as placeholders (`placeholderAdapter`)

### Interactive APIs, streaming & reconnect (P2 / `04-02`)
- [x] Client API fulfils the full `04-01` `RunnerClient`: single-run GET, **answer-question**, **interrupt**; `/system/*` for sidebar controls
- [x] One persistent per-run SSE stream; `POST /turns` returns `{turnId}`; one-turn-per-session (`409 turn_in_progress`)
- [x] Every event carries a monotonic per-run `seq`; reconnect via `afterSeq`/`Last-Event-ID` (no gaps/dupes); `message_delta` ephemeral
- [x] Decisions/answers idempotent + first-write-wins (multi-window); expiry → recoverable fail; invalid → `400`
- [x] Account-scope validation on resume/turn (typed `409 provider_account_changed`); standard error envelope; loopback bind
- [x] Catalog (projects/workflows/steps) served in P2 (fake catalog); real Go Supabase read-path added (`SupabaseCatalogStore`, P5)
- [x] question/approval records + run/step status set + resolved/expired — implemented in the service (survives reconnect). _The `workflow_provider_questions` **DB** persistence attaches with live Supabase._

### Multi-workspace runner + shared app-server (W1)
- [x] `Runner.workspace` demoted; per-thread `cwd` authoritative on the new path (run `cwd` → `TurnRequest.Cwd` → thread/start). _The legacy `runner.go` `ExecutePrompt` path still keys off `r.workspace` and is retired with that fallback (not re-audited)._
- [x] One shared `codex app-server --listen stdio://` reused across threads (`ensureCodexAppServer`); mock-process verified, real binary external
- [x] **One active account at a time:** cross-account serialized; account switch interrupts in-flight (recoverable, not silent replay) (T-43); auto-switch-on-usage-limit documented as the future path (never-silent-drop holds)
- [x] Workspace binding: `StartRunInput.cwd` selects the active `cwd`; `POST /client/active-account` switches account

### Codex JSON-RPC + thread APIs (W2, W3)
- [x] Codex payload builders: `initialize` (captures version/caps), `thread/start` (`cwd` **+ `mcpServers`** incl. `ask_user`), `thread/resume`, `thread/list`, `thread/read`, `turn/start`, interrupt/approval-decision
- [x] **Async dispatcher** handles three message kinds — responses (id→waiter + ctx/timeout), notifications (per-thread routing), **inbound server→client requests routed to a reply-capable handler**; non-blocking read loop, bounded per-turn buffers, stdin write mutex (no synchronous helper / `SendMu` / id `3`)
- [x] **Process lifecycle:** read-loop EOF/error/process-death drains all waiters + turn channels with error, marks the handle dead; `ensureCodexAppServer` recreates on next use (no hung turns)
- [x] `(workspace, threadId)` run→thread mapping implemented in the service. _DB persistence (`workflow_provider_sessions`) attaches with live Supabase._
- [x] `thread/list` by `cwd`, `thread/resume`, `thread/read` param builders + dispatch wired. _A live two-thread/resume run is the external-binary acceptance (T-13/T-15)._

### Event mapping (W4)
- [x] `CodexEventMapper` converts Codex notifications → `ProviderEvent` (`codex_event_mapper.go`)
- [x] Events streamed via the turn bridge / per-run SSE; the adapter pumps mapped events
- [x] Every event persisted (in-memory) with run/step/session correlation ids + `seq`, replayable

### Approval bridge + YOLO SSOT (W5)
- [x] **YOLO resolver** SSOT: one value → Codex sandbox + approval mode **and** runner approval behavior (`resolveYoloPosture`)
- [x] YOLO=true → full-access + never-approve; runner auto-approves; audited gating-disabled
- [x] YOLO=false → workspace-write + untrusted; runner shows approval card before command/file tool use
- [x] **Approval policy engine** (YOLO=false): allowlist auto-approve / denylist auto-deny / else ask; every auto-decision **replies to the inbound request** (T-39)
- [x] `default_tools_approval_mode = "approve"` hack removed; config-ensure writes YOLO-derived values (T-25)
- [x] Codex approval → `permission_required`, turn paused, record persisted
- [x] Client decision validated against policy, forwarded to Codex, turn resumed
- [x] Expiry/interrupt replies deny to the inbound request (provider never hangs). _The error **`ask_user` tool-result** variant on expiry is exercised once the live MCP proxy intercepts the tool call (T-40)._
- [x] Pending approval survives client reconnect (state in runner; reloaded from the run snapshot)
- [x] Codex sandbox + approval documented as required for real safety (operator docs)

### Chat path migration (W6)
- [~] Chat path: the new interactive path drives `turn/start` via the adapter; repointing the legacy chat + retiring `ExecutePrompt` is the documented cut-over (criteria in `04-07-Migration-Notes.md`)
- [x] Prompt assembly (`BuildWorkflowStepPrompt` / `promptPrep`) runs before `turn/start`
- [~] `ExecutePrompt` callers — retained as fallback; tracing + migration is the cut-over step

### Lifecycle, finalizer, fallback (W7, W8)
- [x] Recovery rules: client disconnect does not fail the run (reconnect + `afterSeq` replay); provider-stream death → recoverable fail; approval/question expiry → recoverable fail. _(Status-set kept as the desktop-contract vocabulary; mapping to the `03` set documented.)_
- [x] Finalizer runs after `turn_completed`: final-response + diff + **summary + RAG** artifacts shaped; failure retryable without erasing the turn; **idempotent** (T-41). _Live writes (Supabase/RAG/GDrive) attach at cut-over._
- [x] `ExecutePrompt` one-shot retained as compatibility fallback

### Orchestration port + Supabase (P5 / `04-05`)
- [x] `deriveStepPromptBase` + prompt builders + state machine + orchestrator + **send-with-retry** (`sendTurnWithRetry`) + session-sync (in-memory run→thread) + finalize shaping ported TS→Go (golden-tested against the TS shape). _Golden parity on a **live fixture run** needs the live provider + Supabase._
- [x] Go Supabase access (`SupabaseWorkflowStore` + `SupabaseCatalogStore`), request shaping tested; trust model documented (key from the OS secret store; deployment chooses RLS-scoped vs service-role). _Live reads/writes are the acceptance step._
- [x] **Edge-function reconciliation decided + documented** (`04-07-Migration-Notes.md`): all three `workflow-engine-*` subsumed by the runner. _The Deno-side reduction to thin triggers + redeploy is the cut-over step._
- [x] Concurrent runs safe: per-run locking; idempotent writes; partial-failure retryable (T-42)
- [x] Navigator catalog read-path over real Go Supabase reads (`SupabaseCatalogStore`); live service swaps in at cut-over

### Clients
- [~] Admin Web: the runner exposes the read-only provider event/session/approval/question **audit APIs**; the Admin Web UI timeline + config screens are TS (rendered separately) — _server-orchestration rip-out is the deferred Next.js refactor_
- [x] Interactive client = **Electron desktop app**: workflow/step selector, chat+stream, approval card, question/options card, file links via real IDE CLI, `/` **multi-skill** picker, **system controls** — built with real runner wiring (`HttpWsRunnerClient`)
- [x] Desktop build → Windows + macOS + Linux from one codebase configured (electron-builder + CI matrix). _Producing a **signed/notarized** installer needs CI runners + secrets (acceptance step)._
- [x] React renderer is IDE-agnostic: it talks only to `RunnerClient`; Electron/IDE specifics are isolated in the preload/main `IdeBridge`

---

## Part C — Test-Case Checklist (must pass)

From `04` (test coverage + cross-cutting) and `05` (test plan). Each T-xx links
back to the PP-xx it proves (Part A).

### Adapter unit / contract tests
- [x] **T-01** shared app-server launches; `initialize` succeeds + caps captured (`TestEnsureCodexAppServerInitializes`, mock process) → PP-10, PP-16
- [x] **T-02** two threads/runs with different `cwd` run independently (`TestMultiWorkspaceRunsIndependent`) → PP-24
- [x] **T-03** assembled prompt (not the raw prompt) reaches `turn/start` (`promptPrep`; `TestCodexAdapterRegistersAskUser…`) → PP-18
- [x] **T-04** runner owns step state (`PlanWorkflowProgress` + orchestrator); adapter only emits events → PP-10, PP-11, PP-17
- [x] **T-05** message deltas stream (mapper + adapter pump tests) → PP-05
- [x] **T-06** tool-call maps to `tool_started`/`tool_completed` (`TestMapCodexNotification`) → PP-04
- [x] **T-07** final answer arrives as `message_completed`, separate from logs (mapper) → PP-01, PP-14
- [x] **T-08** approval maps to `permission_required` with cmd/cwd/reason/decisions → PP-03, PP-12
- [x] **T-09** approve resumes / deny stops; decision persisted (`TestApprovalDenyThenApprove`) → PP-03, PP-13
- [x] **T-10** file-change maps to `file_changed`; desktop row opens via real IdeBridge, full path on tooltip → PP-02, PP-04, PP-09, PP-15, PP-21
- [x] **T-11** failed turn maps to `turn_failed` (recoverable) → PP-07
- [x] **T-12** `turn_completed.finalMessage` persisted exactly → PP-20, PP-14
- [~] **T-13** `thread/list` by `cwd` — param builder + dispatch wired; a live filtered list needs the real binary → PP-06, PP-24
- [x] **T-14** finalizer runs after `turn_completed` (final/diff/summary/RAG) (`TestFinalizerHookSurfacesArtifacts`, `TestFinalizeLocalSnapshot…`) → PP-21

### Integration / end-to-end (extends `04` cross-cutting)
- [~] **T-15** runner restart → `thread/resume` reattaches; in-flight re-sendable — account-scope `409` + recoverable re-send tested; the live resume-after-restart needs the real binary → PP-06, PP-25
- [x] **T-16** reconnecting client replays the persisted timeline (`TestEventStreamReplayNoGapsOrDupes`) → PP-07, PP-13
- [x] **T-17** `/` picker attaches selected skill(s); recorded on the turn (`TurnInput.selectedSkills`; desktop multi-skill) → PP-08
- [~] **T-18** Admin Web audit — the runner audit APIs (`/admin/...`) are in + tested; the Admin Web UI timeline is the deferred TS rendering → PP-22

### YOLO SSOT
- [x] **T-21** YOLO=true → no `permission_required`; auto-approved (`TestYoloAutoApproveGatingDisabled`) → PP-26
- [~] **T-22** YOLO=false → deny blocks / approve runs (tested over the fake adapter); a **real terminal** command + Codex sandbox deny is the external-binary verify (Part D caveat) → PP-26, PP-27
- [~] **T-23** posture rides the turn (`req.YoloMode` → `codexYoloDerive` per thread); a live two-thread mixed-YOLO run needs the real binary → PP-26
- [x] **T-24** YOLO=true audited gating-disabled (`policy="yolo_gating_disabled"`) → PP-28
- [x] **T-25** config-ensure writes YOLO-derived values; no standalone `="approve"` (`TestResolveYoloPosture` + GDrive config tests) → PP-26

### Event mapping & retry
- [x] **T-26** `command.completed` maps to `tool_completed` with exit status, distinct from the assistant message → PP-04, PP-29
- [x] **T-27** finalizer failure retried + succeeds while the turn stays completed (`TestFinalizerFailureThenRetry`) → PP-30
- [x] **T-28** a recoverable `turn_failed` is re-sendable (recoverable emitted; re-send is a fresh client turn) → PP-30
- [~] **T-29** `ask_user` (model-driven): registered via `tools/list` + reinforced, pause/resume mechanism tested; whether the **model calls it** is best-effort (needs a live model) → PP-31
- [x] **T-30** workflow-driven `user_question_required` (runner-emitted) renders the same card + resumes (`TestWorkflowDrivenQuestion`) → PP-31

### Dispatcher robustness (`04-03`)
- [x] **T-37** an inbound server→client request is routed to a reply-capable handler (`TestDispatcherInboundRequestRouted`) → PP-03
- [x] **T-38** response timeout frees its waiter; process death drains all waiters + turn channels; backpressure never blocks the loop (`TestDispatcherProcessDeathDrains`, `TestDispatcherCallContextCancel`) → PP-07, PP-30

### P2 API robustness & reconnect (`04-02`)
- [x] **T-31** interrupt cancels an in-flight turn; marked `cancelled` (`TestInterruptCancelsTurn`) → PP-16
- [x] **T-32** reconnect replays with no gaps/dupes, ordered by `seq` (`TestEventStreamReplayNoGapsOrDupes`) → PP-07
- [x] **T-33** decision idempotent + first-write-wins across windows (`TestApprovalDenyThenApprove`) → PP-13
- [x] **T-34** concurrent `POST /turns` → `409 turn_in_progress` (`TestConcurrentTurnConflict`) → PP-11, PP-17
- [x] **T-35** unanswered approval/question expires → recoverable fail (`TestApprovalExpiry`) → PP-30
- [x] **T-36** account mismatch → typed `409 provider_account_changed` (`TestAccountMismatch`) → PP-06, PP-25

### Approval policy & finalizer (`04-04`)
- [x] **T-39** allowlist auto-approves / denylist auto-denies / else card; each auto-decision replies (`TestPolicyAutoDenyNoCard`, `TestPolicyAutoApproveNoCard`) → PP-12, PP-27
- [~] **T-40** expiry replies deny to the inbound request (provider never hangs); the error **`ask_user` tool-result** path needs the live MCP proxy → PP-30, PP-31
- [x] **T-41** finalizer idempotent — a retried finalize does not double-write (`TestFinalizerIdempotent`) → PP-21, PP-30

### Orchestration & multi-account (`04-05` / `04-06`)
- [x] **T-42** two concurrent runs isolated; per-run state not corrupted; finalize idempotent (`TestOrchestratorConcurrentRunsIsolated`, `TestFinalizerIdempotent`) → PP-11, PP-17
- [x] **T-43** account switch interrupts in-flight (recoverable) + hides the other account's threads (`409`); cross-account serialized (`TestAccountSwitchInterruptsAndScopes`). _App-server **process** recreate is mock-verified; real-binary recreate is external._ → PP-24, PP-26

### Regression / fallback
- [x] **T-19** fallback `ExecutePrompt` path still passes (untouched; existing runner tests green) + placeholders visible-but-unavailable (`TestStartRunRejectsDisabledProvider`)
- [~] **T-20** existing workflow control/artifacts/summaries/RAG unchanged — verified at the unit level; a full end-to-end equivalence needs the live backend

---

## Part D — Sign-Off Gate

The refactor is **done** only when all of the following hold:

1. Every PP-xx in Part A is `[x]` with its linked T-xx passing.
2. Every Part B feature item is `[x]`.
3. Every Part C test is `[x]` (incl. T-19/T-20 regression).
4. Caveats explicitly satisfied or documented as out of scope:
   - [x] Dangerous-command safety relies on **YOLO SSOT = `permission_required` + Codex sandbox + FlowPilot policy**, configured together — not approval UX alone — documented in `04-07-Operator-Docs.md`.
   - [x] YOLO=true is an explicit, visible, audited posture (gating disabled), never a silent default (T-24, `policy="yolo_gating_disabled"`).
   - [ ] Codex protocol assumptions verified against the **installed Codex build**: `cwd` placement, `thread/list`/`thread/read` availability, `thread/resume` across restart, and command-exec approval **deny truly blocks** (T-22) — **external verification step; requires a real `codex` binary** (the param builders + graceful-degrade are in, mock-verified).
5. [x] `01` pain points and `05` acceptance criteria reconciled — no open PP without either a passing test or an explicit, recorded deferral.

> **Current state.** Parts 1–3 are `[x]` **except** the recorded external-verification
> deferrals (Part A Status + the `[~]` items in Parts B/C): a run against the real
> `codex` binary, a live Supabase round-trip + golden-parity fixture run, the Admin
> Web Next.js server-orchestration rip-out, and signed desktop installers. All
> deferrals are recorded with a reason (#5), so the gate's reconciliation clause is
> met; final sign-off awaits those external steps + human review.
>
> **All remaining work is consolidated as an actionable plan in
> `04-08-Phase8-Cutover-And-Live-Acceptance.md`** — Part A (implement first: live
> store wiring, DB persistence, Admin Web thin client, `ExecutePrompt` retirement,
> edge-function reduction) and Part B (live acceptance against real infra). This gate
> closes when 04-08 Parts A + B are done.
