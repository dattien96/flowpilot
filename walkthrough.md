# Durable-anchor redesign review: post-restart agent-card order

## Verdict

**NEEDS_FIX.**

The implementation removes synthetic-time round assignment for the normal
step-sidecar path and fixes the known short-gap cases. It does not complete the
prior Terra `REWORK_DESIGN` plan because it still guesses which transcript
messages are synthesis anchors and silently accepts incomplete or ambiguous
durable evidence. Exact durable replay cannot be claimed until those cases are
made explicit.

Scope reviewed: `BUG-314`, `CA-412`, the changed resume code, the new additive
test file, and existing BUG-314 tests. No code was changed in this review.

## Match to the prior Terra plan

| Prior requirement | Status | Evidence / review result |
| --- | --- | --- |
| Carry node, log ordinal, and synthesis-derived cohort on reconstructed events | Done | `stepActivationsFromOrderedLog` emits `nodeID`, `cohort`, and `ord`; `ProviderEvent` carries resume-only metadata. |
| Use append order, not transition timestamps, for durable cohort assignment | Done | Cohort increments while walking `synthesis` `RUNNING` records; durable clustering sorts by cohort then ordinal. |
| Expand reinvoked single-owner nodes and give spawn-lifecycle children one activation | Done, with an ambiguity caveat | Reinvoke uses every same-label activation; shared labels consume one activation in child `startedAt` order. |
| Pair durable synthesis boundaries to ordered joined-result transcript anchors | **Missing** | Transcript rebuild creates a `message_completed` for every assistant frame; placement then uses every pre-follow-up non-empty `message_completed`. It does not identify joined-result turns or pair their `ProviderTurnID`s to synthesis boundaries. |
| Validate synthesis-boundary/anchor cardinality and emit degraded-order state on mismatch | **Missing** | Zero, one, excess, or unrelated assistant messages are silently handled by append/before-last placement. No diagnostic or contract state is emitted. |
| Retain the old time heuristic only for sidecar-less legacy replay | Done | The 45-second/wave helpers are reached only when a child has no durable activation; all-durable pairs use cohort clustering. |
| Add short-gap and provider-parity regressions without editing BUG-314 tests | Done | New test file adds 1-second, dual-reviewer, single-reviewer, flow-mode, and Codex/Claude/Grok cases; `TestBug314*` remains unchanged. |

## Durable-path time dependency

The durable cohort decision itself has no wall-clock dependency: it uses NDJSON
append order and `synthesis RUNNING` count. The 45-second, `-1ms`, and 9/10
logic remains in the sidecar-less fallback only.

However, durable spawn-lifecycle pairing still orders same-label child sessions
by `StartedAt` before assigning the next log activation. That is a timestamp
dependency for child-to-activation identity, not for round clustering. Equal,
skewed, or concurrently persisted start times fall back to run-ID ordering,
which is deterministic but not a durable execution-order guarantee.

## Blocking findings

### R-01 — synthesis anchor selection is still heuristic

Severity: **Important / replay correctness**

`buildFlowHubTranscriptEventsFromTurnLog` emits a visible assistant event for
every assistant/transcript frame, including ordinary hub replies. Later,
`insertUnanchoredFlowAgentLifecycleLocked` treats every non-empty,
pre-follow-up `message_completed` as a synthesis anchor. Consequently an
ordinary hub assistant message before, between, or after joined-result turns
can shift every cohort by one. This contradicts the prior plan's required
joined-result-turn ↔ synthesis-boundary pairing.

Relevant code:

- `interactive_resume.go:3949-3980` creates generic assistant events.
- `interactive_resume.go:3437-3447` selects generic message events as anchors.
- `interactive_resume.go:3516-3529` maps cohort index to that unvalidated list.

Required fix: retain the ordered joined-result transcript anchors (including
their `TurnID`/`ProviderTurnID`); pair those, positionally and only after
validation, to the ordered synthesis `RUNNING` boundaries. Do not infer an
anchor from visible-message position alone.

### R-02 — missing synthesis RUNNING and cardinality mismatch silently claim durable placement

Severity: **Important / replay correctness and observability**

If a sidecar exists but has no synthesis `RUNNING`, all activations receive
cohort 0 and are placed as one round. If there are fewer or more actual
synthesis anchors than cohorts, the implementation either puts all clusters
before the only message, appends them when there is no message, or maps extras
before the final message. None of those paths marks the replay degraded.

This is specifically the failure mode the Terra plan required to validate. It
also violates the durable-replay contract's requirement that causal events not
be bottom-appended: the `len(msgIdxs)==0` path appends reconstructed cards.

Relevant code: `interactive_resume.go:3493-3530`.

Required fix: validate boundary-to-joined-result-anchor cardinality/order.
On mismatch, retain cards in durable cohort order before the last verified
anchor (or the original prompt when none is verified), mark the event/run as
degraded-order, and log a diagnostic. Do not append or represent the output as
exact replay. No new duration threshold is needed.

### R-03 — partial sidecar coverage re-enables the 45-second authority for all cards

Severity: **Important / regression risk**

The presence of a sidecar is not sufficient to keep this replay on the durable
path. A child whose label has no matched activation falls into the legacy
`resumeActivationTimestamps` path. Because placement requires *all* pairs to
be durable, one such pair sends every pair, including those with valid cohorts,
through `clusterFlowAgentPairsByStartGap(..., 45s)`. This reintroduces the
synthetic ordering that CA-412 says is excluded when a sidecar exists.

Relevant code: `interactive_resume.go:2110-2124`, `2169-2180`, and
`3451-3468`.

Required fix: validate sidecar coverage before declaring the result durable.
For a partial sidecar, use one explicit degraded policy for the whole restore;
never mix valid durable cohorts with a timestamp-derived global clustering
decision. Preserve the known durable cohorts in ordinal order and report which
child/activation could not be verified.

