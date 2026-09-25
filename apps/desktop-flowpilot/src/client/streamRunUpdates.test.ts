import test from "node:test";
import assert from "node:assert/strict";
import { HttpWsRunnerClient, RunnerApiError } from "./HttpWsRunnerClient";
import type { RunRealtimeFrame } from "@/types/contract";

// CP-84 / Task-429 (T-5): the SSE parser behind streamRunUpdates — fragmented
// frames, multi-frame chunks, malformed JSON, heartbeat comments, abort and
// non-OK responses. Global fetch is stubbed per test and restored after.
// Additive-only — no existing test touched.

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

test("frame split across chunks is reassembled before parsing", async () => {
  const restore = stubFetch(
    sseBody([`data: ${UPSERT.slice(0, 40)}`, `${UPSERT.slice(40)}\n\n`]),
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

test("multiple frames in one chunk are all yielded in order", async () => {
  const chunk = `data: ${UPSERT}\n\ndata: {"kind":"resync","retryable":true}\n\n`;
  const restore = stubFetch(sseBody([chunk]));
  try {
    const frames = await collect(new HttpWsRunnerClient("http://localhost:4317"));
    assert.deepEqual(frames.map((f) => f.kind), ["upsert", "resync"]);
  } finally {
    restore();
  }
});

// BUG-482: this test previously pinned "skip malformed, keep streaming" —
// that WAS the defect. Authoritative frames must fail the stream so the
// consumer reconnects for a healing snapshot.
test("malformed JSON frame terminates the stream with stream_protocol_error", async () => {
  const restore = stubFetch(
    sseBody(["data: {not-json\n\n", `data: ${UPSERT}\n\n`]),
  );
  try {
    await assert.rejects(
      collect(new HttpWsRunnerClient("http://localhost:4317")),
      (err: unknown) =>
        err instanceof RunnerApiError && (err as RunnerApiError).code === "stream_protocol_error",
    );
  } finally {
    restore();
  }
});

test("heartbeat comment lines are ignored", async () => {
  const restore = stubFetch(
    sseBody([": heartbeat\n\n", `data: ${UPSERT}\n\n`]),
  );
  try {
    const frames = await collect(new HttpWsRunnerClient("http://localhost:4317"));
    assert.equal(frames.length, 1);
  } finally {
    restore();
  }
});

test("non-OK response throws RunnerApiError", async () => {
  const restore = stubFetch(sseBody([]), 503);
  try {
    await assert.rejects(
      collect(new HttpWsRunnerClient("http://localhost:4317")),
      (err: unknown) => err instanceof RunnerApiError,
    );
  } finally {
    restore();
  }
});

test("pre-aborted signal yields nothing", async () => {
  const restore = stubFetch(sseBody([`data: ${UPSERT}\n\n`]));
  try {
    const ctrl = new AbortController();
    ctrl.abort();
    const frames = await collect(new HttpWsRunnerClient("http://localhost:4317"), ctrl.signal);
    assert.equal(frames.length, 0);
  } finally {
    restore();
  }
});
