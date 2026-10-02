import test from "node:test";
import assert from "node:assert/strict";
import { sidebarScrollRestoreTarget } from "./sidebarScrollPreserve";

// BUG-563: focusing an agent card must not reset the right rail's scroll
// position. The restore runs after every commit; these cases pin when it
// applies and when it must stay out of the way.

test("restore target applies when the browser clamped scrollTop below the remembered offset", () => {
  assert.equal(sidebarScrollRestoreTarget(0, 480), 480);
  assert.equal(sidebarScrollRestoreTarget(120, 480), 480);
});

test("no restore when nothing was remembered", () => {
  assert.equal(sidebarScrollRestoreTarget(0, 0), null);
});

test("no restore when the user is already at/past the remembered offset", () => {
  // The scroll event already saved the new position — restoring would fight
  // an intentional upward scroll.
  assert.equal(sidebarScrollRestoreTarget(480, 480), null);
  assert.equal(sidebarScrollRestoreTarget(600, 480), null);
});
