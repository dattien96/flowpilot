# CP-87 Test Steps — Quota-Aware Provider / Model Rotation

- Document ID: `CP-87-Test-Steps`
- Title: `CP-87 Test Steps`
- Phase: `coding-plan`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-87 (Quota-Aware Provider / Model Rotation)`
- Child Documents: ``
- Related Documents: `Task-445, Task-446, Task-447, Task-448, Task-449, Task-450, CP-86-Test-Steps`
- Replaces: ``
- Tags: `verification, quota, rotation, provider-parity, vibe, flow`

## AI Quick View

### Summary

- Automated contract/fixture tests cover Claude, Codex, Grok, OpenCode, Devin.
  Live evidence is limited to installed/authenticated providers: Grok + Devin;
  OpenCode free success path when available. Claude/Codex are never falsely
  reported as live-verified.
- The critical matrix is mode × run type × candidate state:
  `manual|auto × flow|vibe × same-provider|cross-provider|unknown|none`.
- Safety probes prove no global account interruption, no rapid same-provider
  cycling, no silent model-default mutation, and durable restart behavior.

### Current Ask

- Execute after Task-445 → Task-450. All §2 tests must pass before manual/live
  verification. Existing test failures are compared against baseline; new
  failures stop the task.

### Key Decisions

- Provider error fixtures enter through real mapper/adapter boundaries, not a
  direct call to the final classifier.
- Fake-account telemetry explicitly marks exact/heuristic/unknown confidence.
- Vibe is not exempt from gates: in manual mode it must park exactly like Flow.
- Auto mode is tested only with exact workload binding + healthy fresh account;
  uncertainty must route to the same user gate.

### Constraints

- Additive tests; old tests remain regression guards.
- Sanitized fixtures only — no auth token, email, home path, billing URL secret.
- Do not intentionally exhaust live provider accounts.
- Gemini excluded by CP-87 scope.

### Open Questions

- None blocking; the live matrix may skip OpenCode if the free backend is
  unavailable and record the reason.

### Source Refs

- `CP-87 §10 DoD`, `CP-86-Test-Steps`, AGENTS §§2/3/5.

## 1. Goal

Prove quota routing is typed, deterministic, bounded, restart-safe, and has
identical manual/auto semantics across Flow and Vibe — while accurately
reporting which providers were fixture-only vs live-verified.

## 2. Automated Verification

```bash
cd apps/local-runner && go test ./internal/runner/ ./internal/agentpack/ -run 'TestTask44[5-9]|TestTask450' -count=1
cd apps/local-runner && go test -race ./internal/runner/ -run 'TestTask447|TestTask448|TestTask449' -count=1
cd apps/desktop-flowpilot && npm run test:phase1
cd apps/local-runner && go test ./internal/runner/ ./internal/agentpack/ -count=1
```

| Group | Required tests | Pass criteria |
|---|---|---|
| Typed limits | `TestTask445_ClaudeFixtures`, `TestTask445_CodexFixtures`, `TestTask445_GrokFixtures`, `TestTask445_OpenCodeFixtures`, `TestTask445_DevinFixtures`, `TestTask445_RateLimitDistinctFromQuota`, `TestTask445_UnknownShapeAudited`, `test("desktop consumes typed provider_limit_reached without parsing message")` | one schema; kind/reset/retry/source/confidence preserved; TS classifier removed |
| Schema/settings | `TestTask446_WorkloadClassValidation`, `TestTask446_AllProviderNodesClassified`, `TestTask446_HeadroomNormalization`, `TestTask446_RotationModeDefaultManual`, `TestTask446_RunSnapshotsSettings`, `test("settings edits mode priority and class model bindings")` | three classes only; inline nodes omit; account telemetry never represented as token balance |
| Same-provider | `TestTask447_HubDemandUsesPinnedLeg`, `TestTask447_ChildDemandUsesResolvedNodeModel`, `TestTask447_HealthySameProviderAccountWins`, `TestTask447_CooldownBlocksRapidSwitch`, `TestTask447_CooldownPublishesStableTimestamps`, `TestTask447_MaxTwoAutoSwitchesThenGate`, `TestTask447_NoGlobalSetActiveAccount`, `TestTask447_ConcurrentClaimsDoNotOverbook` | per-leg account pin; exactly 20s cooldown; durable start/until timestamps; cap; no unrelated turn interrupt |
| Cross-provider | `TestTask448_EnumeratesRegistryExcludingCurrent`, `TestTask448_WorkloadBindingSelectsModel`, `TestTask448_MissingBindingGates`, `TestTask448_CapabilityMismatchRejected`, `TestTask448_ContextWindowTooSmallRejected`, `TestTask448_DeterministicPriorityTieBreak`, `TestTask448_NewProviderNeedsNoRouterCodeChange` | dynamic provider list; explicit class mapping; deterministic candidate/rejection reasons |
| Flow/Vibe execution | `TestTask449_ManualFlowGates`, `TestTask449_ManualVibeGates`, `TestTask449_AutoFlowRotatesExactCandidate`, `TestTask449_AutoVibeRotatesExactCandidate`, `TestTask449_AutoUnknownQuotaGates`, `TestTask449_RotationCreatesNewLeg`, `TestTask449_RestartRestoresPendingGate`, `TestTask449_RestartDoesNotReplayCommittedRotation`, `TestTask449_StopEndsStructured`, `TestTask449_PressureReset_HeadroomFail_EscalatesToRouting`, `TestTask449_PostCompactionLegOffersRotate`, `TestTask449_BudgetTrigger_ExtendNeverAuto` | Vibe obeys manual; auto only exact; new leg durable; no duplicate effects |
| UI/audit | `test("quota routing setting defaults Manual")`, `test("candidate table shows provider model workload account headroom reset confidence")`, `test("manual selection offers once/run and explicit save-default")`, `test("auto rotation notice shows requested and resolved route")`, `test("same-provider cooldown bar counts 20 seconds from server timestamps")`, `test("cooldown bar survives remount without restarting at 20 seconds")`, `TestTask450_TUICandidateTableAndSettings`, `TestTask450_TUICooldownCountdownUsesServerDeadline`, `TestTask450_AuditCorrelatesRequestedResolvedAndUsage` | complete decision context; countdown visible, accessible, restart-stable; base config unchanged unless explicit save |

## 3. Manual Test Prep

1. Runner + Desktop/TUI real builds.
2. At least two fake/sandbox accounts for one provider; Grok/Devin real accounts
   only for non-destructive status/success checks.
3. Flow + Vibe definitions with all three workload classes.
4. Toggle `quotaRotationMode` between `manual` and `auto`.

## 4. Manual Scenarios

| # | Scenario | Expected |
|---|---|---|
| M-1 | Flow, manual, current account low/exhausted | parks; table; no provider call before selection |
| M-2 | Vibe, manual, same condition | parks despite Vibe auto lifecycle |
| M-3 | Auto, healthy same-provider second account | 20s countdown bar shows safety reason and decreases from server timestamps; after zero, new account-pinned leg starts; no global interruption |
| M-4 | Auto, no same-provider account, exact cross-provider class binding | dynamic preferred provider/model selected; durable notice/audit |
| M-5 | Auto, quota unknown/stale or missing model binding | user table gate, never guessed |
| M-6 | Three rapid same-provider failures | no tight cycling; after two automatic switches the user gate appears |
| M-7 | Kill runner after decision persist/before call, restart | one resolution, one provider call; no duplicate rotation |
| M-8 | User chooses Save as step default | only explicit action updates base step; Use once/run does not |

## 5. Live Evidence Matrix

| Provider | Required evidence |
|---|---|
| Claude | sanitized fixtures + fake CLI/transport end-to-end; mark `fixture_contract` |
| Codex | sanitized fixtures + fake app-server end-to-end; mark `fixture_contract` |
| Grok | fixture failure matrix + live success/quota metadata; never force exhaustion |
| OpenCode | fake ACP failure matrix + free live success path when available |
| Devin | fixture failure matrix + live success/GetUserStatus metadata; never force exhaustion |

## 6. Log & Audit Evidence

For each scenario retain:

- typed limit event: provider, account, kind, source, confidence, reset/retry;
- quota preflight: requested provider/model/account + workload class;
- every accepted/rejected candidate and reason;
- settings policy version + manual/auto mode snapshotted on the run;
- selected route, new leg id, claim/lease revision, delivery outcome;
- CP-86 estimated prompt, `maxUsageTokens`, actual usage;
- provider evidence level: `fixture_contract | live_success | live_limit`.

No audit output may contain auth tokens, provider raw credentials, or unredacted
home paths.

## 7. Verification Complete When

- [ ] Automated groups in §2 are green, including race/recovery tests.
- [ ] Manual matrix M-1 → M-8 is complete or each environment skip has an
      explicit reason and fixture substitute.
- [ ] Claude/Codex are labeled fixture-contract only on this machine.
- [ ] Grok/Devin non-destructive live checks pass; OpenCode live success runs
      when its free backend is available.
- [ ] No global account switch interrupted an unrelated run.
- [ ] Flow and Vibe produce the same gate/auto decision for identical inputs.
- [ ] Audit evidence in §6 is complete, durable, and redacted.
