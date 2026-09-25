# CP-86 Test Steps — Real Usage Accounting & Context Pressure

- Document ID: `CP-86-Test-Steps`
- Title: `CP-86 Test Steps`
- Phase: `coding-plan`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-86 (Real Usage Accounting & Context Pressure Ladder)`
- Child Documents: ``
- Related Documents: `Task-440, Task-441, Task-442, Task-443, Task-444, CP-84-Test-Steps`
- Replaces: ``
- Tags: `verification, token-usage, context-pressure, provider-parity`

## AI Quick View

### Summary

- Kế hoạch verify CP-86: hoàn toàn automated bằng Go tests (fake provider
  adapter + fake catalog) và node --test cho desktop/TUI render — **không
  cần Claude account thật**, không live test bắt buộc.
- Trọng tâm: window enrichment đúng seam, rename không silently accept key
  cũ, usage cap escalate đúng lúc (post-turn, không mid-turn), pressure
  ladder + compaction detection chỉ chạy khi flag ON.

### Current Ask

- Chạy sau khi Task-440 → Task-444 land. §2 phải xanh hết; manual §3 chỉ
  cần desktop build thật cho UI surface (Task-444).

### Key Decisions

- Mọi scenario provider dùng `fake_provider_adapter` — assert trên
  `rs.events` + persisted ndjson, không mock ở UI layer.
- Flag-off byte-identity là first-class case (giống
  `TestTask334_BudgetPackerDisabled_PromptByteIdentical`).
- Parity matrix Claude/Codex/Grok bắt buộc cho mọi path đụng provider
  (AGENTS §5); Gemini loại khỏi ma trận — documented exclusion.

### Constraints

- Baseline suite fail khớp `main` HEAD — fail mới = STOP + report.
- Không edit test cũ để xanh; rename Task-441 cho phép mechanical update
  fixture keys (`maxTokens` → `maxEstPromptTokens`) — đó là rename, không
  phải weaken assertion.
- Không có Claude account trên dev machine → parity evidence = fake
  adapter + mapper-level tests, ghi rõ trong CA entry.

### Open Questions

- Leg-rotation option trong ask_user ≥90%: nếu Task-443 descope rotation
  mechanics, test case tương ứng chuyển thành "option absent, chỉ
  continue/stop" — chốt trong Task-443 trước khi viết UI test.

### Source Refs

- `CP-86 §10 DoD`, `CP-84-Test-Steps` (format), AGENTS §5 parity contract

## 2. Automated Verification

```bash
cd apps/local-runner && go test ./internal/runner/ -run 'TestTask440|TestTask441|TestTask442|TestTask443' -count=1
cd apps/local-runner && go test ./internal/agentpack/ -run 'TestTask441' -count=1
cd apps/local-runner && go test -race ./internal/runner/ -run 'TestTask44' -count=1
cd apps/desktop-flowpilot && npm run test:phase1   # Task-444 render tests (vitest is NOT the runner)
cd apps/local-runner && go test ./internal/runner/ ./internal/agentpack/ -count=1   # regression rộng
```

| Nhóm | Test bắt buộc (đủ tên) | Pass criteria |
|---|---|---|
| Window enrichment (Task-440) | `TestTask440_UsageEventWithoutWindow_EnrichedFromCatalog`, `TestTask440_SelfReportedWindow_NeverOverridden`, `TestTask440_UnknownModel_WindowStaysNil`, `TestTask440_EnrichmentIsPureInMemory_NoIOUnderLock`, `TestTask440_ClaudeCodexGrok_Parity` | snapshot ra khỏi `emitLocked` có window từ catalog khi adapter không report; value provider-report giữ nguyên; unknown → nil |
| Rename (Task-441) | `TestTask441_MaxEstPromptTokens_Parses`, `TestTask441_LegacyMaxTokens_FailsFlowLoad`, `TestTask441_ProfileBudget_UsesRenamedField`, `TestTask441_AllBuiltinFlows_ParseClean` | key mới parse đúng; key cũ fail-closed; `flowNodeProfileBudgetFor` đọc field mới; mọi flow YAML trong pack load được |
| Usage cap (Task-442) | `TestTask442_CumulativeUsageAccumulatesPerNode`, `TestTask442_ExceedFiresAfterTurnNotMidTurn`, `TestTask442_ExceedEmitsAskUserExtendRotateStop`, `TestTask442_RotateDelegatesToRoutingGate`, `TestTask442_GateUnavailable_OffersExtendStopOnly`, `TestTask442_AutoModeNeverSelectsExtend`, `TestTask442_ExtendChoiceRaisesBudgetOnce`, `TestTask442_NoProfileOrNoCap_Uncapped`, `TestTask442_ProviderSilentOnUsage_CapNeverFires`, `TestTask442_AuditRecordsEstVsActual` | Total.TotalTokens cộng dồn đúng node; turn đang chạy xong mới escalate; card đúng options; uncapped khi không profile |
| Pressure ladder (Task-443) | `TestTask443_FlagOff_NoEventsByteIdentical`, `TestTask443_Tier80_EmitsAwarenessOnly`, `TestTask443_Tier90_EmitsAskUserRotateContinueStop`, `TestTask443_UnknownWindow_Silent`, `TestTask443_TokenDropSameLeg_EmitsProviderCompacted`, `TestTask443_PostCompaction_LegMarkedDegraded`, `TestTask443_TokenDropAcrossLeg_NotCompacted`, `TestTask443_EventsPersistedToNdjson` | đúng tier, đúng event type, durable; drop cross-leg (model/provider switch) không false-positive |
| UI surface (Task-444) | `test("step card renders prompt ~Nk est and usage Nk as separate labels")`, `test("compacted event renders inline notice, not ask card")`, `test("pressure >=80 shows banner; >=90 shows ask_user card")`, `test("usage exceeded renders ask_user extend/rotate/stop")`, `test("no usage data renders dash, not fake zero")` | đúng label semantics; awareness ≠ decision card |

## 3. Manual Test Prep

1. `local_runner` thật đang chạy (`:4317`), desktop build thật.
2. `FLOWPILOT_CONTEXT_PRESSURE=1` + `FLOWPILOT_ENABLE_BUDGET_PACKER=1`.
3. Một project có flow `task-harness` (đã rename `maxEstPromptTokens`).

## 4. Manual Scenarios

| # | Scenario | Expected |
|---|---|---|
| M-1 | Chạy flow, mở step card của node coder | thấy `prompt ~Nk est` và `usage Nk` hai label riêng |
| M-2 | Fake adapter push usage ≥80% window | banner hiện; không card; flow chạy tiếp |
| M-3 | Push usage ≥90% | ask_user card với options; chọn `continue` → run tiếp, event persisted |
| M-4 | Push usage drop (190k→18k) cùng leg | inline notice "provider compacted"; không card |
| M-5 | Set `maxUsageTokens` nhỏ, chạy node | sau turn hoàn tất → ask_user extend/rotate/stop; chọn rotate → new leg qua gate; chọn stop → node escalate structured |
| M-6 | Flag OFF restart, chạy lại flow | zero pressure/compact events; prompt byte-identical |

## 5. Parity Evidence

- Claude/Codex/Grok: fake adapter table-driven tests + mapper tests cho
  usage/window shape riêng của từng provider.
- Không có live Claude: ghi trong CA entry — coverage gap chỉ còn ở live
  behavior, schema + event path đã cover bằng fake.
