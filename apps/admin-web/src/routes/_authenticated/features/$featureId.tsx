import { useEffect, useMemo, useRef, useState } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useMutation } from "@tanstack/react-query";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { ContextSource } from "@/domain/model/entity/context-source";
import type { Feature } from "@/domain/model/entity/feature";
import type { Workflow, WorkflowRun } from "@/domain/model/entity/workflow-engine";
import { StartWorkflowRunUseCase } from "@/domain/usecase/workflow-engine/start-workflow-run-usecase";
import { Badge } from "@/presentation/components/ui/badge";

type FeatureRouteDetail = {
  contexts: ContextSource[];
  feature: Feature;
  runs: WorkflowRun[];
  workflows: Workflow[];
};

export const Route = createFileRoute("/_authenticated/features/$featureId")({
  loader: async ({ params }) => {
    const gateways = await createGatewayBundle();
    const feature = await gateways.featureGateway.getFeatureById(params.featureId);

    if (!feature) {
      return { detail: null };
    }

    const [featureContexts, projectContexts, workflows, runs] = await Promise.all([
      gateways.contextSourceGateway.listContextSourcesByFeature(feature.id),
      gateways.contextSourceGateway.listContextSourcesByProject(feature.projectId),
      gateways.workflowEngineGateway.listWorkflows(feature.projectId),
      gateways.workflowEngineGateway.listWorkflowRuns(feature.projectId),
    ]);

    const contexts = Array.from(
      new Map(
        [...projectContexts.filter((context) => !context.featureId), ...featureContexts].map(
          (context) => [context.id, context],
        ),
      ).values(),
    );

    return {
      detail: {
        contexts,
        feature,
        runs,
        workflows,
      } satisfies FeatureRouteDetail,
    };
  },
  component: FeatureDetailPage,
});

export function FeatureDetailPage() {
  const { detail } = Route.useLoaderData();
  const gatewayBundle = useRef(createGatewayBundle());

  if (!detail) {
    return (
      <PageFrame
        title="Feature Not Found"
        description="The requested feature could not be loaded."
      />
    );
  }

  return (
    <FeatureDetailContent
      detail={detail}
      onStartWorkflow={(workflowId, projectId) =>
        new StartWorkflowRunUseCase(gatewayBundle.current.workflowEngineGateway).execute(
          workflowId,
          projectId,
        )
      }
    />
  );
}

export function FeatureDetailContent({
  detail,
  onStartWorkflow,
}: {
  detail: FeatureRouteDetail;
  onStartWorkflow: (workflowId: string, projectId: string) => Promise<WorkflowRun>;
}) {
  const [selectedWorkflowId, setSelectedWorkflowId] = useState(detail.workflows[0]?.id ?? "");
  const [feedback, setFeedback] = useState<string | null>(null);

  useEffect(() => {
    setSelectedWorkflowId(detail.workflows[0]?.id ?? "");
    setFeedback(null);
  }, [detail]);

  const workflowNameById = useMemo(
    () => new Map(detail.workflows.map((workflow) => [workflow.id, workflow.name])),
    [detail.workflows],
  );

  const startWorkflow = useMutation({
    mutationFn: () => onStartWorkflow(selectedWorkflowId, detail.feature.projectId),
    onSuccess: (run) => {
      setFeedback(`Started workflow run ${run.id}.`);
    },
    onError: (error) => {
      setFeedback(error instanceof Error ? error.message : "Unable to start workflow.");
    },
  });

  return (
    <PageFrame
      title={detail.feature.title}
      description={detail.feature.expectedFlow}
      actions={
        <Link params={{ projectId: detail.feature.projectId }} to="/projects/$projectId">
          <Button variant="secondary">Back to project</Button>
        </Link>
      }
    >
      <div className="space-y-6">
        <div className="flex flex-wrap gap-2">
          <Badge>{detail.feature.priority}</Badge>
          <Badge>{detail.feature.status}</Badge>
        </div>

        <section className="rounded-[1.5rem] border border-border bg-background/60 p-5">
          <div className="flex items-start justify-between gap-4">
            <div>
              <h2 className="text-xl font-semibold">Workflow Trigger</h2>
              <p className="mt-2 text-sm text-muted-foreground">
                This launches the active workflow-engine run for the feature's project. It no
                longer uses the legacy feature demo tables.
              </p>
            </div>
          </div>

          <div className="mt-4 grid gap-4 md:grid-cols-2">
            <label className="space-y-2 text-sm">
              <span className="font-medium">Workflow definition</span>
              <select
                className="w-full rounded-2xl border border-border bg-card px-4 py-3"
                onChange={(event) => setSelectedWorkflowId(event.target.value)}
                value={selectedWorkflowId}
              >
                <option value="">Select a workflow</option>
                {detail.workflows.map((workflow) => (
                  <option key={workflow.id} value={workflow.id}>
                    {workflow.name}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <div className="mt-4 rounded-[1.25rem] border border-border bg-card/70 p-4">
            <p className="font-medium">Context sources</p>
            <p className="mt-1 text-sm text-muted-foreground">
              These remain visible for operator review. The current workflow-engine launch path is
              project-scoped and does not yet persist selected context into the run record.
            </p>
            <div className="mt-3 space-y-3">
              {detail.contexts.length === 0 ? (
                <p className="text-sm text-muted-foreground">No context sources available.</p>
              ) : (
                detail.contexts.map((context) => (
                  <div
                    key={context.id}
                    className="rounded-2xl border border-border bg-background px-4 py-3"
                  >
                    <p className="font-medium">{context.title}</p>
                    <p className="mt-1 text-sm text-muted-foreground">{context.rawContent}</p>
                  </div>
                ))
              )}
            </div>
          </div>

          {feedback ? (
            <div className="mt-4 rounded-2xl border border-border bg-card px-4 py-3 text-sm">
              {feedback}
            </div>
          ) : null}

          <div className="mt-4 flex justify-end">
            <Button
              disabled={!selectedWorkflowId || startWorkflow.isPending}
              onClick={() => startWorkflow.mutate()}
            >
              {startWorkflow.isPending ? "Starting..." : "Start workflow run"}
            </Button>
          </div>
        </section>

        <section className="grid gap-4 xl:grid-cols-2">
          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <h2 className="text-xl font-semibold">Feature Brief</h2>
            <div className="mt-4 space-y-4 text-sm">
              <div>
                <p className="font-medium">Business goal</p>
                <p className="mt-1 text-muted-foreground">{detail.feature.businessGoal}</p>
              </div>
              <div>
                <p className="font-medium">User problem</p>
                <p className="mt-1 text-muted-foreground">{detail.feature.userProblem}</p>
              </div>
              <div>
                <p className="font-medium">Acceptance criteria</p>
                <p className="mt-1 text-muted-foreground">{detail.feature.acceptanceCriteria}</p>
              </div>
            </div>
          </div>

          <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
            <h2 className="text-xl font-semibold">Related Runs</h2>
            <div className="mt-4 space-y-3">
              {detail.runs.length === 0 ? (
                <p className="text-sm text-muted-foreground">No workflow runs yet.</p>
              ) : (
                detail.runs.map((run) => (
                  <Link
                    key={run.id}
                    className="flex items-center justify-between rounded-2xl border border-border bg-card px-4 py-3"
                    to="/workflow-runs"
                  >
                    <span className="font-medium">
                      {workflowNameById.get(run.workflowId) ?? run.workflowId}
                    </span>
                    <Badge>{run.status}</Badge>
                  </Link>
                ))
              )}
            </div>
          </div>
        </section>
      </div>
    </PageFrame>
  );
}
