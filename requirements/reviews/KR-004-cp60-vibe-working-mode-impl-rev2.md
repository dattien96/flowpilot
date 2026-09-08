# KR-004: Kill-Review — CP-60 Vibe Working Mode (impl, Claim-Revision 2)

## Metadata

- Review ID: `KR-004`
- Subject: `CP-60` unit-testable impl after CA-769
- Mode: `impl`
- Claim-Revision: `2` (successor of KR-003; not Vòng 3 of the same id)
- Reviewer: triple independent pass (DoDMatrix2, CodeParity2, KillReview2) + Main debate
- Date: `2026-09-08`
- Timebox: `single pass`
- Fix policy: `no` production; additive R3 tests only
- Verdict: **`KILL_CLEAN`**

---

## 1. Claim (đóng băng)

> Unit-testable CP-60 P-1..P-7 runner+pack+Desktop/TUI are fail-closed and DoD-aligned except live/manual demo.

## 2. Artifact in-scope

Same as KR-003 plus CA-769 symbols: `allowVibeRequirement`, `vibeCoderSpawnBlocked`, `hasVibeTddSignatures`, `isVibeTestSourcePath`, `RejectNonCP` before write, reconstruct persist.

## 3. Lớp lỗi in-scope

Fail-closed start-family gate, vibe-only always-block r-requirement, Violations-first resolver, lock-before-sprint, sequential budget, TDD-signatures-before-coder, auto-detect, outcome-face mapping, persist reconstruct.

Not protocol-matrix.

## 4. OOS

| ID | Concern | Track |
| --- | --- | --- |
| O-1 | Live 3×/N× sprint demo + provider matrix click-through | operator |
| O-2 | Plan-mode KR of SS-18/SD-24 | next KR |
| O-3 | Stale CP-60 prose `7 flows` / `DefaultRules 22` | **closed CA-771** — plan now 12/8 and DefaultRules 18 |
| O-4 | Rename `*Forbidden` tests | additive-tests-only |
| O-5 | Back-edge coder reinvoke TDD re-check | signatures persist; forward path gated (A M, not C) |

## 5. Coverage boundary

- Green nghĩa là: named probes pass on current code; Claude/Codex/Grok/OpenCode Case-2 wiring independently confirmed.
- Green KHÔNG nghĩa là: live demo, docs KR-clean, DefaultRules==22, “hết bug hệ thống”.

## 6. Probes

KR-003 P-01..P-16 re-verified closed. Added:

| ID | Requirement | Verifier | Status |
| --- | --- | --- | --- |
| P-17 | MCP vibe deny when only review offered | `TestClaudeMCPVibeRequirementRejectedWhenOnlyReviewOffered` | PASS (CA-770) |
| P-18 | MCP vibe round-trip when allowed | `TestClaudeMCPVibeRequirementRoundTripWhenAllowed` | PASS (CA-770) |
| P-19 | Reconstruct awaiting-lock + idempotent | `TestVibeSession_ReconstructAwaitingLockAndIdempotent` | PASS (CA-770) |

## 7. Inventory findings

None open. KR-003 F-B1/F-B4/F-B2/F-B3/F-C2 remain closed.

## 8. Verdict + lý do stop

`KILL_CLEAN`. 3/3 reviewers 0 C. Stop: Claim-Revision 2 hard cap. No Vòng 3.

## 9. Residual risk

Live demo. Dead helpers `vibeCoderBlocked`/`hasVibeTestArtifact` unused at spawn. Synthesis 1:1 still prompt-level plus Go/TS drift inject.

## 10. Claim tiếp theo

Plan-mode KR of SS-18/SD-24. Live residual KR after operator demo.
