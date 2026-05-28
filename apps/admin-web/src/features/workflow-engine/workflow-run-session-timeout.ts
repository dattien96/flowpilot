import type { WorkflowRunSession } from "@/domain/model/entity/workflow-engine";

type ExpiredWorkflowRunSessionIdsInput = {
  sessionIdleTtlMinutes: number | null | undefined;
  sessions: WorkflowRunSession[] | null | undefined;
  now?: Date;
};

export function getExpiredWorkflowRunSessionIds({
  sessionIdleTtlMinutes,
  sessions,
  now = new Date(),
}: ExpiredWorkflowRunSessionIdsInput): string[] {
  if (!sessionIdleTtlMinutes || sessionIdleTtlMinutes <= 0) {
    return [];
  }

  if (!sessions || sessions.length === 0) {
    return [];
  }

  return sessions
    .filter((session) => {
      if (session.status !== "active") {
        return false;
      }

      const startedAtMs = Date.parse(session.startedAt);
      if (Number.isNaN(startedAtMs)) {
        return false;
      }

      const expiresAtMs = startedAtMs + sessionIdleTtlMinutes * 60_000;
      return now.getTime() >= expiresAtMs;
    })
    .map((session) => session.id);
}

export function markWorkflowRunSessionsCompleted(
  sessions: WorkflowRunSession[],
  sessionIds: string[],
  completedAt: string,
): WorkflowRunSession[] {
  if (sessionIds.length === 0) {
    return sessions;
  }

  const sessionIdsSet = new Set(sessionIds);
  return sessions.map((session) =>
    sessionIdsSet.has(session.id)
      ? {
          ...session,
          status: "completed",
          completedAt,
          processKey: null,
        }
      : session,
  );
}
