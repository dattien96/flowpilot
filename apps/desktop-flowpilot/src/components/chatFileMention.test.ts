import test from "node:test";
import assert from "node:assert/strict";
import {
  filterWorkspaceFiles,
  findActiveAt,
  insertAtMention,
  isAgentAtMention,
} from "./chatFileMention";

test("findActiveAt reads @query at the start and after a space", () => {
  assert.deepEqual(findActiveAt("@ChatIn", 7), { index: 0, query: "ChatIn" });
  assert.deepEqual(findActiveAt("see @apps/foo", 13), { index: 4, query: "apps/foo" });
});

test("findActiveAt ignores email-style @ and stops at a space", () => {
  assert.equal(findActiveAt("a@b.ts", 6), null);
  assert.equal(findActiveAt("@foo bar", 8), null);
});

test("isAgentAtMention keeps start-of-prompt @coder as agent routing", () => {
  assert.equal(isAgentAtMention({ index: 0, query: "coder" }, ["coder"]), true);
  assert.equal(isAgentAtMention({ index: 0, query: "apps/foo" }, ["coder"]), false);
  assert.equal(isAgentAtMention({ index: 4, query: "coder" }, ["coder"]), false);
});

test("filterWorkspaceFiles matches substring and caps", () => {
  const paths = [
    "apps/desktop-flowpilot/src/components/ChatInput.tsx",
    "apps/local-runner/internal/runner/workspace_files.go",
  ];
  assert.deepEqual(filterWorkspaceFiles(paths, "ChatInput"), [paths[0]]);
  assert.equal(filterWorkspaceFiles(paths, "", 1).length, 1);
});

test("insertAtMention replaces @query with the path like a skill name", () => {
  const got = insertAtMention("see @ChatIn please", { index: 4, query: "ChatIn" }, 11, "apps/foo/ChatInput.tsx");
  assert.equal(got.text, "see apps/foo/ChatInput.tsx please");
  assert.equal(got.cursor, 4 + "apps/foo/ChatInput.tsx".length);
});
