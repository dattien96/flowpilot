# Task-230: Firebase Crashlytics MCP Connection And Provider Config

## Metadata

- Document ID: `Task-230`
- Title: `Firebase Crashlytics MCP Connection And Provider Config`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-13`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-05-04: Firebase MCP As A Crash-Context Artifact Source](../../07-Coding-Plan/todo/CP-05-04-Firebase-Mcp.md) (`P-1`, `P-2`), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- Child Documents: `None`
- Related Documents: [CP-05-03: Google Drive MCP Current Implementation Notes](../../07-Coding-Plan/done/CP-05-03-Driver-Mcp.md) (mẫu provider-config injection), [Task-227: Generalize MCP Prompt-Injection And Preflight](./Task-227-Generalize-MCP-Prompt-Injection-And-Preflight.md), [Task-228: Jira Remote-MCP Connection And Provider Config](./Task-228-Jira-Remote-MCP-Connection-And-Provider-Config.md) (mẫu song song)
- Replaces: `None`
- Tags: `mcp`, `firebase`, `crashlytics`, `connection`, `provider-config`

## AI Quick View

### Summary

- Nối Firebase qua **official `firebase-tools` MCP** — launch `npx -y firebase-tools@latest mcp --only crashlytics` (Google-maintained, 4.4k★; đã confirm expose Crashlytics read tools `crashlytics_get_issue`, `crashlytics_list_events`, `crashlytics_batch_get_events`). **Pin version cụ thể** vì các tool Crashlytics còn nhãn Experimental.
- Auth = ADC / service-account phù hợp luồng headless của FlowPilot (không phụ thuộc `firebase login` tương tác). Credential lưu ở runner keyring, env-inject cho MCP server.
- Provider-config injection (mirror `google_drive_mcp_provider_config.go`) cho ≥1 provider CLI + Claude `--strict-mcp-config` extra server.

### Current Ask

- Cho user connect Firebase (service account) và inject config để ≥1 provider CLI thấy MCP server `firebase` (scope crashlytics).

### Key Decisions

- `T-1` Server = **official firebase-tools MCP** (`Q-1` của CP-05-04 → RESOLVED bởi khảo sát 2026-07-13). Không dùng community BigQuery server (niche); giữ nó làm ghi chú tương lai cho analytics.
- `T-2` Launch `npx -y firebase-tools@<pinned> mcp --only crashlytics`; **pin version**, không float `@latest` (Experimental caveat).
- `T-3` Auth = service-account JSON (ADC) trong keyring `firebase:<integrationId>`; **không** vào Supabase.
- `T-4` Provider-config injection per-account-home, stale detection như Drive.

### Constraints

- Secret boundary SD-11 §6.
- Node.js + `npx` sẵn trên runner (như Google Drive MCP).
- Read-only (không bật write/mutation tools; giới hạn `--only crashlytics` + allowlist read tools).
- Crashlytics API `v1alpha` — scope/quyền phải đủ; verify trước khi `connected`.

### Open Questions

- `Q-1` Service-account cần scope gì cho Crashlytics `v1alpha`? Xác nhận khi implement (docs Firebase).
- `Q-2` `firebase-tools mcp` chọn project bằng gì (env `GOOGLE_CLOUD_PROJECT` / `--project`)? Map vào integration `projectId`.

### Source Refs

- `CP-05-04` `P-1`, `P-2`, `Q-1`, `Q-2`.
- Survey 2026-07-13: firebase.google.com/docs/crashlytics/ai-assistance-mcp; issues #8676/#9876 closed.
- `CP-05-03` §11.3/§11.4/§11.7 (provider config mẫu).
- current code: `runner.go` (`mcpBackendSpecs` — thêm firebase), `google_drive_mcp_provider_config.go` (mẫu), `settingsHelpers.ts` (`providerFields.firebase`), `McpSettings.tsx`.

## 1. Goal

Connect Firebase qua official firebase-tools MCP (scope Crashlytics) và cấu hình provider CLI dùng được, credential an toàn trong keyring, status phản ánh verify thật.

## 2. Parent Links

- coding plan: `CP-05-04` (`P-1`, `P-2`)
- tech design: `SD-11` (§3), `SD-23` (`D-5`)
- system spec: `SS-14`
- specific upstream ids: `CP-05-04 P-1/P-2`

## 3. Trigger

Firebase context source (Task-231) cần connection + provider-config để CLI thấy MCP server `firebase`. Khảo sát đã chốt official firebase-tools MCP.

## 4. Exact Change

- `T-1` Thêm Firebase `mcpBackendSpec` (launcher npx firebase-tools, pinned) — hiện chỉ có google_drive + jira.
- `T-2` Form/connection: service-account JSON upload → keyring; verify Crashlytics access → status `connected`.
- `T-3` `firebase_mcp_provider_config.go` (+ test): `Ensure*FirebaseMcpConfig`/`check*`/`detectStale*` cho ≥1 provider + Claude extra server; scope `--only crashlytics`, read-tool allowlist.
- `T-4` Env-inject credential + project selection cho spawned MCP server.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/runner.go` (backend spec + verify + keyring cred), mới `firebase_mcp_provider_config.go` (+ test), `apps/local-runner/internal/cli/root.go` (nếu cần), `apps/desktop-flowpilot/src/components/settings/McpSettings.tsx`, `settingsHelpers.ts` (firebase fields: service-account/app id)
- modules: runner MCP backend + provider-config; desktop MCP settings
- routes: connection/verify endpoints
- tables: `integrations` (type `firebase`, đã hợp lệ)

