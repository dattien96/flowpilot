import test from "node:test";
import assert from "node:assert/strict";
import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { setupDom } from "../testhelpers/domHarness";
import { RemoteSyncPanel } from "./RemoteSyncPanel";
import { useStore } from "../state/store";
import type { RemoteChatSessionSummary, RunHistoryItem, RunnerClient } from "@/types/contract";

// CP-85 P-3/P-5: the on-demand sync screen — owns the remote fetch (mounting
// the panel is the only trigger), per-chat restore, restore-all, and the
// unsynced-local upload section. Runs under node --test via the jsdom harness.
// Additive-only — no existing test touched.

function remote(sourceRunId: string, over: Partial<RemoteChatSessionSummary> = {}): RemoteChatSessionSummary {
  return {
    sourceMachineId: "mch-1",
    sourceRunId,
    providerKey: "codex",
    lastPrompt: `remote chat ${sourceRunId}`,
    updatedAt: "2026-01-02T00:00:00Z",
    ...over,
  } as RemoteChatSessionSummary;
}

function local(runId: string, over: Partial<RunHistoryItem> = {}): RunHistoryItem {
  return {
    runId,
    chatId: "chat-" + runId,
    projectId: "p-1",
    providerKey: "codex",
    status: "completed",
    runKind: "chat",
    startedAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-02T00:00:00Z",
    ...over,
  } as RunHistoryItem;
}

interface Harness {
  container: HTMLElement;
  cleanup: () => Promise<void>;
}

async function renderPanel(onClose: () => void): Promise<Harness> {
  const restoreDom = setupDom();
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root: Root = createRoot(container);
  await act(async () => {
    root.render(<RemoteSyncPanel onClose={onClose} />);
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

function seed(over: {
  client?: Partial<RunnerClient>;
  remote?: RemoteChatSessionSummary[];
  local?: RunHistoryItem[];
}) {
  const s = useStore.getState();
  const saved = {
    client: s.client,
    projects: s.projects,
    selectedProjectId: s.selectedProjectId,
    projectHistoryById: s.projectHistoryById,
    remoteChatSessions: s.remoteChatSessions,
    remoteHistoryLoading: s.remoteHistoryLoading,
    remoteHistoryLoadError: s.remoteHistoryLoadError,
    syncBatchProgress: s.syncBatchProgress,
  };
  useStore.setState({
    // Pure stub — never merge the real client (its streamRunUpdates would let
    // callers arm the SSE reconnect loop and hang this test's process).
    ...(over.client ? { client: over.client as RunnerClient } : {}),
    projects: [{ id: "p-1", name: "Alpha", path: "D:\\p1" }] as never,
    selectedProjectId: "p-1",
    projectHistoryById: { "p-1": over.local ?? [] },
    remoteChatSessions: over.remote ?? [],
    remoteHistoryLoading: false,
    remoteHistoryLoadError: undefined,
    syncBatchProgress: undefined,
  });
  return () => useStore.setState(saved);
}

test("mounting the panel fetches remote sessions and lists them", async () => {
  const calls: string[] = [];
  const restore = seed({
    client: {
      listRemoteChatSessions: async (projectId: string) => {
        calls.push(projectId);
        return [remote("r-1"), remote("r-2")];
      },
    },
  });
  const h = await renderPanel(() => {});
  try {
    await act(async () => {
      await Promise.resolve();
    });
    assert.deepEqual(calls, ["p-1"], "open must fetch remote chats for the selected project");
    const rows = h.container.querySelectorAll(".project-history-item");
    assert.equal(rows.length, 2, "remote rows render");
    assert.match(h.container.textContent ?? "", /remote chat r-1/);
    assert.match(h.container.textContent ?? "", /Remote chats/);
  } finally {
    await h.cleanup();
    restore();
  }
});

test("clicking a remote row restores it and closes the panel", async () => {
  const restored: string[] = [];
  const restore = seed({
    remote: [remote("r-9")],
    client: {
      listRemoteChatSessions: async () => [remote("r-9")],
      restoreChatRun: async (input) => {
        restored.push(input.sourceRunId);
        return {
          runId: "new-local",
          sourceMachineId: input.sourceMachineId,
          sourceRunId: input.sourceRunId,
          providerKey: "codex",
          restoreStatus: "restored",
        };
      },
      // post-restore refresh path
      listRunHistory: async () => [],
    },
  });
  // openHistoryRun runs on restore success — stub it so the test does not pull
  // in the whole resume/stream machinery.
  const s = useStore.getState();
  const savedOpen = s.openHistoryRun;
  useStore.setState({ openHistoryRun: async () => {} });
  let closed = 0;
  const h = await renderPanel(() => {
    closed++;
  });
  try {
    await act(async () => {
      await Promise.resolve();
    });
    const row = h.container.querySelector<HTMLButtonElement>(".project-history-item");
    assert.ok(row, "remote row missing");
    await act(async () => {
      row!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await Promise.resolve();
    });
    assert.deepEqual(restored, ["r-9"], "restore must hit the runner restore endpoint");
    assert.equal(closed, 1, "successful restore opens the chat and closes the panel");
  } finally {
    await h.cleanup();
    useStore.setState({ openHistoryRun: savedOpen });
    restore();
  }
});

test("unsynced local chats list in the upload section and Sync all runs the batch", async () => {
  const synced: string[] = [];
  const restore = seed({
    remote: [],
    local: [
      local("l-1", { lastPrompt: "unsynced one" }),
      local("l-2", { lastPrompt: "already synced", syncStatus: "synced" }),
    ],
    client: {
      listRemoteChatSessions: async () => [],
      listRunHistory: async () => [],
      syncChatRun: async (runId: string) => {
        synced.push(runId);
        return {
          runId,
          sourceMachineId: "mch-1",
          sourceRunId: runId,
          syncStatus: "synced",
          syncedAt: "2026-01-02T00:00:00Z",
          remotePath: "x",
        };
      },
    },
  });
  // runHistory is what syncAllInProject filters — seed it too.
  const s = useStore.getState();
  const savedRunHistory = s.runHistory;
  useStore.setState({
    runHistory: [
      local("l-1", { lastPrompt: "unsynced one" }),
      local("l-2", { lastPrompt: "already synced", syncStatus: "synced" }),
    ],
  });
  const h = await renderPanel(() => {});
  try {
    await act(async () => {
      await Promise.resolve();
    });
    assert.match(h.container.textContent ?? "", /Local chats not synced/);
    assert.match(h.container.textContent ?? "", /unsynced one/);
    assert.doesNotMatch(h.container.textContent ?? "", /already synced/);
    const syncAll = [...h.container.querySelectorAll<HTMLButtonElement>("button")].find((b) =>
      /Sync all/.test(b.textContent ?? ""),
    );
    assert.ok(syncAll, "Sync all button missing");
    await act(async () => {
      syncAll!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await Promise.resolve();
      await Promise.resolve();
    });
    assert.deepEqual(synced, ["l-1"], "only the unsynced local chat syncs up");
  } finally {
    await h.cleanup();
    useStore.setState({ runHistory: savedRunHistory });
    restore();
  }
});

test("Escape key closes the sync panel", async () => {
  const restore = seed({ client: { listRemoteChatSessions: async () => [] } });
  let closed = 0;
  const h = await renderPanel(() => {
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
