"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const strict_1 = __importDefault(require("node:assert/strict"));
const node_test_1 = __importDefault(require("node:test"));
const flowAwaitingUserDrift_1 = require("./flowAwaitingUserDrift");
(0, node_test_1.default)("parseDriftedPaths: nil when not a drift gate", () => {
    strict_1.default.equal((0, flowAwaitingUserDrift_1.parseDriftedPaths)("Reviewer requested escalate"), null);
});
(0, node_test_1.default)("parseDriftedPaths: single and multi path", () => {
    strict_1.default.deepEqual((0, flowAwaitingUserDrift_1.parseDriftedPaths)("flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go"), ["calc_test.go"]);
    strict_1.default.deepEqual((0, flowAwaitingUserDrift_1.parseDriftedPaths)("flow scope drift: wrote outside the frozen contract's declared paths: a.go, b.go"), ["a.go", "b.go"]);
});
(0, node_test_1.default)("parseDriftedPaths: trims trailing period and empty segments", () => {
    strict_1.default.deepEqual((0, flowAwaitingUserDrift_1.parseDriftedPaths)("wrote outside the frozen contract's declared paths: calc_test.go."), ["calc_test.go"]);
    strict_1.default.equal((0, flowAwaitingUserDrift_1.parseDriftedPaths)("wrote outside the frozen contract's declared paths: "), null);
});
(0, node_test_1.default)("awaitingUserDriftState: Allow only on non-cap non-stalled drift", () => {
    const drift = (0, flowAwaitingUserDrift_1.awaitingUserDriftState)({
        blockReason: "escalate",
        gateReason: "flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go",
    });
    strict_1.default.equal(drift.isDrift, true);
    strict_1.default.deepEqual(drift.driftedPaths, ["calc_test.go"]);
    strict_1.default.equal(drift.retryIsPrimary, false);
    const cap = (0, flowAwaitingUserDrift_1.awaitingUserDriftState)({ blockReason: "cap", gateReason: "" });
    strict_1.default.equal(cap.isDrift, false);
    strict_1.default.equal(cap.retryIsPrimary, true);
    const stalled = (0, flowAwaitingUserDrift_1.awaitingUserDriftState)({
        blockReason: "member_stalled",
        gateReason: "flow scope drift: wrote outside the frozen contract's declared paths: a.go",
    });
    strict_1.default.equal(stalled.isDrift, false);
    strict_1.default.equal(stalled.stalled, true);
    const escalate = (0, flowAwaitingUserDrift_1.awaitingUserDriftState)({
        blockReason: "escalate",
        gateReason: "Reviewer requested escalate: missing clamp",
    });
    strict_1.default.equal(escalate.isDrift, false);
    strict_1.default.equal(escalate.retryIsPrimary, true);
});
