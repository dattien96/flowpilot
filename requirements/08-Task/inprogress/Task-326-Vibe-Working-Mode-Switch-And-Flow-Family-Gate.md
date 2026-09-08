# Task-326: Vibe Working-Mode Switch And Flow-Family Gate

## Metadata

- Document ID: `Task-326`
- Title: `working_mode switch (vibe|normal) + fail-closed flow-family gate + /flow picker filter`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-08`
- Last Updated: `2026-09-08`
- Parent Documents: [CP-60: Vibe Working Mode](../../07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md), [SD-24: Vibe Working Mode](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md), [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md)
- Child Documents: `None`
- Related Documents: [Task-321](./Task-321-Vibe-Cp-Driven-Entry.md) (parked `P-6`), [Task-323](./Task-323-Vibe-Sprint-V2-Parity.md) (parked `P-7`), [CP-58](../../07-Coding-Plan/done/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md)
- Replaces: `None`
- Tags: `vibe-mode, working-mode, flow-picker, desktop, tui`
- Feature Keys: `vibe-mode`

## AI Quick View

### Summary

- Slice `CP-60 P-1` only, expanded by the 2026-09-08 operator freeze: a Desktop/TUI **vibe | normal** switch, `working_mode` on the local run, and a **fail-closed flow-family gate**.
- Vibe does **not** run `task-harness` / `cp-harness`. Coding path is `vibe-ingest` (user) → `vibe-sprint` (system, later `P-4`). `vibe-owner-debate` is resolver-only (`P-3`) and **never** appears in `/flow`.
- Normal (`dev`) keeps today's harness picker. Any `vibe-*` start from `/flow` or `POST /client/workflow-runs` is rejected.

### Current Ask

- TDD-first: land §10 signature files (empty bodies) **before** production code; then implement `P-1` until §9 DoD is all ticked. Do **not** land `r-requirement`, Owner resolver, SS Lock card, `vibe-cp-ingest`, or `vibe-sprint` v2.

### Key Decisions

- `T-1` `working_mode ∈ {dev, vibe}` (UI label **normal** = `dev`). Default `dev`. Persist on the **local run record** (`sessions.ndjson`) and as a **session default** for the next start (TUI `prefs.Session`, Desktop chrome). No Supabase `workflow_runs.working_mode` column. Live run mode is immutable; a request that would mutate it → `409 working_mode_pinned`.
- `T-2` Same-commit Desktop **and** TUI toggle (closes `SS-18 Q-4`). TUI: `/vibe` / `/vibe off` + persisted chip (YOLO prefs pattern). Desktop: chat-workspace chrome, not Settings, not Admin. Admin Web has no switch; `X-Client: admin` or missing + `vibe` → `403 working_mode_client_forbidden`.
- `T-3` Flow-family gate is SSOT in the runner. Function `flowAllowedForWorkingMode(mode, flowID, startKind)`. User-start vs system-start:

  | Source | `dev` (normal) | `vibe` |
  |---|---|---|
  | User `/flow` or `POST /client/workflow-runs` | harness five (`selectableIn: [flow]`) | **only** `vibe-ingest` |
  | System child (later `P-4` slicer) | deny vibe system ids | `vibe-sprint` |
  | System resolver (later `P-3`) | deny vibe system ids | `vibe-owner-debate` |

  Always reject user-start of `vibe-sprint`, `vibe-owner-debate`, and (until `P-6`) `vibe-cp-ingest`. Empty `startKind` = `user`. Frozen codes: family mismatch → `400 working_mode_flow_forbidden`; unknown `working_mode` → `400 invalid_working_mode`; Admin/missing-client + `vibe` → `403 working_mode_client_forbidden`; mutate live run mode → `409 working_mode_pinned`; empty/unknown flow id and hidden flows (`review-loop`, `rag-harness`, `cp-harness-smoke`) → `400 invalid_flow_ref`. Wire enum is `dev`|`vibe` only — never send `normal`.
- `T-4` List SSOT is runner `flowPickerOptions(workingMode)` (not `BuiltinOrchestrationOptions`, which is chat subMode). TUI `filterFlowSuggestions` and Desktop picker consume that id set. Vibe list = `[vibe-ingest]`. Dev list = exactly the five harness ids. `vibe-sprint` / `vibe-owner-debate` never listed. Do not flip YAML `selectableIn` on vibe flows.
- `T-5` Additive tests only — new files in §10. Signatures first. Gate is provider-agnostic (grep: no `providerKey`). HTTP: one primary provider + one other-provider spot check with fakes, not a 3× live-start matrix.

### Constraints

- Additive tests only; no pre-existing picker/harness test edits. Old `invalid_flow_ref` / review-loop-hidden contracts stay green.
- `feature_key: vibe-mode`.
- Do not add `POST /client/flows/run` (route does not exist). Enforce on `POST /client/workflow-runs` + `flowPickerOptions`.
- Do not start `vibe-sprint` / `vibe-owner-debate` from this Task. Do not rewrite `vibe-sprint.yaml`.
- Pack inventory stays **11 flows / 8 agents**. `vibe-ingest` stays `selectableIn: []`; vibe picker offers it by id.
- Will not undo: CA-755 hidden review-loop, CP-58 harness picker, CP-61 hub-done gate.

### Open Questions

- `Q-1` Resolved — TUI `/vibe` / `/vibe off` + persisted chip (YOLO prefs pattern).
- `Q-2` Resolved — Desktop chat-workspace chrome toggle, not Settings. Same commit as TUI; §10.6 is mandatory.

### Source Refs

- `CP-60 P-1`, operator freeze 2026-09-08, reviewer pass 2026-09-08 (this Task). `SS-18 Q-4` closed. `filterFlowSuggestions` (`tui/app/helpers.go`). `GET /client/chat/builtin-orchestration-options` is **out** of this gate (chat subMode). Task-321/323 parked.

## 1. Goal

A user can switch **normal ↔ vibe** on Desktop/TUI. Normal still launches harness flows via `/flow`. Vibe only lets the user start `vibe-ingest`. The runner rejects the other family even if the client lies. `vibe-owner-debate` is never a `/flow` row.

## 2. Parent Links

- coding plan: `CP-60 P-1` (this Task implements `P-1` + the 2026-09-08 flow-family freeze).
- tech design: `SD-24 D-1` (`working_mode` resolver discriminant, local only).
- system spec: `SS-18 BR-1`, `AC-1` (Desktop/TUI only), `AC-8` (Dev non-regression), `Q-4` closed as toggle.
- specific upstream ids: `CP-60 P-1`, `SS-18 Q-4`.

## 3. Trigger

Operator freeze 2026-09-08: vibe is not “also call task-harness”. Coding in vibe is the vibe family only. Normal must not start `vibe-*`. Owner debate is system-only and must not leak into `/flow`.

## 4. Exact Change

- `T-1` Add `WorkingMode` (`dev` | `vibe`) on the local run. Accept `working_mode` on `POST /client/workflow-runs` only when `X-Client: desktop|tui`. Missing → `dev`. Admin / missing client + `vibe` → `403 working_mode_client_forbidden`. Stamp at start; follow-up cannot change it (`409 working_mode_pinned`).
- `T-2` TUI `/vibe` / `/vibe off` + chip; Desktop chrome toggle. Persist next-start default. Same commit.
- `T-3` `flowAllowedForWorkingMode(mode, flowID, startKind)` on `createRun`. User+vibe → only `vibe-ingest` (bare or `flowpilot-core-flow-pack/vibe-ingest`). User+dev → exactly the five harness ids (bare or pack-prefixed). Codes in `T-3` Key Decision.
- `T-4` `flowPickerOptions(workingMode)` returns user-startable ids. TUI/Desktop consume it. YAML `selectableIn` unchanged.
- `T-5` Additive tests in §10. Signatures first.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/{interactive_service.go,interactive_handlers.go,local_file_session_store.go}` (start gate + `flowPickerOptions`), `apps/local-runner/internal/tui/app/{app.go,helpers.go,launch.go}` (`/vibe` + `/flow` filter), `apps/desktop-flowpilot/**` (chrome toggle + picker). New `task326_*_test.go` only.
- modules: `runner`, TUI, Desktop. Not `flowgate`. Not `chat_builtin_orchestration.go` unless a comment-only pointer.
- routes: `POST /client/workflow-runs` only. No new `/client/flows/run`.
- tables: none.

