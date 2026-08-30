"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const mentionHighlight_1 = require("./mentionHighlight");
(0, node_test_1.default)("findMentionSpans highlights relative and windows file paths", () => {
    const text = "see apps/foo/ChatInput.tsx and C:\\work\\bar.go please";
    const spans = (0, mentionHighlight_1.findMentionSpans)(text);
    strict_1.default.equal(spans.length, 2);
    strict_1.default.equal(text.slice(spans[0].start, spans[0].end), "apps/foo/ChatInput.tsx");
    strict_1.default.equal(spans[0].kind, "file");
    strict_1.default.equal(text.slice(spans[1].start, spans[1].end), "C:\\work\\bar.go");
});
(0, node_test_1.default)("findMentionSpans highlights [skill] and selected skill names", () => {
    const text = "use [coding] and review this";
    const spans = (0, mentionHighlight_1.findMentionSpans)(text, ["review"]);
    strict_1.default.equal(spans.map((s) => text.slice(s.start, s.end)).join(","), "[coding],review");
    strict_1.default.ok(spans.every((s) => s.kind === "skill"));
});
(0, node_test_1.default)("findMentionSpans skips emails and urls", () => {
    const spans = (0, mentionHighlight_1.findMentionSpans)("mail a@b.com and https://example.com/x.ts");
    strict_1.default.equal(spans.length, 0);
});
