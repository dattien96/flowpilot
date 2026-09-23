# BUG-375: Devin `file_changed` never emitted — `WrittenPaths`-driven gate rules dead on devin

## Metadata

- Document ID: `BUG-375`
- Title: `Devin mutation kind arrives on tool_call (mapped to tool_started only); tool_call_update never carries kind → zero file_changed → r-ca/r-contract/r-dod/reproduce-lock dead`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-35-Context-And-Regression-Engine-Rollout](../../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md), [CP-64-Reproduce-First-TDD-Gate](../../07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md), [CP-63-IDE-Grade-LSP-Runtime](../../07-Coding-Plan/done/CP-63-IDE-Grade-LSP-Runtime.md), [CP-70-Devin-Provider-Integration](../../07-Coding-Plan/done/CP-70-Devin-Provider-Integration.md)
- Feature Keys: `ai-providers, context-regression-engine`

## AI Quick View

### Summary

- `devin_event_mapper.go` maps `tool_call` → `EventToolStarted` only (`:52-58`); `EventFileChanged` is emitted only inside `mapDevinToolCallUpdate` (`:76-100`) when the `tool_call_update` carries a mutation `kind`/title.
- Live devin wire sends `kind:"edit"/"write"` + `rawInput.file_path` ONLY on the initial `tool_call` frame; all **229** `tool_call_update` notifications carried zero `kind` fields (`BUG-LIVE-002-wire-proof.txt`). `file_changed` is therefore structurally unreachable on devin.
- `TurnResult.WrittenPaths = fin.ChangedFiles` stays empty → every WrittenPaths-driven rule is silently dead on devin: `r-ca` (change-audit), `r-contract`, `r-dod-*`, artifact rules, and the reproduce-first lock (CP-64). GitDiff-driven rules (`r-scope`, `r-code-drift`, `r-reg`) still fire.
- Observed: CP-35 L-35-2 (devin turn-1138 changed code, wrote no CA note → accepted with only warn); CP-64 BUG-LIVE-CP64-01 (reproduce lock never recorded — `frozen_contracts` v1 has no `read_only_paths`); CP-63 blocker note (devin run-1 emitted zero file_changed → `lspDiagnosticsForTurn` early-returns); CP-70 R2 known-issue ceiling.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.
- Fix direction noted by testers: derive mutation kind + paths from the `tool_call` frame (where devin actually sends `kind`/`rawInput`) instead of waiting for them on `tool_call_update`.

## Bug report

- **Symptom**: Devin turns that write/edit files emit `tool_started`/`tool_completed` but zero `file_changed` events; flow-gate `WrittenPaths` is always empty.
- **Expected**: A devin file write produces `file_changed` (like opencode/grok) → `WrittenPaths` populated → `r-ca`/`r-contract`/reproduce-lock enforce.
- **Actual**: CP-35 `run-1`/`turn-1138`: devin changed `calc.go` + added `calc_cube_test.go`, wrote NO `change-audit/` note; gate saw `diffLen=2` but emitted only `action=warn rules=[r-scope r-code-drift]` + `action=accepted` (`evt-1344`) — no `r-ca`, no reprompt despite `hasCode=true hasCA=false`. CP-64 `run-1`: zero file_changed → reproduce lock skipped entirely.
- **Impact**: The whole file-mutation enforcement surface is bypassed on devin — a code change with no CA note is accepted; reproduce-lock cannot lock the RED test read-only; LSP post-write diagnostics never trigger via devin.

## Reproduction

1. Runner with `FLOWPILOT_DEVIN_AGENT=1`, provider `devin`/`devin/swe-2-max`, `yoloMode:true`, gate `enforce`.
2. Send a turn instructing a code change and explicitly NO `change-audit/` note (cp35 `L352-devin-prompt-turn-1138.txt`, run-1).
3. Observe events: `tool_started`/`tool_completed` stream, **zero** `file_changed`; gate violation event carries only `r-scope`/`r-code-drift` warn → accepted.
4. Contrast (grok control cp35 run-745): identical scenario → `evt-953` `flow_gate_violation` `reprompt` `rules=[r-ca r-contract]` → reprompt turn-954 → CA note written → accepted.
5. Reproduce-lock variant (cp64 run-1): devin writes the failing reproduce test → `frozen_contracts.ndjson` v1 has no `read_only_paths` because the lock step never saw a file event.

## Root cause

- `apps/local-runner/internal/runner/devin_event_mapper.go:52-58` — `case "tool_call"` returns `EventToolStarted` only; `rawInput.file_path` + `kind:"edit"` on this frame are discarded for file-event purposes.
- `devin_event_mapper.go:60-61, 76-100` — `tool_call_update` → `mapDevinToolCallUpdate` is the ONLY branch that emits `EventFileChanged`, gated on `devinToolMutationKind(update)` (:112-119) which requires `kind` (or mutation title) on the update — devin never sends it there (229/229 updates lacked `kind`).
- Downstream: `flowgate/evaluate.go:36` — `r-ca` triggers on `HasCodeChangesInList(tr.WrittenPaths)`; `WrittenPaths = fin.ChangedFiles` populated only from `EventFileChanged` → permanently empty on devin. Opposite polarity of BUG-382 (opencode over-emits on denied writes).

## Evidence

- `~/fp-beds/lt-evidence/cp35/RESULT.md` (BUG-LIVE-002), `BUG-LIVE-002-wire-proof.txt` (229 tool_call_update / 0 kind), `run1-gate-violations.json` (`evt-1344` warn+accepted), `L352-devin-prompt-turn-1138.txt`, `runner.log` L432-434 (tool_call kind=edit), L2330-2332, L2489-2493.
- `~/fp-beds/lt-evidence/cp64/RESULT.md` + `BUG-LIVE-CP64-01-devin-no-file-changed-no-lock.md`, `run1-l641/*-events.json` (0 file_changed), `run1-l641/frozen_contracts.ndjson` (v1 no `read_only_paths`), `runner.log:581` (raw edit w/ `rawInput.file_path` on tool_call).
- `~/fp-beds/lt-evidence/cp63/RESULT.md` ("Blocking known issues — CP35-002": devin run-1 wrote `lsp_probe.go` fine but zero `file_changed` → `lspDiagnosticsForTurn` early-returns).
- `~/fp-beds/lt-evidence/cp70/RESULT.md` (R2 note: `file_changed`/WrittenPaths unreachable; gate reprompt fired from gate-scan, not file events).

## Severity

- high

## Completion Notes (implemented 2026-09-22, CA-916)

- Root cause: `tool_call_update` frames carry only `{toolCallId, status}` — mutation kind/paths exist only on the start `tool_call` frame, which mapped to tool_started and was dropped for file_changed purposes.
- Fix: `devinCorrelateToolNotification` enriches bare update frames with cached title/kind/locations/rawInput so `mapDevinToolCallUpdate` sees a complete frame and emits `EventFileChanged` on terminal mutation updates.
- Files: `internal/runner/devin_event_mapper.go`, `internal/runner/devin_adapter.go`.
- Tests: `bug_devin_toolcall_correlation_test.go`. Downstream gates (r-ca/r-contract/r-dod) now see mutation events on Devin turns.
