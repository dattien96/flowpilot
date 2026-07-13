# Task-231: Firebase Crashlytics Context Source And Runtime Target

## Metadata

- Document ID: `Task-231`
- Title: `Firebase Crashlytics Context Source And Runtime Target`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-13`
- Parent Documents: [CP-05-04: Firebase MCP As A Crash-Context Artifact Source](../../07-Coding-Plan/todo/CP-05-04-Firebase-Mcp.md) (`P-3`, `P-4`, `P-5`, `P-6`)
- Child Documents: `None`
- Related Documents: [Task-226: MCP Context-Source Adapter Dispatch Refactor](./Task-226-MCP-Context-Source-Adapter-Dispatch-Refactor.md) (blocker), [Task-227: Generalize MCP Prompt-Injection And Preflight](./Task-227-Generalize-MCP-Prompt-Injection-And-Preflight.md) (blocker), [Task-230: Firebase Crashlytics MCP Connection And Provider Config](./Task-230-Firebase-Crashlytics-MCP-Connection-And-Provider-Config.md) (blocker), [Task-229: Jira Issue Context Source And Runtime Target Picker](./Task-229-Jira-Issue-Context-Source-And-Runtime-Target-Picker.md) (mẫu song song)
- Replaces: `None`
- Tags: `mcp`, `firebase`, `crashlytics`, `context-source`, `context-artifact`, `runtime-question`, `prompt-injection`, `ui`

## AI Quick View

### Summary

- Đăng ký `firebase.crashlytics` trong `ContextSourceRegistry` (mirror `jiraIssueSource`/`mcpDriverSource`), opt-in, qua điểm mở rộng Task-226.
- Runtime-target resolver `resolveFirebaseTargetForRun` (mirror `resolveMCPDriverTargetForRun`) dùng `AskWorkflowQuestion` hỏi **crash issue id / app / date range**; loại `firebase.*` khỏi collect; inject bounded target note `appendFirebaseTargetPrompt` (đọc stack trace + top frames của crash đã chọn qua tool `crashlytics_get_issue` / `crashlytics_list_events`).
- UI: `contextSourceOptions` entry `{ id: "firebase.crashlytics", label: "Firebase Crashlytics" }` + sub-config target mode; wiring use-case "Investigate Crash" (thường kèm `source.excerpt` để đối chiếu code).

### Current Ask

- Cho `context_artifact.v1` bật Firebase Crashlytics làm source, runtime hỏi crash target, AI đọc crash qua Firebase MCP trong đúng scope — không dump full report vào package.

### Key Decisions

- `T-1` `firebase.crashlytics` là `ContextSource` `Deterministic()==true`, opt-in.
- `T-2` Loại `firebase.*` khỏi `FlowContextPackage` collect; chỉ inject target note (mirror `filterString` Drive/Jira).
- `T-3` Question options: crash issue id (free-text) / chọn app / date range (v1); rich picker sau.
- `T-4` Target note bounded: server `firebase`, tool `crashlytics_get_issue`/`crashlytics_list_events`, "chỉ crash issue đã chọn, đọc top frames", content cap (crash report có thể lớn — CP-05-04 `Q-5`), failure codes (contract Task-227).

### Constraints

- Blocker: Task-226, Task-227, Task-230.
- No-vector/deterministic; `PackageID` không đổi; BUG-236.
- Source chỉ hiện khi Firebase integration `status=connected` (CP-44 `P-7`).
- Read-only.

### Open Questions

- `Q-1` Content cap crash report bao nhiêu, và note hướng đọc top frames thay vì toàn bộ (CP-05-04 `Q-5`).
- `Q-2` Có mở rộng `firebase.*` (Firestore/RemoteConfig) sau không — v1 chỉ Crashlytics.

### Source Refs

- `CP-05-04` `P-3/P-4/P-5/P-6`, `Q-5`.
- `CP-44` `§11.5`, Task-194.
- current code: `context_sources_builtin.go`, `context_source_mcp.go`, `flow_executor.go` (`resolveMCPDriverTargetForRun`, filter, `appendGoogleDriveTargetPrompt`), `interactive_service.go` (`AskWorkflowQuestion`), `WorkflowsSettings.tsx` (`contextSourceOptions`).

## 1. Goal

Cho step "Investigate Crash" lấy crash context deterministic qua `context_artifact.v1`: runtime chọn crash target, AI đọc chi tiết crash qua Firebase MCP, đối chiếu code (`source.excerpt`), không nhồi report vào package.

## 2. Parent Links

- coding plan: `CP-05-04` (`P-3/P-4/P-5/P-6`)
- tech design: `SD-22` (`D-5`), `SD-23` (`D-9/D-11`)
- system spec: `SS-14` (US-9/AC-16)
- specific upstream ids: `CP-05-04 P-3`, `P-4`, `P-5`

## 3. Trigger

Sau connection (230) + refactor nền (226/227), đây là slice biến Firebase thành context source dùng được end-to-end cho use case investigate crash.

## 4. Exact Change

- `T-1` Đăng ký `firebaseCrashlyticsSource` qua điểm mở rộng Task-226; không default-enable; adapter scheme `firebase:`.
- `T-2` `resolveFirebaseTargetForRun` + workflow question (crash id / app / date); degrade khi expire/interrupt.
- `T-3` Loại `firebase.*` khỏi collect; `appendFirebaseTargetPrompt` bounded note (contract Task-227) + content cap.
- `T-4` Hint field target Firebase trên `FlowContextHints`/`BehaviorInput`.
- `T-5` UI: `contextSourceOptions` entry + sub-config target mode.
- `T-6` Wiring use-case "Investigate Crash" (`firebase.crashlytics` + `source.excerpt`).

## 5. Touched Areas

- files: `context_sources_builtin.go`, `context_source_mcp.go`, `flow_executor.go`, `mcp_prompt_instructions.go` (Firebase contract branch), `WorkflowsSettings.tsx`
- modules: runner context assembly, interactive question, prompt augmentation, desktop artifact authoring
- routes: (tùy chọn) firebase picker
- tables: none

## 6. Acceptance Check

- Tạo `context_artifact.v1` bật `firebase.crashlytics` (+ `source.excerpt`), bind INPUT, run flow → runtime hỏi crash → prompt handoff có bounded target note; package **không** chứa full crash report.
- Unknown firebase source id → fail flow-load rõ.
- Skip question → degrade.
- `go test ./apps/local-runner/internal/runner/...` xanh.

## 7. Out of Scope

- Connection/provider-config (Task-230).
- Adapter dispatch + prompt generalization (Task-226/227).
- Firestore/RemoteConfig/write.

## 8. Completion Notes

- result: **done, with one deliberate scope boundary vs. Task-229's Jira equivalent.**
  - `apps/local-runner/internal/runner/context_source_firebase.go` (new): `firebaseCrashlyticsSource` — `ContextSource` mirroring `jiraIssueSource`, opt-in, `Deterministic()==true`, using `mcpBoundedFetch`. Registered in `registerBuiltinContextSources`.
  - **Deliberately no production adapter wired** (documented in the file's package doc): unlike Jira, Firebase has no pre-existing, already-tested REST integration in this codebase to reuse for a registry-level adapter. CP-05-04 P-1/Q-1 already resolved the sole intended access path as the official `firebase-tools` MCP (`crashlytics_get_issue`/`crashlytics_list_events`) called by the AI itself during its turn. Hand-rolling an unverified Crashlytics Management API v1alpha REST client (with its own service-account JWT/OAuth token exchange) would be fabricated surface area with no way to verify correctness in this environment — so `SetFirebaseCrashlyticsAdapter` exists (mirrors `SetJiraIssueAdapter`) but is intentionally not called from `AttachRunner`. A configured crash ref with no adapter degrades cleanly to a Collect warning via the existing "no adapter configured" branch (same contract as `mcp.driver`/`jira.issue`) — verified by a dedicated test (`TestFirebaseCrashlyticsSourceNoAdapterDegrades`).
  - `flow_executor.go`: `resolveFirebaseCrashTargetForRun`/`normalizeFirebaseCrashTarget`/`appendFirebaseCrashTargetPrompt` (mirror the Jira runtime-target trio exactly); wired into `startInlineEntryChain` — filtered out of collect, target injected as a bounded prompt note naming the `firebase` MCP server and its Crashlytics tools.
  - `FlowContextHints`/`BehaviorInput` gained `FirebaseCrashRef` (mirror `JiraIssueRef`).
  - UI: `contextSourceOptions` gained `firebase.crashlytics` + a sub-note (mirrors the Jira note) explaining the runtime-question behavior and the connected-Firebase dependency.
  - Tests: `context_source_firebase_test.go` (new, 9 cases — bounded section/SourceRef, empty-ref no-op, **no-adapter degrade** (the key Task-231-specific case), adapter-error degrade, registered-not-default, no-vector guard, normalize helper, prompt-note bounded-scope + no-op). `go build ./...` clean; `go vet` clean; `go test ./internal/runner/... -run 'Firebase|firebase'` — 23 pass (14 from Task-230 + 9 new); full suite `go test ./internal/runner/...` — 1438 passed / 16 failed / 18 skipped (same 16 pre-existing/unrelated failures across all six tasks so far, zero new regressions). `tsc --noEmit` in `apps/desktop-flowpilot` clean.
- follow-ups: (1) if a deterministic, testable Crashlytics access path is ever wanted outside the AI-turn's own MCP tool calls (e.g. for the Test Console), wire a real `FirebaseCrashlyticsAdapter` then — the seam (`SetFirebaseCrashlyticsAdapter`) is already in place; (2) wire a concrete built-in "Investigate Crash" flow binding `context_artifact.v1` with `firebase.crashlytics` + `source.excerpt` enabled, as the live E2E vehicle (same follow-up shape as Task-229's).
- upstream docs updated: none required — CP-05-04 `P-3`/`P-4`/`P-5` describe exactly this shape, including the "no REST fallback, MCP-only" framing that justifies the no-adapter decision above.

## 9. Addendum (same session, later): production adapter wired

The "no adapter wired" decision above was revisited after review — deliberately leaving Firebase's registry validation-only was too conservative once a safe, verifiable way to give it a real adapter was found. Added `apps/local-runner/internal/runner/firebase_tools_mcp_client.go`: `firebaseToolsMcpAdapter`, a real MCP **client** speaking standard JSON-RPC (the same `mcpRequest`/`mcpResponse` shapes already used for the Drive/Telegram proxy *servers*) to a spawned `firebase-tools mcp --only crashlytics` subprocess — the official access path CP-05-04 already resolved on, not a hand-rolled Crashlytics REST client. Protocol building/parsing (`buildFirebaseCrashlyticsToolCallRequest`, `parseMcpToolCallTextResult`) is pure and fully unit tested; the subprocess spawn is tested end-to-end via a fake `sh`-based stand-in server (`TestFirebaseToolsMcpAdapterFetchEndToEnd`) rather than requiring `npx`/a live Firebase project. Wired into `AttachRunner` via `SetFirebaseCrashlyticsAdapter(newFirebaseToolsMcpAdapter(r))`. `context_source_firebase.go`'s package doc updated to match. This resolves CP-05-04 `DOD-4`. `DOD-7` (a live flow run against a real GCP project) remains open — that needs an actual Firebase project, not a code gap.
