# KR-001: Kill-Review — CP-43 Change Contract Plan

## Metadata

- Review ID: `KR-001`
- Subject: `CP-43` (`requirements/07-Coding-Plan/inprogress/CP-43-Change-Contract-And-Canonical-Intent-Signature.md`) + child Tasks 184–188
- Mode: `plan` (plan integrity + plan↔status↔code consistency for *claimed* deliverables; not a full runtime crash audit)
- Claim-Revision: `1`
- Reviewer: `Grok 4.5 (kill-review skill / SP-05)`
- Date: `2026-07-16`
- Timebox: `single pass`
- Fix policy: `no` (inventory only)
- Authority: [SP-05](../04-System-Principle/SP-05-Kill-Review-Closed-Claim-Contract.md)
- Verdict: **`KILL_WITH_FINDINGS`**

---

## 1. Claim (frozen)

> **CP-43’s coding plan and child-task graph (P-1…P-5 / Task-184…188), as written on disk, are internally consistent, correctly linked, honest about done vs open work, and aligned with landed upstream (SD-21, CP-44/CP-50) so remaining open DoD can close CP-43 without redefining intent integrity.**

## 2. In-scope artifacts

- CP-43 metadata, AI Quick View, §§1–10 (esp. §4 work breakdown, §7 validation, §10 DoD)
- Child tasks: Task-184 (done), Task-185 (in_progress), Task-186 (done), Task-187 (done), Task-188 (in_progress)
- Spot-check of *claimed* implementation anchors only:
  - `apps/local-runner/internal/changecontract/scope.go`
  - `apps/local-runner/internal/runner/context_sources_builtin.go` (feature.history vs canonical.head)
  - `apps/local-runner/internal/runner/context_source_canonical_head_test.go`
  - `apps/local-runner/internal/flowgate/rules.go` (rule IDs present)

## 3. Failure classes (closed space)

| ID | Class |
| --- | --- |
| FC-1 | Broken or stale document links / wrong folder status |
| FC-2 | Contradictory “done” vs open checkboxes / phase status |
| FC-3 | Plan claims symbol-level / rename-safe scope that code+task explicitly did not build |
| FC-4 | Plan claims packing mechanism superseded by CP-50 without CP-43/Task-188 rewrite |
| FC-5 | Acceptance/validation items still listed as v1 but documented unimplemented (E-4, budget drop, E2E) |
| FC-6 | Silent degrade that weakens US-3 “flag anything else” without plan callout as intentional product rule |

## 4. OOS

| ID | Concern | Tracked at |
| --- | --- | --- |
| OOS-1 | Durable turn dispatch / Stop / crash-matrix | CP-51 / SD-24 |
| OOS-2 | Full multi-agent cohort / bus durability | SD-19 / future CP |
| OOS-3 | Live Claude/Codex provider E2E prompt order (execution of manual test) | Task-188 follow-up (not re-run here) |
| OOS-4 | Full `go test ./internal/runner` flake baseline | Task-185 completion notes |
| OOS-5 | CP-47 `r-dod`, other flowgate rules unrelated to contract/head | CP-47 |
| OOS-6 | BUG-288 gate re-entry / three-tier residual beyond change-contract seam | BUG-288 / CP-51 |
| OOS-7 | Deep correctness of every changecontract unit test | future `impl` Kill-Review |

## 5. Coverage boundary

- **Green means:** this Kill-Review found no open Critical/Important *in-claim* plan defects for the frozen claim.
- **Green does NOT mean:** CP-43 product is complete; runtime is bug-free; symbol-level scope works; packing/budget is perfect; or all CH-* scenarios pass.

---

## 6. Probes

