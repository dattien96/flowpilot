import test from "node:test";
import assert from "node:assert/strict";

import { useStore, scaffoldChatTestHooks } from "./store";
import type { TimelineItem } from "./timelineReducer";

// CA-1000: "Run AI Scaffold" opens Chat and renders the scaffold turn as a
// live transcript — prompt row, one streaming assistant bubble, phase lines,
// terminal result — fed by the runner's persisted progress feed, exactly like
// the TUI /init experience. The dispatch POST blocks for the whole turn; the
// feed carries output/phase/result meanwhile.

type Snap = {
  projectId?: string;
  active: boolean;
  events: Array<Record<string, unknown>>;
  nextSeq: number;
  result?: unknown;
};

function ev(seq: number, kind: string, extra: Record<string, unknown> = {}): Record<string, unknown> {
  return { seq, time: "2026-06-14T10:00:00Z", kind, phase: "", ...extra };
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

interface ScaffoldStub {
  calls: Array<{ url: string; method: string }>;
  progressAfters: number[];
  dispatchBody?: Record<string, unknown>;
  restore: () => void;
}

function stubScaffoldEndpoints(opts: {
  progress: Array<Snap | Error>;
  dispatch?: { body?: unknown; status?: number; error?: Error };
}): ScaffoldStub {
  const queue = [...opts.progress];
  const calls: Array<{ url: string; method: string }> = [];
  const progressAfters: number[] = [];
  const stub: ScaffoldStub = {
    calls,
    progressAfters,
    restore: () => {},
  };
  const original = globalThis.fetch;
  globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    calls.push({ url, method: init?.method ?? "GET" });
    if (url.includes("/scaffold/progress")) {
      progressAfters.push(Number(new URL(url).searchParams.get("after") ?? "0"));
      const next = queue.length > 0 ? queue.shift()! : { active: false, events: [], nextSeq: 0 };
      if (next instanceof Error) throw next;
      return jsonResponse(next);
    }
    if (url.includes("/scaffold")) {
      stub.dispatchBody = JSON.parse(String(init?.body ?? "{}")) as Record<string, unknown>;
      const dispatch = opts.dispatch ?? { body: { status: "done" } };
      if (dispatch.error) throw dispatch.error;
      return jsonResponse(dispatch.body ?? { status: "done" }, dispatch.status ?? 200);
    }
    return new Response("not found", { status: 404 });
  };
  stub.restore = () => {
    globalThis.fetch = original;
  };
  return stub;
}

function seedChatSurface(): void {
  useStore.setState({
    projects: [{ id: "p1", name: "App", path: "/tmp/app" } as never],
    selectedProjectId: "p1",
    chatMode: "normal_chat",
    selectedProvider: "claude",
    status: "idle",
    timeline: [],
    runId: undefined,
    chatId: undefined,
    scaffoldSession: undefined,
    supportedModels: [],
    runHistory: [],
    agentRuns: [],
    pendingApprovals: [],
    pendingQuestions: [],
    _runSnapshots: {},
    _runReplaySeq: {},
    drafts: {},
  } as never);
}

function withFastScaffoldClock(): () => void {
  const prev = { ...scaffoldChatTestHooks };
  scaffoldChatTestHooks.settleDelayMs = 2;
  scaffoldChatTestHooks.pollMs = 2;
  return () => {
    scaffoldChatTestHooks.settleDelayMs = prev.settleDelayMs;
    scaffoldChatTestHooks.pollMs = prev.pollMs;
  };
}

function timelineTexts(items: TimelineItem[], kind?: string): string[] {
  return items.filter((it) => !kind || it.kind === kind).map((it) => ("text" in it ? it.text : ""));
}

test("runScaffoldChat seeds a fresh transcript and streams provider output into one assistant bubble", async () => {
  seedChatSurface();
  const restoreClock = withFastScaffoldClock();
  const stub = stubScaffoldEndpoints({
    progress: [
      {
        active: true,
        nextSeq: 4,
        events: [
          ev(1, "phase", { phase: "started", text: "AI scaffold turn started" }),
          ev(2, "output", { text: "Generating package.json" }),
          ev(3, "phase", { phase: "ai_turn", attempt: 1, text: "AI turn running (attempt 1)" }),
        ],
      },
      { active: true, nextSeq: 5, events: [ev(4, "output", { text: "…done. Running gate." })] },
      {
        active: false,
        nextSeq: 6,
        events: [ev(5, "result", { phase: "done", result: { status: "done", message: "Scaffold done." } })],
        result: { status: "done", message: "Scaffold done." },
      },
    ],
  });
  try {
    await useStore.getState().runScaffoldChat({ projectId: "p1", workingDirectory: "/tmp/app" });
    const s = useStore.getState();
    assert.equal(s.scaffoldSession?.active, false);
    const prompt = s.timeline.find((it) => it.kind === "prompt");
    assert.ok(prompt, "prompt row seeded");
    const bubble = s.timeline.filter((it) => it.kind === "assistant");
    assert.equal(bubble.length, 1, "single assistant bubble");
    assert.equal(bubble[0].kind === "assistant" && bubble[0].text, "Generating package.json…done. Running gate.");
    assert.equal(bubble[0].kind === "assistant" && bubble[0].finalized, true);
    const systemTexts = timelineTexts(s.timeline, "system");
    assert.ok(systemTexts.some((t) => t.includes("AI turn running")), "phase milestone rendered");
    assert.ok(systemTexts.some((t) => t.includes("Scaffold done.")), "terminal message rendered");
    assert.deepEqual(stub.dispatchBody?.trigger, "manual");
    assert.ok(
      calls_includeWorkingDirectory(stub.calls, "workingDirectory=%2Ftmp%2Fapp"),
      "progress polling stays binding-scoped",
    );
  } finally {
    stub.restore();
    restoreClock();
  }
});

