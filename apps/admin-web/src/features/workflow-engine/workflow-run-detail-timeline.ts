import type { ApprovalDecision, WorkflowStep } from "@/domain/model/entity/workflow";
import type { WorkflowRunSession } from "@/domain/model/entity/workflow-engine";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";

export interface WorkflowOutputRecord {
  id: string;
  workflowRunId: string;
  workflowStepId: string;
  projectId: string;
  outputType: string;
  version: number;
  title: string;
  contentMarkdown: string;
  isApproved: boolean;
  createdAt: string;
  promptText?: string;
  actualPromptText?: string;
  stdoutText?: string;
  stderrText?: string;
  commandText?: string;
  localPath?: string;
}

export type WorkflowStepTimelineItem =
  | {
      kind: "output";
      key: string;
      createdAt: string;
      output: WorkflowOutputRecord;
    }
  | {
      kind: "decision";
      key: string;
      createdAt: string;
      decision: ApprovalDecision;
    };

export type WorkflowStepSessionItem =
  | {
      kind: "prompt";
      key: string;
      createdAt: string;
      title: string;
      content: string;
      isSecondary: boolean;
      metaNote?: string;
    }
  | {
      kind: "output";
      key: string;
      createdAt: string;
      output: WorkflowOutputRecord;
    };

export type WorkflowStepPromptSessionItem = Extract<WorkflowStepSessionItem, { kind: "prompt" }>;
export type WorkflowStepOutputSessionItem = Extract<WorkflowStepSessionItem, { kind: "output" }>;

export interface WorkflowStepPromptGroup {
  key: string;
  prompt: WorkflowStepPromptSessionItem;
  attempts: WorkflowStepOutputSessionItem[];
}

export interface WorkflowStepSessionGroup {
  key: string;
  session: WorkflowRunSession | null;
  items: WorkflowStepSessionItem[];
  promptGroups: WorkflowStepPromptGroup[];
}

export interface WorkflowStepSessionStart {
  createdAt: string;
  providerSessionId?: string | null;
  sessionId?: string | null;
}

function normalize(value: string | null | undefined) {
  return (value ?? "").trim().toLowerCase();
}

function compareCreatedAtAsc(left: { createdAt: string }, right: { createdAt: string }) {
  return left.createdAt.localeCompare(right.createdAt);
}

function findStepForArtifact(steps: WorkflowStep[], artifact: LocalRunnerArtifact) {
  const artifactStepKey = normalize(artifact.workflowStepKey);
  return (
    steps.find((step) => normalize(step.stepKey) === artifactStepKey) ??
    steps.find((step) => normalize(step.stepName) === artifactStepKey) ??
    null
  );
}

export function mapArtifactsToWorkflowOutputs(
  artifacts: LocalRunnerArtifact[],
  steps: WorkflowStep[],
): WorkflowOutputRecord[] {
  return artifacts.map((artifact) => {
    const step = findStepForArtifact(steps, artifact);

    return {
      id: artifact.artifactId,
      workflowRunId: artifact.workflowRunId,
      workflowStepId: step ? step.id : artifact.workflowStepKey,
      projectId: artifact.projectId,
      outputType: "document",
      version: 1,
      title: artifact.title,
      contentMarkdown: artifact.contentMarkdown,
      isApproved: true,
      createdAt: artifact.createdAt,
      promptText: artifact.promptText,
      actualPromptText: artifact.actualPromptText,
      stdoutText: artifact.stdoutText,
      stderrText: artifact.stderrText,
      commandText: artifact.commandText,
      localPath: artifact.localPath,
    };
  });
}

export function mergeWorkflowOutputs(
  baseOutputs: WorkflowOutputRecord[],
  localOutputs: WorkflowOutputRecord[],
) {
  const merged = new Map<string, WorkflowOutputRecord>();

  for (const output of baseOutputs) {
    merged.set(output.id, output);
  }

  for (const output of localOutputs) {
    const current = merged.get(output.id);
    merged.set(output.id, current ? { ...current, ...output } : output);
  }

  return Array.from(merged.values()).sort(compareCreatedAtAsc);
}

export function groupOutputsByStep(
  outputs: WorkflowOutputRecord[],
) {
  const grouped = new Map<string, WorkflowOutputRecord[]>();

  for (const output of outputs) {
    const current = grouped.get(output.workflowStepId) ?? [];
    current.push(output);
    current.sort(compareCreatedAtAsc);
    grouped.set(output.workflowStepId, current);
  }

  return grouped;
}

