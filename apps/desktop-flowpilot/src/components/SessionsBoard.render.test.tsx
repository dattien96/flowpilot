import test from "node:test";
import assert from "node:assert/strict";
import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { setupDom } from "../testhelpers/domHarness";
import { SessionsBoard } from "./SessionsBoard";
import { useStore } from "../state/store";
import type { AttentionItem } from "../state/attentionQueue";
import type { RunHistoryItem } from "@/types/contract";

// CP-82 / Task-422 (KR-005): first React-render coverage for the sessions
// monitor — the pure boardModel tests cover grouping math; these prove the
// JSX actually wires rows to openRunAtAttention, renders waiting chips, and
// closes on Escape. Runs under node --test via the jsdom harness.
// Additive-only — no existing test touched.

function hist(runId: string, projectId: string, over: Partial<RunHistoryItem> = {}): RunHistoryItem {
  return {
    runId,
    chatId: "chat-" + runId,
    projectId,
    providerKey: "claude",
    status: "running",
    startedAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...over,
  } as RunHistoryItem;
}

function attn(runId: string, kind: AttentionItem["kind"], projectId = "p-1"): AttentionItem {
  return {
    runId,
    chatId: "chat-" + runId,
    projectId,
    runTitle: "t-" + runId,
    kind,
    waitingSince: "2026-01-01T00:00:00Z",
  };
}

interface Harness {
  container: HTMLElement;
  cleanup: () => Promise<void>;
}

async function renderBoard(onClose: () => void): Promise<Harness> {
  const restoreDom = setupDom();
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root: Root = createRoot(container);
  await act(async () => {
    root.render(<SessionsBoard onClose={onClose} />);
  });
  return {
    container,
    cleanup: async () => {
      await act(async () => {
        root.unmount();
      });
      container.remove();
      restoreDom();
    },
  };
}

function seedStore(over: {
  history?: Record<string, RunHistoryItem[]>;
  projects?: { id: string; name: string }[];
  attention?: AttentionItem[];
  focusedRunId?: string;
  openRunAtAttention?: (runId: string, chatId: string, projectId?: string) => Promise<void>;
}) {
  const s = useStore.getState();
  const saved = {
    projectHistoryById: s.projectHistoryById,
    projects: s.projects,
    attentionItems: s.attentionItems,
    runId: s.runId,
    openRunAtAttention: s.openRunAtAttention,
  };
  useStore.setState({
    projectHistoryById: over.history ?? {},
    projects: (over.projects ?? []) as never,
    attentionItems: over.attention ?? [],
    runId: over.focusedRunId,
    ...(over.openRunAtAttention ? { openRunAtAttention: over.openRunAtAttention as never } : {}),
  });
  return () =>
    useStore.setState({
      projectHistoryById: saved.projectHistoryById,
      projects: saved.projects,
      attentionItems: saved.attentionItems,
      runId: saved.runId,
      openRunAtAttention: saved.openRunAtAttention,
    });
}

test("renders sections grouped by project with run rows and waiting chip", async () => {
  const restore = seedStore({
    history: {
      "p-1": [hist("r-wait", "p-1", { status: "waiting_approval", lastPrompt: "fix the flaky test" })],
      "p-2": [hist("r-run", "p-2", { status: "running" })],
    },
    projects: [
      { id: "p-1", name: "Alpha" },
      { id: "p-2", name: "Beta" },
    ],
    attention: [attn("r-wait", "approval")],
  });
  const h = await renderBoard(() => {});
  try {
    const names = [...h.container.querySelectorAll(".board-section-name")].map((n) => n.textContent);
    assert.deepEqual(names, ["Alpha", "Beta"]);
    assert.equal(h.container.querySelectorAll(".board-row").length, 2);
    const chip = h.container.querySelector(".attention-kind");
    assert.ok(chip, "waiting row must render the attention-kind chip");
    assert.match(chip!.textContent ?? "", /Approval/);
    assert.match(h.container.textContent ?? "", /fix the flaky test/);
  } finally {
    await h.cleanup();
    restore();
  }
});

test("row click routes through openRunAtAttention then closes", async () => {
  const calls: string[] = [];
  const restore = seedStore({
    history: { "p-1": [hist("r-1", "p-1")] },
    projects: [{ id: "p-1", name: "Alpha" }],
    openRunAtAttention: async (runId, chatId, projectId) => {
      calls.push(`open:${runId}:${chatId}:${projectId}`);
    },
  });
  let closed = 0;
  const h = await renderBoard(() => {
    closed++;
  });
  try {
    const row = h.container.querySelector<HTMLButtonElement>(".board-row");
    assert.ok(row);
    await act(async () => {
      row!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    assert.equal(closed, 1, "click must close the board");
    assert.deepEqual(calls, ["open:r-1:chat-r-1:p-1"]);
  } finally {
    await h.cleanup();
    restore();
  }
});

test("Escape key closes the board", async () => {
  const restore = seedStore({ history: {}, projects: [] });
  let closed = 0;
  const h = await renderBoard(() => {
    closed++;
  });
  try {
    await act(async () => {
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    });
    assert.equal(closed, 1);
  } finally {
    await h.cleanup();
    restore();
  }
});

test("empty state renders when no runs exist", async () => {
  const restore = seedStore({ history: {}, projects: [] });
  const h = await renderBoard(() => {});
  try {
    const empty = h.container.querySelector(".board-empty");
    assert.ok(empty, "empty state element missing");
    assert.match(empty!.textContent ?? "", /No runs yet/);
  } finally {
    await h.cleanup();
    restore();
  }
});

test("focused run row carries the focused class", async () => {
  const restore = seedStore({
    history: { "p-1": [hist("r-focus", "p-1"), hist("r-other", "p-1")] },
    projects: [{ id: "p-1", name: "Alpha" }],
    focusedRunId: "r-focus",
  });
  const h = await renderBoard(() => {});
  try {
    const rows = [...h.container.querySelectorAll<HTMLButtonElement>(".board-row")];
    const focused = rows.filter((r) => r.classList.contains("focused"));
    assert.equal(focused.length, 1, "exactly one focused row");
    assert.match(focused[0].title, /r-focus/);
  } finally {
    await h.cleanup();
    restore();
  }
});
