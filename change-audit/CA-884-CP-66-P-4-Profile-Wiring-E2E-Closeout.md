# CA-884 — CP-66 P-4 profile wiring + E2E + closeout (Task-376)

# ---8<--- flowpilot:change-ledger
feature_key: living-knowledge-base
source_doc_id: Task-376
change_type: feature
summary: knowledge.flow opted into scout/plan_writer of both harnesses; 5 E2E tests green; CP-66 DOD 5/5 closed with evidence
# --->8---

## Why

P-1→P-3 built, exposed, and refreshed knowledge that no node consumed.
P-4 closes the value loop: planners get distilled flow context (~500
tokens instead of 40–60k raw-code tokens), coders stay on raw code
(token-economy guard), and non-opted flows render exactly as before.

## Change

- **`task-harness.yaml` + `bug-plan-harness.yaml`** (data only):
  scout → `[conventions, knowledge.flow, canonical.head, feature.history]`;
  plan_writer → `[conventions, knowledge.flow, canonical.head, feature.history,
  change.contract, source.excerpt]`; reviewer/coder/maxTokens untouched.
  No other flow YAML touched; no node-level ContextSources touched.
- Docs: Task-376 completion notes (result + §7 manual evidence +
  follow-ups); CP-66 → done (P-1→P-4 statuses, DOD 5/5 ticked with
  per-item evidence); child links → done/; CP-63 link → ../todo/ (still
  in progress); Task-373→375 parent links → done/CP-66.

## Tests

- 5/5 new E2E green on the REAL embedded pack + REAL builder: profile
  declares (order pinned: conventions, knowledge.flow, ...), pack
  validates (sequencing P-4-after-P-2 proven), plan_writer renders the
  distilled Checkout section (~500 tokens, no Login leak), coder carries
  no knowledge body, rag-harness renders none.
- Full CP-66 tally: 28 runnable new tests + 1 env-gated live test green
  (9 distiller/writer + 4 bootstrap + 6 source + 4 hook + 5 profile + 1
  live-index proof); agentpack + knowledge packages green; blast-radius
  runner suites green. Correction to CA-882: the P-2 slice added 6 tests,
  not 7 (miscount in the ledger; test names listed in §Tests above).
- R1: no old test edited. Known pre-existing failures (documented,
  untouched): supabase HOME-config env failure (CA-882 proof), 2
  rag-harness 3s-timing tests (CA-883 clean-HEAD proof). Manual §7:
  distilled flowpilot itself — overview 574 B, 10 flow sections 11.5 KB
  (~300 tokens each, real purpose/symbols/files), models ranked (Struct
  Catalog × 3 flows), index 16 KB with all entries resolving.
- R2: Case-1 agnostic end to end — YAML wiring is data, render path reads
  files + locus only; providerKey appears nowhere on the path.
- R3: E2E covers both harnesses × (declare + validate + render +
  negative coder + negative third flow) plus the §7 live proof.

## Prior CA claims kept intact

- CA-881→883 intact (no production edits to distiller/source/hook in
  P-4; knowledge + bootstrap + source suites re-green after the YAML
  change). CP-64/CP-65 DODs unaffected (their flows never opt in —
  rag-harness negative test locks exactly that).
