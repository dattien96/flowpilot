# BUG-366 — Allow on FEATURE-KEYS.md scope drift returns 422 amend_failed

## Metadata

- Document ID: `BUG-366`
- Title: `Allow on doc/audit frozen-scope drift 422s — change-audit/FEATURE-KEYS.md cannot widen scope`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-10`
- Last Updated: `2026-09-10`
- Parent Documents: [Task-309: TUI And Desktop Retry/Stop/Allow For Frozen-Contract Drift](../../08-Task/done/Task-309-TUI-Desktop-Retry-Stop-Allow-For-Frozen-Contract-Drift.md), [Task-266: Enforce Frozen Scope At Coder Gate And Amendments](../../08-Task/done/Task-266-Enforce-Frozen-Scope-At-Coder-Gate-And-Amendments.md), [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- Child Documents: `None`
- Related Documents: [CA-427](../../../change-audit/CA-427-enforce-frozen-scope-at-coder-gate-and-amendments.md), [BUG-278](../done/) (CA notes exempt; FEATURE-KEYS.md is not), [BUG-327](../done/)
- Replaces: `None`
- Tags: `change-contract, frozen-scope, allow, vibe-ingest, regression`

## AI Quick View

### Summary

- Live vibe-ingest (gate-sandbox snake, grok-4.5): coder wrote `change-audit/FEATURE-KEYS.md` outside the frozen contract. The gate correctly parked (`flow scope drift: … FEATURE-KEYS.md`). Operator clicked `[Allow]` and the runner returned `422 amend_failed`: amendment path is not a concrete code target.
- `[Allow]` is advertised for any frozen-scope drift (Task-309) but `AmendFrozenContract` only accepts `IsConcreteCodeTarget` paths. `.md` files fail that predicate (`flowgate.IsDocOrAuditFile`), so Allow is a dead-end. Retry re-sees the already-written file and re-parks.
- Fix: Allow persists specific doc/audit files on `FrozenContractRecord.AllowedExtraPaths` (DeclaredPaths stays code-only). Next gate pass treats them as in-scope. Library `AmendFrozenContract` still rejects Makefile/globs (CA-427 Finding 5). First write of FEATURE-KEYS.md still parks (BUG-278 / BUG-327).

### Current Ask

- Clicking `[Allow]` on a park whose drifted paths are specific doc/audit files (especially `change-audit/FEATURE-KEYS.md`) must 200, widen frozen scope, and resume — not 422.

### Key Decisions

- `V-1` First write of `FEATURE-KEYS.md` remains drift (BUG-278: only `change-audit/CA-*.md` is auto-exempt). Allow is the operator control, not a silent exemption.
- `V-2` Doc/audit extras live on `AllowedExtraPaths`, not `DeclaredPaths`, so retrieval-locus / GitNexus targeting stay code-only.
- `V-3` `AmendFrozenContract` (library) still rejects non-concrete paths. Only `AmendFrozenContractForAllow` (the `/agent-loop/amend` Allow path) accepts specific doc/audit files. Makefile / globs / flags still 422.

### Constraints

- `feature_key: change-contract`.
- R1: additive tests only; old `TestAmendFrozenContractRejectsNonConcretePathExplicitly` / `TestAmendFlow_RejectsNonConcretePath` / `TestBUG327_FeatureKeysWriteStillBlocks` stay green and untouched.
- R2: provider-agnostic (HTTP amend + frozen-store; no `providerKey`).

### Open Questions

- Residual: extension-less files (`Makefile`) still 422 on Allow. Same CA-427 class; not in the live repro. Follow-up if an operator hits it.

### Source Refs

- Live: TUI screenshot 2026-09-10, run-654339, `gate-sandbox` snake prompt, flow `vibe-ingest`, coder `WAITING_USER_APPROVAL`, `[Allow]` → `Error: runner API error 422 (amend_failed): changecontract: amendment path "change-audit/FEATURE-KEYS.md" is not a concrete code target and cannot widen scope`.
- Code: `changecontract.AmendFrozenContract` (`frozen_scope.go`), `IsConcreteCodeTarget` (`paths.go`), `handleAmendFlow` (`interactive_handlers.go`), Task-309 Allow chip.

## 1. Issue Summary

The operator is offered `[Allow] — continue with new scope (match code changed)` on a frozen-contract scope-drift park. When the extra file is `change-audit/FEATURE-KEYS.md` (audit-logging skill registering a new feature key), Allow fails with 422 and the flow stays blocked. Retry cannot clear it because the file is already in the git diff against the freeze baseline.

## 2. Parent Links

- impacted coding plan: `CP-55` P-4 (amendment), `CP-43` F3 (Allow UX via Task-309)
- impacted tech design: `SD-21` change contract / frozen scope
- impacted system spec: `SS-13` §13 feature-key registry (`FEATURE-KEYS.md`)

## 3. Environment and Reproduction

- environment: TUI `flowpilot chat`, provider grok-4.5, workspace `/Users/tiendat/Desktop/BE/gate-sandbox`, flow `vibe-ingest`.
- reproduction steps: snake prompt → freeze a code-only contract → coder appends `change-audit/FEATURE-KEYS.md` → park → click `[Allow]`.
- frequency: deterministic whenever a frozen writer touches a specific `.md` / `change-audit/` file that is not `CA-*.md`.

## 4. Expected vs Actual

- expected: `[Allow]` unions the drifted path into frozen scope and resumes the writer.
- actual: `POST /agent-loop/amend {"paths":["change-audit/FEATURE-KEYS.md"]}` → 422 `amend_failed`; flow remains `blocked: escalate`.

## 5. Impact

- users affected: any Flow with a frozen `agent.code` writer whose Allow path includes a doc/audit file; live-hit on vibe-ingest when the coder registers a feature key.
- workflows affected: vibe-ingest, rag-harness, any flow using Task-309 Allow.
- severity: high for the live run (Allow is the only chip that matches the already-written file; Retry loops).

## 6. Root Cause

- hypothesis: Allow posts the gate-reported paths into `AmendFrozenContract`, which requires `IsConcreteCodeTarget`.
- confirmed cause: `IsConcreteCodeTarget` returns false for `flowgate.IsDocOrAuditFile` (every `*.md`, `change-audit/**`, `requirements/**`). CA-427 Finding 5 made that an explicit error instead of a silent no-op. Task-309 later wired Allow without a path for those files.
- evidence: error string is `frozen_scope.go` (`amendment path %q is not a concrete code target and cannot widen scope`); `TestBUG327_FeatureKeysWriteStillBlocks` proves the park; `TestAmendFlow_RejectsNonConcretePath` proves the 422 for non-concrete shapes.

## 7. Fix Strategy

- `F-1` Add `FrozenContractRecord.AllowedExtraPaths`. `FrozenContractScopeDrift` treats them as in-scope.
- `F-2` Add `AmendFrozenContractForAllow` used by `handleAmendFlow`: concrete paths → `DeclaredPaths`; specific doc/audit files → `AllowedExtraPaths`. Globs / flags / Makefile still error.
- `F-3` Keep `AmendFrozenContract` rejecting non-concrete paths so CA-427 tests stay the regression guard.
- `F-4` A later concrete amend copies `AllowedExtraPaths` forward so extras are not dropped.

## 8. Validation

- `V-1` New `TestBUG366_*` in `changecontract` and `runner`: FEATURE-KEYS.md Allow 200, extras persist, mixed `.go`+`.md`, Makefile still 422, first write still parks, second gate after Allow does not re-park.
- `V-2` Old `TestAmendFrozenContractRejectsNonConcretePathExplicitly`, `TestAmendFlow_RejectsNonConcretePath`, `TestBUG327_FeatureKeysWriteStillBlocks` untouched and green.

## 9. Regression Guard

- tests: `changecontract/bug366_allow_doc_amend_test.go`, `runner/bug366_amend_allow_feature_keys_test.go`
- alerts: none
- audit checks: CA-821

## 10. Follow-Up Document Updates

- upstream docs that must change: none (Task-309 intent already says Allow matches code changed; this implements it for doc/audit drift).
- notes left unchanged on purpose: CA-427 Finding 5 library rejection of Makefile/globs; BUG-278 CA-note exemption (FEATURE-KEYS.md still parks until Allow).
