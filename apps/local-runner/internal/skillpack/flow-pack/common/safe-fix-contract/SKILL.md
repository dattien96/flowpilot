---
name: safe-fix-contract
description: >
  Operator safety pack for every bug fix and coding task: never break old tests
  (fail → report and stop), prove Codex/Claude/Grok parity, add enough new tests
  to cover the situations, and read feature_key + change-audit CA history before
  coding. Use PROACTIVELY on bugfix, regression, resume/order/provider work,
  and any task the operator reminds "don't fail old tests / 3 providers / cover cases".
version: 6
---

# safe-fix-contract

Umbrella skill for the operator's non-negotiable fix delivery rules. It **wraps**
and **requires** the finer-grained skills below — do not treat this as optional
flavor text.

| Child skill | Path (after install) | What it enforces |
|---|---|---|
| **additive-tests-only** | `…/skills/additive-tests-only/SKILL.md` | Only **add** tests; never edit the legacy suite without asking |
| **cross-provider-parity** | `…/skills/cross-provider-parity/SKILL.md` | Prove or test **Claude + Codex + Grok**, not only the reported provider |
| **oracle-rule** | `…/skills/oracle-rule/SKILL.md` | Failing test → fix **production code**, not the assertion |
| **context-discipline** | `…/skills/context-discipline/SKILL.md` | Build on latest CA history for the feature; don't re-undo prior work |

**Read the child skills when this skill fires** — this file is the gate and checklist;
the children hold the detailed procedure.

---

## Agent memory (MANDATORY)

> **AI agents (Claude, Codex, Grok, Cursor, Gemini, etc.): pin this skill in long-term
> session/project memory** whenever you work in FlowPilot on bugs, tasks, or
> regressions. The operator will keep repeating these rules if they are not loaded.
>
> Remember as a durable project rule:
>
> 1. **Old tests must stay green.** If any pre-existing test fails or won't compile because of your change → **STOP**, report the failure (test name + why), do **not** edit the old test to make it pass, and do not claim done.
> 2. **All three providers.** Shared or provider-touching behavior must be verified for **Claude, Codex, and Grok** (or explicitly proven provider-agnostic with evidence). One provider green is not enough.
> 3. **New tests must cover the real situations** — not only the single happy path that fixed the reported bug. Matrix the edge cases (restart, multi-round, missing sidecar, follow-up, multi-provider, etc.).
> 4. **Before coding:** resolve `feature_key` + read recent **CA-*** notes so you know what was already fixed and do not regress prior CA claims.
>
> Store a short form in memory, e.g.:
> `FlowPilot safe-fix-contract: no old-test edits; stop on old fail; Claude+Codex+Grok; additive matrix tests; read FEATURE-KEYS + CA history first.`

If your runtime has a memory/note tool, write that short form once per project and
reload this skill on every bug/task session.

---

## The three operator hard rules

### R1 — No regression of the old suite (fail → report → stop)

Companion: **additive-tests-only** + **oracle-rule**.

| Do | Don't |
|---|---|
| Keep every pre-existing test **unchanged** | Edit assertions/fixtures/timeouts in old tests |
| If an old test fails: **STOP**, print name + stack/reason | Silently skip, delete, or weaken the test |
| Prefer fix production code or a thin compatibility shim | "Fix the test" to green CI |
| Ask the user before any legacy-suite edit | Drive-by cleanup of old tests |

**Stop condition (blocking):**

```text
OLD_TEST_FAILED or OLD_TEST_NEEDS_EDIT
  → stop implementation claims
  → report: test path, failure output, suspected cause (regression vs API break vs wrong AC)
  → wait for user if only an old-test edit can unblock
```

Green old tests (untouched) are the primary regression signal. That signal is
destroyed if you edit them without approval.

### R2 — Code must work for all three providers

Companion: **cross-provider-parity**.

Before calling a provider-touching fix done:

1. **Classify** the path: provider-agnostic / shared-logic-per-adapter / per-provider
   (see child skill Step 1 — grep/read evidence required).
2. **Verify:**
   - agnostic → state evidence; prefer a parameterized test over `ProviderKey{…}`
   - shared wiring → read each adapter call site
   - per-provider → exercise **Claude, Codex, and Grok** (table or subtests)
3. Record classification + evidence in the change-audit note.
4. Any provider left unverified → **STOP** and say so explicitly.

Never close with "should work for the others" without a check.

### R3 — New tests must cover enough cases

Companion: **additive-tests-only** (how to add) + this section (what is "enough").

Adding **one** happy-path test for the reported repro is **not** enough when the
surface has multiple situations (as with resume order, flow vs chat, multi-round,
missing sidecars, follow-ups, three providers).

**Minimum coverage checklist** (adapt to the bug; skip only with stated reason):

