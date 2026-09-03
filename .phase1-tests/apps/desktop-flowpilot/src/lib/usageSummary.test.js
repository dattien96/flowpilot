"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const usageSummary_1 = require("./usageSummary");
(0, node_test_1.default)("contextRemainingPercent matches Grok-style remaining window percent", () => {
    strict_1.default.equal((0, usageSummary_1.contextRemainingPercent)(12000, 128000), 91);
    strict_1.default.equal((0, usageSummary_1.contextRemainingPercent)(128000, 128000), 0);
    strict_1.default.equal((0, usageSummary_1.contextRemainingPercent)(0, 100000), 100);
    strict_1.default.equal((0, usageSummary_1.contextRemainingPercent)(10, 0), 0);
});
(0, node_test_1.default)("formatAccountRemainingLabel prefers weekly credits like Grok /usage", () => {
    strict_1.default.equal((0, usageSummary_1.formatAccountRemainingLabel)({ remaining7dPercent: 87, remaining5hPercent: 40 }), "7d 87%");
    strict_1.default.equal((0, usageSummary_1.formatAccountRemainingLabel)({ remaining7dPercent: null, remaining5hPercent: 40 }), "5h 40%");
    strict_1.default.equal((0, usageSummary_1.formatAccountRemainingLabel)({
        remaining7dPercent: null,
        remaining5hPercent: null,
        usageDetailLines: [{ label: "Weekly limit", remainingPercent: 99 }],
    }), "Weekly limit 99%");
    strict_1.default.equal((0, usageSummary_1.formatAccountRemainingLabel)(null), null);
});
