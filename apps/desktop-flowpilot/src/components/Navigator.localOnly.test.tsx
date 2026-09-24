import test from "node:test";
import assert from "node:assert/strict";
import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { setupDom } from "../testhelpers/domHarness";
import { Navigator } from "./Navigator";
import { useStore } from "../state/store";
import type { RemoteChatSessionSummary, RunHistoryItem, RunnerClient } from "@/types/contract";

// CP-85 P-1/P-2: the Navigator is a local-only surface. Mounting it must NOT
// fetch remote (Drive) chat sessions and must NOT render a "Remote Chats"
// section — that flow lives behind the "Open Sync" button, which mounts
// RemoteSyncPanel; mounting the panel is what fires listRemoteChatSessions.
// Runs under node --test via the jsdom harness. Additive-only.

function local(runId: string, over: Partial<RunHistoryItem> = {}): RunHistoryItem {
  return {
    runId,
    chatId: "chat-" + runId,
    projectId: "p-1",
    providerKey: "codex",
    status: "completed",
    runKind: "chat",
    lastPrompt: `local chat ${runId}`,
    startedAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-02T00:00:00Z",
    ...over,
  } as RunHistoryItem;
}

function seed(clientOver: Partial<RunnerClient>, items: RunHistoryItem[]) {
  const s = useStore.getState();
  const saved = {
    client: s.client,
    loadProjects: s.loadProjects,
    projects: s.projects,
    selectedProjectId: s.selectedProjectId,
    projectHistoryById: s.projectHistoryById,
    runHistory: s.runHistory,
    remoteChatSessions: s.remoteChatSessions,
    status: s.status,
    attentionItems: s.attentionItems,
  };
  useStore.setState({
    // Pure stub — never merge the real client (its streamRunUpdates would let
    // callers arm the SSE reconnect loop and hang this test's process).
    client: clientOver as RunnerClient,
    // loadProjects also reaches the admin catalog via a real fetch to the
    // runner port — on a dev machine with the runner up, undici keep-alive
    // pins the socket open for minutes and the test file never exits. The
    // store seeds everything the Navigator needs, so the action is a no-op.
    loadProjects: async () => {},
    projects: [{ id: "p-1", name: "Alpha", path: "D:\\p1" }] as never,
    selectedProjectId: "p-1",
    projectHistoryById: { "p-1": items },
    runHistory: items,
    remoteChatSessions: [],
    status: "idle",
    attentionItems: [],
  });
  return () => useStore.setState(saved);
}

async function flush() {
  // Let the loadProjects chain (listProjects -> admin/skills/accounts) and the
  // mount-time loadRunHistory settle; the admin use-case does one refused
  // fetch to the runner port, which needs a real macrotask.
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 20));
    });
  }
}

test("Navigator renders local history only — no Remote Chats section, no remote fetch", async () => {
  const remoteCalls: string[] = [];
  const restore = seed(
    {
      listRunHistory: async () => [local("l-1")],
      listRemoteChatSessions: async (projectId: string) => {
        remoteCalls.push(projectId);
        return [] as RemoteChatSessionSummary[];
      },
    },
    [local("l-1")],
  );
  const restoreDom = setupDom();
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root: Root = createRoot(container);
  try {
    await act(async () => {
      root.render(<Navigator />);
    });
    await flush();

    assert.doesNotMatch(container.textContent ?? "", /Remote Chats/, "remote section must not render");
    assert.deepEqual(remoteCalls, [], "Navigator must never fetch remote chat sessions");
    assert.match(container.textContent ?? "", /local chat l-1/, "local rows still render");
    const openSync = [...container.querySelectorAll<HTMLButtonElement>("button")].find((b) =>
      /Open Sync/.test(b.textContent ?? ""),
    );
    assert.ok(openSync, "Open Sync button missing");

    // Clicking it mounts the sync screen — THAT is the remote-fetch trigger.
    await act(async () => {
      openSync!.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await Promise.resolve();
    });
    await act(async () => {
      await Promise.resolve();
    });
    assert.deepEqual(remoteCalls, ["p-1"], "remote fetch must fire only from the sync screen");
    assert.match(container.textContent ?? "", /Remote chats \(Drive\)/);
  } finally {
    await act(async () => {
      root.unmount();
    });
    container.remove();
    restoreDom();
    restore();
  }
});