test("runScaffoldChat never renders the previous run's tail while the dispatch is still landing", async () => {
  seedChatSurface();
  const restoreClock = withFastScaffoldClock();
  // First poll arrives before the runner's begin cleared the hub: it still
  // serves the PREVIOUS run's events, including its own "started" marker.
  const stub = stubScaffoldEndpoints({
    progress: [
      {
        active: false,
        nextSeq: 6,
        events: [
          ev(1, "phase", { phase: "started", text: "old run started" }),
          ev(2, "output", { text: "OLD OUTPUT MUST NOT RENDER" }),
          ev(3, "result", { phase: "done", result: { status: "done" } }),
        ],
        result: { status: "done" },
      },
      {
        active: true,
        nextSeq: 12,
        events: [
          ev(10, "phase", { phase: "started", text: "AI scaffold turn started" }),
          ev(11, "output", { text: "fresh output" }),
        ],
      },
      {
        active: false,
        nextSeq: 13,
        events: [ev(12, "result", { phase: "done", result: { status: "done", message: "done" } })],
        result: { status: "done", message: "done" },
      },
    ],
  });
  try {
    await useStore.getState().runScaffoldChat({ projectId: "p1", workingDirectory: "/tmp/app" });
    const texts = timelineTexts(useStore.getState().timeline);
    assert.ok(!texts.some((t) => t.includes("OLD OUTPUT MUST NOT RENDER")), "stale tail never renders");
    assert.ok(texts.some((t) => t.includes("fresh output")), "new run output renders");
  } finally {
    stub.restore();
    restoreClock();
  }
});

test("runScaffoldChat surfaces a failed dispatch as an error line and ends the session", async () => {
  seedChatSurface();
  const restoreClock = withFastScaffoldClock();
  const stub = stubScaffoldEndpoints({
    progress: [{ active: false, events: [], nextSeq: 1 }],
    dispatch: { status: 409, body: { error: { message: "scaffold already running" } } },
  });
  try {
    await useStore.getState().runScaffoldChat({ projectId: "p1", workingDirectory: "/tmp/app" });
    const s = useStore.getState();
    assert.equal(s.scaffoldSession?.active, false);
    const errors = s.timeline.filter((it) => it.kind === "system" && it.tone === "error");
    assert.ok(errors.some((it) => it.kind === "system" && it.text.includes("scaffold already running")));
  } finally {
    stub.restore();
    restoreClock();
  }
});

test("resetRun during a scaffold detaches the watcher without cancelling the server turn", async () => {
  seedChatSurface();
  const restoreClock = withFastScaffoldClock();
  // Feed stays active forever — the POST never settles inside this test.
  const stub = stubScaffoldEndpoints({
    progress: [
      { active: true, nextSeq: 3, events: [ev(1, "phase", { phase: "started" }), ev(2, "output", { text: "partial" })] },
    ],
    dispatch: { error: new Error("unused") },
  });
  // Never-settling dispatch: simulate a long-running turn.
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url.endsWith("/scaffold") && init?.method === "POST") {
      return new Promise<Response>(() => {});
    }
    return originalFetch(input, init);
  };
  try {
    const running = useStore.getState().runScaffoldChat({ projectId: "p1", workingDirectory: "/tmp/app" });
    await new Promise((resolve) => setTimeout(resolve, 30));
    assert.equal(useStore.getState().scaffoldSession?.active, true);
    useStore.getState().resetRun();
    assert.equal(useStore.getState().scaffoldSession, undefined);
    assert.equal(useStore.getState().timeline.length, 0);
    await running;
    assert.equal(useStore.getState().timeline.length, 0, "pump stopped writing after detach");
  } finally {
    globalThis.fetch = originalFetch;
    stub.restore();
    restoreClock();
  }
});

test("sendPrompt is refused while a scaffold turn writes the workspace", async () => {
  seedChatSurface();
  const restoreClock = withFastScaffoldClock();
  const stub = stubScaffoldEndpoints({
    progress: [{ active: true, nextSeq: 2, events: [ev(1, "phase", { phase: "started" })] }],
  });
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    if (url.endsWith("/scaffold") && init?.method === "POST") {
      return new Promise<Response>(() => {});
    }
    return originalFetch(input, init);
  };
  try {
    const running = useStore.getState().runScaffoldChat({ projectId: "p1", workingDirectory: "/tmp/app" });
    await new Promise((resolve) => setTimeout(resolve, 20));
    const before = useStore.getState().timeline.length;
    await useStore.getState().sendPrompt("hello");
    const s = useStore.getState();
    const warnings = s.timeline.filter((it) => it.kind === "system" && it.tone === "warn");
    assert.ok(warnings.length > 0, "warn line explains the refused send");
    assert.ok(
      !s.timeline.slice(before).some((it) => it.kind === "prompt"),
      "no prompt row was added",
    );
    useStore.getState().resetRun();
    await running;
  } finally {
    globalThis.fetch = originalFetch;
    stub.restore();
    restoreClock();
  }
});

function calls_includeWorkingDirectory(calls: Array<{ url: string }>, needle: string): boolean {
  return calls.some((call) => call.url.includes(needle));
}
