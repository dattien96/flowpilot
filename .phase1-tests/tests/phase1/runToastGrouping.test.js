"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const runToastGrouping_1 = require("../../apps/desktop-flowpilot/src/app/runToastGrouping");
(0, node_test_1.default)("always collapses — even a single notification shows grouped", () => {
    strict_1.default.equal((0, runToastGrouping_1.shouldCollapseToasts)(0), false);
    strict_1.default.equal((0, runToastGrouping_1.shouldCollapseToasts)(runToastGrouping_1.COLLAPSIBLE_TOAST_THRESHOLD), true);
    strict_1.default.equal((0, runToastGrouping_1.shouldCollapseToasts)(runToastGrouping_1.COLLAPSIBLE_TOAST_THRESHOLD + 2), true);
});
(0, node_test_1.default)("builds a stable grouped summary by notification kind", () => {
    strict_1.default.equal((0, runToastGrouping_1.buildToastGroupSummary)([
        { kind: "approval" },
        { kind: "question" },
        { kind: "approval" },
        { kind: "done" },
    ]), "Completed 1 · Approvals 2 · Questions 1");
});
