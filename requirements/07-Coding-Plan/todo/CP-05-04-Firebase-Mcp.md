# CP-05-04: Firebase MCP As A Crash-Context Artifact Source

## Metadata

- Document ID: `CP-05-04`
- Title: `Firebase MCP As A Crash-Context Artifact Source`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md)
- Child Documents: [Task-230: Firebase Crashlytics MCP Connection And Provider Config](../../08-Task/todo/Task-230-Firebase-Crashlytics-MCP-Connection-And-Provider-Config.md) (`P-1`/`P-2`), [Task-231: Firebase Crashlytics Context Source And Runtime Target](../../08-Task/todo/Task-231-Firebase-Crashlytics-Context-Source-And-Runtime-Target.md) (`P-3`/`P-4`/`P-5`); prereq: [Task-226](../../08-Task/todo/Task-226-MCP-Context-Source-Adapter-Dispatch-Refactor.md) + [Task-227](../../08-Task/todo/Task-227-Generalize-MCP-Prompt-Injection-And-Preflight.md) (shared refactors, parented to CP-05-06)
- Related Documents: [CP-05-06: Jira MCP As A Context Artifact Source](./CP-05-06-Jira-MCP.md) (sibling input source — chia chung refactor adapter-dispatch + prompt-generalization), [CP-44: Pluggable Context Source Registry](../done/CP-44-Pluggable-Context-Source-Registry.md), [CP-45: Generic Artifact Types And Instances](../done/CP-45-Generic-Artifact-Types-And-Instances.md), [CP-05-03: Google Drive MCP Current Implementation Notes](../done/CP-05-03-Driver-Mcp.md) (mẫu provider-CLI-owns-MCP), [CP-05-05: Telegram MCP](./CP-05-05-Tele-Mcp.md)
- Replaces: `None`
- Tags: `mcp`, `firebase`, `crashlytics`, `context-source`, `context-artifact`, `artifact-framework`, `flow-mode`

## AI Quick View

### Summary

- CP-05-04 gắn **Firebase** vào artifact framework như một **context source cho INPUT step**, y hệt mô hình Jira (CP-05-06) và `mcp.driver` (CP-44/CP-45): đăng ký một `ContextSource` mới (`firebase.crashlytics`) trong `ContextSourceRegistry`, opt-in, cho `context_artifact.v1` tick chọn — **không** thêm artifact type mới.
- Use case đích: **Investigate Crash On Firebase** — một step bind `context_artifact.v1` instance có bật `firebase.crashlytics` (thường kèm `source.excerpt` để AI đối chiếu stack trace với code), runtime hỏi **crash issue / app / date range**, rồi AI dùng Firebase MCP tools đọc chi tiết crash trong đúng scope.
- Giữ đúng 2 lớp: **Connection tĩnh (per-project)** = Firebase project + app + service-account credential (Surface ①); **Runtime target động (per-run)** = user chọn Crashlytics issue id / app / khoảng thời gian (Surface ④) qua `AskWorkflowQuestion`.
- Firebase **bị loại khỏi collect-path** của `FlowContextPackage`; runner chỉ inject *target note* để AI tự fetch bằng Firebase MCP tools — **không** dump toàn bộ crash report vào package.
- Chia chung với CP-05-06 hai refactor tiên quyết: **adapter/source scheme-dispatch** (`P-5` của Jira) và **prompt-injection generalization** (`P-6` của Jira). CP-05-04 chỉ thêm branch Firebase, không lặp lại refactor.

### Current Ask

- Thiết kế + lên kế hoạch triển khai Firebase (Crashlytics) như một MCP-backed context source cắm vào `context_artifact.v1`, với runtime-target picker (crash issue/app/date) và prompt-injection theo mẫu `mcp.driver`/Jira, phục vụ use case investigate crash.

### Key Decisions

