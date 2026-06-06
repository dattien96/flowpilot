export type ArtifactBrowserGroupItem = {
  projectId: string | null;
  projectName: string | null;
  storageType: "local_only" | "supabase" | "google_drive";
  workflowRunId: string;
  workflowRunStepId: string | null;
  createdAt: string;
};

export interface GroupedWorkflowStep<TArtifact extends ArtifactBrowserGroupItem> {
  workflowStepId: string;
  artifacts: TArtifact[];
}

export interface GroupedWorkflowRun<TArtifact extends ArtifactBrowserGroupItem> {
  workflowRunId: string;
  storageType: TArtifact["storageType"];
  workflowSteps: GroupedWorkflowStep<TArtifact>[];
}

export interface GroupedStorageType<TArtifact extends ArtifactBrowserGroupItem> {
  storageType: TArtifact["storageType"];
  workflowRuns: GroupedWorkflowRun<TArtifact>[];
}

export interface GroupedProject<TArtifact extends ArtifactBrowserGroupItem> {
  projectId: string;
  projectName: string;
  storageTypes: GroupedStorageType<TArtifact>[];
}

export function storageTypeSortKey(storageType: ArtifactBrowserGroupItem["storageType"]) {
  switch (storageType) {
    case "local_only":
      return 0;
    case "supabase":
      return 1;
    case "google_drive":
      return 2;
  }
}

export function groupArtifactsForDisplay<TArtifact extends ArtifactBrowserGroupItem>(
  artifacts: TArtifact[],
): GroupedProject<TArtifact>[] {
  const projectMap = new Map<string, Map<TArtifact["storageType"], Map<string, TArtifact[]>>>();
  const projectNameMap = new Map<string, string>();

  for (const artifact of artifacts) {
    const projectId = artifact.projectId || "unknown";
    const projectName = artifact.projectName || "Unknown Project";
    projectNameMap.set(projectId, projectName);

    if (!projectMap.has(projectId)) {
      projectMap.set(projectId, new Map());
    }

    const storageTypeMap = projectMap.get(projectId)!;
    if (!storageTypeMap.has(artifact.storageType)) {
      storageTypeMap.set(artifact.storageType, new Map());
    }

    const workflowRunMap = storageTypeMap.get(artifact.storageType)!;
    const workflowRunId = artifact.workflowRunId || "unknown";
    if (!workflowRunMap.has(workflowRunId)) {
      workflowRunMap.set(workflowRunId, []);
    }
    workflowRunMap.get(workflowRunId)!.push(artifact);
  }

  const result: GroupedProject<TArtifact>[] = [];
  projectMap.forEach((storageTypeMap, projectId) => {
    const storageTypes: GroupedStorageType<TArtifact>[] = [];

    storageTypeMap.forEach((workflowRunMap, storageType) => {
      const workflowRuns: GroupedWorkflowRun<TArtifact>[] = [];

      workflowRunMap.forEach((runArtifacts, workflowRunId) => {
        const sortedArtifacts = [...runArtifacts].sort(
          (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
        );
        const workflowStepMap = new Map<string, TArtifact[]>();
        for (const artifact of sortedArtifacts) {
          const workflowStepId = artifact.workflowRunStepId ?? "unknown";
          if (!workflowStepMap.has(workflowStepId)) {
            workflowStepMap.set(workflowStepId, []);
          }
          workflowStepMap.get(workflowStepId)!.push(artifact);
        }

        const workflowSteps = Array.from(workflowStepMap.entries())
          .map(([workflowStepId, stepArtifacts]) => ({
            workflowStepId,
            artifacts: stepArtifacts,
          }))
          .sort((a, b) => {
            const aLatest = a.artifacts[0]?.createdAt ? new Date(a.artifacts[0].createdAt).getTime() : 0;
            const bLatest = b.artifacts[0]?.createdAt ? new Date(b.artifacts[0].createdAt).getTime() : 0;
            return bLatest - aLatest;
          });

        workflowRuns.push({
          workflowRunId,
          storageType,
          workflowSteps,
        });
      });

      workflowRuns.sort((a, b) => {
        const aLatest = a.workflowSteps[0]?.artifacts[0]?.createdAt
          ? new Date(a.workflowSteps[0].artifacts[0].createdAt).getTime()
          : 0;
        const bLatest = b.workflowSteps[0]?.artifacts[0]?.createdAt
          ? new Date(b.workflowSteps[0].artifacts[0].createdAt).getTime()
          : 0;
        return bLatest - aLatest;
      });

      storageTypes.push({
        storageType,
        workflowRuns,
      });
    });

    storageTypes.sort((a, b) => storageTypeSortKey(a.storageType) - storageTypeSortKey(b.storageType));

    result.push({
      projectId,
      projectName: projectNameMap.get(projectId) || "Unknown Project",
      storageTypes,
    });
  });

  return result.sort((a, b) => a.projectName.localeCompare(b.projectName));
}
