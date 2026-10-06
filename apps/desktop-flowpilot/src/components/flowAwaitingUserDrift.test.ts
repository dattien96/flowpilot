import assert from "node:assert/strict";
import test from "node:test";
import { awaitingUserDriftState, parseDriftedPaths } from "./flowAwaitingUserDrift";

test("parseDriftedPaths: nil when not a drift gate", () => {
  assert.equal(parseDriftedPaths("Reviewer requested escalate"), null);
});

test("parseDriftedPaths: single and multi path", () => {
  assert.deepEqual(
    parseDriftedPaths("flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go"),
    ["calc_test.go"],
  );
  assert.deepEqual(
    parseDriftedPaths("flow scope drift: wrote outside the frozen contract's declared paths: a.go, b.go"),
    ["a.go", "b.go"],
  );
});

test("parseDriftedPaths: trims trailing period and empty segments", () => {
  assert.deepEqual(
    parseDriftedPaths("wrote outside the frozen contract's declared paths: calc_test.go."),
    ["calc_test.go"],
  );
  assert.equal(parseDriftedPaths("wrote outside the frozen contract's declared paths: "), null);
});

test("parseDriftedPaths: strips external-drift prose tail (live run-183756)", () => {
  // gate_hook appends "; not written via this leg's tool calls: <paths>
  // — if these are operator edits, amend ..." — the tail must never reach
  // the amend payload or it joins declared_paths verbatim and the gate
  // re-fires forever.
  assert.deepEqual(
    parseDriftedPaths(
      "flow scope drift: wrote outside the frozen contract's declared paths: core/a.go; not written via this leg's tool calls: core/a.go — if these are operator edits, amend the contract to sanction them",
    ),
    ["core/a.go"],
  );
  assert.deepEqual(
    parseDriftedPaths(
      "flow scope drift: wrote outside the frozen contract's declared paths: a.go, b.go; not written via this leg's tool calls: a.go, b.go — if these are operator edits, amend the contract to sanction them",
    ),
    ["a.go", "b.go"],
  );
});

test("awaitingUserDriftState: Allow only on non-cap non-stalled drift", () => {
  const drift = awaitingUserDriftState({
    blockReason: "escalate",
    gateReason: "flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go",
  });
  assert.equal(drift.isDrift, true);
  assert.deepEqual(drift.driftedPaths, ["calc_test.go"]);
  assert.equal(drift.retryIsPrimary, false);

  const cap = awaitingUserDriftState({ blockReason: "cap", gateReason: "" });
  assert.equal(cap.isDrift, false);
  assert.equal(cap.retryIsPrimary, true);

  const stalled = awaitingUserDriftState({
    blockReason: "member_stalled",
    gateReason: "flow scope drift: wrote outside the frozen contract's declared paths: a.go",
  });
  assert.equal(stalled.isDrift, false);
  assert.equal(stalled.stalled, true);

  const escalate = awaitingUserDriftState({
    blockReason: "escalate",
    gateReason: "Reviewer requested escalate: missing clamp",
  });
  assert.equal(escalate.isDrift, false);
  assert.equal(escalate.retryIsPrimary, true);
});

test("awaitingUserDriftState: canAmend exposes the declare-paths affordance on amendable parks (BUG-638)", () => {
  // Live run-306526: a RESCOPE escalate verdict names the missing path in
  // prose — no drift marker, so isDrift=false and the Allow button never
  // rendered. The park must still offer a declare-paths input.
  const rescope = awaitingUserDriftState({
    blockReason: "escalate",
    gateReason:
      "DECISION REQUESTED (rescope/contract amend): declare core/security-rasp/src/main/cpp/CMakeLists.txt in scope — required by the contract's own wiring intent",
  });
  assert.equal(rescope.isDrift, false);
  assert.equal(rescope.canAmend, true);

  // Drift parks stay amendable (the Allow button consumes the same affordance).
  assert.equal(
    awaitingUserDriftState({
      blockReason: "escalate",
      gateReason: "flow scope drift: wrote outside the frozen contract's declared paths: a.go",
    }).canAmend,
    true,
  );

  // Owner-actioned parks keep their own buttons — no amend field.
  assert.equal(awaitingUserDriftState({ blockReason: "member_stalled" }).canAmend, false);
  assert.equal(awaitingUserDriftState({ blockReason: "vibe_sprint_boundary" }).canAmend, false);
});

test("awaitingUserDriftState: vibe_sprint_boundary flagged so the card can label Continue", () => {
  // CA-1097 (live run-3362): a finished vibe sprint parks on
  // vibe_sprint_boundary; the action is "start the next task", not a retry —
  // the desktop button read "Retry" while the TUI already renders [Continue].
  const boundary = awaitingUserDriftState({ blockReason: "vibe_sprint_boundary" });
  assert.equal(boundary.isSprintBoundary, true);
  assert.equal(boundary.stalled, false);
  assert.equal(boundary.isCap, false);
});
