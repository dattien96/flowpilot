# 06 - Definition of Done & Verification Checklists

This document is the **acceptance gate** for the Codex app-server refactor. It maps
every pain point in `01` to a Definition-of-Done (DOD) criterion and how it is
verified, then lists the feature checklist (from `03`–`05`) and the test-case
checklist that must all pass before the feature is marked done.

Final decision in effect (see `05`): **one shared Codex app-server, multi-workspace
runner, `cwd`-per-thread; chat session = Codex thread; lean on Codex thread APIs;
persist only `(workspace, threadId)`.**

Legend: `[ ]` not done · `[~]` partial · `[x]` done.

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

**Coverage rule:** the refactor is not done until every PP-xx row is `[x]` and its
linked T-xx passes.

---

## Part B — Feature Checklist (must be built)

Grounded in `05` work items (W1–W8) and `04` implementation order.

### Runner contract & shared types
- [ ] Normalized `ProviderEvent` union defined as Go types (`turn_started`, `message_delta`, `message_completed`, `tool_started`, `tool_completed`, `file_changed`, `permission_required`, `turn_failed`, `turn_completed`)
- [ ] `ProviderRuntimeAdapter`-equivalent contract in the runner; core imports no Codex-specific types
- [ ] Provider registry with Codex implemented, Claude/Gemini as disabled placeholders

### Multi-workspace runner + shared app-server (W1)
- [ ] `Runner.workspace` (`runner.go:121`) demoted to a default; per-thread `cwd` is authoritative
- [ ] One shared `codex app-server --listen stdio://` per runner, reused across workspaces/threads
- [ ] Workspace registration/binding path; client can select active `cwd`

### Codex JSON-RPC + thread APIs (W2, W3)
- [ ] Codex payload builders next to `geminiACP*Params` (`sessions.go:265`): `initialize`, `thread/start` (with `cwd`), `thread/resume`, `thread/list`, `thread/read`, `turn/start`, approval decision
- [ ] `(workspace, threadId)` persistence (no custom session registry)
- [ ] `thread/list` filtered by `cwd`, `thread/resume`, `thread/read` wired

### Event mapping (W4)
- [ ] `CodexEventMapper` converts Codex turn/item notifications → `ProviderEvent`
- [ ] Events streamed via existing `SessionStreamCallback` (`sessions.go:653`)
- [ ] Every event persisted with run/step/session correlation ids, replayable

### Approval bridge + YOLO SSOT (W5)
- [ ] **YOLO resolver**: one resolved per-turn YOLO value maps to Codex thread/turn sandbox + approval mode **and** runner approval behavior (SSOT table in `05`)
- [ ] YOLO=true → Codex full-access + never-approve; runner auto-approves; run audited as gating-disabled
- [ ] YOLO=false → Codex workspace-write + on-request; runner shows approval card
- [ ] Always-on `default_tools_approval_mode = "approve"` hack removed; config-ensure writes YOLO-derived values
- [ ] Codex approval → `permission_required`, turn paused, record persisted
- [ ] Client decision validated against FlowPilot policy, forwarded to Codex, turn resumed
- [ ] Pending approval survives client reconnect (state in runner)
- [ ] Codex sandbox + approval config documented as required for real safety

### Chat path migration (W6)
- [ ] Chat repointed from `ExecutePrompt` to Codex-thread `turn/start` via `SendMessageWithCallback`
- [ ] Prompt optimization runs before `turn/start`
- [ ] All `ExecutePrompt` callers traced (workflow engine + `root.go:1239`) and migrated

### Lifecycle, finalizer, fallback (W7, W8)
- [ ] `LiveSession.Status` formalized to the `03` state set + recovery rules
- [ ] `TurnFinalizer` runs after `turn_completed`: artifact, diff snapshot, summary, RAG, step status; failure retryable without erasing the turn
- [ ] `ExecutePrompt` one-shot retained as compatibility fallback

### Clients
- [ ] Admin Web: provider/policy config + read-only provider event/session audit
- [ ] Interactive client = **Electron desktop app** MVP (`apps/desktop-flowpilot/`): workflow/step selector, chat+stream, approval card, file links via IDE CLI, `/` + skill picker
- [ ] Desktop app builds + runs on Windows and macOS from one codebase (macOS signed/notarized via CI)
- [ ] React webview kept IDE-agnostic (no Electron/IDE specifics) so a future VS Code/JetBrains plugin can reuse it

---

## Part C — Test-Case Checklist (must pass)

