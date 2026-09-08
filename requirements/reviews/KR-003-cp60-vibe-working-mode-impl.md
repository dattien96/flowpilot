# KR-003: Kill-Review — CP-60 Vibe Working Mode (impl)

## Metadata

- Review ID: `KR-003`
- Subject: `CP-60` unit-testable impl after CA-763..CA-769
- Mode: `impl`
- Claim-Revision: `1`
- Reviewer: triple independent pass (DoDMatrix, CodeParity, KillReview) + Main debate
- Date: `2026-09-08`
- Timebox: `single pass + 1 critical-batch delta`
- Fix policy: `critical-batch` (operator: all unit-testable DoD; live demo OOS)
- Verdict: **`KILL_WITH_FINDINGS`** (0 C open after delta; live demo residual)

---

## 1. Claim (đóng băng)

> Unit-testable CP-60 P-1..P-7 runner+pack+Desktop/TUI are fail-closed and DoD-aligned except live/manual demo.

## 2. Artifact in-scope

- `workingmode`, `flowgate` r-requirement + signature card
- `runner` vibe_gate/lock/cp/sprint/requirement-outcome, gate_hook, flow_executor coder guard, session persist/reconstruct
- pack `vibe-*.yaml` + `vibe-requirement-outcome`
- TUI `/vibe` `/vibe-cp`, Desktop detect + Browse
- Tests listed in probes

## 3. Lớp lỗi in-scope

Fail-closed start-family gate, vibe-only always-block r-requirement, Violations-first resolver, lock-before-sprint, sequential budget, TDD-signatures-before-coder, auto-detect, outcome-face mapping, persist round-trip.

Not protocol-matrix.

## 4. OOS

| ID | Concern | Track |
| --- | --- | --- |
| O-1 | Live 3×/N× sprint demo + provider matrix click-through | operator live test |
| O-2 | Plan-mode KR of SS-18/SD-24 docs | next KR |
| O-3 | Stale CP-60 prose `7 flows` / `DefaultRules 22` | docs; code is 12/8 and 18 |
| O-4 | Rename `*Forbidden` tests that now allow CP ingest | additive-tests-only: keep names |

## 5. Coverage boundary

- Green nghĩa là: named probes pass; production symbols match unit-testable DoD.
- Green KHÔNG nghĩa là: live demo, docs KR-clean, pack count 7, DefaultRules==22.

## 6. Probes

| ID | Requirement | Verifier | Status |
| --- | --- | --- | --- |
| P-01 | Dev omits r-requirement | `TestEnabledRulesFor_DevOmitsRequirement` | PASS |
| P-02 | Vibe includes; not in DefaultRules | `TestEnabledRulesFor_VibeIncludesRequirement`, `TestRequirementRuleNotInDefaultRules` | PASS |
| P-03 | Green+drift / tamper / not-green | `TestRRequirement_*` | PASS |
| P-04 | Resolver requirement vs debate vs Dev | `TestClassifyVibeGate_*` | PASS |
| P-05 | Lock park/resume | `TestVibeLock_*` | PASS |
| P-06 | TUI autodetect | `TestTUIVibe_*` | PASS |
| P-07 | Detect/RejectNonCP | `TestDetectVibeEntry_*` | PASS |
| P-08 | Task plan parse | `TestCollectVibeTaskPlan_*`, `TestVibeCpSlicer_*` | PASS |
| P-09 | Budget / lock blocks sprint | `TestVibeSprint_*` | PASS |
| P-10 | TDD signatures before coder | `TestVibeCoderSpawnBlocked_*`, `TestHasVibeTddSignatures_*` | PASS after delta |
| P-11 | Pack topology | `TestPack_VibeSprint*`, `TestPack_VibeCpIngest*` | PASS |
| P-12 | Outcome face + Codex negative | `TestVibeRequirement*`, `TestCodexAdapterRejectsVibeRequirementWhenOnlyReviewOffered` | PASS after delta |
| P-13 | Admin vibe 403 | `TestHTTPStart_*` / `TestFlowAllowed_*` | PASS |
| P-14 | DefaultRules count | `TestDefaultRules` (18, not 22) | PASS as code |
| P-15 | Persist reconstruct | `TestVibeSession_ReconstructRestoresPlanAndLock` | PASS after delta |
| P-16 | Non-tech card | `TestFormatRequirementCard_*` | PASS |

## 7. Inventory findings

| ID | Sev | Kind | Evidence | Status |
| --- | --- | --- | --- | --- |
| F-B1 | C | code | Codex vibe tool gated on review flag `codex_adapter.go` | **fixed** CA-769 (`allowVibeRequirement`) |
| F-B4 | C | code | coder spawn counted any `_test.go` | **fixed** CA-769 (`vibeCoderSpawnBlocked` + signatures file) |
| F-B2 | I | code | TS `.test.ts` skipped in drift inject | **fixed** CA-769 |
| F-B3 | I | code | CP paste wrote before `RejectNonCP` | **fixed** CA-769 |
| F-C2 | I | test-gap | persist snapshot without reconstruct | **fixed** CA-769 additive test |
| F-C1 | I | doc | CA prose DefaultRules 22 vs 18 | **OOS code**; claim-revision |
| F-C3 | M | test-name | `*Forbidden` tests now allow | **OOS**; do not rename |

## 8. Verdict + lý do stop

`KILL_WITH_FINDINGS`. Unit-testable claim holds after critical-batch. Stop: no Vòng 2. Live demo remains operator.

## 9. Residual risk

- Live lock-card + N× sprint not executed here.
- Synthesis 1:1 completeness still prompt-level plus Go drift inject.
- Single-owner warn unprobed.

## 10. Claim tiếp theo

Plan-mode KR of SS-18/SD-24. Live residual KR after operator demo.
