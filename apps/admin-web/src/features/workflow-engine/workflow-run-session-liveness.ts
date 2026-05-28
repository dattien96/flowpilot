import type { WorkflowRunSession } from "@/domain/model/entity/workflow-engine";

type MissingWorkflowRunSessionIdsInput = {
  sessions: WorkflowRunSession[] | null | undefined;
  liveProcessKeys: string[] | null | undefined;
};

export function getMissingWorkflowRunSessionIds({
  sessions,
  liveProcessKeys,
}: MissingWorkflowRunSessionIdsInput): string[] {
  if (!sessions || sessions.length === 0) {
    return [];
  }

  if (!liveProcessKeys) {
    return [];
  }

  const liveProcessKeySet = new Set(
    liveProcessKeys
      .map((processKey) => processKey.trim())
      .filter((processKey) => processKey.length > 0),
  );

  return sessions
    .filter((session) => {
      if (session.status !== "active") {
        return false;
      }

      const processKey = session.processKey?.trim() ?? "";
      if (processKey.length === 0) {
        return false;
      }

      return !liveProcessKeySet.has(processKey);
    })
    .map((session) => session.id);
}
