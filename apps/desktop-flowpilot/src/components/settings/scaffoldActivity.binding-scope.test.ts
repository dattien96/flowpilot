import test from "node:test";
import assert from "node:assert/strict";

import { fetchScaffoldProgress } from "./projectEngine";

// BUG-549: Engine Settings scopes the scaffold feed to the selected binding by
// forwarding workingDirectory to the runner; callers without a binding context
// (Projects page) keep the legacy project-level query.

function captureFetch() {
  const calls: string[] = [];
  const original = globalThis.fetch;
  globalThis.fetch = async (input: RequestInfo | URL) => {
    calls.push(typeof input === "string" ? input : input instanceof URL ? input.href : input.url);
    return new Response(
      JSON.stringify({ projectId: "p1", active: false, events: [], nextSeq: 1 }),
      { status: 200, headers: { "content-type": "application/json" } },
    );
  };
  return {
    calls,
    restore: () => {
      globalThis.fetch = original;
    },
  };
}

test("fetchScaffoldProgress appends workingDirectory when a binding is selected", async () => {
  const stub = captureFetch();
  try {
    await fetchScaffoldProgress("p1", 7, undefined, "/Users/x/proj-mac");
    assert.equal(stub.calls.length, 1);
    const url = new URL(stub.calls[0]);
    assert.equal(url.searchParams.get("workingDirectory"), "/Users/x/proj-mac");
    assert.equal(url.searchParams.get("after"), "7");
  } finally {
    stub.restore();
  }
});

test("fetchScaffoldProgress omits workingDirectory for project-level callers", async () => {
  const stub = captureFetch();
  try {
    await fetchScaffoldProgress("p1", 3);
    assert.equal(stub.calls.length, 1);
    const url = new URL(stub.calls[0]);
    assert.equal(url.searchParams.get("workingDirectory"), null);
    assert.equal(url.searchParams.get("after"), "3");
  } finally {
    stub.restore();
  }
});
