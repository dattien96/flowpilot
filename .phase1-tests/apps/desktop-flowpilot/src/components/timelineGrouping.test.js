"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const timelineGrouping_1 = require("./timelineGrouping");
const approvalDetails = { decisions: [{ value: "approve", label: "Approve" }, { value: "deny", label: "Deny" }] };
const questionOptions = [{ label: "Python", value: "Python" }, { label: "TypeScript", value: "TypeScript" }];
function findGroup(groups, kind) {
    return groups.find((g) => g.kind === kind);
}
(0, node_test_1.default)("BUG-157 UX: a single pending approval renders as a plain card, no group wrapper", () => {
    const timeline = [
        { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
    ];
    const groups = (0, timelineGrouping_1.buildTimelineGroups)(timeline);
    strict_1.default.equal(groups.length, 1);
    strict_1.default.equal(groups[0].kind, "approval");
});
(0, node_test_1.default)("BUG-157 UX: several concurrently pending approvals fold into one approval-group", () => {
    const timeline = [
        { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
        { kind: "approval", id: "appr-2", approvalId: "appr-2", details: approvalDetails },
        { kind: "approval", id: "appr-3", approvalId: "appr-3", details: approvalDetails },
    ];
    const groups = (0, timelineGrouping_1.buildTimelineGroups)(timeline);
    strict_1.default.equal(groups.length, 1, "all three should collapse into a single group, not three separate cards");
    const group = groups[0];
    strict_1.default.equal(group.kind, "approval-group");
    strict_1.default.deepEqual(group.items.map((i) => i.approvalId), ["appr-1", "appr-2", "appr-3"]);
});
(0, node_test_1.default)("resolving every item in a group keeps them grouped instead of bursting back into full cards", () => {
    // Regression: clicking "Approve all" must not dissolve the group. Grouping is by
    // consecutive run, not "still pending", so a fully-resolved run stays one group.
    const timeline = [
        { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails, decision: "approve" },
        { kind: "approval", id: "appr-2", approvalId: "appr-2", details: approvalDetails, decision: "approve" },
        { kind: "approval", id: "appr-3", approvalId: "appr-3", details: approvalDetails, decision: "approve" },
    ];
    const groups = (0, timelineGrouping_1.buildTimelineGroups)(timeline);
    strict_1.default.equal(groups.length, 1, "resolved run must stay folded, not burst into 3 separate cards");
    const group = groups[0];
    strict_1.default.equal(group.kind, "approval-group");
    strict_1.default.deepEqual(group.items.map((i) => i.approvalId), ["appr-1", "appr-2", "appr-3"]);
});
(0, node_test_1.default)("a mixed run of resolved and still-pending approvals stays one group", () => {
    const timeline = [
        { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails, decision: "approve" },
        { kind: "approval", id: "appr-2", approvalId: "appr-2", details: approvalDetails },
        { kind: "approval", id: "appr-3", approvalId: "appr-3", details: approvalDetails },
    ];
    const groups = (0, timelineGrouping_1.buildTimelineGroups)(timeline);
    strict_1.default.equal(groups.length, 1);
    const group = groups[0];
    strict_1.default.equal(group.kind, "approval-group");
    strict_1.default.deepEqual(group.items.map((i) => i.approvalId), ["appr-1", "appr-2", "appr-3"]);
});
(0, node_test_1.default)("a resolved approval run followed later by an unrelated new one stays separate (not part of the old group)", () => {
    const timeline = [
        { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails, decision: "approve" },
        { kind: "approval", id: "appr-2", approvalId: "appr-2", details: approvalDetails, decision: "approve" },
        { kind: "tool", id: "tool-1", toolName: "search", status: "success" },
        { kind: "approval", id: "appr-3", approvalId: "appr-3", details: approvalDetails },
    ];
    const groups = (0, timelineGrouping_1.buildTimelineGroups)(timeline);
    strict_1.default.equal(groups.length, 3, "old resolved group, tool-group, then the new single pending approval");
    strict_1.default.equal(groups[0].kind, "approval-group");
    strict_1.default.equal(groups[1].kind, "tool-group");
    strict_1.default.equal(groups[2].kind, "approval");
});
(0, node_test_1.default)("questions and approvals group independently (group-by-kind, not merged into one ask group)", () => {
    const timeline = [
        { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
        { kind: "approval", id: "appr-2", approvalId: "appr-2", details: approvalDetails },
        { kind: "question", id: "q-1", questionId: "q-1", prompt: "Pick one", options: questionOptions },
        { kind: "question", id: "q-2", questionId: "q-2", prompt: "Pick two", options: questionOptions },
    ];
    const groups = (0, timelineGrouping_1.buildTimelineGroups)(timeline);
    strict_1.default.equal(groups.length, 2);
    const approvalGroup = findGroup(groups, "approval-group");
    const questionGroup = findGroup(groups, "question-group");
    strict_1.default.equal(approvalGroup.items.length, 2);
    strict_1.default.equal(questionGroup.items.length, 2);
});
(0, node_test_1.default)("a single pending question renders as a plain card, no group wrapper", () => {
    const timeline = [
        { kind: "question", id: "q-1", questionId: "q-1", prompt: "Pick one", options: questionOptions },
    ];
    const groups = (0, timelineGrouping_1.buildTimelineGroups)(timeline);
    strict_1.default.equal(groups.length, 1);
    strict_1.default.equal(groups[0].kind, "question");
});