| ID | Requirement (binary) | Verifier | Status |
| --- | --- | --- | --- |
| P-01 | Child Documents paths resolve on disk | `test -f` each Child Document path from CP-43 metadata | ❌ |
| P-02 | Task parent/child links to CP-43 resolve | `test -f` parent path cited in Task-185/188 | ❌ |
| P-03 | §10 done flags match task Status fields | Compare CP-43 §10 vs task Metadata Status | ✅ (P-1/3/4 done, P-2/5 open — consistent *as flags*) |
| P-04 | §4.5 / Task-188 packing story matches current code | Read `featureHistorySource.Fetch` + Task-244 tests | ❌ |
| P-05 | Goal/§4.2 “symbol-level when structure available” matches shipped ScopeDiff | Read `scope.go` + Task-185 §6 | ❌ |
| P-06 | §7 rename-only false-drift acceptance is either done or explicitly not-v1 in Goal | §7 vs Task-185 §6 open item | ❌ |
| P-07 | §4.2 / constraints E-4 ignore set either implemented or removed from v1 AC | Task-185 §6 + codebase search | ❌ |
| P-08 | Empty/malformed declared scope behavior is specified | `scope.go` empty DeclaredPaths early return vs Goal US-3 | ❌ |
| P-09 | Rule IDs `r-contract`/`r-scope`/`r-spec-drift`/`r-code-drift` exist in defaults | `flowgate/rules.go` DefaultRules | ✅ |
| P-10 | CP-50 absorption of context-source pieces is reflected in CP-43 open P-5 wording | CP-43 §3 update vs §4.5/§10 P-5 still describing feature.history prepend | ❌ |
| P-11 | No silent contradiction: §10 “US-3 satisfied” vs P-2 unchecked | CP-43 §10 lines | ❌ |
| P-12 | Open Questions that re-open resolved Key Decisions are closed or marked residual | Task-185 Open Questions vs CP-43 Q-3 | ❌ (minor) |

---

## 7. Findings inventory

| ID | Sev | Kind | Evidence | Why in-claim | Owner | Status |
| --- | --- | --- | --- | --- | --- | --- |
| **F-01** | **C** | doc-contradiction | Task-188 §6 claims Head packing done via `featureHistorySource.Fetch` prepends `RenderHeadBlock`. Code: `context_sources_builtin.go:190-192` — Head is **not** prepended; separate `canonical.head` (Task-244). Test `TestFeatureHistorySourceNoLongerPrependsHead` **fails** if prepend returns. CP-43 §10 P-5 still marks prepend ✓ under Task-188 story. | FC-4, P-04, P-10 | Task-188 + CP-43 §4.5/§10 rewrite | **open** |
| **F-02** | **I** | plan | CP-43 Child Documents still point at `08-Task/todo/Task-185…188` and list 186/187 as if under todo; actual: 185/188 in `inprogress/`, 186/187 in `done/`. Task-185/188 Parent Documents link `07-Coding-Plan/todo/CP-43-…` — **file missing** (`todo` path); live file is `inprogress/`. | FC-1, P-01, P-02 | Doc hygiene on CP-43 + Task-185/188 | **open** |
| **F-03** | **I** | plan | Goal, Key Decisions `P-2`, §4.2, Task-185 Goal/T-2 still require **symbol-level** scope when `structure.Available()`. Shipped: `scope.go:18-41` — no hunk→symbol extraction; `outSymbols` never populated; HighSeverity is **path-level Dependents**. §10 admits file-level-only but leaves P-2 unchecked without **rewriting** the Goal/AC to a deliberate file-level v1 contract. | FC-3, P-05 | CP-43 Goal/§4.2 + Task-185 + optional SD-21 note | **open** |
| **F-04** | **I** | plan | §7 Validation: “rename-only diff must **not** trip `r-scope` when GitNexus resolves it to an in-scope symbol”. Task-185 §6 explicitly **not implemented** (no symbol API). Still listed as plan validation success path, not “deferred / not v1”. | FC-5, P-06 | CP-43 §7 + Task-185 | **open** |
| **F-05** | **I** | plan | §4.2 / Task-185 constraints claim exclude `SS-14 E-4` per-project ignore set. Task-185 §6: **E-4 not implemented anywhere**. Still part of written detection contract. | FC-5, P-07 | Future task or shrink CP-43 AC | **open** |
| **F-06** | **I** | code / plan | `ScopeDiff`: `len(DeclaredPaths)==0` → no out-of-scope (`scope.go:25-26`). Weakens “flag anything else” if store has empty paths (malformed/partial declare). Not called out in CP-43 §4.1/§4.2 as intentional product rule (only code comment “degrade gracefully”). | FC-6, P-08 | CP-43 §4.2 or changecontract + tests | **open** |
| **F-07** | **I** | plan | §10 marks “Satisfies SS-14 US-3 … at file-level” **[x]** while P-2 remains **[ ]**. Readers can treat US-3 closed while scope-drift task still open — mixed completion signal for CP close. | FC-2, P-11 | CP-43 §10 reword | **open** |
| **F-08** | **I** | plan | Open P-5 residual (budget demote history + true scope diff UI + live E2E) is real, but **success criteria still written as feature.history prepend** (superseded). Without rewrite, implementers may “re-add prepend” and fight CP-50 tests. | FC-4, P-10 | Task-188 Current Ask / §4 rewrite | **open** |
| **F-09** | **M** | plan | Task-185 Open Question still asks whether `r-scope` may block at file level when structure absent — already resolved in CP-43 Q-3 / Key Decisions (no). | FC-1, P-12 | Task-185 | **open** |
| **F-10** | **M** | plan | CP-43 Related Documents link `CP-23` to `./CP-23-Auto-Learn-To-Skill.md` under coding-plan relative path — filename is wrong-way/auto-learn title collision risk; not verified as the “Context Control” doc readers expect. | FC-1 | CP-43 Related Documents | **open** |
| **F-11** | **M** | test-gap | Empty-DeclaredPaths no-drift behavior has code comment but no CP-level acceptance row forcing a test that *documents* product choice (test may exist for graceful degrade — not elevated to plan AC). | FC-6 | Task-185 tests / §6 | **open** |

