---
name: cross-provider-parity
description: When a bugfix or task touches any provider-specific code path (Claude, Codex, or Grok), determine whether it is truly provider-agnostic or genuinely per-provider, and verify + test all three providers before calling the work done. Never assume a fix "just works" for the other two providers because it fixed the one that surfaced the bug.
version: 6
---

# cross-provider-parity

Use **PROACTIVELY** on every bugfix, task, or refactor that touches Claude, Codex, or Grok adapter code, posture/permission resolution, provider-keyed dispatch or session logic, or any function that takes (or could plausibly need) a `providerKey`.

Companion to **additive-tests-only** (new tests only) and **oracle-rule** (fix code, not the assertion). This skill covers the complementary contract: **do not close a provider-touching bug/task until you have explicitly confirmed its behavior for Claude, Codex, and Grok** — either by ruling out divergence with evidence, or by testing all three.

## The Rule

> A bug reported against ONE provider is not proof the other two are unaffected, and a fix that makes ONE provider pass is not proof the other two still work.
> Before marking a provider-touching fix done: **either (a) prove the code path is provider-agnostic with a grep/read, or (b) test Claude, Codex, and Grok explicitly.**

Real precedent this rule exists for: BUG-296 (Claude's `--permission-prompt-tool` wiring broke silently while Codex/Grok never had the same gap — confirmed only by reading each adapter's `handleInbound`, not by assumption) and BUG-298 (a dispatch-ledger predicate took no `providerKey` at all, so one provider-parameterized test correctly covered all three at once). Both directions matter: sometimes providers diverge for a real reason, sometimes they're identical and asserting that identity IS the verification.

## Step 1 — Classify the code path

Before writing or testing anything, determine which case you're in:

1. **Provider-agnostic** — the function/file never branches on `providerKey` and has no per-provider config. Prove this with a grep (`providerKey`, `ProviderKey`, the specific `Provider*` constant names) across the changed function and its direct callers/callees. State the grep result explicitly (in the fix, the doc, or the commit) — "confirmed provider-agnostic: `<function>` takes no `providerKey` and never branches on one" — not just an assumption.
2. **Shared logic, provider-specific wiring** — a common function (e.g. an approval bridge, a posture resolver) is called from each provider's own adapter, and each adapter's calling convention could differ even if the shared logic doesn't. Read EACH adapter's actual call site — do not assume they all wire it the same way. (This is exactly how BUG-296 hid: the shared `RequestApproval` bridge was correct, but only Claude's launch-time flag omitted the channel that reaches it.)
3. **Genuinely per-provider** — each provider has its own implementation of the behavior in question (e.g. `claudeArgs`, `codexYoloDerive`, Grok's `handleInbound` yolo branch). Assume nothing carries over between them; each needs its own verification.

## Step 2 — Verify accordingly

- **Case 1 (agnostic)**: one representative test is sufficient — but consider a provider-parameterized test (loop or sub-tests over `ProviderKey{Claude, Codex, Grok}`) anyway when the function is cheap to parameterize, so a FUTURE change that accidentally introduces provider-specific behavior trips the guard immediately instead of silently becoming Case 2 or 3 unnoticed.
- **Case 2 (shared logic, per-adapter wiring)**: read (and, if testable in isolation, exercise) each adapter's actual call site into the shared logic. Do not stop at "the shared function is correct" — confirm each adapter actually reaches it the same way.
- **Case 3 (per-provider)**: write or run a test for EACH of the three. A fix that only tests the provider that surfaced the bug report is incomplete — the other two need their own pass/fail signal, not an inference.

## Allowed

1. Ship a fix touching only one provider's file, IF Step 1 concluded (with evidence) that the other two providers are structurally unaffected — cite the evidence (grep result, adapter code read) in the fix/doc/commit.
2. Use one provider-parameterized test (table test or sub-tests) instead of three separate near-duplicate tests, when the underlying function takes `providerKey` as data rather than branching on it in source.
3. Skip testing a provider whose adapter genuinely cannot reach the changed code path at all (e.g. a Claude-only CLI flag) — but say so explicitly, do not silently omit it.

## Forbidden without stating the reasoning

1. Closing a provider-touching bug/task after verifying only the ONE provider from the report, with no comment on the other two.
2. Assuming "the shared function is correct" is the same as "all three adapters are correct" (Case 2's trap — see BUG-296).
3. Copy-pasting a fix's reasoning from one provider to another without reading that OTHER provider's actual adapter code (e.g. assuming Codex/Grok have the same launch-time gating Claude does, without checking).

## Workflow (bug / task touching a provider)

```text
1. Identify every provider that COULD be touched by the change (not just the one reported).
2. Classify: agnostic / shared-logic-per-adapter-wiring / genuinely-per-provider (Step 1).
3. Case agnostic      → grep-confirm no providerKey branch; state it explicitly.
   Case shared-wiring → read each adapter's own call site into the shared logic.
   Case per-provider  → write/run a test for Claude, Codex, AND Grok.
4. Record which case applied and the evidence in the fix's BugFix doc / change-audit note.
5. Any provider left unverified → STOP → say so explicitly, do not imply full coverage.
```

## How this prevents regression

| Signal | Meaning |
|--------|---------|
| Grep/read confirms no `providerKey` branch | Genuinely safe to fix once for all three |
| Each adapter's call site independently confirmed | Shared-logic fix actually reaches all three, not just the one tested |
| Claude, Codex, and Grok all tested | Per-provider fix locked in for all three, not inferred from one |
| "Should also work for the others" with no check | The exact gap that let BUG-296 hide for Claude alone |

## Relationship to other skills

| Skill | Focus |
|-------|--------|
| **oracle-rule** | Failing test → fix **code**, not the assertion |
| **additive-tests-only** | New work → **add** tests only; do not modify the legacy suite without asking |
| **cross-provider-parity** | Provider-touching work → prove or test **Claude + Codex + Grok**, never just the one reported |
| **safe-fix-contract** | Operator umbrella wrapping this skill with old-test stop + matrix coverage + CA history |

All three apply together on any bug fix or coding task that touches provider-specific code.
Prefer loading **safe-fix-contract** when the operator restates the full delivery rules.
