"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const questionAnswer_1 = require("../../apps/desktop-flowpilot/src/components/questionAnswer");
(0, node_test_1.default)("single-select manual submit uses typed other text instead of a previous option", () => {
    strict_1.default.equal((0, questionAnswer_1.resolveQuestionManualSubmit)(["Python"], " Rust ", false), "Rust");
});
(0, node_test_1.default)("single-select manual submit waits for typed other text", () => {
    strict_1.default.equal((0, questionAnswer_1.resolveQuestionManualSubmit)(["Python"], "", false), undefined);
});
(0, node_test_1.default)("multi-select manual submit preserves selected options and appends typed other text", () => {
    strict_1.default.deepEqual((0, questionAnswer_1.resolveQuestionManualSubmit)(["Python"], "Rust", true), ["Python", "Rust"]);
});
