import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";
import type { RunHistoryItem } from "@/types/contract";

// CP-84 / Task-431 (T-5): notification click → openRunAtAttention deep link.
// App.tsx resolves the attention item's projectId/chatId then calls this
// store path; these tests pin the cross-project and fallback behavior.
// Additive-only — attention_queue.test.ts untouched.

function hist(runId: string, projectId = "p-1"): RunHistoryItem {
  return {
    runId,
    chatId: "chat-" + runId,
    projectId,
    providerKey: "claude",
    status: "waiting_approval",
    startedAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  } as RunHistoryItem;
}

test("openRunAtAttention switches project before opening the run", async () => {
  const order: string[] = [];
  const origSelect = useStore.getState().selectProject;
  const origOpen = useStore.getState().openHistoryRun;
  const origProject = useStore.getState().selectedProjectId;
  const origHistoryById = useStore.getState().projectHistoryById;
  useStore.setState({
    selectedProjectId: "p-current",
    projectHistoryById: { "p-2": [hist("r-attn", "p-2")] },
    selectProject: (async (pid: string) => {
      order.push("select:" + pid);
    }) as never,
    openHistoryRun: (async (runId: string, item?: RunHistoryItem) => {
      order.push("open:" + runId + ":" + (item?.projectId ?? "?"));
    }) as never,
  });
  try {
    await useStore.getState().openRunAtAttention("r-attn", "chat-r-attn", "p-2");
    assert.deepEqual(order, ["select:p-2", "open:r-attn:p-2"]);
  } finally {
    useStore.setState({
      selectedProjectId: origProject,
      projectHistoryById: origHistoryById,
      selectProject: origSelect,
      openHistoryRun: origOpen,
    });
  }
});

test("openRunAtAttention same-project opens without switching", async () => {
  let selected = "";
  let opened = "";
  const origSelect = useStore.getState().selectProject;
  const origOpen = useStore.getState().openHistoryRun;
  const origProject = useStore.getState().selectedProjectId;
  const origHistoryById = useStore.getState().projectHistoryById;
  useStore.setState({
    selectedProjectId: "p-1",
    projectHistoryById: { "p-1": [hist("r-attn")] },
    selectProject: (async (pid: string) => {
      selected = pid;
    }) as never,
    openHistoryRun: (async (runId: string) => {
      opened = runId;
    }) as never,
  });
  try {
    await useStore.getState().openRunAtAttention("r-attn", "chat-r-attn", "p-1");
    assert.equal(selected, "");
    assert.equal(opened, "r-attn");
  } finally {
    useStore.setState({
      selectedProjectId: origProject,
      projectHistoryById: origHistoryById,
      selectProject: origSelect,
      openHistoryRun: origOpen,
    });
  }
});

test("openRunAtAttention falls back to runHistory when item missing from projectHistoryById", async () => {
  let openedItem: RunHistoryItem | undefined;
  const origOpen = useStore.getState().openHistoryRun;
  const origProject = useStore.getState().selectedProjectId;
  const origHistoryById = useStore.getState().projectHistoryById;
  const origRunHistory = useStore.getState().runHistory;
  useStore.setState({
    selectedProjectId: "p-1",
    projectHistoryById: {},
    runHistory: [hist("r-local")],
    openHistoryRun: (async (_runId: string, item?: RunHistoryItem) => {
      openedItem = item;
    }) as never,
  });
  try {
    // Item evicted from the attention queue between notify and click —
    // openRunAtAttention must still resolve via the polled history slice.
    await useStore.getState().openRunAtAttention("r-local", "chat-r-local");
    assert.equal(openedItem?.runId, "r-local");
  } finally {
    useStore.setState({
      selectedProjectId: origProject,
      projectHistoryById: origHistoryById,
      runHistory: origRunHistory,
      openHistoryRun: origOpen,
    });
  }
});
