import test from "node:test";
import assert from "node:assert/strict";
import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { setupDom } from "../testhelpers/domHarness";
import { SpectatorPane } from "./SpectatorPane";
import { useStore } from "../state/store";
import type { AttentionItem } from "../state/attentionQueue";
import type { RunHistoryItem } from "@/types/contract";

// CP-82 / Task-425 (KR-005): React-render coverage for the spectator pane —
// deriveSpectatorView unit tests cover the data projection; these pin the
// JSX contract: null for the focused run, missing-data placeholder, body
// click → openRunAtAttention, close button → closeSpectator.
// Additive-only — no existing test touched.

function hist(runId: string, projectId: string, over: Partial<RunHistoryItem> = {}): RunHistoryItem {
  return {
    runId,
    chatId: "chat-" + runId,
    projectId,
    providerKey: "codex",
    status: "running",
    startedAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...over,
  } as RunHistoryItem;
}

interface Harness {
  container: HTMLElement;
  cleanup: () => Promise<void>;
}

async function renderPane(): Promise<Harness> {
  const restoreDom = setupDom();
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root: Root = createRoot(container);
  await act(async () => {
    root.render(<SpectatorPane />);
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
  spectatorRunId?: string;
  spectatorProjectId?: string;
  focusedRunId?: string;
  openRunAtAttention?: (runId: string, chatId: string, projectId?: string) => Promise<void>;
  closeSpectator?: () => void;
}) {
  const s = useStore.getState();
  const saved = {
    projectHistoryById: s.projectHistoryById,
    projects: s.projects,
    attentionItems: s.attentionItems,
    spectatorRunId: s.spectatorRunId,
    spectatorProjectId: s.spectatorProjectId,
    runId: s.runId,
    openRunAtAttention: s.openRunAtAttention,
    closeSpectator: s.closeSpectator,
  };
  useStore.setState({
    projectHistoryById: over.history ?? {},
    projects: (over.projects ?? []) as never,
    attentionItems: over.attention ?? [],
    spectatorRunId: over.spectatorRunId,
    spectatorProjectId: over.spectatorProjectId,
    runId: over.focusedRunId,
    ...(over.openRunAtAttention ? { openRunAtAttention: over.openRunAtAttention as never } : {}),
    ...(over.closeSpectator ? { closeSpectator: over.closeSpectator as never } : {}),
  });
  return () =>
    useStore.setState({
      projectHistoryById: saved.projectHistoryById,
      projects: saved.projects,
      attentionItems: saved.attentionItems,
      spectatorRunId: saved.spectatorRunId,
      spectatorProjectId: saved.spectatorProjectId,
      runId: saved.runId,
      openRunAtAttention: saved.openRunAtAttention,
      closeSpectator: saved.closeSpectator,
    });
}

test("renders nothing when spectated run is the focused run", async () => {
  const restore = seedStore({
    history: { "p-1": [hist("r-1", "p-1")] },
    projects: [{ id: "p-1", name: "Alpha" }],
    spectatorRunId: "r-1",
    focusedRunId: "r-1",
  });
  const h = await renderPane();
  try {
    assert.equal(h.container.querySelector(".spectator-pane"), null);
  } finally {
    await h.cleanup();
    restore();
  }
});

test("renders watched run with status, title, last line", async () => {
  const restore = seedStore({
    history: {
      "p-1": [
        hist("r-watch", "p-1", {
          status: "waiting_question",
          lastPrompt: "wire the gate",
          lastMessage: "which model should I use?",
        }),
      ],
    },
    projects: [{ id: "p-1", name: "Alpha" }],
    attention: [
      {
        runId: "r-watch",
        chatId: "chat-r-watch",
        projectId: "p-1",
        runTitle: "wire the gate",
        kind: "question",
        waitingSince: "2026-01-01T00:00:00Z",
      },
    ],
    spectatorRunId: "r-watch",
    spectatorProjectId: "p-1",
  });
  const h = await renderPane();
  try {
    const pane = h.container.querySelector(".spectator-pane");
    assert.ok(pane);
    assert.match(pane!.textContent ?? "", /Watching · Alpha/);
    assert.match(pane!.textContent ?? "", /wire the gate/);
    assert.match(pane!.textContent ?? "", /which model should I use/);
    const chip = pane!.querySelector(".attention-kind");
    assert.ok(chip, "waiting chip missing");
    assert.match(chip!.textContent ?? "", /Question/);
  } finally {
    await h.cleanup();
    restore();
  }
});

test("body click promotes via openRunAtAttention; close button calls closeSpectator", async () => {
  const opened: string[] = [];
  let closed = 0;
  const restore = seedStore({
    history: { "p-2": [hist("r-x", "p-2")] },
    projects: [{ id: "p-2", name: "Beta" }],
    spectatorRunId: "r-x",
    spectatorProjectId: "p-2",
    openRunAtAttention: async (runId, chatId, projectId) => {
      opened.push(`${runId}:${chatId}:${projectId}`);
    },
    closeSpectator: () => {
      closed++;
    },
  });
  const h = await renderPane();
  try {
    const body = h.container.querySelector<HTMLButtonElement>(".spectator-body");
    assert.ok(body);
    await act(async () => {
      body!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    assert.deepEqual(opened, ["r-x:chat-r-x:p-2"]);

    const closeBtn = h.container.querySelector<HTMLButtonElement>('button[aria-label="Stop watching"]');
    assert.ok(closeBtn);
    await act(async () => {
      closeBtn!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    assert.equal(closed, 1);
  } finally {
    await h.cleanup();
    restore();
  }
});

test("missing run data renders placeholder, not a crash", async () => {
  const restore = seedStore({
    history: {},
    projects: [],
    spectatorRunId: "r-ghost",
    spectatorProjectId: "p-9",
  });
  const h = await renderPane();
  try {
    assert.match(h.container.textContent ?? "", /not loaded yet/);
  } finally {
    await h.cleanup();
    restore();
  }
});