## 6. Acceptance Check

- §9 DoD all ticked. §10 empty signatures land before production diffs.
- `/vibe` then `/flow` → only `vibe-ingest`. User-start harness / debate / sprint → `400 working_mode_flow_forbidden`.
- `/vibe off` then `/flow` → exactly the five harness ids, no `vibe-*`. User-start `vibe-ingest` → `400 working_mode_flow_forbidden`.
- Admin or missing `X-Client` + `vibe` → `403 working_mode_client_forbidden`.
- Live run mode unchanged when session toggle flips. Restart replays stamped mode.
- Pack 11/8, vibe `selectableIn: []`, sprint still v1. Old tests untouched + green.

## 7. Out of Scope

- `P-2` `r-requirement` / `RequirementDrift`.
- `P-3` resolver auto-start of `vibe-owner-debate`.
- `P-4` SS Preview & Lock and ingest → sequential `vibe-sprint`.
- `P-6` / Task-321 `vibe-cp-ingest`.
- `P-7` / Task-323 `vibe-sprint` v2.
- New HTTP `POST /client/flows/run`.
- Chat `BuiltinOrchestrationOptions` / Bug-subMode picker (keep today's empty/hidden contracts).

## 8. Completion Notes

- result: `pending implementation` (DoD/signature guide patched after reviewer FAIL 2026-09-08)
- follow-ups: `P-2` `r-requirement`; `P-3` resolver; `P-4` lock + auto `vibe-sprint`; then unpark Task-321 / Task-323
- upstream docs updated: `CP-60 P-1`; `SS-18 Q-4`; this Task Q-1/Q-2 closed

## 9. Definition of Done

Product (all required):

- [ ] `WorkingMode` (`dev`|`vibe`) stamps the local run at `StartRun`; missing → `dev`; unknown → `400 invalid_working_mode`; never a Supabase column.
- [ ] TUI `/vibe` / `/vibe off` + chip **and** Desktop chrome toggle, same commit; label **normal** = `dev`; default `dev`; persists next-start default.
- [ ] Request that would mutate a live run's mode → `409 working_mode_pinned`; session default may still flip for the **next** run.
- [ ] Restart / reopen replays stamped `working_mode` from `sessions.ndjson`.
- [ ] `X-Client: desktop|tui` may send `vibe`. `X-Client: admin` or missing + `vibe` → `403 working_mode_client_forbidden`. Admin/missing + `dev` (or omitted) still starts harness.
- [ ] User-start allowlist `dev`: exactly `task-harness`, `bug-harness`, `bug-plan-harness`, `cp-harness`, `context-coding-review-synthesis` (bare or pack-prefixed).
- [ ] User-start allowlist `vibe`: **only** `vibe-ingest` (bare or pack-prefixed).
- [ ] User-start deny (both modes): `vibe-sprint`, `vibe-owner-debate` → `400 working_mode_flow_forbidden` and **not** in `flowPickerOptions`.
- [ ] User-start deny `vibe` + harness family (bare or pack-prefixed) → `400 working_mode_flow_forbidden`; no run created.
- [ ] User-start deny `dev` + any `vibe-*` (bare or pack-prefixed) → `400 working_mode_flow_forbidden`; no run created.
- [ ] Hidden flows (`review-loop`, `rag-harness`, `cp-harness-smoke`) stay `400 invalid_flow_ref` in both modes; not listed. Precedence: hidden-id uses `invalid_flow_ref`, not `working_mode_flow_forbidden`.
- [ ] `vibe-cp-ingest` user-start → `400 working_mode_flow_forbidden`; not listed.
- [ ] Empty `startKind` treated as `user`. Empty flow id → `400 invalid_flow_ref`.
- [ ] Pure gate allows `system`+`vibe`+`vibe-sprint` and `system`+`vibe`+`vibe-owner-debate` (no spawn in this Task). `system`+`dev`+those ids deny. `system`+`vibe`+harness deny.
- [ ] `flowPickerOptions(vibe)` == `[vibe-ingest]`. `flowPickerOptions(dev)` == the five harness ids (order stable, count==5). TUI `/flow` and Desktop picker show that id set only.
- [ ] YAML `selectableIn` on `vibe-ingest` / `vibe-sprint` / `vibe-owner-debate` remains `[]`. Pack still 11 flows / 8 agents. `vibe-sprint.yaml` still v1 (`plan → freeze → tdd → coder → synthesis`, no `context`/`validate`/`audit` nodes).
- [ ] `POST /client/workflow-runs` enforces the start gate. No `/client/flows/run` route added.
- [ ] Normal chat (no flowRef) in `dev` byte-for-byte. Chat `BuiltinOrchestrationOptions` unchanged.

Safe-fix (all required):

- [ ] Pre-existing tests **untouched** and **green** (review-loop hidden, harness picker, CP-61 `TestCP61HubDone` not edited).
- [ ] New tests = §10 files only; cover use + edge + error (`SS-04 §3.5.8`).
- [ ] R2: `flowAllowedForWorkingMode` takes no `providerKey` (grep). HTTP start: primary provider + one other-provider spot check (fakes OK).
- [ ] Prior CA not undone: CA-755, CP-58 picker, CP-61 hub-done.
- [ ] `change-audit/CA-NNN-*.md` with `feature_key: vibe-mode`, `source_doc_id: Task-326`.

Explicit not-claimed:

- [ ] `r-requirement` / Owner auto-start / SS Lock / sequential `vibe-sprint` / `vibe-cp-ingest` / sprint v2 / `POST /client/flows/run`.

## 10. Test Signature Guide (TDD)

Order: (1) create the files below with **empty bodies** (`func TestX(t *testing.T) {}` + Input/Expect comments). (2) production code. (3) fill bodies. Do **not** write production first. Do **not** edit pre-existing tests.

Cover **use / edge / error** per `SS-04 §3.5.8`. Assert **id sets and error codes**, not label copy.

Dev harness five (pin everywhere): `task-harness`, `bug-harness`, `bug-plan-harness`, `cp-harness`, `context-coding-review-synthesis`.

### 10.1 `apps/local-runner/internal/runner/task326_working_mode_gate_test.go`

Pure `flowAllowedForWorkingMode(mode, flowID, startKind)`.

Use:

```
// Scenario: normal user may start task-harness.
// Input: mode=dev startKind=user flowID=task-harness
// Expect: nil error
func TestFlowAllowed_DevUserTaskHarness(t *testing.T) {}

// Scenario: normal user may start each of the five harness ids.
// Input: mode=dev startKind=user; table of the five ids
// Expect: nil error per named subtest
func TestFlowAllowed_DevUserHarnessFamily(t *testing.T) {}

// Scenario: vibe user may start vibe-ingest (bare and pack-prefixed).
// Input: mode=vibe startKind=user flowID=vibe-ingest and flowpilot-core-flow-pack/vibe-ingest
// Expect: nil error both
func TestFlowAllowed_VibeUserIngest(t *testing.T) {}

// Scenario: future system child may start vibe-sprint only on a vibe run.
// Input: mode=vibe startKind=system flowID=vibe-sprint
// Expect: nil error
func TestFlowAllowed_VibeSystemSprint(t *testing.T) {}

// Scenario: future resolver may start vibe-owner-debate only on a vibe run.
// Input: mode=vibe startKind=system flowID=vibe-owner-debate
// Expect: nil error
func TestFlowAllowed_VibeSystemOwnerDebate(t *testing.T) {}
```

Edge:

```
// Scenario: missing mode is dev.
// Input: mode="" startKind=user flowID=task-harness
// Expect: nil error
func TestFlowAllowed_EmptyModeDefaultsDev(t *testing.T) {}

// Scenario: empty startKind is user (fail-closed).
// Input: mode=vibe startKind="" flowID=vibe-sprint
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_EmptyStartKindIsUser(t *testing.T) {}

// Scenario: UI word "normal" is not a wire value.
// Input: mode=normal startKind=user flowID=task-harness
// Expect: error code invalid_working_mode
func TestFlowAllowed_NormalAliasRejectedOnWire(t *testing.T) {}

// Scenario: unknown mode rejected.
// Input: mode=prod startKind=user flowID=task-harness
// Expect: error code invalid_working_mode
func TestFlowAllowed_UnknownModeRejected(t *testing.T) {}

// Scenario: vibe-cp-ingest is not user-startable yet.
// Input: mode=vibe startKind=user flowID=vibe-cp-ingest
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_VibeUserCpIngestForbidden(t *testing.T) {}

// Scenario: empty flow id is invalid_flow_ref.
// Input: mode=dev startKind=user flowID=""
// Expect: error code invalid_flow_ref; no panic
func TestFlowAllowed_EmptyFlowID(t *testing.T) {}

// Scenario: system cannot start vibe-sprint on a dev run.
// Input: mode=dev startKind=system flowID=vibe-sprint
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_DevSystemSprintForbidden(t *testing.T) {}

// Scenario: system cannot start vibe-owner-debate on a dev run.
// Input: mode=dev startKind=system flowID=vibe-owner-debate
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_DevSystemOwnerDebateForbidden(t *testing.T) {}

// Scenario: system cannot start harness on a vibe run.
// Input: mode=vibe startKind=system flowID=task-harness
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_VibeSystemHarnessForbidden(t *testing.T) {}

// Scenario: pack-prefixed harness is still forbidden in vibe.
// Input: mode=vibe startKind=user flowID=flowpilot-core-flow-pack/task-harness
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_VibeUserPackPrefixedHarnessForbidden(t *testing.T) {}

// Scenario: pack-prefixed vibe-ingest is still forbidden in dev.
// Input: mode=dev startKind=user flowID=flowpilot-core-flow-pack/vibe-ingest
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_DevUserPackPrefixedVibeForbidden(t *testing.T) {}
```

Error:

```
// Scenario: vibe user cannot start task-harness.
// Input: mode=vibe startKind=user flowID=task-harness
// Expect: error code working_mode_flow_forbidden
func TestFlowAllowed_VibeUserTaskHarnessForbidden(t *testing.T) {}

// Scenario: vibe user cannot start any other harness id.
// Input: mode=vibe startKind=user; table of the remaining four harness ids
// Expect: error code working_mode_flow_forbidden per named subtest
func TestFlowAllowed_VibeUserHarnessFamilyForbidden(t *testing.T) {}

// Scenario: dev user cannot start any vibe-* id including unlanded cp-ingest.
// Input: mode=dev startKind=user flowID in {vibe-ingest, vibe-sprint, vibe-owner-debate, vibe-cp-ingest}
// Expect: error code working_mode_flow_forbidden each
func TestFlowAllowed_DevUserVibeFamilyForbidden(t *testing.T) {}

// Scenario: user can never start sprint or owner-debate.
// Input: startKind=user flowID in {vibe-sprint, vibe-owner-debate} mode in {dev, vibe}
// Expect: error code working_mode_flow_forbidden all four
func TestFlowAllowed_UserNeverStartsSprintOrDebate(t *testing.T) {}
```

### 10.2 `apps/local-runner/internal/runner/task326_working_mode_http_test.go`

`POST /client/workflow-runs` only. Fake adapters. Primary provider `claude`; one spot check `grok` on one allow and one deny. No live CLI.

Use:

```
// Scenario: desktop vibe start vibe-ingest stamps the run.
// Input: X-Client=desktop working_mode=vibe flowRef=vibe-ingest provider=claude
// Expect: 2xx; store WorkingMode=vibe; run exists
func TestHTTPStart_VibeDesktopIngestStampsMode(t *testing.T) {}

// Scenario: tui omitted mode starts task-harness as dev.
// Input: X-Client=tui working_mode omitted flowRef=task-harness provider=claude
// Expect: 2xx; store WorkingMode=dev
func TestHTTPStart_DevTuiTaskHarnessDefaultMode(t *testing.T) {}

// Scenario: grok spot-check allow path.
// Input: X-Client=tui working_mode=vibe flowRef=vibe-ingest provider=grok
// Expect: 2xx; WorkingMode=vibe
func TestHTTPStart_VibeIngestGrokSpotCheck(t *testing.T) {}
```

Edge:

```
// Scenario: pack-prefixed ingest id is accepted in vibe.
// Input: X-Client=tui working_mode=vibe flowRef=flowpilot-core-flow-pack/vibe-ingest provider=claude
// Expect: 2xx; WorkingMode=vibe
func TestHTTPStart_VibePackPrefixedIngest(t *testing.T) {}

// Scenario: admin + dev harness still allowed.
// Input: X-Client=admin working_mode omitted flowRef=task-harness provider=claude
// Expect: 2xx; WorkingMode=dev
func TestHTTPStart_AdminDevHarnessAllowed(t *testing.T) {}

// Scenario: missing X-Client + dev harness still allowed.
// Input: no X-Client working_mode omitted flowRef=task-harness provider=claude
// Expect: 2xx; WorkingMode=dev
func TestHTTPStart_MissingClientDevHarnessAllowed(t *testing.T) {}

// Scenario: desktop and tui vibe ingest are equivalent.
// Input: same body working_mode=vibe flowRef=vibe-ingest; X-Client=desktop then tui; provider=claude
// Expect: both 2xx; both WorkingMode=vibe
func TestHTTPStart_DesktopTuiVibeIngestEquivalent(t *testing.T) {}
```

Error:

```
// Scenario: vibe + task-harness does not create a run.
// Input: X-Client=desktop working_mode=vibe flowRef=task-harness provider=claude
// Expect: 400 working_mode_flow_forbidden; store has no new run id
func TestHTTPStart_VibeTaskHarnessNoRunCreated(t *testing.T) {}

// Scenario: grok spot-check deny path.
// Input: X-Client=desktop working_mode=vibe flowRef=task-harness provider=grok
// Expect: 400 working_mode_flow_forbidden; no new run
func TestHTTPStart_VibeTaskHarnessGrokSpotCheck(t *testing.T) {}

// Scenario: pack-prefixed harness forbidden in vibe.
// Input: X-Client=tui working_mode=vibe flowRef=flowpilot-core-flow-pack/task-harness provider=claude
// Expect: 400 working_mode_flow_forbidden; no new run
func TestHTTPStart_VibePackPrefixedHarnessForbidden(t *testing.T) {}

// Scenario: dev + vibe-ingest forbidden and creates no run.
// Input: X-Client=tui working_mode=dev flowRef=vibe-ingest provider=claude
// Expect: 400 working_mode_flow_forbidden; store has no new run id
func TestHTTPStart_DevVibeIngestForbidden(t *testing.T) {}

// Scenario: user vibe-owner-debate forbidden even in vibe.
// Input: X-Client=desktop working_mode=vibe flowRef=vibe-owner-debate provider=claude
// Expect: 400 working_mode_flow_forbidden; no new run
func TestHTTPStart_UserOwnerDebateForbidden(t *testing.T) {}

// Scenario: user vibe-sprint forbidden even in vibe.
// Input: X-Client=tui working_mode=vibe flowRef=vibe-sprint provider=claude
// Expect: 400 working_mode_flow_forbidden; no new run
func TestHTTPStart_UserSprintForbidden(t *testing.T) {}

// Scenario: user vibe-cp-ingest forbidden (not in pack yet) over HTTP.
// Input: X-Client=tui working_mode=vibe flowRef=vibe-cp-ingest provider=claude
// Expect: 400 working_mode_flow_forbidden; no new run
func TestHTTPStart_UserCpIngestForbidden(t *testing.T) {}

// Scenario: admin cannot start vibe.
// Input: X-Client=admin working_mode=vibe flowRef=vibe-ingest provider=claude
// Expect: 403 working_mode_client_forbidden; no new run
func TestHTTPStart_AdminVibeForbidden(t *testing.T) {}

// Scenario: missing X-Client cannot start vibe.
// Input: no X-Client working_mode=vibe flowRef=vibe-ingest provider=claude
// Expect: 403 working_mode_client_forbidden; no new run
func TestHTTPStart_MissingClientVibeForbidden(t *testing.T) {}

// Scenario: unknown working_mode rejected before start.
// Input: X-Client=tui working_mode=prod flowRef=task-harness provider=claude
// Expect: 400 invalid_working_mode; no new run
func TestHTTPStart_UnknownModeRejected(t *testing.T) {}

// Scenario: hidden flows stay invalid_flow_ref in both modes (not family-forbidden).
// Input: X-Client=tui; flowRef in {review-loop, rag-harness, cp-harness-smoke}; working_mode=dev and vibe; provider=claude
// Expect: 400 invalid_flow_ref each; code is not working_mode_flow_forbidden
func TestHTTPStart_HiddenFlowsStayInvalidFlowRef(t *testing.T) {}

// Scenario: mutating a live run's working_mode is pinned.
// Input: start dev+task-harness; then request that sets working_mode=vibe on that runId
// Expect: 409 working_mode_pinned; store WorkingMode still dev
func TestHTTPStart_MidRunModePinned(t *testing.T) {}

// Scenario: normal chat with no flowRef is unaffected.
// Input: X-Client=tui working_mode=dev no flowRef runKind=chat provider=claude
// Expect: 2xx chat run; WorkingMode=dev; no flow family error
func TestHTTPStart_NormalChatUnaffected(t *testing.T) {}
```

### 10.3 `apps/local-runner/internal/runner/task326_working_mode_picker_test.go`

Target: `flowPickerOptions(workingMode)` only.

Use:

```
// Scenario: vibe picker is exactly vibe-ingest.
// Input: flowPickerOptions(vibe)
// Expect: ids == [vibe-ingest] (count==1)
func TestPicker_VibeListsOnlyIngest(t *testing.T) {}

// Scenario: dev picker is exactly the five harness ids.
// Input: flowPickerOptions(dev)
// Expect: id set == {task-harness, bug-harness, bug-plan-harness, cp-harness, context-coding-review-synthesis}; count==5
func TestPicker_DevListsHarnessFamily(t *testing.T) {}
```

Edge:

```
// Scenario: missing mode lists as dev.
// Input: flowPickerOptions("")
// Expect: same id set as flowPickerOptions(dev)
func TestPicker_EmptyModeListsDev(t *testing.T) {}
```

Error (leaks):

```
// Scenario: owner-debate never listed.
// Input: flowPickerOptions(dev) and flowPickerOptions(vibe)
// Expect: neither contains vibe-owner-debate
func TestPicker_OwnerDebateNeverListed(t *testing.T) {}

// Scenario: vibe-sprint never listed.
// Input: flowPickerOptions(dev) and flowPickerOptions(vibe)
// Expect: neither contains vibe-sprint
func TestPicker_SprintNeverListed(t *testing.T) {}

// Scenario: vibe-cp-ingest never listed.
// Input: flowPickerOptions(dev) and flowPickerOptions(vibe)
// Expect: neither contains vibe-cp-ingest
func TestPicker_CpIngestNeverListed(t *testing.T) {}

// Scenario: vibe list has none of the five harness ids.
// Input: flowPickerOptions(vibe)
// Expect: intersection with the five ids is empty
func TestPicker_VibeOmitsHarnessFamily(t *testing.T) {}

// Scenario: dev list has no vibe-* ids.
// Input: flowPickerOptions(dev)
// Expect: no vibe-ingest / vibe-sprint / vibe-owner-debate
func TestPicker_DevOmitsVibeFamily(t *testing.T) {}

// Scenario: review-loop / rag-harness / cp-harness-smoke stay hidden both modes.
// Input: flowPickerOptions(dev) and flowPickerOptions(vibe)
// Expect: none of those three ids present
func TestPicker_HiddenFlowsStayHidden(t *testing.T) {}
```

### 10.4 `apps/local-runner/internal/runner/task326_working_mode_persist_test.go`

Use:

```
// Scenario: stamped vibe survives local store round-trip.
// Input: start vibe+ingest; reload sessions.ndjson
// Expect: WorkingMode=vibe
func TestPersist_VibeModeRoundTrip(t *testing.T) {}

// Scenario: omitted mode persists as dev.
// Input: start harness with no working_mode; reload
// Expect: WorkingMode=dev
func TestPersist_MissingModePersistsDev(t *testing.T) {}
```

Edge:

```
// Scenario: legacy run JSON without the field loads as dev.
// Input: fixture session missing working_mode
// Expect: WorkingMode=dev; no panic
func TestPersist_LegacyRecordDefaultsDev(t *testing.T) {}

// Scenario: reopen after restart keeps stamped vibe.
// Input: start vibe+ingest; new service load from same store; lookup runId
// Expect: WorkingMode=vibe
func TestPersist_RestartReplaysStampedMode(t *testing.T) {}
```

Error:

```
// Scenario: flipping session default does not mutate the live run.
// Input: start dev harness; set session default vibe; read run
// Expect: run.WorkingMode still dev
func TestPersist_SessionToggleDoesNotMutateLiveRun(t *testing.T) {}
```

### 10.5 `apps/local-runner/internal/tui/app/task326_tui_vibe_flow_filter_test.go`

Use:

```
// Scenario: /vibe then /flow suggestions are only vibe-ingest.
// Input: slash /vibe; input="/flow "; collectSuggestions
// Expect: suggestion id set == {vibe-ingest} or {flowpilot-core-flow-pack/vibe-ingest}
func TestTUIFlowSuggest_VibeOnlyIngest(t *testing.T) {}

// Scenario: /vibe off then /flow omits vibe-*.
// Input: slash /vibe off; input="/flow "
// Expect: suggestion ids include the five harness ids; no vibe-ingest / vibe-sprint / vibe-owner-debate
func TestTUIFlowSuggest_DevOmitsVibe(t *testing.T) {}

// Scenario: /vibe persists as next-start default.
// Input: slash /vibe; persistTUISessionPrefs / reload prefs
// Expect: saved working mode vibe
func TestTUIVibeCommand_OnOffPersistsDefault(t *testing.T) {}
```

Edge:

```
// Scenario: typing /flow vibe in dev does not arm ingest.
// Input: workingMode=dev; input="/flow vibe"; Tab/Enter
// Expect: launch not armed with vibe-ingest
func TestTUIFlowSuggest_DevQueryVibeDoesNotArm(t *testing.T) {}
```

Error:

```
// Scenario: /flow vibe-owner-debate in vibe is rejected with frozen code.
// Input: workingMode=vibe; slash "/flow vibe-owner-debate"
// Expect: user-visible error containing working_mode_flow_forbidden; launch not armed
func TestTUIFlowStart_OwnerDebateRejected(t *testing.T) {}

// Scenario: /flow task-harness in vibe is rejected with frozen code.
// Input: workingMode=vibe; slash "/flow task-harness"
// Expect: user-visible error containing working_mode_flow_forbidden; launch not armed
func TestTUIFlowStart_HarnessRejectedInVibe(t *testing.T) {}
```

### 10.6 `apps/desktop-flowpilot/src/state/task326_working_mode.test.ts` (mandatory)

Same commit as TUI. No skip.

```
// Scenario: chrome toggle vibe filters picker to ingest.
// Input: workingMode=vibe
// Expect: picker id set == {vibe-ingest}

// Scenario: chrome toggle normal hides vibe-*.
// Input: workingMode=dev
// Expect: picker has the five harness ids; no vibe-* ids

// Scenario: label Normal never sent on the wire.
// Input: toggle label Normal then start
// Expect: POST body working_mode is "dev" or omitted, never "normal"

// Scenario: chrome toggle persists next-start default.
// Input: set vibe; reload store
// Expect: default workingMode=vibe
```

### 10.7 `apps/local-runner/internal/agentpack/task326_vibe_pack_inventory_test.go`

Use:

```
// Scenario: builtin pack count is unchanged.
// Input: LoadBuiltinPack()
// Expect: len(Flows)==11; len(Agents)==8
func TestPack_InventoryUnchanged(t *testing.T) {}
```

Edge:

```
// Scenario: vibe flows stay hidden from the Dev selectableIn catalog.
// Input: definitions vibe-ingest, vibe-sprint, vibe-owner-debate
// Expect: each Builtin.SelectableIn is empty
func TestPack_VibeSelectableInEmpty(t *testing.T) {}
```

Error (regression of v1):

```
// Scenario: vibe-sprint topology is still v1.
// Input: vibe-sprint.yaml nodes and edges
// Expect: node ids == {preflight_contract_plan, preflight_contract_freeze, tdd, coder, synthesis}; no context/validate/audit nodes; single continue back-edge synthesis→coder
func TestPack_VibeSprintStillV1(t *testing.T) {}
```

### 10.8 Verification commands (after bodies exist)

```
cd apps/local-runner
go test ./internal/runner/ -count=1 -timeout 180s -run 'TestFlowAllowed_|TestHTTPStart_|TestPicker_|TestPersist_'
go test ./internal/tui/app/ -count=1 -timeout 180s -run 'TestTUIFlow|TestTUIVibe'
go test ./internal/agentpack/ -count=1 -timeout 60s -run 'TestPack_InventoryUnchanged|TestPack_VibeSelectableInEmpty|TestPack_VibeSprintStillV1'
go test ./internal/runner/ -count=1 -timeout 180s -run 'TestCP61HubDone|TestBugSubModeOffersNoOrchestrationAfterReviewLoopHide'
```

Old-test fail → STOP, do not edit the old test (`safe-fix-contract` R1).
