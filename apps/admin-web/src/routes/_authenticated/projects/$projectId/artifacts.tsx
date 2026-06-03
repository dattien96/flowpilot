import { createFileRoute, Link, useRouter } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { ProjectSectionNav } from "@/components/project/project-section-nav";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";
import type { Project } from "@/domain/model/entity/project";
import type { ArtifactRun, WorkflowRun } from "@/domain/model/entity/workflow-engine";
import { ArtifactRunBrowserPanel } from "@/presentation/components/artifacts/artifact-run-browser-panel";
import { Button } from "@/presentation/components/ui/button";

type ProjectArtifactsLoaderGateways = {
  projectGateway: {
    getProjectById: (projectId: string) => Promise<Project | null>;
  };
  workflowEngineGateway: {
    listArtifactRuns: (projectId?: string) => Promise<ArtifactRun[]>;
    listWorkflowRuns: (projectId?: string) => Promise<WorkflowRun[]>;
  };
  localRunnerGateway: {
    listArtifacts: () => Promise<LocalRunnerArtifact[]>;
  };
};

export async function loadProjectArtifactsData(
  gateways: ProjectArtifactsLoaderGateways,
  projectId: string,
) {
  const [project, localArtifacts, artifactRuns, workflowRuns] = await Promise.all([
    gateways.projectGateway.getProjectById(projectId),
    gateways.localRunnerGateway.listArtifacts(),
    gateways.workflowEngineGateway.listArtifactRuns(projectId),
    gateways.workflowEngineGateway.listWorkflowRuns(projectId),
  ]);
  const validWorkflowRunIds = new Set(workflowRuns.map((run) => run.id));

  return {
    project,
    localArtifacts: localArtifacts.filter(
      (artifact) =>
        artifact.projectId === projectId &&
        artifact.workflowRunId.trim() !== "" &&
        validWorkflowRunIds.has(artifact.workflowRunId),
    ),
    artifactRuns,
    projectId,
  };
}

export const Route = createFileRoute("/_authenticated/projects/$projectId/artifacts")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    return loadProjectArtifactsData(gateways, params.projectId);
  },
  component: ProjectArtifactsPage,
});

function ProjectArtifactsPage() {
  const router = useRouter();
  const { artifactRuns, localArtifacts, project, projectId } = Route.useLoaderData();

  if (!project) {
    return null;
  }

  return (
    <ProjectArtifactsContent
      key={project.id}
      artifactRuns={artifactRuns}
      localArtifacts={localArtifacts}
      onArtifactsChanged={async () => {
        await router.invalidate();
      }}
      project={project}
      projectId={projectId}
    />
  );
}

function ProjectArtifactsContent({
  artifactRuns,
  localArtifacts,
  onArtifactsChanged,
  project,
  projectId,
}: {
  artifactRuns: ArtifactRun[];
  localArtifacts: LocalRunnerArtifact[];
  onArtifactsChanged: () => Promise<void>;
  project: Project;
  projectId: string;
}) {
  return (
    <PageFrame description={`Artifacts generated in ${project.name}.`} title="Project Artifacts">
      <div className="space-y-6">
        <ProjectSectionNav projectId={projectId} />

        <div className="flex flex-wrap gap-2">
          <Link to="/artifacts">
            <Button variant="secondary">Global artifact list</Button>
          </Link>
          <Link to="/settings/artifacts">
            <Button variant="secondary">Artifact storage settings</Button>
          </Link>
        </div>

        <ArtifactRunBrowserPanel
          artifactRuns={artifactRuns}
          localArtifacts={localArtifacts}
          loadRemoteArtifactContent={async (artifactRun) => {
            if (!artifactRun.id.trim() || !artifactRun.remotePath.trim()) {
              return null;
            }

            const params = new URLSearchParams({
              remotePath: artifactRun.remotePath,
              storageProvider: artifactRun.storageProvider ?? "supabase",
            });
            if (artifactRun.remoteObjectId?.trim()) {
              params.set("remoteObjectId", artifactRun.remoteObjectId.trim());
            }
            if (artifactRun.projectId?.trim()) {
              params.set("projectId", artifactRun.projectId.trim());
            }

            const response = await fetch(
              `/api/local-runner/artifacts/${artifactRun.id}/open?${params.toString()}`,
            );
            if (!response.ok) {
              return null;
            }

            return await response.text();
          }}
          onArtifactsChanged={onArtifactsChanged}
          projects={[project]}
          scopeLabel={project.name}
        />
      </div>
    </PageFrame>
  );
}