From `04` (test coverage + cross-cutting) and `05` (test plan). Each T-xx links
back to the PP-xx it proves (Part A).

### Adapter unit / contract tests
- [ ] **T-01** start session: shared app-server launches once; `initialize` succeeds → PP-10, PP-16
- [ ] **T-02** two threads with different `cwd` run independently in one app-server → PP-24
- [ ] **T-03** optimized prompt is what reaches `turn/start` (not the raw prompt) → PP-18
- [ ] **T-04** runner owns step state; adapter cannot mutate workflow state → PP-10, PP-11, PP-17
- [ ] **T-05** message deltas stream through `SessionStreamCallback` → PP-05
- [ ] **T-06** tool-call turn maps to `tool_started` / `tool_completed` → PP-04
- [ ] **T-07** final answer arrives as `message_completed`, separate from logs → PP-01, PP-14
- [ ] **T-08** approval request maps to `permission_required` with cmd/cwd/reason/decisions → PP-03, PP-12
- [ ] **T-09** approve resumes turn; deny stops it; decision persisted → PP-03, PP-13
- [ ] **T-10** file-change turn maps to `file_changed`; row opens in editor, full path copyable → PP-02, PP-04, PP-09, PP-15, PP-21
- [ ] **T-11** failed turn maps to `turn_failed` (recoverable flag) → PP-07
- [ ] **T-12** `turn_completed.finalMessage` persisted exactly → PP-20, PP-14
- [ ] **T-13** `thread/list` filtered by `cwd` returns that workspace's threads → PP-06, PP-24
- [ ] **T-14** finalizer runs after `turn_completed` (artifact/summary/RAG/step) → PP-21

### Integration / end-to-end (extends `04` cross-cutting)
- [ ] **T-15** runner restart → `thread/resume(id)` reattaches with context; in-flight turn lost is re-sendable → PP-06, PP-25
- [ ] **T-16** reconnecting client replays persisted event timeline → PP-07, PP-13
- [ ] **T-17** `/` picker inserts selected skill; skill recorded on the turn → PP-08
- [ ] **T-18** Admin Web shows provider event/session/approval audit for a run → PP-22

### YOLO SSOT
- [ ] **T-21** YOLO=true → Codex configured full-access + never-approve; no `permission_required` emitted; commands auto-run; runner auto-approves proxy → PP-26
- [ ] **T-22** YOLO=false → dangerous **terminal** command triggers `permission_required`; **deny blocks** execution; approve runs it → PP-26, PP-27
- [ ] **T-23** two threads in one app-server run at different YOLO levels; posture rides the turn, not the process → PP-26
- [ ] **T-24** YOLO=true run is audited as gating-disabled (record explains absent approvals) → PP-28
- [ ] **T-25** config-ensure writes YOLO-derived Codex sandbox/approval; no standalone `="approve"` remains → PP-26

### Event mapping & retry
- [ ] **T-26** `commandExecution` notification maps to `tool_started`/`tool_completed` with exit status, separate from the assistant message → PP-04, PP-29
- [ ] **T-27** finalizer failure is retried and succeeds while the provider turn stays `completed` (turn not erased) → PP-30
- [ ] **T-28** a `turn_failed` with `recoverable=true` can be re-sent and completes → PP-30

### Regression / fallback
- [ ] **T-19** fallback `ExecutePrompt` path still passes existing runner tests
- [ ] **T-20** existing workflow control, artifacts, summaries, Supabase RAG unchanged

---

## Part D — Sign-Off Gate

The refactor is **done** only when all of the following hold:

1. Every PP-xx in Part A is `[x]` with its linked T-xx passing.
2. Every Part B feature item is `[x]`.
3. Every Part C test is `[x]` (incl. T-19/T-20 regression).
4. Caveats explicitly satisfied or documented as out of scope:
   - [ ] Dangerous-command safety relies on **YOLO SSOT = `permission_required` + Codex sandbox + FlowPilot policy**, configured together — not approval UX alone (`05` boundary note).
   - [ ] YOLO=true is an explicit, visible, audited posture (gating disabled), never a silent default.
   - [ ] Codex protocol assumptions verified against the **installed Codex build**: `cwd` param placement, `thread/list` / `thread/read` availability, `thread/resume` guarantee across restart, and that command-exec approval is emitted and **deny truly blocks** (T-22).
5. `01` pain points and `05` acceptance criteria reconciled — no open PP without either a passing test or an explicit, recorded deferral.

> Until Part D passes, the feature stays **not done** regardless of demo readiness.
