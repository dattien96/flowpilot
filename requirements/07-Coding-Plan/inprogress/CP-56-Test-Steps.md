# CP-56 Test Steps — Terminal TUI Chat And Flow Client

## Metadata

- Document ID: `CP-56-Test-Steps`
- Title: `Terminal TUI — Test Steps And Evidence Log`
- Phase: `coding_plan`
- Status: `approved` (tracks approved CP-56; fill evidence as Tasks 278–289 land)
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-13` (added Task-290 P-8c + Task-291 YOLO write signatures)
- Parent Documents: [CP-56: Terminal TUI Chat And Flow Client](./CP-56-Terminal-TUI-Chat-And-Flow-Client.md)
- Child Documents: Task-278 … Task-291 (see CP-56 §11)
- Related Documents: [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md)
- Replaces: `None`
- Tags: `cli-tui, testing, verification`
- Feature Keys: `cli-tui`

---

## AI Quick View

### Summary

- Machine-checkable test signatures and manual acceptance steps for CP-56 phases P-0–P-9 plus P-8b/P-8c and P-2 residual Task-291.
- Prefer fake HTTP servers for unit/integration; live runner only for manual DOD rows.
- Additive tests only; runner/desktop suites must stay green without edits.

### Current Ask

- Keep this file updated as each phase lands (☑ + evidence).
- Do not mark a phase done in CP-56 until its rows here are closed.

### Key Decisions

- `T-1` Every public TUI helper gets at least one fast unit test.
- `T-2` Client JSON body shapes are locked against desktop contract field names.
- `T-3` Live manual rows need a configured account; runner may be auto-started by CLI (Desktop-parity ensure) unless `--no-start-runner`.

### Constraints

- No edits to pre-existing runner/desktop tests without operator approval.
- No new runner endpoints solely to make tests easier.

### Open Questions

- None beyond CP-56 `Q-1`–`Q-4`.

### Source Refs

- CP-56 §4–§10; desktop `HttpWsRunnerClient.ts`; `interactive_handlers.go` routes.

---

## 1. Goal

Define machine-checkable signatures, live acceptance steps, and evidence required to approve and complete CP-56 without changing runner business logic.

## 2. Input Documents

- [CP-56](./CP-56-Terminal-TUI-Chat-And-Flow-Client.md)
- [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md)
- Desktop `HttpWsRunnerClient.ts`, `store.ts`, and image normalization behavior
- Existing runner `/health`, `/providers`, `/client/*`, and SSE contracts

## 3. Implementation Strategy

- overall approach: test the TUI as a black-box client with `httptest`, deterministic stream fixtures, and stub self-executable process tests
- sequencing logic: close each P-0–P-9 automated slice before its matching live acceptance
- dependencies: configured Desktop catalogs/accounts for live checks; Windows and macOS hosts for process-detachment evidence

## 4. Work Breakdown

### 4.1 Automated test signatures (by phase)

### P-0 — Client

| ID | Signature | Package | Status |
|---|---|---|---|
| A0.1 | `TestClientHealth_OK` | `internal/tui/client` | ☐ |
| A0.2 | `TestClientStartRun_NormalChatBodyShape` | `internal/tui/client` | ☐ |
| A0.3 | `TestClientStreamEvents_RespectsAfterSeqAndParsesTokenUsage` | `internal/tui/client` | ☐ |
| A0.4 | `TestClientSendTurn_IncludesSelectedSkillsAndYolo` | `internal/tui/client` | ☐ |
| A0.5 | `TestClientListSkills_QueryParams` | `internal/tui/client` | ☐ |
| A0.6 | `TestClientSubmitApproval_PostsDecision` | `internal/tui/client` | ☐ |
| A0.7 | `TestEnsureRunner_ReusesWhenHealthy` | `internal/tui/runnerboot` | ☐ |
| A0.8 | `TestEnsureRunner_NoStartFlag_ErrorsWhenDown` | `internal/tui/runnerboot` | ☐ |
| A0.9 | `TestWaitHealthy_TimesOut` | `internal/tui/runnerboot` | ☐ |
| A0.10 | `TestEnsureRunner_SpawnsSelfWhenDown` | `internal/tui/runnerboot` | ☐ |
| A0.11 | `TestClientSendTurnEvents_FiltersOtherTurnsAndStopsOnOwnTerminal` | `internal/tui/client` | ☐ |
| A0.12 | `TestEnsureRunner_ConcurrentBindWinnerIsReused` | `internal/tui/runnerboot` | ☐ |
| A0.13 | `TestEnsureRunner_RejectsForeignHealthPayload` | `internal/tui/runnerboot` | ☐ |
| A0.14 | `TestProviderEvent_DecodesProviderTurnIDAndGateDTOs` | `internal/tui/client` | ☐ |
| A0.15 | `TestChatCommand_RegistersOnRoot` | `internal/cli` | ☐ |
| A0.16 | `TestEnsureRunner_RejectsWorkspaceMismatchOnReuse` | `internal/tui/runnerboot` | ☐ |
| A0.17 | `TestResolveRunnerWorkspace_DoesNotUseSelectedProject` | `internal/tui/runnerboot` | ☐ |
| A0.18 | `TestSpawnAttrs_DetachedPerOS` | `internal/tui/runnerboot` | ☐ |
| A0.19 | `TestEnsureRunner_SpawnFailsFastOnForeignPortUse` | `internal/tui/runnerboot` | ☐ |

### P-1 — Chat shell

| ID | Signature | Package | Status |
|---|---|---|---|
| A1.1 | `TestModel_SubmitPrompt_StartsRunThenTurn` | `internal/tui/app` | ☐ |
| A1.2 | `TestMapEvent_MessageDeltaAppendsStreamingLine` | `internal/tui/app` | ☐ |
| A1.3 | `TestMapEvent_TurnCompletedFinalizesAssistant` | `internal/tui/app` | ☐ |
| A1.4 | `TestModel_StopKey_CallsInterrupt` | `internal/tui/app` | ☐ |
| A1.5 | `TestMapEvent_ToolAndFileRows` | `internal/tui/app` | ☐ |
| A1.6 | `TestStreamEventMsg_RearmsUntilOwnTurnTerminal` | `internal/tui/app` | ☐ |
| A1.7 | `TestResolveStartupProject_ExplicitThenCwdThenPicker` | `internal/tui/app` | ☐ |
| A1.8 | `TestResolveStartupProject_AmbiguousDoesNotPickFirst` | `internal/tui/app` | ☐ |
| A1.9 | `TestSendTurn_RetriesTransientCodesWithStableIdempotencyKey` | `internal/tui/app` | ☐ |

### P-2 — Controls

| ID | Signature | Package | Status |
|---|---|---|---|
| A2.1 | `TestSessionControls_YoloToggle` | `internal/tui/app` | ☐ |
| A2.2 | `TestSessionControls_ProviderLockedAfterRunStarted` | `internal/tui/app` | ☐ |
| A2.3 | `TestSendTurn_ChatModeResendsModelAndYoloEachTurn` | `internal/tui/app` | ☐ |
| A2.4 | `TestParseChatFlags_Defaults` | `internal/tui/app` | ☐ |
| A2.5 | `TestSlashProvider_Model_Reasoning` | `internal/tui/app` | ☐ |
| A2.6 | `TestOptionsFromProviders_UsesLiveModelsAndReasoningCapabilities` | `internal/tui/app` | ☐ |
| A2.7 | `TestSessionControls_RejectsUnavailableModelAndUnsupportedReasoning` | `internal/tui/app` | ☐ |
| A2.8 | `TestSessionControls_DefaultModelSendsNonNilEmptyPointer` | `internal/tui/app` | ☐ |
| A2.9 | `TestSlashYolo_GrokCallsPostureEndpointBeforeFlipping` | `internal/tui/app` | ☐ |

### P-3 — Skills

| ID | Signature | Package | Status |
|---|---|---|---|
| A3.1 | `TestSkillState_ListAndAttach` | `internal/tui/app` | ☐ |
| A3.2 | `TestSkillState_Clear` | `internal/tui/app` | ☐ |
| A3.3 | `TestSkillState_UnknownNameErrors` | `internal/tui/app` | ☐ |
| A3.4 | `TestTurnInput_IncludesPendingSkillsThenClearsAfterAccepted` | `internal/tui/app` | ☐ |
| A3.5 | `TestTurnInput_PostFailureRetainsPendingSkills` | `internal/tui/app` | ☐ |

### P-3b — Images

| ID | Signature | Package | Status |
|---|---|---|---|
| A3b.1 | `TestLoadImageAttachment_PNG` | `internal/tui/app` | ☐ |
| A3b.2 | `TestLoadImageAttachment_RejectsMissingFile` | `internal/tui/app` | ☐ |
| A3b.3 | `TestLoadImageAttachment_RejectsOversize` | `internal/tui/app` | ☐ |
| A3b.4 | `TestImageState_AttachListClear` | `internal/tui/app` | ☐ |
| A3b.5 | `TestTurnInput_IncludesAttachmentsThenClearsAfterAccepted` | `internal/tui/app` | ☐ |
| A3b.6 | `TestTurnInput_WorkflowModeOmitsAttachments` | `internal/tui/app` | ☐ |
| A3b.7 | `TestLoadImageAttachment_DownscalesLongEdgeAndSetsDimensions` | `internal/tui/app` | ☐ |
| A3b.8 | `TestImageState_RejectsNonVisionProviderAndSeventhImage` | `internal/tui/app` | ☐ |
| A3b.9 | `TestTurnInput_PostFailureRetainsPendingImages` | `internal/tui/app` | ☐ |
| A3b.10 | `TestLoadImageAttachment_RejectsRawOrDecodedMemoryBomb` | `internal/tui/app` | ☐ |

### P-4 — Flow / Step

| ID | Signature | Package | Status |
|---|---|---|---|
| A4.1 | `TestResolveFlowRef_BuiltinPack` | `internal/tui/app` | ☐ |
| A4.2 | `TestResolveFlowRef_WorkflowCatalog` | `internal/tui/app` | ☐ |
| A4.3 | `TestResolveFlowRef_MissingHintsDesktopSettings` | `internal/tui/app` | ☐ |
| A4.4 | `TestResolveStepRef_ByName` | `internal/tui/app` | ☐ |
| A4.5 | `TestLaunchArm_FirstTurnSendsSubModeAndFlowRef` | `internal/tui/app` | ☐ |
| A4.6 | `TestModeSwitch_ClearsIncompatibleArm` | `internal/tui/app` | ☐ |
| A4.7 | `TestCatalogResolvers_FilterProjectAndWorkflowClientSide` | `internal/tui/app` | ☐ |
| A4.8 | `TestLaunchArm_RejectsChangeAfterRunStarted` | `internal/tui/app` | ☐ |
| A4.9 | `TestResolveFlowRef_SubModeCarriedFromMatchedOption` | `internal/tui/app` | ☐ |
| A4.10 | `TestLaunchArm_FirstTurnSendsChangeTypeBugfix` | `internal/tui/app` | ☐ |

### P-5 — Statusline

| ID | Signature | Package | Status |
|---|---|---|---|
| A5.1 | `TestFormatUsageLine_ContextAndLastTurn` | `internal/tui/app` | ☐ |
| A5.2 | `TestStatusModel_RefreshAccount_PicksActive` | `internal/tui/app` | ☐ |
| A5.3 | `TestStatusModel_View_TruncatesGracefully` | `internal/tui/app` | ☐ |
| A5.4 | `TestStatusModel_ApplyTokenUsage_UpdatesSegments` | `internal/tui/app` | ☐ |
| A5.5 | `TestTerminalEvent_RefreshesActiveAccount` | `internal/tui/app` | ☐ |
| A5.6 | `TestFormatUsageLine_UsesSelectedModelContextFallback` | `internal/tui/app` | ☐ |
| A5.7 | `TestProviderAccountSummary_DecodesSnakeCase` | `internal/tui/client` | ☐ |

### P-6 — Agents

| ID | Signature | Package | Status |
|---|---|---|---|
| A6.1 | `TestProjectAgents_OrdersMainFirst` | `internal/tui/app` | ☐ |
| A6.2 | `TestFocusAgent_DisablesSendOnChild` | `internal/tui/app` | ☐ |
| A6.3 | `TestFocusMain_RestoresSend` | `internal/tui/app` | ☐ |
| A6.4 | `TestCycleAgent_Wraps` | `internal/tui/app` | ☐ |
| A6.5 | `TestAgentGraphUpdated_RefreshesStatusline` | `internal/tui/app` | ☐ |
| A6.6 | `TestFocusRoundTrip_PreservesPerRunTimelineAndCursor` | `internal/tui/app` | ☐ |
| A6.7 | `TestFocusSwitch_CancelsPriorStream` | `internal/tui/app` | ☐ |
| A6.8 | `TestFocusStream_IgnoresStaleGenerationMessages` | `internal/tui/app` | ☐ |
| A6.9 | `TestChildFocus_MainStreamStillUpdatesAgentsAndTokens` | `internal/tui/app` | ☐ |

### P-7 — Gates

| ID | Signature | Package | Status |
|---|---|---|---|
| A7.1 | `TestGateFromEvent_PermissionRequired` | `internal/tui/app` | ☐ |
| A7.2 | `TestGateFromEvent_UserQuestion` | `internal/tui/app` | ☐ |
| A7.3 | `TestHandleGateAnswer_SubmitsApprovalDecision` | `internal/tui/app` | ☐ |
| A7.4 | `TestHandleGateAnswer_RejectsInvalidOption` | `internal/tui/app` | ☐ |
| A7.5 | `TestInterrupt_WhileStreaming` | `internal/tui/app` | ☐ |
| A7.6 | `TestFlowGateViolation_SubmitsAdvertisedDecision` | `internal/tui/app` | ☐ |
| A7.7 | `TestBlockedLoop_ContinueAndStopUseExistingEndpoints` | `internal/tui/app` | ☐ |
| A7.8 | `TestResolvedGateReplay_RendersReadOnly` | `internal/tui/app` | ☐ |
| A7.9 | `TestGateFromEvent_PreservesDecisionValuesAndQuestionValues` | `internal/tui/app` | ☐ |
| A7.10 | `TestGateFromEvent_UsesDecisionValueNotLabel` | `internal/tui/app` | ☐ |
| A7.11 | `TestSubmitApproval_SendsRememberOnlyForExec` | `internal/tui/client` | ☐ |
| A7.12 | `TestAnswerQuestion_UsesOptionValueFallingBackToLabel` | `internal/tui/app` | ☐ |

### P-8 — Resume / headless

| ID | Signature | Package | Status |
|---|---|---|---|
| A8.1 | `TestRunHeadless_PrintsFinalMessage` | `internal/tui/app` | ☐ |
| A8.2 | `TestRunHeadless_NonZeroOnTurnFailed` | `internal/tui/app` | ☐ |
| A8.3 | `TestResume_ReplaysFromSeqZero` | `internal/tui/app` | ☐ |
| A8.4 | `TestNewCommand_ClearsAllSessionState` | `internal/tui/app` | ☐ |
| A8.5 | `TestResume_ReplaysThroughLastEventSeqBeforeFollowingLive` | `internal/tui/app` | ☐ |
| A8.6 | `TestRunHeadless_GateInterruptsAndExitsNonZero` | `internal/tui/app` | ☐ |
| A8.7 | `TestStreamWithReconnect_UsesLastSeqAndStopsOnCancel` | `internal/tui/client` | ☐ |
| A8.8 | `TestResume_FollowupUsesResumedStepIDAndModeMetadata` | `internal/tui/app` | ☐ |

### P-8b — Codex-style assistant markdown (Task-289)

| ID | Signature | Package | Status |
|---|---|---|---|
| A8.9 | `TestRenderMarkdown_HeadingsListsAndInline` | `internal/tui/app` | ☑ |
| A8.10 | `TestRenderMarkdown_FenceUnclosedDoesNotPanic` | `internal/tui/app` | ☑ |
| A8.11 | `TestRenderMarkdown_TableFitsOrPipeFallback` | `internal/tui/app` | ☑ |
| A8.12 | `TestRenderMarkdown_CacheHitOnUnchangedWidthAndHash` | `internal/tui/app` | ☑ |
| A8.13 | `TestChatRows_ScrollDoesNotIncrementMarkdownParseCount` | `internal/tui/app` | ☑ |
| A8.14 | `TestCopyChip_StillCopiesRawMarkdownSource` | `internal/tui/app` | ☑ |
| A8.15 | `TestRenderMarkdown_HugeFenceIsBounded` | `internal/tui/app` | ☑ |

### P-8c — Long-chat load-earlier windowing (Task-290)

| ID | Signature | Package | Status |
|---|---|---|---|
| A8.16 | `TestChatWindow_ShowsNewestSixPromptGroups` | `internal/tui/app` | ☑ |
| A8.17 | `TestChatWindow_LoadEarlierRevealsPreviousPage` | `internal/tui/app` | ☑ |
| A8.18 | `TestChatWindow_PendingGateStaysVisible` | `internal/tui/app` | ☑ |
| A8.19 | `TestChatWindow_NewResetsWindow` | `internal/tui/app` | ☑ |

### P-2 residual — Chat YOLO=on ordinary write (Task-291)

| ID | Signature | Package | Status |
|---|---|---|---|
| A2.10 | `TestChatYoloOn_OrdinaryWriteDoesNotPrompt_Claude` | `internal/runner` or `internal/tui/app` | ☐ |
| A2.11 | `TestChatYoloOn_OrdinaryWriteDoesNotPrompt_Codex` | `internal/runner` or `internal/tui/app` | ☐ |
| A2.12 | `TestChatYoloOn_OrdinaryWriteDoesNotPrompt_Grok` | `internal/runner` or `internal/tui/app` | ☐ |
| A2.13 | `TestChatYoloOff_OrdinaryWriteStillGates` | `internal/runner` | ☐ |
| A2.14 | `TestChatYoloOn_AskUserStillPrompts` | `internal/runner` | ☐ |

### P-9 — Boundary

| ID | Signature | Package | Status |
|---|---|---|---|
| A9.1 | `TestPackageBoundary_TuiDoesNotImportRunnerInternals` | `internal/tui/app` | ☐ |
| A9.2 | `TestEnsureRunner_DefaultAutostartEnabled` | `internal/tui/runnerboot` | ☐ |
| A9.3 | `TestDocs_CommandAndSlashSurfaceMatchesRegisteredCommands` | `internal/tui/app` | ☐ |
| A9.4 | `TestAllTUITasksHaveCliTuiAuditEvidence` | `internal/tui/app` | ☐ |
| A9.5 | `TestLegacyConsoleStatusline_UsesASCIIFallback` | `internal/tui/app` | ☐ |

---

## 5. Touched Areas

- files: additive tests under `apps/local-runner/internal/tui/**` and `internal/cli/**`
- modules: existing `flowpilot-runner`
- database: none
- external systems: live provider accounts only in manual acceptance

## 6. Data or Migration Steps

- schema: none
- data backfill: none
- config updates: none; tests use existing runner URL/port precedence

## 7. Validation Plan

### 7.1 Commands (local)

```bash
cd apps/local-runner
go test ./internal/tui/... ./internal/cli/... -count=1
go test ./internal/runner/... -count=1
```

---

### 7.2 Manual acceptance (live runner)

Prereq: Desktop has configured project + at least one provider account. Runner may be started by CLI ensure (or already up from Desktop).

| ID | Steps | Expected | Status | Evidence |
|---|---|---|---|---|
| M1 | `flowpilot chat --project <id> --provider <p>` then send `hello` | Stream + statusline account/tokens | ☐ | |
| M2 | `/yolo off`, provoke shell/tool approval | Inline gate; `y` continues turn | ☐ | |
| M3 | `/skill list` then `/skill <name>` then prompt | Skill attached on turn (runner receives selectedSkills) | ☐ | |
| M3b | `/image ./fixture.png` then ask about it | Turn carries `attachments[]`; model can see image (vision provider) | ☐ | |
| M3c | Stop runner, run `flowpilot chat` | CLI auto-starts `runner serve`, waits healthy, then TUI works | ☐ | |
| M3d | Runner already up (Desktop), run `flowpilot chat` | Reuses existing runner; no second bind / no crash | ☐ | |
| M3e | Runner down + `flowpilot chat --no-start-runner` | Exits non-zero with clear error; no spawn | ☐ | |
| M4 | `/flow flowpilot-core-flow-pack/review-loop` + prompt | Flow starts; agents appear on statusline | ☐ | |
| M5 | Tab / `/agent` cycle to child then main | Child read-only; main can send | ☐ | |
| M6 | `/step <workflow>/<step>` launch | Step run starts without Desktop open for chat | ☐ | |
| M7 | Empty flows project: `/flow list` | Hint to Desktop Settings, no crash | ☐ | |
| M8 | Change `/provider` after first turn | Rejected; suggest `/new` | ☐ | |
| M9 | `flowpilot chat -p "ping" --project <id> --provider <p>` | Prints final text, exit 0 | ☐ | |
| M10 | `--resume <runId>` | Timeline rebuilds; follow-up works | ☐ | |
| M11 | Confirm no Settings authoring in TUI | No create-workflow UI; docs state boundary | ☐ | |
| M12 | `git diff` allowlist | Only `internal/tui/**`, additive `internal/cli/chat.go`/registration, module deps, `.github/workflows/cli-tui.yml`, docs, audit | ☐ | |
| M13 | On Windows and macOS: stop runner, launch `flowpilot chat`, then exit TUI | Same executable starts detached runner; no wrapper orphan; runner remains healthy | ☐ | |
| M14 | Trigger `flow_gate_violation` and blocked loop | Advertised decision plus Continue/Stop work without Desktop | ☐ | |
| M15 | Run with multiple configured projects and no `--project` outside their paths | Project picker appears; first project is not silently selected | ☐ | |
| M16 | Run `-p` and trigger approval/question | Turn is interrupted and command exits non-zero with guidance, not timeout | ☐ | |
| M17 | Windows legacy conhost/non-UTF terminal | Viewport/status/agents use ASCII fallback without mojibake or wrap corruption | ☐ | |
| M18 | Assistant reply with heading, GFM table, fence, and `**bold**` | Readable inside `markdown` box; `[copy]` pastes raw source; scroll stays smooth on a long transcript | ☐ | |

---

### 7.3 Evidence log

| Date | Phase | Who | Result | Notes |
|---|---|---|---|---|
| | | | | |

---

## 8. Rollout and Fallback

- rollout order: P-0 through P-9; no phase closes without its automated rows
- fallback path: Desktop remains available; omit the additive `chat` command registration if rollback is needed
- monitoring: runner startup log path, CLI stderr, and evidence table

## 9. Risks

- `R-1` platform process tests pass only on one OS — require M13 evidence on Windows and macOS
- `R-2` fake SSE fixtures miss replay ordering — require multi-turn resume and live M10
- `R-3` flow mode appears complete but blocks on a desktop-only card — require A7.6–A7.9 and M14
- `R-4` test inventory drifts from Task documents — Task-288 validates the documented command/slash/test surface

## 10. Definition of Done

- [ ] All A0–A9 signatures exist and pass
- [ ] Manual M1–M18 closed with evidence
- [ ] Runner suite green without modifying pre-existing tests
- [ ] CP-56 §10 checklist complete
