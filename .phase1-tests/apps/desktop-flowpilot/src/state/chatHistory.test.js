"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const chatHistory_1 = require("./chatHistory");
const item = (over) => ({
    runId: "run-x",
    projectId: "p",
    providerKey: "codex",
    status: "completed",
    startedAt: "",
    updatedAt: "",
    ...over,
});
(0, node_test_1.default)("groupRunsByChatId collapses legs under one chat with the latest head", () => {
    const rows = (0, chatHistory_1.groupRunsByChatId)([
        item({ runId: "run-1", chatId: "cht_a", legSeq: 0, providerKey: "codex" }),
        item({ runId: "run-2", chatId: "cht_a", legSeq: 1, providerKey: "grok" }),
        item({ runId: "run-wf" }),
    ]);
    strict_1.default.equal(rows.length, 2);
    strict_1.default.equal(rows[0].group?.legs.length, 2);
    strict_1.default.equal(rows[0].item.runId, "run-2"); // latest leg is the face
    strict_1.default.equal(rows[0].group?.legs[0].runId, "run-1"); // legs keep insertion order (oldest first)
    strict_1.default.equal(rows[0].group?.legs[1].runId, "run-2");
    strict_1.default.equal(rows[1].group, undefined); // untagged passes through 1:1
});
(0, node_test_1.default)("flattenGroupedHistory exposes a legsCount chip", () => {
    const rows = (0, chatHistory_1.groupRunsByChatId)([
        item({ runId: "run-1", chatId: "cht_a", legSeq: 0 }),
        item({ runId: "run-2", chatId: "cht_a", legSeq: 1 }),
    ]);
    const flat = (0, chatHistory_1.flattenGroupedHistory)(rows);
    strict_1.default.equal(flat.length, 1);
    strict_1.default.equal(flat[0].legsCount, 2);
});
