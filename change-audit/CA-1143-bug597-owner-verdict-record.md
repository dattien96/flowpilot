# CA-1143 — BUG-597: cohort-member verdict calls rejected when the hub doesn't gate on machine verdicts

- **Bug:** BUG-597 (live run-100368). `SubmitFlowControl`'s cohort-member
  path only reached `recordReviewCohortMemberVerdict` when
  `requireVerdict` was true — i.e. when `hubInboundCohortName` mapped the
  resolved hub. The owner-debate graph (`debate_synthesis` hub,
  `owner_1`/`owner_2` posture=verdict_only cohort) is not in that map, so an
  owner that correctly called `submit_review_outcome` fell through to the
  generic rejection — "this is a cohort review step" — and the verdict was
  never recorded. Legs were then re-driven with "verdict never recorded"
  reprompts (flow-diag shows both owners reprompted twice), and if the
  topology were also clobbered the verdict buffer stayed empty while the
  debate could never conclude.
- **Fix:** the gate is now `requireVerdict || in.viaReviewOutcome` — a cohort
  member that *does* call `submit_review_outcome` always gets its verdict
  recorded into the parent buffer. The buffer is inert for hubs that don't
  gate on it: recording costs nothing when `hubInboundCohortName` has no
  mapping, and lands the verdict for any hub that does.
- **Contract preserved:** non-verdict flow-control calls from cohort members
  still hit the rejection when `requireVerdict` holds (test asserts the
  rejected path unchanged); prose-verdict members and non-cohort legs are
  untouched — the change only widens which *verdict tool calls* reach the
  recorder, never which callers may settle or continue a flow.
- **Follow-up guard (same CA):** recording now lets owner labels reach
  `lastReviewCohortVerdicts`, and `hubProseVerdictDerivesFlowStatus` was the
  only verdict reader that iterated the whole merged map without filtering
  to the active hub's expected cohort — a post-restore sprint `synthesis`
  finishing in prose could derive `continue`/`done` off owner verdicts.
  It now restricts to the active hub's `hubInboundCohortName` labels when
  the topology declares them (unmapped hubs and cohort-less graphs keep the
  legacy whole-map derive). Every other reader — `hubDoneVerdictError`,
  `synthesisDoneVerdictError`, `hubDoneCohortHasChangesRequested`,
  `missingVerdictLabelsWithLiveMember`, `resumeVerdictDeficientMembers` —
  already filters to expected labels, verified.
- **Tests:** `bug597_owner_verdict_record_test.go` — red before fix: an
  owner-cohort `submit_review_outcome` under the debate hub records a
  verdict row on the parent; a non-verdict cohort call under a gated hub is
  still rejected; a sprint synthesis derives `done` only from its own
  review cohort (owner `changes_requested` + reviewer `approved` → done,
  owner approvals alone → escalate).
- **Refs:** BUG-597. Live evidence: run-100368 flow-diag —
  `owner_1`/`owner_2` reprompted with "verdict never recorded" after
  completing turns that did submit verdicts.