| Axis | Ask |
|---|---|
| **Reported repro** | Exact user path red before / green after |
| **Near-miss shapes** | Same feature, different shape that previously slipped (e.g. dual-reviewer vs single-reviewer, short gap vs long R1, chat vs flow) |
| **Degraded inputs** | Missing sidecar, missing synthesis boundary, partial data |
| **Ordering / lifecycle** | Restart, reconnect, follow-up clamp, no bottom-append |
| **Providers** | Codex / Claude / Grok matrix when shared or Case 2/3 |
| **Counts + order** | Not only "how many cards" — also "where they sit in the timeline" when UI order matters |
| **Legacy path** | No-sidecar / old fixtures still produce cards without panic |

Prefer **new files** (`bugNNN_…_test.go`, `runNNNN_…_test.go`, matrix files).  
Do not weaken or rewrite old tests to "cover" new cases.

---

## Fix guide: feature_key + change-audit history (before code)

Companion: **context-discipline** + **audit-logging**.

Before writing production code for a bug/task:

### 1. Resolve `feature_key`

1. Open `change-audit/FEATURE-KEYS.md`.
2. Pick the kebab-case key that matches the area (e.g. `agent-flow-engine`, `chat-history`, `google-drive`).
3. If none fits, **add** a new key there first (same commit as the fix) — do not invent an unregistered key.

### 2. Read what this feature already fixed

1. List recent CA notes for that key, e.g.:

   ```bash
   rg -l "feature_key: <key>" change-audit/CA-*.md | sort
   # or: ls -t change-audit/CA-*.md | head
   ```

2. Read the **latest** `CA-NNN-*.md` entries for that key (at least the last 3–5
   relevant ones). Capture:
   - what bug/task they closed
   - root cause one-liner
   - files/symbols touched
   - tests added
   - explicit **out of scope** / residual risks

3. State at the start of the work (in chat or plan):

   ```text
   feature_key: <key>
   prior CA: CA-NNN (summary…), CA-MMM (summary…)
   will not undo: <prior claims>
   ```

### 3. Use history while fixing

| Use | Avoid |
|---|---|
| Extend prior durable contracts | Re-introduce a bug a CA already closed |
| Reuse helpers/patterns from prior CA | Parallel re-implementation of the same resume path |
| Add tests that lock **this** regression **and** near-misses | Happy-path-only tests that leave the next shape unguarded |

### 4. After the fix

Write/update `change-audit/CA-NNN-….md` with the ledger block (see **audit-logging**).
Reference which prior CA claims remain intact.

---

## End-to-end workflow (bug / task)

```text
0. MEMORY: ensure safe-fix-contract short form is loaded for this project
1. HISTORY: feature_key + recent CA notes for that key (this skill § Fix guide)
2. PLAN: scope files; classify provider impact (cross-provider-parity)
3. CODE: production fix only; no silent old-test edits (additive-tests-only)
4. TESTS:
   a. ADD new regression + matrix tests (R3 checklist)
   b. RUN new tests + related OLD patterns
   c. If ANY old test fails → STOP + report (R1)
5. PROVIDERS: Claude + Codex + Grok verified or proven agnostic (R2)
6. AUDIT: CA note + commit format with [feature] key
7. DONE only if R1 + R2 + R3 all hold
```

### Done definition (all required)

- [ ] Pre-existing tests **untouched** and **green**
- [ ] New tests cover reported repro **and** matrix near-misses (R3)
- [ ] Claude / Codex / Grok parity proven or tested (R2)
- [ ] Prior CA claims for the feature not undone (history step)
- [ ] Change-audit entry written

If any box fails → not done.

---

## Relationship map

```text
                    ┌─────────────────────────┐
                    │   safe-fix-contract    │  ← operator umbrella (this file)
                    │   + agent memory note   │
                    └───────────┬─────────────┘
            ┌───────────────────┼───────────────────┐
            ▼                   ▼                   ▼
   additive-tests-only   cross-provider-parity   oracle-rule
            │                   │                   │
            └───────────────────┴───────────────────┘
                                │
                    context-discipline + audit-logging
                    (feature_key + CA history + CA write)
```

| Situation | Primary skill |
|---|---|
| Tempted to edit an old test | additive-tests-only + R1 stop |
| Bug on one provider only reported | cross-provider-parity |
| Test fails after your change | oracle-rule (fix code) |
| Don't know what was fixed before | this skill § Fix guide + context-discipline |
| Closing a fix | this skill Done definition |

---

## Anti-patterns (forbidden)

1. "Old test failed so I updated expected values."
2. "Fixed for Grok; Claude/Codex should be fine."
3. "Added one unit test for the happy path; ship."
4. "Reimplemented resume order from scratch without reading CA-412 / CA-408."
5. Claiming done while an old test is red or unrun after a production change.

---

## Why this skill exists

Real sequence this pack prevents:

1. BUG-314 fixed card **count** with tests that only asserted counts.
2. Later commits broke card **order** after restart.
3. Old tests stayed green (no order assertions on the new path) → regression shipped.
4. Operator had to re-state: don't break old tests, cover all cases, all three providers.

**safe-fix-contract** forces history + matrix coverage + parity + stop-on-old-fail
so that pattern does not repeat.
