# CP-53 — Step Test Guide (Review Loop / Verifier Gate Leaks)

## Metadata

- Document ID: `CP-53-TEST-STEPS`
- Title: `CP-53 Verification Steps By Phase`
- Phase: `verification`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-08-11`
- Last Updated: `2026-09-08`
- Parent Documents: [CP-53: Review Loop — Bịt các lỗ rò của Verifier Gate](./CP-53-Review-Loop.md)
- Related Documents: [Task-272](../../08-Task/done/Task-272-CP53-Gate-Observability-Metrics.md) … [Task-277](../../08-Task/done/Task-277-CP53-R-Newtest-Reprompt-Rule.md), BUG-288, BUG-289, Task-155, Task-156, [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Tags: `flowgate, review-loop, verification, test-steps, cp-53`

## AI Quick View

- **What:** Checklist automated + manual cho 6 phase CP-53 (P-6→P-1→P-2→P-3→P-4→P-5).
- **Why:** Operator / agent tick pass-fail; enforce safe-fix (additive tests, stop on old fail, provider matrix for P-2).
- **Working dir:** `cd apps/local-runner` cho Go; repo root cho `scripts/gate-check`.
- **Status:** Plan-cut 2026-08-11 — **steps scaffold ready**; fill concrete `-run` names as each Task lands.

## Safe-fix gates (read before ticking)

| Gate | Rule |
|------|------|
| R1 | Old `flowgate` / `gate_hook` / review-loop tests must stay **untouched + green**. New coverage in `cp53_*` / `task27x_*` files only. |
| R2 | P-2 requires Claude+Codex+Grok matrix. Other phases: document provider-agnostic evidence. |
| R3 | Each phase needs happy + degraded + near-miss — not one happy unit. |
| Oracle | Failing old test → fix production code, do not edit assertion. |

---

## Phase map

| Phase | Task | Hole | Automated § | Manual § |
|-------|------|------|-------------|----------|
| **P-6** Observability | Task-272 | Q-A | §6 | 6.M* |
| **P-1** `gate_blind` | Task-273 | H-1, H-2 | §1 | 1.M* |
| **P-2** Done verdict | Task-274 | H-3 | §2 | 2.M* |
| **P-3** Dogfood | Task-275 | H-5 | §3 | 3.M* |
| **P-4** Waiver ledger | Task-276 | H-4 | §4 | 4.M* |
| **P-5** `r-newtest` | Task-277 | H-3 coverage | §5 | 5.M* |

**Recommended run order for verification:** same as implementation — P-6 → P-1 → P-2 → P-3 → P-4 → P-5.

---

## §6 — Observability spike (P-6 / Task-272)

**Mục tiêu:** Metric emit inspect được; không đổi pass/block semantics.

### Automated (bắt buộc khi Task-272 done)

```bash
go test ./internal/runner/ -count=1 -run 'TestCP53GateMetric|TestGateMetric' -v
```

| Step | Pass khi | Tick |
|------|----------|------|
| 6.1 | Block path emits metric event / log line | [ ] |
| 6.2 | Override-accept path emits override metric | [ ] |
| 6.3 | Missing cost fields → no panic; fallback turn-count | [ ] |
| 6.4 | Related **old** gate tests still green (untouched) | [ ] |

### Manual

| Step | Hành động | Pass khi | Tick |
|------|-----------|----------|------|
| 6.M1 | Force a gate block in a scratch workspace | `[gate-metric]` (or `.flowpilot/` artifact) shows block | [ ] |
| 6.M2 | Accept a test override (if UI available) | override metric recorded | [ ] |

---

## §1 — Baseline fail-closed / `gate_blind` (P-1 / Task-273)

**Mục tiêu:** Missing baseline / EnvError / red-at-capture không bao giờ green im lặng. Corrupt baseline vẫn BUG-288 fail-closed.

### Automated

```bash
go test ./internal/flowgate/... -count=1 -run 'TestCP53GateBlind|TestGateBlind|TestFlakyQuarantine' -v
go test ./internal/runner/ -count=1 -run 'TestCP53GateBlind|TestGateBlind' -v
# Preserve BUG-288 contracts (do not edit these tests):
go test ./internal/runner/ -count=1 -run 'TestBug288|CorruptBaseline|LoadBaseline' -v
```

| Step | Pass khi | Tick |
|------|----------|------|
| 1.1 | Missing baseline + production diff → `gate_blind` block in enforce | [ ] |
| 1.2 | Missing baseline + docs-only diff → no cry-wolf block (or warn-only per Task) | [ ] |
| 1.3 | Oracle `EnvError` → blind surfaced | [ ] |
| 1.4 | Red-at-capture + quarantine entry behaves per schema | [ ] |
| 1.5 | Corrupt baseline still fail-closed (old BUG-288 tests green) | [ ] |

### Manual

| Step | Hành động | Pass khi | Tick |
|------|-----------|----------|------|
| 1.M1 | Xóa `.flowpilot/**/test_baseline.json`, edit production file, run turn | SSE/event `gate_blind` block, không silent pass | [ ] |
| 1.M2 | Corrupt baseline JSON bytes | Block (existing corrupt path), not treated as "missing" | [ ] |
| 1.M3 | Fresh workspace first capture success | Can establish baseline without permanent blind | [ ] |

---

## §2 — Done requires machine verdict (P-2 / Task-274)

**Mục tiêu:** `review-loop` `synthesis→done` cần PASS `submit-review-outcome`.

### Automated

```bash
go test ./internal/runner/ -count=1 -run 'TestCP53ReviewDoneVerdict|TestReviewLoopDoneRequires|PreflightContractUsesShared' -v
```

| Step | Pass khi | Tick |
|------|----------|------|
| 2.1 | No verdict → cannot transition to done | [x] |
| 2.2 | FAIL verdict → cannot done | [x] |
| 2.3 | PASS verdict → done allowed | [x] |
| 2.4 | `continue` path still works | [x] |
| 2.5 | **Matrix** Claude + Codex + Grok (fake adapters) all green | [x] |
| 2.6 | Normal chat without review-loop unaffected | [x] |

### Manual

| Step | Hành động | Pass khi | Tick |
|------|-----------|----------|------|
| 2.M1 | Start bug `review-loop`; synthesizer tries done without tool PASS | Stuck / continue / escalate — not terminal done | [ ] |
| 2.M2 | Reviewers submit PASS via tool | Can reach done | [ ] |
| 2.M3 | Repeat 2.M1–2.M2 on second provider if live available | Same enforce | [ ] |

**Provider class:** shared flow runtime — R2 matrix mandatory.

---

## §3 — Dogfood gate-check (P-3 / Task-275)

**Mục tiêu:** Repo FlowPilot tự chặn commit regression (Go hard; TS independent).

### Automated / scripted

```bash
# From repo root (names finalized in Task-275):
./scripts/gate-check --help
./scripts/gate-check --go-only   # expect 0 on clean tree
```

| Step | Pass khi | Tick |
|------|----------|------|
| 3.1 | Clean tree → gate-check exit 0 (Go) | [ ] |
| 3.2 | Intentional Go regression fixture → non-zero | [ ] |
| 3.3 | TS baseline missing → Go path still evaluates (D-6) | [x] |

### Manual

| Step | Hành động | Pass khi | Tick |
|------|-----------|----------|------|
| 3.M1 | Break a green Go test; `git commit` | pre-commit refuses | [ ] |
| 3.M2 | Claude Code Stop after dirty regression | Stop hook runs check / warns | [ ] |
| 3.M3 | Documented uninstall of hooks | Can remove without code revert | [ ] |

**Env note:** Prove on **Linux/CI** or WSL. Windows native: use Git Bash/WSL; do not fail CP on missing `sh` adapter noise.

---

## §4 — Waiver ledger (P-4 / Task-276)

**Mục tiêu:** Override accept = debt có hạn; hết hạn re-arm `r-reg`.

### Automated

```bash
go test ./internal/flowgate/... -count=1 -run 'TestCP53Waiver|TestWaiverLedger|TestOverrideExpiry' -v
go test ./internal/runner/ -count=1 -run 'TestCP53Waiver|TestWaiver' -v
```

| Step | Pass khi | Tick |
|------|----------|------|
| 4.1 | Accept writes ledger with reason + expiry | [ ] |
| 4.2 | Accept without reason rejected (or forced placeholder — per Task) | [ ] |
| 4.3 | After expiry, `r-reg` re-arms for same test | [ ] |
| 4.4 | Test goes green before expiry → override clears (BUG-289 preserved) | [ ] |

### Manual

| Step | Hành động | Pass khi | Tick |
|------|-----------|----------|------|
| 4.M1 | Trigger `r-reg` decision card → Accept with reason | Ledger file shows entry | [ ] |
| 4.M2 | Backdate expiry → rerun gate | Blocks again | [ ] |

---

## §5 — `r-newtest` (P-5 / Task-277)

**Mục tiêu:** Production change không có test mới → reprompt; remediation = ADD only.

### Automated

```bash
go test ./internal/flowgate/... -count=1 -run 'TestCP53RNewtest|TestRuleNewtest|TestRNewtest' -v
```

| Step | Pass khi | Tick |
|------|----------|------|
| 5.1 | Prod-only diff → `r-newtest` reprompt | [ ] |
| 5.2 | Prod + **newly added** `*_test.go` → no `r-newtest` | [ ] |
| 5.3 | Docs/CA-only → no fire | [ ] |
| 5.4 | Editing only **existing** tests does **not** satisfy r-newtest (oracle-rule) | [ ] |
| 5.5 | Rule disable via config stops firing | [ ] |

### Manual

| Step | Hành động | Pass khi | Tick |
|------|-----------|----------|------|
| 5.M1 | Turn edits production only | Reprompt cites `r-newtest`; text says add new test | [ ] |
| 5.M2 | Add new test file covering change | Gate proceeds (other rules may still apply) | [ ] |

---

## Cross-phase regression bundle (after all Tasks)

```bash
cd apps/local-runner
go test ./internal/flowgate/... -count=1 -timeout 10m
go test ./internal/runner/ -count=1 -timeout 20m -run 'TestCP53|TestGate|TestBug288|TestBug289|TestReviewLoop|TestFlowGate'
```

| Step | Pass khi | Tick |
|------|----------|------|
| X.1 | Full `flowgate` package green | [ ] |
| X.2 | CP-53 + related gate/review-loop runner tests green | [ ] |
| X.3 | No pre-existing test file modified without written operator allow | [ ] |
| X.4 | Each Task has CA with `feature_key` + R2 classification | [ ] |

---

## CP-53 verification-complete when

- All phase automated steps for shipped Tasks are ticked **pass**.
- Manual steps for P-1 (1.M1), P-2 (2.M1–2.M2), P-3 (3.M1) ticked on at least one real environment (Linux/WSL preferred).
- Safe-fix DoD in [CP-53 §11.2](./CP-53-Review-Loop.md) satisfied for every completed Task.
