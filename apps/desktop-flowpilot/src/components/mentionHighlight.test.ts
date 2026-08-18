import test from "node:test";
import assert from "node:assert/strict";
import { findMentionSpans } from "./mentionHighlight";

test("findMentionSpans highlights relative and windows file paths", () => {
  const text = "see apps/foo/ChatInput.tsx and C:\\work\\bar.go please";
  const spans = findMentionSpans(text);
  assert.equal(spans.length, 2);
  assert.equal(text.slice(spans[0].start, spans[0].end), "apps/foo/ChatInput.tsx");
  assert.equal(spans[0].kind, "file");
  assert.equal(text.slice(spans[1].start, spans[1].end), "C:\\work\\bar.go");
});

test("findMentionSpans highlights [skill] and selected skill names", () => {
  const text = "use [coding] and review this";
  const spans = findMentionSpans(text, ["review"]);
  assert.equal(spans.map((s) => text.slice(s.start, s.end)).join(","), "[coding],review");
  assert.ok(spans.every((s) => s.kind === "skill"));
});

test("findMentionSpans skips emails and urls", () => {
  const spans = findMentionSpans("mail a@b.com and https://example.com/x.ts");
  assert.equal(spans.length, 0);
});