### Severity counts (open)

| Sev | Count |
| --- | ---: |
| Critical | **1** |
| Important | **7** |
| Minor | **3** |

---

## 8. Verdict and stop reason

**Verdict: `KILL_WITH_FINDINGS`**

Stop reason (SP-05 §6):

1. Claim frozen (this report §1–5).  
2. Probe set frozen (§6); meta-check: in-claim failure classes FC-1…FC-6 each have ≥1 probe.  
3. Inventory frozen with evidence; no “suspected” rows.  
4. Single-pass timebox complete.  
5. Not `KILL_CLEAN` (open C/I). Not `KILL_BLOCKED` (docs readable). Not `ABORT_RECLAIM` (claim was valid; subject is messy, not wrong topic).

**Do not start Round 2 on KR-001.**  
Next actions: fix batch against F-01…F-08 (docs/plan first), then **delta** Kill-Review on same claim, or open a **new claim** `impl` KR for runtime of Task-185 only.

---

## 9. Residual risk (explicitly not proven)

- Runtime races between gate_hook contract capture and multi-turn re-entry (BUG-288 neighbourhood).  
- Correctness of intent_signature / UpdateHead under concurrent writers.  
- Whether `canonical.head` default set always wins ordering vs `change.contract` / history in every flow YAML (CP-50 covers much of this — not re-audited).  
- Admin/desktop panel UX completeness beyond Task-188 notes.  
- FoldDecisions keyword heuristic misses (Task-187 known limitation — accepted residual).  
- Full runner test suite flake classification.

## 10. Next claims (optional, not this review)

| Suggested KR | Claim one-liner |
| --- | --- |
| KR-00x `impl` Task-185 | File-level `r-scope`/`r-contract` match Task-185 §6 checked boxes only. |
| KR-00x `impl` packing | `canonical.head` + `change.contract` sources honor CP-50 + residual Task-188 budget/UI only. |
| KR-00x plan CP-43 close | After F-01…F-08 doc rewrite, remaining open DoD is only {budget log, scope-diff HTTP, E2E} or explicit won’t-fix. |

---

## Appendix — Recommended fix order (not executed)

1. **F-01 / F-08:** Rewrite CP-43 §4.5 + Task-188 Goal/DoD: packing authority = **`canonical.head` source (CP-50)**; remaining = budget demotion, UI scope-diff, E2E. Delete “feature.history prepends Head” as done claim.  
2. **F-02:** Fix all links to `inprogress/CP-43` and correct task folder paths; move Child Documents list to real statuses.  
3. **F-03 / F-04:** Either (A) amend Goal/SD-21 language to **file-level v1** and move symbol/rename to explicit future, or (B) open a real task for symbol extraction (conflicts with D-2 — prefer A).  
4. **F-05:** Drop E-4 from CP-43 v1 AC or file a tiny task.  
5. **F-06:** Spec empty-declared-paths → `r-contract` / treat as undeclared vs no-op.  
6. **F-07:** Uncheck or reword §10 US-3 until P-2 formally closed under amended contract.

---

*End of KR-001. Kill-Review terminated per SP-05.*
