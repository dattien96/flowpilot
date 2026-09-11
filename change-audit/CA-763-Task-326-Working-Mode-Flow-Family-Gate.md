# CA-763 — Vibe working_mode switch and fail-closed flow-family gate

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-326
change_type: feature
summary: Stamp working_mode on local runs; Desktop/TUI vibe|normal switch; fail-closed user-start family gate and picker
# --->8---

## Change

Task-326 / CP-60 P-1.

- `workingmode.FlowAllowedForWorkingMode` + `FlowPickerOptions` are the SSOT (TUI imports this package, not `internal/runner`).
- `POST /client/workflow-runs` accepts `workingMode` + optional `flowRef`. Empty mode → `dev`. `X-Client: desktop|tui` required for `vibe` (else `403 working_mode_client_forbidden`). Family mismatch → `400 working_mode_flow_forbidden`. Hidden ids → `400 invalid_flow_ref`. Start `runId` → `409 working_mode_pinned`.
- Persist `working_mode` on `sessions.ndjson` only. No Supabase column.
- TUI `/vibe` / `/vibe off` + VIBE chip; Desktop chrome Normal|Vibe toggle. Next-start default only; live run immutable.
- YAML `selectableIn` on vibe flows stays `[]`. Pack still 11/8. `vibe-sprint` still v1.

## Tests

New files only:

- `task326_working_mode_gate_test.go`
- `task326_working_mode_http_test.go` (Codex fakes; gate is provider-agnostic)
- `task326_working_mode_picker_test.go`
- `task326_working_mode_persist_test.go`
- `task326_tui_vibe_flow_filter_test.go`
- `task326_vibe_pack_inventory_test.go`
- `src/state/task326_working_mode.test.ts`

## Provider impact

**Case 1 agnostic.** `FlowAllowedForWorkingMode` takes no `providerKey` (grep clean on `internal/workingmode`). HTTP start uses the same gate for any provider; unit tests use Codex because DefaultProviderRegistry fakes do not implement Claude/Grok runtimes.

## Will not undo

- CA-755 hidden review-loop
- CP-58 harness picker
- CP-61 hub-done (`TestCP61HubDone` not edited)

## Residual

- Task-321 (`P-6`) / Task-323 (`P-7`) still parked for implementation: adding `vibe-cp-ingest` would fail legacy `TestLoadBuiltinPack` (`len(Flows)==11`); v2 sprint would fail new `TestPack_VibeSprintStillV1`. Unpark after P-2..P-5 and an explicit allow to update the pack-count old test.
- Manual Desktop/TUI click-through DoD not claimed.
