# Task-257: Capability Evidence Spike

## Metadata

- Document ID: `Task-257`
- Title: `Capability Evidence Spike`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex review`
- Created: `2026-07-16`
- Last Updated: `2026-07-16`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md) (P-0), [SD-24: Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§6.3)
- Child Documents: `None`
- Related Documents: [Task-249](./Task-249-Live-Dispatch-Integration-And-Stop-Fences.md) (consumer of the evidence)
- Replaces: `None`
- Tags: `agent-flow-engine, capability-matrix, spike, evidence, p0-gate`

## AI Quick View

### Summary

- The dedicated owner of the **P-0 capability-evidence spike** (plan-review #5 #7 — previously circular: P-0 required the spike, but the matrix cells were assigned to Task-249, which can only run after P-0).
- Scoped completion covers Codex + Grok + Claude. Their probes establish the valid negative outcome **unprovable (as a receipt) ⇒ `uncertain`** for receipt/attach, so none of the three gets an unsafe receipt or attach seam. Claude's probe additionally exercised a live kill-mid-turn test and recorded an evidenced (non-authoritative) reconcile artifact that Codex/Grok's single-shot probes did not establish. Gemini remains deferred and V2-disabled until its own evidence is recorded.
- **Throwaway harness only, zero production code** — this task is explicitly runnable *inside* the P-0 gate because it IS the gate's evidence step.

### Current Ask

- Keep the Codex/Grok/Claude scoped evidence current and run the deferred Gemini probe before enabling that provider in V2. CP-51's scoped `CE-CG` and `CE-CL` are complete; `CE-GEM` remains deferred.

### Key Decisions

- `T-1` Evidence is **recorded, reproducible fact**: for each cell — the exact operation transcript (request/response or event log excerpt), provider version, date, and the concluded guarantee class. Stored beside SD-24 (`requirements/06-System-Tech-Design/evidence/SD-24/<provider>.md`), linked from the matrix.
- `T-2` An unverifiable cell is a **valid outcome**: the cell is rewritten to "no proof ⇒ `uncertain` window stands" (the protocol is safe either way — SD-24 Q-1); what is not valid is leaving *(verify live)* in place.

### Constraints

- No production code, no changes under `apps/` — the harness is scratch/throwaway (may live in a spike branch or scratchpad).
- Needs live provider access (Codex, Grok, Claude, Gemini accounts) — the one CP-51 item that cannot be done from documents.

### Open Questions

- None — the questions ARE the matrix cells; each gets an evidence-backed answer or an explicit "unprovable ⇒ uncertain".

### Source Refs

- SD-24 §6.3 (the *(verify live)* cells + verified operation shapes: `codex_adapter.go:167/226`, `grok_adapter.go:285/223`, `claude_adapter.go:173`); CP-51 P-0 + ledgers `CE-CG` / `CE-CL` / `CE-GEM`.

## 1. Goal

Replace every enabled-provider assumption in the SD-24 §6.3 capability matrix with recorded evidence. The enabled scope is Codex + Grok + Claude; Gemini stays V2-disabled until its own evidence is recorded.

## 2. Parent Links

- coding plan: CP-51 (`P-0` — the gate's evidence step)
- tech design: [SD-24](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) §6.3
- system spec: SS-17 (indirect — receipt fidelity bounds the `uncertain` window)
- specific upstream ids: CP-51 ledger `CE-CG` / `CE-CL` / `CE-GEM`; SD-24 Q-1

## 3. Trigger

Plan review #5 #7: P-0 demanded the spike "before all coding" while assigning the work to Task-249 (post-P-0) — ownerless and circular. This task breaks the cycle with a dedicated pre-gate owner, environment list, and evidence format.

## 4. Exact Change

- `T-1` Per provider **before V2 enablement** (Codex, Grok, Claude, Gemini), run a minimal live turn against a scratch workspace and capture:
  1. **Acceptance receipt**: what arrives between the prompt send (`turn/start` / `session/prompt` / `writeUserTurn` / spawn) and the first output — is there an ack distinct from end-of-turn? Timing captured.
  2. **Query/reconcile**: after killing the client mid-turn, what durable artifact exists (rollout file tail, session dir, transcript) and can it distinguish completed vs in-flight vs never-ran?
  3. **Attach**: can an in-flight turn be re-attached after restart (`session/load`, resume-by-rollout), or is respawn-as-new the only option?
- `T-2` Write `evidence/SD-24/<provider>.md` per `Key Decisions T-1` format; update each **enabled** provider row in SD-24 §6.3 in place (remove its *(verify live)* cells), adjusting guarantee class where evidence demands. A deferred provider must be marked V2-disabled until this step is done.
- `T-2a` Each evidence file uses this fixed, redacted template: provider/version/date/environment; exact command or harness commit; request timestamp; receipt predicate and `ReceiptID` derivation; reconcile artifact/path; attach outcome; sanitized transcript excerpt; conclusion and adapter `file:function` that must call `TurnBridge.Accepted` (or explicit “none”). Replace account IDs, prompts, tokens and workspace paths with stable placeholders; retain event ordering/timestamps so the result is reproducible.
- `T-3` Flip CP-51 ledger `CE-CG` (Codex/Grok) and `CE-CL` (Claude) with evidence links; keep `CE-GEM` open, and enforce V2 provider exclusion until each deferred matrix row is resolved. Note any matrix change that affects Task-249/250 sketches.

## 5. Touched Areas

- files: `requirements/06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md` (§6.3 cells), new `requirements/06-System-Tech-Design/evidence/SD-24/*.md`, CP-51 ledger rows `CE-CG` / `CE-CL` / `CE-GEM`.
- modules: none (throwaway harness outside the repo's build).
- routes: none.
- tables: none.

## 6. Acceptance Check

- `V-1` Scope: Codex/Grok/Claude rows contain no *(verify live)* marker and link to their evidence conclusions; Gemini remains explicitly marked deferred + V2-disabled.
- `V-2` Codex, Grok, and Claude evidence files contain the three question outcomes, provider version, date, and explicit `unprovable ⇒ uncertain` conclusion. Gemini requires the same before its V2 enablement.
- `V-3` CP-51 `CE-CG` and `CE-CL` rows ✅ with Codex/Grok/Claude evidence links; `CE-GEM` remains ☐; the matrix and Task-249 flag that Codex/Grok/Claude have no `Accepted` seam and Gemini cannot enter V2.
- `V-4` No production code changed by the evidence-gathering harness itself (`git diff apps/` limited to the reviewed `providerV2Enabled` gate flip + its tests, per the recorded evidence — no receipt/reconcile/attach logic added).

## 7. Out of Scope

- Implementing the receipt/reconcile logic (Task-249/250) — this task only establishes the facts.
- Provider adapter changes of any kind.

## 8. Completion Notes

- result: **Codex/Grok scoped evidence complete (2026-07-16)** — Codex CLI `gpt-5.4-mini` and Grok CLI `grok-4.5` both completed the scratch probe. Neither exposed acceptance/reconcile/attach proof, which is an explicit valid result: `unprovable ⇒ uncertain`; neither has an `Accepted` or attach seam in V2.
- result: **Claude scoped evidence complete (2026-07-17)** — Claude Code CLI `2.1.211` (`claude-sonnet-5`) ran three live probes including a kill-mid-turn reconcile+attach test (process identified and terminated by exact `--session-id` command-line match, no other running `claude.exe` instance touched). No acceptance receipt or attach capability found (same negative class as Codex/Grok); unlike Codex/Grok, a durable per-session reconcile artifact was positively evidenced (non-authoritative — see [claude.md](../../06-System-Tech-Design/evidence/SD-24/claude.md)). `providerV2Enabled("claude")` flipped to default-enabled (three-outcome only, no `Accepted` seam), mirroring the existing Codex/Grok gate.
- follow-ups: Gemini evidence remains deferred; Gemini stays V2-disabled until its own Task-257 conclusion.
- upstream docs updated: Codex/Grok/Claude matrix rows and scoped CP-51 ledger (`CE-CG`, `CE-CL` ✅, `CE-GEM` ☐); Task-249 consumes the negative result and has no receipt call for any of the three providers.
