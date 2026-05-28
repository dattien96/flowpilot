import { describe, expect, it } from "vitest";

import { getMissingWorkflowRunSessionIds } from "./workflow-run-session-liveness";

const baseSession = {
  workflowRunId: "run-1",
  provider: "codex",
  model: "gpt-5.4",
  transportType: "codex_mcp",
  providerSessionId: "thread-1",
  processKey: "proc-1",
  metadataJson: { is_main: true },
  startedAt: "2026-05-28T09:00:00.000Z",
  completedAt: null,
  processPid: 4321,
};

describe("workflow-run-session-liveness", () => {
  it("returns active session ids missing from the live runner session list", () => {
    const sessionIds = getMissingWorkflowRunSessionIds({
      sessions: [
        {
          ...baseSession,
          id: "session-live",
          processKey: "proc-live",
          status: "active",
        },
        {
          ...baseSession,
          id: "session-stale",
          processKey: "proc-stale",
          status: "active",
        },
        {
          ...baseSession,
          id: "session-completed",
          processKey: null,
          status: "completed",
          completedAt: "2026-05-28T09:12:00.000Z",
        },
      ],
      liveProcessKeys: ["proc-live"],
    });

    expect(sessionIds).toEqual(["session-stale"]);
  });

  it("ignores empty live process keys", () => {
    const sessionIds = getMissingWorkflowRunSessionIds({
      sessions: [
        {
          ...baseSession,
          id: "session-live",
          processKey: "proc-live",
          status: "active",
        },
      ],
      liveProcessKeys: ["", "   ", "proc-live"],
    });

    expect(sessionIds).toEqual([]);
  });
});
