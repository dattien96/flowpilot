# CA-883 — CP-66 P-3 audit incremental update hook (Task-375)

# ---8<--- flowpilot:change-ledger
feature_key: living-knowledge-base
source_doc_id: Task-375
change_type: feature
summary: Audit completion fires background knowledge refresh (3 settle sites, single choke point, verdict-touching nothing); 4 hook tests green; 2 pre-existing timing failures proven unrelated
# --->8---

## Why

P-1 can write incrementally but nobody called it per lifecycle; without the
audit trigger the base goes stale after the first task and knowledge.flow
serves outdated context — killing the "Living" value. P-3 wires the update
exactly at reviewed-code completion (never mid-flight), absolutely
non-blocking and verdict-neutral.

## Change

- **`internal/runner/knowledge_bootstrap.go`** (additive): single choke
  point `onAuditNodeCompleted` (the ONLY knowledge trigger in any flow) +
  pure `auditKnowledgePaths` filter (shared concrete-code predicate —
  docs excluded, test files included as API-tracking code).
- **`internal/runner/flow_validate_audit_dispatch.go`** (+9 lines at 3
  settle sites in runAuditNode): vibe auto-finalize done, successor
  advance (only when okAdv), final flow-done settle — each AFTER the
  settle is recorded, so failed settles never mark knowledge fresh.
  No verdict, state, or edge logic touched.
- Placement proof (review-visible): implement/validate/reviewer nodes have
  no path to the choke point; unbootstrapped workspaces no-op inside
  UpdateAsync (never a mid-flow full rebuild); errors log `[knowledge]` only.

## Tests

- 4 new hook tests green: AC-1 end-to-end hook composition (affected
  section rewritten with fresh content, others byte-identical, index
  synced); AC-2 corrupt base (worker returns normally, bytes untouched;
  seam returns in <<2s); path filter (nil/doc-only → empty, test files
  pass as code); AC-4 unbootstrapped no-op (nothing created mid-flow).
- R1: `TestValidatePassedSpawnsReviewerCohortMember` +
  `TestValidatePassedStillChainsInlineAuditForLegacyEdges` fail — proven
  PRE-EXISTING via detached worktree at clean HEAD (identical failures
  without P-3 edits; 3s-timeout async timing tests in a loaded env). Old
  tests NOT touched. (Same session's supabase-config HOME incident from
  CA-882 also noted — env, not regression.)
- R2: Case-1 agnostic — hook carries paths only, no providerKey anywhere
  on the path.
- R3 matrix: happy (audit refresh), near-miss (successor-advance site,
  vibe site, doc-only turn, test-file turn), degraded (corrupt index,
  failing redistill, missing base), lifecycle (failed settle → no fire,
  serialization via P-1 locks, fire-and-forget timing assert).

## Prior CA claims kept intact

- CA-881/882 untouched (bootstrap only gains additive methods; source
  behavior identical — P-1/P-2 suites re-green). No flow YAML, registry,
  or default changed.