- `P-1` **Firebase là context source, không phải artifact type mới.** Đăng ký `firebase.crashlytics` trong `ContextSourceRegistry` ([context_sources_builtin.go:141](../../../apps/local-runner/internal/runner/context_sources_builtin.go:141)), opt-in (không trong `defaultContextSourceIDs`), mirror `mcpDriverSource`/`jiraIssueSource`.
- `P-2` **Transport = Firebase MCP + provider-CLI-owns-MCP** (CP-05-03 §11). Ứng viên: Firebase/Crashlytics MCP server (community hoặc FlowPilot-owned proxy mirror `google_drive_proxy_mcp.go`). Runner config provider, preflight, inject prompt-instruction; provider CLI là MCP client. `Q-1` chốt server cụ thể.
- `P-3` **Tách Connection (tĩnh) vs Runtime target (động)** giống `mcp.driver`/Jira: project/app/credential cấu hình 1 lần ở Surface ①; crash issue/app/date chọn per-run ở Surface ④ qua `AskWorkflowQuestion` ([interactive_service.go:3827](../../../apps/local-runner/internal/runner/interactive_service.go:3827)).
- `P-4` **Firebase bị loại khỏi collect-path**; inject *target note* bounded qua `appendFirebaseTargetPrompt` (mirror `appendGoogleDriveTargetPrompt`), **không** append full crash report vào `FlowContextPackage`.
- `P-5` **Tái dùng refactor adapter-dispatch của CP-05-06** (`P-5` Jira) — Firebase chỉ thêm một adapter/scheme (`firebase:`), không tự làm lại.
- `P-6` **Tái dùng prompt-generalization của CP-05-06** (`P-6` Jira) — chỉ thêm Firebase MCP contract (server name, tool allowlist, failure codes) + `PreflightFirebaseMcp`.
- `P-7` **Read-only v1** (đọc crash issue/detail/stack); không ghi Firebase.
- `P-8` **Runtime picker v1 = free-text crash issue id + select app/date range**; rich picker sau (`Q-3`).

### Constraints

- Kế thừa bất biến CP-41/CP-44: deterministic, **no vector/embedding/similarity**; Firebase source lookup tường minh theo crash id, mang `SourceRef`, degrade-mềm khi Firebase down.
- Không phá `FlowContextPackage`/`PackageID`; Firebase chỉ qua prompt-note (P-4).
- **Secret boundary (SD-11 §6):** Firebase service-account JSON / OAuth token chỉ ở runner keyring ([secret_store.go](../../../apps/local-runner/internal/runner/secret_store.go)); **không** vào Supabase `config_encrypted`, không log.
- Nguồn ngoài chỉ tới MCP đã connect (CP-44 `P-7`); options giới hạn theo integration `status=connected`.
- Giữ BUG-236 (binding ở `step_definitions`) + `contextSourceOptions` đồng bộ tay với Go registry.

### Open Questions

