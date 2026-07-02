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
(0, node_test_1.default)("resolved approvals are excluded from the group and still render individually as history", () => {
    const timeline = [
        { kind: "approval", id: "appr-0", approvalId: "appr-0", details: approvalDetails, decision: "approve" },
        { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
        { kind: "approval", id: "appr-2", approvalId: "appr-2", details: approvalDetails },
    ];
    const groups = (0, timelineGrouping_1.buildTimelineGroups)(timeline);
    strict_1.default.equal(groups.length, 2, "one resolved card in place + one group for the two pending ones");
    strict_1.default.equal(groups[0].kind, "approval");
    const group = groups[1];
    strict_1.default.equal(group.kind, "approval-group");
    strict_1.default.deepEqual(group.items.map((i) => i.approvalId), ["appr-1", "appr-2"]);
});
(0, node_test_1.default)("tool calls interleaved between concurrent approvals do not break the grouping", () => {
    const timeline = [
        { kind: "approval", id: "appr-1", approvalId: "appr-1", details: approvalDetails },
        { kind: "tool", id: "tool-1", toolName: "search", status: "success" },
        { kind: "approval", id: "appr-2", approvalId: "appr-2", details: approvalDetails },
    ];
    const groups = (0, timelineGrouping_1.buildTimelineGroups)(timeline);
    // tool-group for the single tool call, then the approval-group at the position of appr-2
    strict_1.default.equal(groups.length, 2);
    strict_1.default.equal(groups[0].kind, "tool-group");
    const group = groups[1];
    strict_1.default.equal(group.kind, "approval-group");
    strict_1.default.deepEqual(group.items.map((i) => i.approvalId), ["appr-1", "appr-2"]);
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