export function buildWorkflowStepTimeline(
  outputs: WorkflowOutputRecord[],
  decisions: ApprovalDecision[],
): WorkflowStepTimelineItem[] {
  const items: WorkflowStepTimelineItem[] = [
    ...outputs.map((output) => ({
      kind: "output" as const,
      key: `output-${output.id}`,
      createdAt: output.createdAt,
      output,
    })),
    ...decisions.map((decision) => ({
      kind: "decision" as const,
      key: `decision-${decision.id}`,
      createdAt: decision.createdAt,
      decision,
    })),
  ];

  return items.sort((left, right) => {
    const createdAtCompare = left.createdAt.localeCompare(right.createdAt);
    if (createdAtCompare !== 0) {
      return createdAtCompare;
    }

    if (left.kind === right.kind) {
      return left.key.localeCompare(right.key);
    }

    return left.kind === "decision" ? -1 : 1;
  });
}

function compareSessionItemAsc(
  left: WorkflowStepSessionItem,
  right: WorkflowStepSessionItem,
) {
  if (
    left.kind === "prompt" &&
    right.kind === "prompt" &&
    (left.key.startsWith("prompt-replay-") || right.key.startsWith("prompt-replay-"))
  ) {
    if (left.key.startsWith("prompt-replay-") && !right.key.startsWith("prompt-replay-")) {
      return -1;
    }

    if (right.key.startsWith("prompt-replay-") && !left.key.startsWith("prompt-replay-")) {
      return 1;
    }
  }

  const createdAtCompare = left.createdAt.localeCompare(right.createdAt);
  if (createdAtCompare !== 0) {
    return createdAtCompare;
  }

  if (left.kind === right.kind) {
    return left.key.localeCompare(right.key);
  }

  if (left.kind === "prompt") {
    return -1;
  }

  if (right.kind === "prompt") {
    return 1;
  }

  return left.key.localeCompare(right.key);
}

function getSessionRecoveryMode(session: WorkflowRunSession | null) {
  return session?.metadataJson?.recovery?.mode ?? null;
}

function getReplayPromptCount(session: WorkflowRunSession | null) {
  const value = session?.metadataJson?.recovery?.replayCheckpointCount;
  return typeof value === "number" && value > 0 ? value : null;
}

function resolveOutputSession(output: WorkflowOutputRecord, sortedSessions: WorkflowRunSession[]) {
  let targetSession = sortedSessions[0] ?? null;

  for (let index = 0; index < sortedSessions.length; index += 1) {
    const session = sortedSessions[index];
    const nextSession = sortedSessions[index + 1];
    const startsBeforeOutput = output.createdAt >= session.startedAt;
    const startsBeforeNext = !nextSession || output.createdAt < nextSession.startedAt;

    if (startsBeforeOutput && startsBeforeNext) {
      return session;
    }

    if (nextSession && output.createdAt >= nextSession.startedAt) {
      targetSession = nextSession;
    }
  }

  return targetSession;
}

function resolveSessionFromStarts(
  createdAt: string,
  sessionStarts: WorkflowStepSessionStart[],
  sortedSessions: WorkflowRunSession[],
) {
  if (sessionStarts.length > 0) {
    const matchedStart = [...sessionStarts]
      .reverse()
      .find((sessionStart) => sessionStart.createdAt <= createdAt);

    if (matchedStart?.sessionId) {
      const byId = sortedSessions.find((session) => session.id === matchedStart.sessionId);
      if (byId) {
        return byId;
      }
    }

    if (matchedStart?.providerSessionId) {
      const byProviderSessionId = sortedSessions.find(
        (session) => session.providerSessionId === matchedStart.providerSessionId,
      );
      if (byProviderSessionId) {
        return byProviderSessionId;
      }
    }
  }

  return null;
}

function buildReplayPromptContent(session: WorkflowRunSession) {
  const replayPromptCount = getReplayPromptCount(session);
  if (replayPromptCount && replayPromptCount > 0) {
    return `Replayed ${replayPromptCount} previous prompt${replayPromptCount === 1 ? "" : "s"} into this new session before continuing.`;
  }

  return "Replayed previous session context into this new session before continuing.";
}