- `Q-1` **(RESOLVED 2026-07-13 → official `firebase-tools` MCP)** Dùng `npx -y firebase-tools@<pinned> mcp --only crashlytics` (Google-maintained, 4.4k★; expose `crashlytics_get_issue`/`crashlytics_list_events`/`crashlytics_batch_get_events`; gap fetch-crash-report cũ đã đóng — issues #8676/#9876 closed). **Pin version** vì tool Crashlytics còn nhãn Experimental. Community BigQuery server (`tjdam007`) chỉ giữ cho analytics-style query. Xem [Task-230](../../08-Task/todo/Task-230-Firebase-Crashlytics-MCP-Connection-And-Provider-Config.md).
- `Q-2` Credential: Firebase service-account JSON hay OAuth? Crashlytics API cần scope gì? Ảnh hưởng form Surface ① (hiện chỉ `projectId, environment`).
- `Q-3` Runtime picker v1 free-text crash id đủ chưa, hay cần list top crashes qua MCP?
- `Q-4` `firebase.crashlytics` có mở rộng thành `firebase.*` (Firestore, RemoteConfig, Performance) không? v1 chỉ Crashlytics.
- `Q-5` Crash report có thể rất lớn — cap content bao nhiêu (mirror `mcpDriverContentCap` 8 KB)? Và target note nên hướng AI đọc stack trace + top frames, không toàn bộ.

### Source Refs

- `SD-11` §2 (Add MCP sequence), §3 (provider strategies), §6 (secret boundary). Ghi chú: SD-11 chưa có mục Firebase riêng — CP-05-04 dùng chung mô hình proxy/remote MCP; cần bổ sung SD-11 khi Task land.
- `SD-23` `D-5` (`context_artifact.v1` compose lên registry), `D-9` (deterministic/no-vector), `D-11` (prompt-assembly seam).
- `SD-22` `D-1/D-2/D-5`.
- `CP-44` `P-5/P-7`, `§11.5/§11.6`.
- `CP-05-03` §11 (provider-CLI-owns-MCP), §11.10/§11.11 (prompt + preflight).
- `CP-05-06` `P-5` (adapter-dispatch refactor), `P-6` (prompt generalization) — chia chung.
- current code: `context_source_mcp.go`, `context_sources_builtin.go`, `flow_executor.go`, `mcp_prompt_instructions.go`, `runner.go` (`mcpBackendSpecs` — chưa có Firebase backend), `settingsHelpers.ts` (`providerFields.firebase`), `WorkflowsSettings.tsx` (`contextSourceOptions`).

## 1. Goal

Cho step "Investigate Crash On Firebase" (và các step cần crash context khác) khả năng lấy dữ liệu Crashlytics **có kiểm soát, deterministic, auditable** qua Firebase MCP, bằng cách gắn Firebase vào artifact framework như một context source của `context_artifact.v1` — tái dùng nguyên mô hình `mcp.driver`/Jira (connection tĩnh + runtime target động + prompt-note + provider-CLI-uses-MCP), không thêm artifact type mới, không phá `FlowContextPackage`.

## 2. Input Documents

- `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`
- `requirements/06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md`
- `requirements/07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md`
- `requirements/07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md`
- `requirements/07-Coding-Plan/done/CP-05-03-Driver-Mcp.md`
- `requirements/07-Coding-Plan/todo/CP-05-06-Jira-MCP.md`
- `apps/local-runner/internal/runner/context_source_mcp.go`
- `apps/local-runner/internal/runner/mcp_prompt_instructions.go`
- `apps/local-runner/internal/runner/flow_executor.go`
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
- `apps/desktop-flowpilot/src/components/settings/settingsHelpers.ts`

## 3. Implementation Strategy

- overall approach:
  - Clone pipeline `mcp.driver`/Jira cho Firebase: connection (integration `firebase` + service-account keyring) → register `firebase.crashlytics` source → runtime-target resolver + question (crash id/app/date) → prompt-note → provider-config injection cho Firebase MCP.
  - Runner không parse crash report; nó config Firebase MCP cho provider, preflight, inject `## Required MCP Usage` + target note, launch CLI.
- sequencing logic:
  - Phụ thuộc CP-05-06 land trước cho `P-5` (adapter-dispatch) + `P-6` (prompt-generalization). Sau đó: connection `P-1` → chọn MCP server `Q-1` → provider-config `P-2` → register source `P-3` → UI `P-4` → runtime target/question → use-case → tests.
- dependencies:
  - CP-44/CP-45 (done). CP-05-06 (adapter-dispatch + prompt-generalization refactors).
  - Node.js/credential runtime cho Firebase MCP server (tùy `Q-1`).

## 4. Work Breakdown

- `P-1` **Connection layer.**
  - `integrations` type `firebase` đã trong CHECK constraint; form hiện `projectId, environment` ([settingsHelpers.ts:46](../../../apps/desktop-flowpilot/src/components/settings/settingsHelpers.ts:46)) — bổ sung **service-account JSON upload / app id** (theo `Q-2`), lưu credential ở keyring `firebase:<integrationId>` (mirror `jiraCredential`). Thêm `mcpBackendSpec` firebase (hiện chưa có — chỉ google_drive + jira).
  - Verify: gọi Crashlytics API nhẹ để đổi status `connected`.
- `P-2` **Firebase MCP + provider-config injection.**
  - Chọn server (`Q-1`); thêm `firebase_mcp_provider_config.go` mirror `google_drive_mcp_provider_config.go` cho codex/gemini/claude/grok + Claude `--strict-mcp-config` extra server.
- `P-3` **Register `firebase.crashlytics` source** (`firebaseCrashlyticsSource` mirror `mcpDriverSource`; `Deterministic()==true`; không default). Thêm hint field target (mirror `MCPDriverRef`).
- `P-4` **UI: source + sub-config.**
  - Thêm `{ id: "firebase.crashlytics", label: "Firebase Crashlytics" }` vào `contextSourceOptions` ([WorkflowsSettings.tsx:64](../../../apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx:64)); sub-config: target mode (`ask`/`fixed`/`from-prompt`), chọn connected Firebase integration + default app.
- `P-5` **Runtime target resolver + question.**
  - `resolveFirebaseTargetForRun` mirror `resolveMCPDriverTargetForRun`; question options = crash issue id (free-text) / chọn app / date range; `filterString` loại `firebase.*` khỏi collect; `appendFirebaseTargetPrompt` inject note bounded (đọc stack trace + top frames của crash đã chọn, không broad).
- `P-6` **Use-case wiring:** built-in/pack step "Investigate Crash" bind `context_artifact.v1` (`firebase.crashlytics` + `source.excerpt`).
- `P-7` **Test Console (CP-05-02):** thêm Firebase read action (`firebase_get_crash`) khi shell sẵn.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/context_source_mcp.go` (thêm `firebaseCrashlyticsSource` + scheme `firebase:`)
  - `apps/local-runner/internal/runner/context_sources_builtin.go` (register)
  - `apps/local-runner/internal/runner/flow_executor.go` (`resolveFirebaseTargetForRun`, `appendFirebaseTargetPrompt`, filter)
  - `apps/local-runner/internal/runner/mcp_prompt_instructions.go` (Firebase contract branch — trên nền generalization của CP-05-06)
  - mới: `apps/local-runner/internal/runner/firebase_mcp_provider_config.go` (+ test)
  - `apps/local-runner/internal/runner/runner.go` (Firebase backend spec + keyring cred + verify)
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (`contextSourceOptions` + sub-config)
  - `apps/desktop-flowpilot/src/components/settings/settingsHelpers.ts` (Firebase fields: service-account/app id)
- modules: runner context assembly, MCP provider-config, prompt augmentation, interactive question, desktop settings.
- database: không thêm bảng (type `firebase` đã hợp lệ).
- external systems: Firebase/Crashlytics API qua MCP server (`Q-1`).

## 6. Data or Migration Steps

- schema: không migration DB.
- data backfill: không.
- config updates: provider MCP config per-account-home; `context_artifact.v1` instance mang `firebase.crashlytics` + target-mode sub-config.

## 7. Validation Plan

- tests to add:
  - `TestFirebaseCrashlyticsSourceRegisteredDeterministic` — đăng ký, deterministic, không default.
  - `TestResolveFirebaseTargetForRun*` — ask/fixed/degrade (mirror Drive/Jira).
  - `TestAppendFirebaseTargetPromptBoundedScope` — note giới hạn crash đã chọn; failure codes.
  - `TestFirebaseExcludedFromContextPackageCollect`.
  - `TestEnsureFirebaseMcpProviderConfig*` — write/stale.
  - `TestValidateFlowArtifactBindingsUnknownFirebaseSourceFailsFast`.
- manual checks:
  - Connect Firebase (service account) → `connected`; tạo instance `firebase.crashlytics` + `source.excerpt`; bind vào step Investigate Crash; run → runtime hỏi crash id → AI đọc stack trace qua MCP + đối chiếu code; package không dump full report.
- failure cases:
  - Chưa connect → source ẩn / preflight chặn.
  - Credential invalid → `MCP_AUTH_REQUIRED`.
  - Skip question → degrade, step chạy bằng source còn lại.
  - Crash report quá lớn → cap content + note hướng đọc top frames (`Q-5`).

## 8. Rollout and Fallback

- rollout order: sau CP-05-06 (`P-5/P-6` sẵn) → connection → server chọn → provider-config → source+UI (flag) → runtime question → use-case.
- fallback path: Firebase down → warning + degrade; step vẫn chạy bằng source khác.
- monitoring: log source enabled, resolve time, target chosen (crash id, không secret), preflight fail reason.

## 9. Risks

- `R-1` **Chưa chốt MCP server (`Q-1`).** Rủi ro chọn sai/community server không ổn định. Mitigation: khảo sát + PoC trước Task; giữ tùy chọn FlowPilot proxy.
- `R-2` **Credential model phức tạp** (service account vs OAuth). Mitigation: `Q-2` chốt sớm; keyring-only.
- `R-3` **Crash report lớn** phá bounded-context. Mitigation: content cap + target note hướng top frames (`Q-5`).
- `R-4` **Phụ thuộc refactor CP-05-06.** Nếu CP-05-06 chưa land, Firebase phải tự làm `P-5/P-6`. Mitigation: sequence sau Jira, hoặc nâng refactor thành CP dùng chung.
- `R-5` **Secret leak.** Mitigation: keyring-only, redact log, không vào Supabase.
- `R-6` **Determinism erosion.** Mitigation: lookup tường minh theo crash id; giữ `No vector retrieval used`.

## 10. Definition of Done

- [x] `DOD-1` `firebase.crashlytics` đăng ký trong registry, opt-in, deterministic, có test; hiện trong `contextSourceOptions` khớp registry. — Task-231, `context_source_firebase_test.go`.
- [x] `DOD-2` Connect Firebase; credential chỉ ở keyring, không Supabase/log; status `connected`. — Task-230, keyring + structural service-account JSON validation, test thật (`TestTriggerIntegrationConnectionFirebaseConnectsWithValidServiceAccount`). **"End-to-end" ở mức structural/local** — chưa gọi live GCP/Crashlytics API thật để verify credential (cần project GCP thật).
- [x] `DOD-3` Firebase MCP server chốt (`Q-1` → official `firebase-tools` MCP) + provider-config injection cho Claude, stale detection có test. — Task-230.
- [ ] `DOD-4` **Chưa xong (theo đúng nghĩa).** `context_artifact.v1` bật `firebase.crashlytics` bind vào step → runtime hỏi crash target → prompt handoff có target note bounded. — Cơ chế + prompt-note + degrade path đã test (Task-231), nhưng **chưa chạy qua một step/flow thật** để chứng minh "bind vào step" end-to-end; production adapter cố ý chưa wire (xem CP §Q-1/note Task-231).
- [x] `DOD-5` Prompt/preflight có branch Firebase (trên nền generalization CP-05-06); preflight chặn khi chưa connect với lỗi rõ. — Task-227/230.
- [x] `DOD-6` Guard bất biến: no-vector/deterministic, `PackageID` không đổi, BUG-236, source ngoài chỉ MCP đã connect. — giữ nguyên, có test (`TestFlowContextPackageStillHasNoVectorDependencyWithFirebaseSource`).
- [ ] `DOD-7` **Chưa xong.** Use-case "Investigate Crash" chạy live end-to-end (AI đọc crash qua MCP + đối chiếu code). — chưa wire flow cụ thể, chưa có GCP/Crashlytics thật để chạy live.
- [x] `DOD-8` Read-only v1 enforced. — `buildFirebaseMcpInstructions` chỉ liệt kê tool đọc Crashlytics.