## 6. Acceptance Check

- Connect Firebase (service account) → status `connected`; credential chỉ ở keyring (test).
- Provider config cho ≥1 CLI có MCP server `firebase` scope crashlytics; stale detection có test.
- Version firebase-tools được pin.
- `go test ./apps/local-runner/internal/runner/...` xanh.

## 7. Out of Scope

- Register `firebase.crashlytics` context source (Task-231).
- Runtime target picker / prompt-note (Task-231).
- Write/mutation Crashlytics tools; BigQuery analytics server.

## 8. Completion Notes

- result: **done — more complete than Task-228's Jira equivalent**, because Firebase auth (service-account JSON / ADC) needs no live browser OAuth handshake, so the full connection flow is real and testable end-to-end (only the *live* Crashlytics API call itself is unverifiable in this sandbox).
  - `apps/local-runner/internal/runner/firebase_mcp_provider_config.go` (new): `firebaseCredential` + keyring save/load/delete (mirrors `jiraCredential`); `validateFirebaseServiceAccountJSON` (structural validation — type/project_id/private_key/client_email — no live call, mirrors Drive's Desktop-OAuth-JSON structural checks); `resolveConnectedFirebaseCredential`; `detectFirebaseBackend`; `firebaseCredentialFilePath`/`writeFirebaseCredentialFile` (materializes the keyring JSON to a real file so `GOOGLE_APPLICATION_CREDENTIALS` can point at it, mirroring Drive's credential-file model); `EnsureClaudeFirebaseMcpConfig`/`checkClaudeFirebaseMcpConfig` (reuses the **existing typed** `claudeConfig`/`claudeMcpServer` structs from `google_drive_mcp_provider_config.go` — same stdio-launcher pattern as Drive, unlike Jira's remote-HTTP shape); `PreflightFirebaseMcp` + `buildFirebaseMcpInstructions` (Crashlytics tool names from the 2026-07-13 survey: `crashlytics_get_issue`, `crashlytics_list_events`, `crashlytics_batch_get_events`; failure code `FIREBASE_CONTENT_NOT_FOUND`), registered into Task-227's `mcpInstructionSpecs` as `"firebase"`.
  - `runner.go`: added the **first-ever** `mcpBackendSpec` entry for `firebase` (previously only `google_drive`+`jira` existed); wired a real `case "firebase":` in `TriggerIntegrationConnection` (previously firebase fell into a shared no-op placeholder branch with telegram/figma) that validates the service-account JSON, saves the credential, and marks the backend connected.
  - `types.go`: `IntegrationConnectionRequest` gained `FirebaseProjectID`/`FirebaseEnvironment`/`ServiceAccountJSON` fields.
  - **Secret-boundary fix (applies beyond just Firebase):** found and fixed a real SD-11 §6 violation while wiring the desktop UI — `McpSettings.tsx`'s `createIntegration()` was writing the *entire* `buildConfig()` output (including secret fields) straight into Supabase `config_encrypted`. Added `stripSecretFields(type, config)` in `settingsHelpers.ts` (currently scoped to `firebase.serviceAccountJson`) and changed `createIntegration()` to: (1) create the Supabase row with secrets stripped, (2) immediately call `testIntegration()` with the **full** config (secret included) so the runner receives+stores it in its keyring exactly once, (3) `updateIntegration(id, {status:"connected"})` on success. Also added `firebase` to the "show Test button" condition (previously only jira/google_drive). Renamed `providerFields.firebase` keys to `firebaseProjectId`/`firebaseEnvironment`/`serviceAccountJson` (from `projectId`/`environment`) — required because the flat JSON body sent to the runner already uses top-level `projectId` for the FlowPilot project id; reusing that key for the Firebase GCP project id would silently collide per Go's `encoding/json` duplicate-tag rule.
  - Tests: `firebase_mcp_provider_config_test.go` (new, 14 cases — service-account validation valid/empty/malformed/wrong-type/missing-fields, connect-flow reject/accept, provider-config write/idempotent/credential-file-materialized/not-connected, preflight ready/not-connected/provider-not-configured, registry dispatch). `settingsHelpers.test.ts` (+2 — `stripSecretFields` removes the right field / leaves other types unchanged). `go build ./...` clean; `go vet` clean; `go test ./internal/runner/... -run 'Firebase|firebase'` — 14 pass; full suite `go test ./internal/runner/...` — 1429 passed / 16 failed / 18 skipped (same 16 pre-existing/unrelated failures, zero regressions, count grew only by new passing tests). `tsc --noEmit` in `apps/desktop-flowpilot` clean; `node --test` on the compiled `settingsHelpers.test.js` — 10/10 pass.
  - **Not verified live (explicit, matches Task-228's documented limitation):** an actual Crashlytics API call against a real GCP project/service-account. What's real and tested: JSON structural validation, keyring storage, credential-file materialization, provider-config injection, and the Task-227 prompt/preflight dispatch — everything up to the point where `npx -y firebase-tools mcp --only crashlytics` would actually be launched by the provider CLI.
  - **Deliberately not pinned:** `firebase-tools` version is unpinned (`npx -y firebase-tools ...`), consistent with this codebase's existing convention for Google Drive's own unpinned `npx -y @piotr-agier/google-drive-mcp` — the CP-05-04 survey's "pin a known-good version" recommendation is noted as a follow-up hardening item, not fabricated as a specific version string here.
- follow-ups: (1) pin a verified `firebase-tools` version once one is confirmed against a real environment; (2) extend the `stripSecretFields`/create-then-connect-then-update-status pattern to Jira's `createIntegration()` path too — Jira's `apiToken` currently still round-trips through Supabase `config_encrypted` the same way Firebase's secret did before this fix (a pre-existing gap from CP-05-01, out of this task's scope to retroactively fix, but now clearly identified); (3) Task-231 should call `mcpBoundedFetch` for its own fetch, matching Task-229's Jira sources.
- upstream docs updated: none required — CP-05-04 `P-1`/`P-2` describe exactly this shape; the secret-boundary fix is a correction to existing (pre-CP-05-04) behavior, recorded here since it's an SD-11 §6 compliance fix, not a new design decision.