function findLatestPromptBeforeSession(
  groups: Map<string, WorkflowStepSessionGroup>,
  sortedSessions: WorkflowRunSession[],
  sessionIndex: number,
  sessionStartedAt: string,
) {
  for (let index = sessionIndex - 1; index >= 0; index -= 1) {
    const previousSession = sortedSessions[index];
    if (!previousSession) {
      continue;
    }

    const previousGroup = groups.get(previousSession.id);
    if (!previousGroup) {
      continue;
    }

    const promptCandidates = previousGroup.items.filter(
      (item): item is WorkflowStepPromptSessionItem =>
        item.kind === "prompt" &&
        item.key !== "prompt-initial" &&
        item.createdAt <= sessionStartedAt,
    );

    if (promptCandidates.length === 0) {
      continue;
    }

    return promptCandidates.sort((left, right) =>
      right.createdAt.localeCompare(left.createdAt),
    )[0] ?? null;
  }

  return null;
}

export function buildWorkflowStepSessionGroups({
  outputs,
  decisions,
  sessions,
  sessionStarts = [],
  initialPrompt,
}: {
  outputs: WorkflowOutputRecord[];
  decisions: ApprovalDecision[];
  sessions: WorkflowRunSession[];
  sessionStarts?: WorkflowStepSessionStart[];
  initialPrompt?:
    | {
        createdAt: string;
        content: string;
      }
    | null;
}): WorkflowStepSessionGroup[] {
  const sortedSessions = [...sessions].sort((left, right) =>
    left.startedAt.localeCompare(right.startedAt),
  );

  const outputItems: WorkflowStepOutputSessionItem[] = outputs.map((output) => ({
    kind: "output",
    key: `output-${output.id}`,
    createdAt: output.createdAt,
    output,
  }));

  const promptItems: WorkflowStepPromptSessionItem[] = [];
  if (initialPrompt?.content.trim()) {
    promptItems.push({
      kind: "prompt",
      key: "prompt-initial",
      createdAt: initialPrompt.createdAt,
      title: "Initial Prompt",
      content: initialPrompt.content,
      isSecondary: false,
    });
  }

  for (const decision of decisions) {
    promptItems.push({
      kind: "prompt",
      key: `prompt-decision-${decision.id}`,
      createdAt: decision.createdAt,
      title: "Follow-up",
      content: decision.comment || `Decision: ${decision.decision}`,
      isSecondary: true,
    });
  }

  const sortedPrompts = [...promptItems].sort((left, right) =>
    left.createdAt.localeCompare(right.createdAt)
  );

  const outputSessionById = new Map<string, WorkflowRunSession | null>();
  for (const outputItem of outputItems) {
    const sessionFromStarts = resolveSessionFromStarts(
      outputItem.output.createdAt,
      sessionStarts,
      sortedSessions,
    );
    outputSessionById.set(
      outputItem.output.id,
      sessionFromStarts ?? resolveOutputSession(outputItem.output, sortedSessions),
    );
  }

  // Group attempts under prompts based on chronological order
  const promptGroupsMap = new Map<string, WorkflowStepPromptGroup>();
  for (const prompt of sortedPrompts) {
    promptGroupsMap.set(prompt.key, {
      key: `prompt-group-${prompt.key}`,
      prompt,
      attempts: [],
    });
  }

  const orphans: WorkflowStepOutputSessionItem[] = [];

  for (const outputItem of outputItems) {
    const matchingPrompt = [...sortedPrompts]
      .reverse()
      .find((p) => p.createdAt <= outputItem.output.createdAt);

    if (matchingPrompt) {
      const pg = promptGroupsMap.get(matchingPrompt.key);
      if (pg) {
        pg.attempts.push(outputItem);
      }
    } else {
      orphans.push(outputItem);
    }
  }

  // Create a placeholder if attempts exist before the first prompt
  if (orphans.length > 0) {
    const placeholderKey = "prompt-placeholder-orphans";
    const placeholderPrompt: WorkflowStepPromptSessionItem = {
      kind: "prompt",
      key: placeholderKey,
      createdAt: orphans[0].createdAt,
      title: "System Run",
      content: "Execution started.",
      isSecondary: false,
    };
    promptGroupsMap.set(placeholderKey, {
      key: `prompt-group-${placeholderKey}`,
      prompt: placeholderPrompt,
      attempts: orphans,
    });
    sortedPrompts.unshift(placeholderPrompt);
  }

  // Map PromptGroups to Sessions
  const sessionPromptGroupsMap = new Map<string, WorkflowStepPromptGroup[]>();
  const noSessionPromptGroups: WorkflowStepPromptGroup[] = [];

  for (const session of sortedSessions) {
    sessionPromptGroupsMap.set(session.id, []);
  }

  for (const prompt of sortedPrompts) {
    const pg = promptGroupsMap.get(prompt.key);
    if (!pg) continue;

    let targetSession: WorkflowRunSession | null = null;
    if (pg.attempts.length > 0) {
      targetSession = outputSessionById.get(pg.attempts[0].output.id) ?? null;
    } else {
      if (prompt.key === "prompt-initial") {
        targetSession = sortedSessions[0] ?? null;
      } else {
        targetSession = resolveSessionFromStarts(
          prompt.createdAt,
          sessionStarts,
          sortedSessions,
        );
      }

      if (!targetSession) {
        targetSession =
          [...sortedSessions]
            .reverse()
            .find((session) => prompt.createdAt >= session.startedAt) ??
          sortedSessions[sortedSessions.length - 1] ??
          null;
      }
    }

    if (targetSession) {
      sessionPromptGroupsMap.get(targetSession.id)?.push(pg);
    } else {
      noSessionPromptGroups.push(pg);
    }
  }

  // Handle bootstrap replay metaNotes
  for (let sessionIndex = 0; sessionIndex < sortedSessions.length; sessionIndex += 1) {
    const session = sortedSessions[sessionIndex];
    if (getSessionRecoveryMode(session) !== "bootstrap_replay") {
      continue;
    }

    const sessionGroups = sessionPromptGroupsMap.get(session.id) ?? [];
    let firstPromptGroup = sessionGroups[0];

    if (!firstPromptGroup) {
      let migratedGroup: WorkflowStepPromptGroup | null = null;
      for (let prevIndex = sessionIndex - 1; prevIndex >= 0; prevIndex -= 1) {
        const prevSession = sortedSessions[prevIndex];
        const prevGroups = sessionPromptGroupsMap.get(prevSession.id) ?? [];
        if (prevGroups.length > 0) {
          migratedGroup = prevGroups[prevGroups.length - 1];
          sessionPromptGroupsMap.set(prevSession.id, prevGroups.slice(0, -1));
          break;
        }
      }

      if (migratedGroup) {
        sessionPromptGroupsMap.get(session.id)?.unshift(migratedGroup);
        firstPromptGroup = migratedGroup;
      }
    }

    if (firstPromptGroup) {
      firstPromptGroup.prompt.metaNote = buildReplayPromptContent(session);
    }
  }

  // Map to final WorkflowStepSessionGroup[] structure
  const finalGroups: WorkflowStepSessionGroup[] = [];

  if (sortedSessions.length === 0 || (sortedSessions.length > 0 && noSessionPromptGroups.length > 0)) {
    const allPromptGroups = [...noSessionPromptGroups];
    if (sortedSessions.length === 0) {
      for (const [_, pg] of promptGroupsMap) {
        if (!allPromptGroups.some((item) => item.key === pg.key)) {
          allPromptGroups.push(pg);
        }
      }
    }

    allPromptGroups.sort((a, b) => a.prompt.createdAt.localeCompare(b.prompt.createdAt));

    const flatItems: WorkflowStepSessionItem[] = [];
    for (const pg of allPromptGroups) {
      flatItems.push(pg.prompt);
      flatItems.push(...pg.attempts);
    }

    finalGroups.push({
      key: "session-unknown",
      session: null,
      items: flatItems,
      promptGroups: allPromptGroups,
    });
  }

  for (const session of sortedSessions) {
    const pgs = sessionPromptGroupsMap.get(session.id) ?? [];
    if (pgs.length === 0) {
      continue;
    }

    pgs.sort((a, b) => a.prompt.createdAt.localeCompare(b.prompt.createdAt));

    const flatItems: WorkflowStepSessionItem[] = [];
    for (const pg of pgs) {
      flatItems.push(pg.prompt);
      flatItems.push(...pg.attempts);
    }

    finalGroups.push({
      key: session.id,
      session,
      items: flatItems,
      promptGroups: pgs,
    });
  }

  return finalGroups;
}
