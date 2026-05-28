import { describe, expect, it } from "vitest";

import {
  getExpiredWorkflowRunSessionIds,
  markWorkflowRunSessionsCompleted,
} from "./workflow-run-session-timeout";

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
};

describe("workflow-run-session-timeout", () => {
  it("returns active session ids after the session idle TTL has elapsed", () => {
    const sessionIds = getExpiredWorkflowRunSessionIds({
      sessionIdleTtlMinutes: 10,
      sessions: [
        {
          ...baseSession,
          id: "session-main",
          status: "active",
          startedAt: "2026-05-28T09:10:00.000Z",
        },
        {
          ...baseSession,
          id: "session-completed",
          status: "completed",
          processKey: null,
          startedAt: "2026-05-28T09:00:00.000Z",
          completedAt: "2026-05-28T09:12:00.000Z",
        },
      ],
      now: new Date("2026-05-28T09:21:00.000Z"),
    });

    expect(sessionIds).toEqual(["session-main"]);
  });

  it("does not return sessions before the idle TTL expires", () => {
    const sessionIds = getExpiredWorkflowRunSessionIds({
      sessionIdleTtlMinutes: 10,
      sessions: [
        {
          ...baseSession,
          id: "session-main",
          status: "active",
          startedAt: "2026-05-28T09:10:00.000Z",
        },
      ],
      now: new Date("2026-05-28T09:19:59.000Z"),
    });

    expect(sessionIds).toEqual([]);
  });

  it("marks timed-out sessions as completed locally", () => {
    const completedAt = "2026-05-28T09:21:00.000Z";
    const sessions = markWorkflowRunSessionsCompleted(
      [
        {
          ...baseSession,
          id: "session-main",
          status: "active",
        },
      ],
      ["session-main"],
      completedAt,
    );

    expect(sessions[0]).toMatchObject({
      id: "session-main",
      status: "completed",
      completedAt,
      processKey: null,
    });
  });
});
