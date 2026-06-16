# CA-076: Fix Codex V2 Approval Decision Mapping

## Scope

- Fixed the Codex app-server approval reply path in `apps/local-runner/internal/runner/codex_adapter.go`.
- Added regression coverage in `apps/local-runner/internal/runner/codex_appserver_test.go`.
- Documented the follow-up bug as `BUG-066`.

## Completed

- Added method-aware approval response mapping so legacy app-server approval methods still receive `approved | approved_for_session | denied | abort`.
- Added v2 approval mapping for `item/commandExecution/requestApproval` and `item/fileChange/requestApproval`, returning `accept | acceptForSession | decline | cancel`.
- Added a guard so unsupported inbound server requests no longer receive a misleading generic approval response.
- Verified the live Codex `0.137.0` schema using `codex app-server generate-ts` before patching the mapper.

## Verification

- `go test ./internal/runner -run "Test(CodexAdapterApprovalRoundTrip|CodexAdapterV2ApprovalRoundTrip|CodexReviewDecisionMapsToAppServerEnum|CodexReviewDecisionMapsV2AppServerEnum)" -count=1` - pass.
- `go build ./...` from `apps/local-runner` - pass.
- `go test ./internal/runner -count=1` - ran, but unrelated pre-existing failures remain in Google Drive credential/config tests, provider-home skill merge tests, and Windows `sh` session tests.

## Residual Notes

- GitNexus MCP tools were not available in this session; impact analysis was done by local inspection of `codexAdapter.handleInbound`, the dispatcher inbound route, and the generated Codex app-server schema.
- `item/permissions/requestApproval` uses a non-decision response shape and is intentionally not solved in this bug.
