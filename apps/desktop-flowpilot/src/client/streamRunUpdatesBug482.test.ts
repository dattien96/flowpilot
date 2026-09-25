import test from "node:test";
import assert from "node:assert/strict";
import { HttpWsRunnerClient, RunnerApiError } from "./HttpWsRunnerClient";
import type { RunRealtimeFrame } from "@/types/contract";

// BUG-482: mux frames are authoritative level-state changes — silently
// skipping a malformed one leaves stale lane/decision state with no resync
// until an unrelated mutation or the 30s poll. The parser must terminate
// the stream with a typed stream_protocol_error so consumeRunUpdatesLoop
// reconnects for a healing full snapshot. Metadata in diagnostics only —
// never payload text.

const enc = new TextEncoder();

function sseBody(chunks: string[]): ReadableStream<Uint8Array> {
  return new ReadableStream({
    start(controller) {
      for (const c of chunks) controller.enqueue(enc.encode(c));
      controller.close();
    },
  });
}

function stubFetch(body: ReadableStream<Uint8Array>, status = 200): () => void {
  const orig = globalThis.fetch;
  globalThis.fetch = (async () =>
    new Response(body, {
      status,
      headers: { "content-type": "text/event-stream" },
    })) as typeof fetch;
  return () => {
    globalThis.fetch = orig;
  };
}

async function collect(client: HttpWsRunnerClient, signal?: AbortSignal): Promise<RunRealtimeFrame[]> {
  const out: RunRealtimeFrame[] = [];
  for await (const f of client.streamRunUpdates(signal)) out.push(f);
  return out;
}

const UPSERT = JSON.stringify({
  kind: "upsert",
  run: {
    runId: "r1",
    projectId: "p-1",
    revision: 1,
    status: "running",
    updatedAt: "2026-01-01T00:00:00Z",
  },
});

test("BUG-482: malformed authoritative frame throws typed stream_protocol_error", async () => {
  const restore = stubFetch(
    sseBody([`data: ${UPSERT}\n\n`, "data: {not-json\n\n", `data: ${UPSERT}\n\n`]),
  );
  try {
    const frames: RunRealtimeFrame[] = [];
    await assert.rejects(
      (async () => {
        for await (const f of new HttpWsRunnerClient("http://localhost:4317").streamRunUpdates()) {
          frames.push(f);
        }
      })(),
      (err: unknown) =>
        err instanceof RunnerApiError &&
        (err as RunnerApiError).code === "stream_protocol_error",
    );
    // Frames BEFORE the malformed one were delivered; frames after it on the
    // same connection must NOT be yielded — the state they describe is
    // unverifiable and only a fresh snapshot can heal it.
    assert.deepEqual(frames.map((f) => f.kind), ["upsert"]);
  } finally {
    restore();
  }
});

test("BUG-482: protocol error diagnostic carries metadata, never payload text", async () => {
  const secret = "user-decision-payload-secret-text";
  const restore = stubFetch(sseBody([`data: {"kind":"upsert","run":{"x":"${secret}"}\n\n`]));
  try {
    await assert.rejects(
      collect(new HttpWsRunnerClient("http://localhost:4317")),
      (err: unknown) => {
        assert.ok(err instanceof RunnerApiError);
        const msg = String((err as RunnerApiError).message);
        assert.ok(!msg.includes(secret), "error leaked frame payload text");
        return (err as RunnerApiError).code === "stream_protocol_error";
      },
    );
  } finally {
    restore();
  }
});

test("BUG-482: CRLF frame separators parse like LF", async () => {
  const restore = stubFetch(
    sseBody([`data: ${UPSERT}\r\n\r\ndata: {"kind":"resync","retryable":true}\r\n\r\n`]),
  );
  try {
    const frames = await collect(new HttpWsRunnerClient("http://localhost:4317"));
    assert.deepEqual(frames.map((f) => f.kind), ["upsert", "resync"]);
  } finally {
    restore();
  }
});

test("BUG-482: multi-line data: fields concatenate per SSE spec", async () => {
  // Split at a JSON token boundary — the spec joins data lines with "\n",
  // which is legal JSON whitespace between tokens (but not inside a string).
  const pivot = UPSERT.indexOf(`"run"`);
  const restore = stubFetch(
    sseBody([`data: ${UPSERT.slice(0, pivot)}\ndata: ${UPSERT.slice(pivot)}\n\n`]),
  );
  try {
    const frames = await collect(new HttpWsRunnerClient("http://localhost:4317"));
    assert.equal(frames.length, 1);
    assert.equal(frames[0].kind, "upsert");
    assert.equal(frames[0].run?.runId, "r1");
  } finally {
    restore();
  }
});

test("BUG-482: malformed mid-snapshot chunk throws — partial burst cannot commit", async () => {
  const good = JSON.stringify({
    kind: "snapshot", snapshotId: "s9", complete: false,
    runs: [{ runId: "rA", projectId: "p-1", revision: 1, status: "running", updatedAt: "2026-01-01T00:00:00Z" }],
  });
  const restore = stubFetch(
    sseBody([`data: ${good}\n\n`, "data: {truncated\n\n", `data: ${UPSERT}\n\n`]),
  );
  try {
    const frames: RunRealtimeFrame[] = [];
    await assert.rejects(
      (async () => {
        for await (const f of new HttpWsRunnerClient("http://localhost:4317").streamRunUpdates()) {
          frames.push(f);
        }
      })(),
      (err: unknown) => err instanceof RunnerApiError && (err as RunnerApiError).code === "stream_protocol_error",
    );
    // Only the first (valid) chunk surfaced; the malformed chunk killed the
    // connection before any trailing frame could pretend the burst completed.
    assert.equal(frames.length, 1);
  } finally {
    restore();
  }
});
