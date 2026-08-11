# CP-55 — Step Test Guide (Flow-First Preflight Contract & Canonical Acceptance)

## Metadata

- Document ID: `CP-55-TEST-STEPS`
- Title: `CP-55 Verification Steps By Phase`
- Phase: `verification`
- Status: `active`
- Owner: `FlowPilot`
- Created: `2026-08-11`
- Parent Documents: [CP-55: Flow-First Preflight Contract, Context Retrieval, And Canonical Acceptance](./CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [CP-43-Test-Steps](../inprogress/CP-43-Test-Steps.md), [CP-54-Test-Steps](../inprogress/CP-54-Test-Steps.md)
- Related Documents: Task-263…271 / CA-424…432 (xem bảng phase bên dưới)
- Tags: `flow, change-contract, preflight, canonical-acceptance, verification`

## AI Quick View

- **What:** Checklist test 9 phase CP-55 — Flow lifecycle freeze → context → writer gate → pending Canonical → terminal finalize.
- **Why:** Xác nhận guarantee chỉ áp Flow mode; Normal chat không bị phá.
- **Working dir:** `cd apps/local-runner` (Go); `npm run test:phase1 -- --tsconfig tsconfig.phase1-tests.json` (TS acceptance nodes).

## 9 phase — map test

| Phase | Nội dung | Task / CA | Verify |
|-------|----------|-----------|--------|
| **P-1** | `agent.code`, `contract.freeze`, safety topology, `acceptance_nodes` | Task-263 / CA-424 | Phần 1 |
| **P-2** | Frozen preflight model + store | Task-264 / CA-425 | Phần 2 |
| **P-3** | Planner + freeze runtime + advance chain | Task-265 / CA-426 | Phần 3 |
| **P-4** | Frozen scope gate + amendment | Task-266 / CA-427 | Phần 4 |
| **P-5** | Pending Canonical → finalize @ `done` | Task-267 / CA-428 | Phần 5 |
| **P-6** | Deterministic history scorer | Task-268 / CA-429 | → [CP-54-Test-Steps §3](../inprogress/CP-54-Test-Steps.md) |
| **P-7** | Wire ranking vào `feature.history` | Task-269 / CA-430 | → [CP-54-Test-Steps §4](../inprogress/CP-54-Test-Steps.md) |
| **P-8** | Migrate 3 built-in flows + E2E/restart/parity | Task-270 / CA-431 | Phần 8 |
| **P-9** | Docs + rollout evidence | Task-271 / CA-432 | Phần 9 |

---

## Phần 1 — Writer semantics & safety topology (P-1)

**Mục tiêu:** Behavior IDs tồn tại; validator bắt freeze dominate writer; acceptance boundary; `acceptance_nodes` persist.

### Automated — Go

```bash
go test ./internal/agentpack/... -count=1 -run 'TestValidateFlowSafetyTopology|TestLoadFlowFS.*Acceptance|TestBuiltinRecordFromFlow|TestBuiltInCodingFlowsPassSafetyTopology|TestPreflightBeforeEveryWriter' -v
go test ./internal/runner/ -count=1 -run 'TestDefaultRegistryResolvesContractFreeze|TestAgentCode|TestContractFreezePlaceholder' -v
go test ./internal/runner/ -count=1 -run 'TestCp55AcceptanceNodes|TestRecordFromWorkflowRow.*Acceptance' -v
```

| Step | Test / area | Pass khi |
|------|-------------|----------|
| 1.1 | `TestValidateFlowSafetyTopologyRejectsWriterWithoutFreeze` | Writer không có freeze trước → fail load |
| 1.2 | `TestValidateFlowSafetyTopologyRejectsDonePathWithoutAcceptance` | Path tới done bypass acceptance → fail |
| 1.3 | `TestBuiltInCodingFlowsPassSafetyTopologyValidation` | 3 built-in flows pass |
| 1.4 | `TestLoadFlowFSParsesRootAcceptanceNodesKey` | YAML `acceptance_nodes` parse |
| 1.5 | `cp55_acceptance_nodes_persistence_test.go` | Go Supabase store round-trip |
| 1.6 | `flow_safety_topology_builtin_migration_test.go` | Mỗi flow có preflight trước writer |

### Automated — TypeScript (desktop Settings path)

```bash
npm run test:phase1 -- --tsconfig tsconfig.phase1-tests.json cp55AcceptanceNodes
```

| Step | File | Pass khi |
|------|------|----------|
| 1.T1 | `cp55AcceptanceNodesRoundTrip.test.ts` | Supabase admin repo read/save/clone `acceptanceNodes` |
| 1.T2 | `cp55AcceptanceNodesWorkflowDraft.test.ts` | `WorkflowsSettings` draft carries field |

### Manual

| Step | Hành động | Pass khi |
|------|-----------|----------|
| 1.M1 | Load flow thiếu `acceptance_nodes` + có `agent.code` | Validation error rõ ràng |
| 1.M2 | Clone workflow có acceptance nodes | Deep copy qua client Supabase |

---

## Phần 2 — Frozen preflight model & storage (P-2)

**Mục tiêu:** `FrozenStore` immutable/versioned; tách khỏi legacy `Contract`; `IsConcreteCodeTarget` shared.

### Automated

```bash
go test ./internal/changecontract/... -count=1 -run 'TestParsePreflight|TestValidatePreflight|TestFreezeContract|TestSaveFrozen|TestGetFrozenForStep|TestFrozenStore|TestNormalizeDeclaredCodePaths' -v
```

| Step | Test | Pass khi |
|------|------|----------|
| 2.1 | `TestParsePreflightDraftRejectsTrailingProse` | Strict JSON |
| 2.2 | `TestSaveFrozenRejectsMutationOfExistingContractID` | Immutable after freeze |
| 2.3 | `TestGetFrozenForStepReturnsLatestActiveVersion` | Version + supersede |
| 2.4 | `TestFrozenStoreCorruptContractsLineFailsClosed` | Strict reload |
| 2.5 | `TestContractStoreLoadsLegacyRecordsWithoutNewFields` | Legacy store không bị phá |

---

## Phần 3 — Contract planner, freeze node, advance chain (P-3)

**Mục tiêu:** Planner JSON → Go freeze → advance tới context/coder; package render khi freeze→coder trực tiếp.

### Automated

```bash
go test ./internal/runner/ -count=1 -run 'TestRunContractFreezeNode' -v
go test ./internal/runner/ -count=1 -run 'TestPreflightContractPlanTurnMarker' -v
```

| Step | Test | Pass khi |
|------|------|----------|
| 3.1 | `TestRunContractFreezeNodePersistsBeforeCoderSpawn` | Freeze trước writer |
| 3.2 | `TestRunContractFreezeNodeRejectsInvalidDraft` | Bad JSON → fail closed |
| 3.3 | `TestRunContractFreezeNodeRejectsPlannerChangedFiles` | Planner không được sửa file |
| 3.4 | `TestRunContractFreezeNodeAdvancesToContextProduce` | rag-harness shape |
| 3.5 | `TestPreflightContractPlanTurnMarkerAnchoredAgainstPrefixCollision` | Fake adapter marker an toàn |

### Manual

| Step | Hành động | Pass khi |
|------|-----------|----------|
| 3.M1 | Start `review-loop` (bug sub-mode) | Entry = `preflight_contract_plan`, không phải coder |
| 3.M2 | Demo mode (no real Codex) | Fake adapter trả planner JSON hợp lệ |

---

## Phần 4 — Frozen scope gate & amendments (P-4)

**Mục tiêu:** Coder chỉ sửa trong frozen scope; drift fail-closed; amend widens paths.

### Automated

```bash
go test ./internal/changecontract/... -count=1 -run 'TestFrozenContractScopeDrift|TestAmendFrozenContract|TestIsFrozenStoreBookkeepingPath|TestIsPendingCanonicalStoreBookkeepingPath' -v
go test ./internal/runner/ -count=1 -run 'TestFlowCoderRequiresFrozenContract|TestFlowScopeDrift|TestAmend' -v
```

| Step | Test | Pass khi |
|------|------|----------|
| 4.1 | `TestFrozenContractScopeDriftReportsUnexpectedPaths` | Out-of-scope listed |
| 4.2 | `TestAmendFrozenContractWidensDeclaredPaths` | Amendment version mới |
| 4.3 | `TestIsFrozenStoreBookkeepingPathRejectsEverythingElse` | Không exempt directory-wide |
| 4.4 | `TestFlowCoderRequiresFrozenContract` | Không freeze → không coder gate OK |

### Manual

| Step | Hành động | Pass khi |
|------|-----------|----------|
| 4.M1 | Coder sửa file ngoài frozen list | Gate block / escalate; không finalize |
| 4.M2 | Amendment thêm path → coder retry | Gate pass in-scope |

---

## Phần 5 — Pending Canonical @ terminal `done` (P-5)

**Mục tiêu:** Flow coder gate **stage** pending; **`done`** finalize; stop/fail abandon; Normal chat immediate write.

### Automated

```bash
go test ./internal/changecontract/... -count=1 -run 'TestPendingCanonical|TestStageHeadWrite|TestFinalizePartial' -v
go test ./internal/runner/ -count=1 -run 'TestCoderGateStagesPending|TestFlowDoneFinalizes|TestFlowFailureAbandons|TestFlowStopAbandons|TestFinalizePartialFailure|TestFlowDoneFinalizationFailureEscalates|TestNormalChatCanonical|TestFlowRootHubTurnWritesCanonicalHeadImmediately|TestAdHocCodingChildOutsideFlowEngine' -v
go test ./internal/runner/ -count=1 -run 'TestFrozenAgentCodeWriterStagesPending|TestFrozenAgentCodeWriterFinalizesOnTerminalDone|TestFinalizeRejectsForgedPending' -v
```

| Step | Test | Pass khi |
|------|------|----------|
| 5.1 | `TestCoderGateStagesPendingCanonicalUpdate` | Gate pass → pending, không Head file |
| 5.2 | `TestFlowDoneFinalizesCanonicalHead` | `done` → Head thật |
| 5.3 | `TestFlowFailureAbandonsPendingCanonicalUpdate` | Fail → abandon |
| 5.4 | `TestFinalizePartialFailureCommitsNoHeadInTheBatch` | Atomic multi-feature |
| 5.5 | `TestFlowDoneFinalizationFailureEscalatesLoopForOperatorAction` | Finalize fail → escalate |
| 5.6 | `TestNormalChatCanonicalPathRemainsBackwardCompatible` | Chat không pending |
| 5.7 | `TestFinalizeRejectsForgedPendingCanonicalRecord` | HMAC anti-forge |

### Manual

| Step | Hành động | Pass khi |
|------|-----------|----------|
| 5.M1 | Flow review `continue` nhiều round | Head chưa đổi giữa các round |
| 5.M2 | Flow synthesis/audit `done` | Head cập nhật đúng 1 lần |
| 5.M3 | Stop Flow giữa chừng | Pending abandoned; Head cũ giữ nguyên |

---

## Phần 8 — Built-in migration + E2E (P-8)

**Mục tiêu:** 3 flows migrated; restart safe; provider parity; Normal chat unrestricted.

### Automated

```bash
go test ./internal/runner/ -count=1 -timeout 15m \
  -run 'TestFrozenContractExistsBeforeFirstContextPackage|TestFirstCoderContextUsesCurrentFlowDeclaredPaths|TestFirstCoderContextRanksFeatureHistoryByCurrentLocus|TestValidateFailLeavesCanonicalHeadUnchanged|TestReviewContinueLeavesCanonicalHeadUnchanged|TestTerminalDoneUpdatesCanonicalHeadExactlyOnce|TestClaudePreflight|TestCodexPreflight|TestGrokPreflight|TestNormalChatRemainsUsable|TestRestartBetweenFreezeAndCoder|TestRestartBetweenCoderAndValidation|TestRestartDuringTerminalFinalization' -v
```

| Step | Test | Pass khi |
|------|------|----------|
| 8.1 | `TestFrozenContractExistsBeforeFirstContextPackage` | Freeze trước context |
| 8.2 | `TestFirstCoderContextRanksFeatureHistoryByCurrentLocus` | CP-54 ranking trong Flow |
| 8.3 | `TestValidateFailLeavesCanonicalHeadUnchanged` | rag-harness validate fail |
| 8.4 | `TestTerminalDoneUpdatesCanonicalHeadExactlyOnce` | Idempotent finalize |
| 8.5 | `TestClaudePreflightContractUsesSharedParserAndGate` | Provider parity |
| 8.6 | `TestNormalChatRemainsUsableWithoutFlowGuarantees` | Chat không bắt Flow |
| 8.7 | `TestRestartBetweenFreezeAndCoderPreservesContract` | Durable recovery |

### Manual — 3 built-in flows

| Flow | Check |
|------|-------|
| `review-loop` | plan → freeze → coder (no context node) → reviewers → synthesis → done |
| `rag-harness` | plan → freeze → context → implement → validate → audit → done |
| `context-coding-review-synthesis` | plan → freeze → context → coder → review → synthesis |

| Step | Pass khi |
|------|----------|
| 8.M1 | Mỗi flow start được từ desktop Flow/Chat picker |
| 8.M2 | Coder prompt có context package (không bare intent) |
| 8.M3 | Scope drift tại coder bị chặn |

---

## Phần 9 — Documentation (P-9)

**Mục tiêu:** CP-43/CP-54 current-state synced; evidence recorded — **không có test code**.

| Step | Pass khi |
|------|----------|
| 9.1 | CP-55 metadata status = all 9 phases done |
| 9.2 | CA-424…432 tồn tại, 1 Task + 1 CA per slice |
| 9.3 | CP-43/CP-54 cross-ref CP-55 |

---

## Full-loop E2E (một Flow session)

| # | Bước | Phase |
|---|------|-------|
| 1 | Start built-in flow | P-1/P-8 |
| 2 | Planner turn → valid JSON | P-3 |
| 3 | Freeze inline → `FrozenStore` record | P-2/P-3 |
| 4 | Context package (nếu có node) ranked history | P-6/P-7 |
| 5 | Coder in-scope → gate stage pending | P-4/P-5 |
| 6 | Review/validate fail → retry, Head unchanged | P-5/P-8 |
| 7 | Terminal acceptance → finalize Head once | P-5 |
| 8 | Normal chat turn → immediate Head (control) | P-5 |

### One-shot bundle (smoke)

```bash
go test ./internal/agentpack/... ./internal/changecontract/... -count=1 -timeout 10m
go test ./internal/runner/ -count=1 -timeout 20m \
  -run 'TestRunContractFreeze|TestFlowCoderRequiresFrozen|TestFlowDoneFinalizes|TestFrozenAgentCodeWriter|TestBuiltInCodingFlowsPassSafetyTopology|TestNormalChatRemainsUsable'
```

**Baseline regression:** full `./internal/runner/...` — kỳ vọng ~15 pre-existing failures documented in CA-431; **0 new** deterministic regressions from CP-55.

---

## Checklist tổng (operator)

| Phase | Automated | Manual |
|-------|-----------|--------|
| P-1 Topology | ☐ | ☐ 1.M* |
| P-2 Frozen store | ☐ | — |
| P-3 Freeze runtime | ☐ | ☐ 3.M* |
| P-4 Scope gate | ☐ | ☐ 4.M* |
| P-5 Pending Canonical | ☐ | ☐ 5.M* |
| P-6/P-7 Ranking | ☐ (CP-54 doc) | ☐ |
| P-8 E2E/parity | ☐ | ☐ 8.M* |
| P-9 Docs | ☐ audit | — |

**CP-55 coi verification-complete khi:** bundle trên xanh + manual full-loop 8 bước pass trên ít nhất `review-loop` và `rag-harness`.
