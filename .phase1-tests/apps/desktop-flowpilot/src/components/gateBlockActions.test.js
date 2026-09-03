"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const strict_1 = __importDefault(require("node:assert/strict"));
const node_test_1 = __importDefault(require("node:test"));
const gateBlockActions_1 = require("./gateBlockActions");
(0, node_test_1.default)("BUG-291: regression decision cards cannot orphan a blocked child with a local dismiss", () => {
    strict_1.default.equal((0, gateBlockActions_1.gateBlockSecondaryAction)(["keep-test-fix-code", "suggest-requirement-change"]), "stop-flow");
});
(0, node_test_1.default)("BUG-291: plain informational gate blocks remain locally acknowledgeable", () => {
    strict_1.default.equal((0, gateBlockActions_1.gateBlockSecondaryAction)(undefined), "dismiss");
    strict_1.default.equal((0, gateBlockActions_1.gateBlockSecondaryAction)([]), "dismiss");
});