## Edge cases that remain unprotected

| Case | Current behavior | Required protection |
| --- | --- | --- |
| Synthesis node naming | `isSynthesisStepNode` accepts any node ID containing `synthesis`; an agent node such as `synthesis-reviewer` would be discarded as a boundary rather than restored as a card. | Resolve the actual hub-inline synthesis node(s) from the persisted/active flow definition, or use an explicit durable node role; test names such as `grok-synthesis` and an agent ID containing `synthesis`. |
| Concurrent same-node activations | One `openIdx[nodeID]` is overwritten by a second `RUNNING`; the first activation remains unterminated and the terminal pairs only with the latest one. | Keep a per-node queue/stack with an explicit pairing policy, or reject/diagnose unsupported overlap. Add a same-node concurrent activation test. |
| Concurrent shared-label children | Shared-label sessions are paired to log activations by `StartedAt`, then run ID on ties. The durable log has no child-run ID, so this cannot prove which child belongs to which activation. | Carry a durable child lifecycle/activation ID if exact identity is required; until then mark tied/ambiguous pairings degraded and test deterministic preservation. |
| Missing synthesis RUNNING | A non-empty sidecar prevents legacy fallback, but every activation remains cohort 0. | Detect the missing boundary when multiple rounds/anchors exist; use the degraded durable-order policy above, not a time fallback. |
| Cohort versus anchor cardinality mismatch | Mapping uses all visible pre-follow-up messages, with no equality/order check. It also compacts sparse cohort numbers: cohorts 0 and 2 become cluster indexes 0 and 1, so cohort 2 is placed before message 1 rather than anchor 2. | Join only verified synthesis anchors; preserve the actual cohort ID for placement and explicitly handle 0, fewer, extra, and sparse anchors. |

## Test assessment

The new file is additive and has useful coverage:

- dual reviewer, short gap, and one synthesis message;
- run-58237 long-reviewer flow mode;
- one-second gap proving durable clustering does not use the 45-second rule;
- single-reviewer BUG-314 shape for Codex, Claude, and Grok;
- shared dual-reviewer shape for Codex, Claude, and Grok.

Provider classification is provider-agnostic for this placement code: the
changed functions do not branch on `ProviderKey`, and the new table tests
exercise all three providers. This meets the three-provider order matrix for
the covered happy paths.

Gaps against the durable-replay contract remain:

1. The tests call `appendResumedParentAnnotations` on a manually seeded
   `interactiveRun`; they do not exercise the production restart/reconstruct
   boundary that seeds turn-log transcript anchors.
2. No test has an ordinary assistant message plus joined-result transcript
   turns, so R-01 is unobserved.
3. No tests cover missing synthesis `RUNNING`, fewer/more anchors than
   boundaries, sparse cohort IDs, partial sidecar coverage, required degraded
   diagnostic, or the no-bottom-append rule in those cases.
4. No test covers same-node overlap, equal start times, or concurrent
   shared-label pairing.
5. The lifecycle contract also requires restored prompts/responses exactly
   once, no duplicated cards, and settled terminal UI state through restart;
   these order-only helper tests do not prove those rows for the redesigned
   path.

Required additions must be additive and run the production resume boundary for
Codex, Claude, and Grok. At minimum: valid joined-result anchors; an unrelated
assistant frame; missing boundary; fewer/more anchors; same-node overlap;
equal-start shared-label children; and a restart assertion for no duplicates,
causal location, and terminal state.

## Regression risk

### BUG-314

The core count regression is protected for a normal, named synthesis boundary:
the original `TestBug314*` suite is untouched, and the focused suite passes.
Risk remains for custom synthesis naming, incomplete sidecars, and any replay
where transcript anchors are not exactly the assumed generic assistant-message
sequence. Those conditions can put all later BUG-314 cards in the wrong
cohort even though their count survives.

### No-sidecar fixtures

The legacy helpers remain reachable only where durable activations cannot be
formed, preserving current no-sidecar behavior. This is intentional but still
has the historical 45-second/synthetic-time limitations; it must be labelled
legacy/degraded, not durable replay. The focused legacy-plus-new suite passed
except `TestRun1264SettleFinalizesWhenFlowDoneDespiteStalePendingGate`, which
could not bind an IPv6 loopback listener in this sandbox (`operation not
permitted`), so this review cannot independently certify that one test.

## Verification

Executed from `apps/local-runner`:

```text
go test ./internal/runner -count=1 \
  -run 'TestBug314|TestRun52518|TestRun58237|TestDurableCohort|TestRun24377|TestRun5695|TestRun20332'
PASS: 32 tests
```

The broader command including `TestRun1264...` reached only the sandbox's
forbidden IPv6 listener creation; that is an environment limitation, not a
passing regression result.

## Concrete fix list before approval

1. Replace generic `message_completed` anchor indexing with validated ordered
   joined-result transcript anchors paired to synthesis `RUNNING` boundaries.
2. Add explicit mismatch detection and degraded-order diagnostics; preserve
   cohorts before the last verified anchor rather than bottom-appending.
3. Prevent partial-sidecar children from making all valid cohorts fall back to
   timestamp clustering; use one declared degraded policy instead.
4. Replace substring synthesis-node detection with flow-defined hub-role
   identification.
5. Define and implement/diagnose the policy for overlapping same-node
   `RUNNING` records and ambiguous same-label child pairing. Do not use time
   thresholds to resolve either.
6. Add the missing production-restart and three-provider contract tests above,
   leaving existing BUG-314 and legacy tests unchanged.

Until items 1-6 are complete, the durable-anchor redesign should not be
accepted as closing Terra's prior `REWORK_DESIGN` verdict.
