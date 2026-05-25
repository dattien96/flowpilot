import { useEffect, useMemo, useRef, useState } from "react";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import {
  REASONING_EFFORT_OPTIONS,
  type ArtifactDefinition,
} from "@/domain/model/entity/workflow-engine";
import { ListStepDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-step-definitions-usecase";
import type { StepDefinition } from "@/domain/model/entity/workflow-engine";
import { ListArtifactDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-artifact-definitions-usecase";

type StepSortOption = "name-asc" | "name-desc" | "updated-asc" | "updated-desc";

export const Route = createFileRoute("/_authenticated/workflow-steps")({
  component: WorkflowStepsPage,
});

export function WorkflowStepsPage() {
  const location = useLocation();
  const gatewayBundle = useRef(createGatewayBundle());
  const listStepDefinitionsUseCase = useRef(
    new ListStepDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const listArtifactDefinitionsUseCase = useRef(
    new ListArtifactDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const [steps, setSteps] = useState<StepDefinition[]>([]);
  const [artifactDefinitions, setArtifactDefinitions] = useState<ArtifactDefinition[]>([]);
  const [sortBy, setSortBy] = useState<StepSortOption>("updated-desc");

  useEffect(() => {
    void Promise.all([
      listStepDefinitionsUseCase.current.execute(),
      listArtifactDefinitionsUseCase.current.execute().catch(() => []),
    ]).then(([nextSteps, nextArtifactDefinitions]) => {
      setSteps(nextSteps);
      setArtifactDefinitions(nextArtifactDefinitions);
    });
  }, []);

  const artifactDefinitionNames = useMemo(() => {
    return new Map(artifactDefinitions.map((definition) => [definition.key, definition.name]));
  }, [artifactDefinitions]);

  const sortedSteps = useMemo(() => {
    return steps.slice().sort((left, right) => {
      switch (sortBy) {
        case "name-asc":
          return left.name.localeCompare(right.name);
        case "name-desc":
          return right.name.localeCompare(left.name);
        case "updated-asc":
          return new Date(left.updatedAt || 0).getTime() - new Date(right.updatedAt || 0).getTime();
        case "updated-desc":
        default:
          return new Date(right.updatedAt || 0).getTime() - new Date(left.updatedAt || 0).getTime();
      }
    });
  }, [sortBy, steps]);

  if (location.pathname !== "/workflow-steps") {
    return <Outlet />;
  }

  const formatArtifactSummary = (artifactKeys: string[] | undefined) => {
    if (!artifactKeys || artifactKeys.length === 0) {
      return "None";
    }

    return artifactKeys
      .map((artifactKey) => artifactDefinitionNames.get(artifactKey) ?? artifactKey)
      .join(", ");
  };

  const formatReasoningSummary = (reasoningEffort?: string | null) => {
    if (!reasoningEffort) {
      return "Inherited";
    }

    return (
      REASONING_EFFORT_OPTIONS.find((option) => option.value === reasoningEffort)?.label ??
      reasoningEffort
    );
  };

  return (
    <PageFrame
      title="Step Definitions"
      description="Full catalog of reusable workflow step definitions, including built-in and custom steps."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/workflows" search={{ projectId: undefined }}>
            <Button variant="secondary">Workflow definitions</Button>
          </Link>
          <Link to="/workflow-steps/create">
            <Button>Create step</Button>
          </Link>
        </div>
      }
    >
      <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
          Main Workflow Reference
        </p>
        <p className="mt-3 text-sm text-muted-foreground">
          Idea - Business - Architecture - Tech Spec - Code Plan - TDD - Coding
          Implementation Checklist - Review
        </p>
      </div>

      <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
        <label className="space-y-2 text-sm">
          <span className="font-medium">Sort by</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3 md:max-w-sm"
            value={sortBy}
            onChange={(event) => setSortBy(event.target.value as StepSortOption)}
          >
            <option value="name-asc">Alphabet (A-Z)</option>
            <option value="name-desc">Alphabet (Z-A)</option>
            <option value="updated-desc">Updated time (Newest first)</option>
            <option value="updated-asc">Updated time (Oldest first)</option>
          </select>
        </label>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        {sortedSteps.map((step) => (
          <div
            key={step.stepType}
            className="rounded-[1.5rem] border border-border bg-background/60 p-5"
          >
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-xl font-semibold">{step.name}</h3>
                <p className="mt-2 text-sm text-muted-foreground">{step.description}</p>
              </div>
              <span className="rounded-full border border-border px-3 py-1 text-xs font-semibold text-muted-foreground">
                {step.agentType}
              </span>
            </div>
            <p className="mt-4 text-xs text-muted-foreground">Key: {step.stepType}</p>
            <p className="mt-2 text-xs text-muted-foreground">
              MCPs: {step.requiredMcps.length > 0 ? step.requiredMcps.join(", ") : "None"}
            </p>
            <p className="mt-2 text-xs text-muted-foreground">
              Skills: {step.requiredSkills.length > 0 ? step.requiredSkills.join(", ") : "None"}
            </p>
            <div className="mt-2 grid gap-2 text-xs text-muted-foreground md:grid-cols-2">
              <p>Model: {step.model}</p>
              <p>Reasoning: {formatReasoningSummary(step.reasoningEffort)}</p>
            </div>
            <p className="mt-2 text-xs text-muted-foreground">
              Input artifacts: {formatArtifactSummary(step.inputArtifactDefinitions)}
            </p>
            <p className="mt-2 text-xs text-muted-foreground">
              Output artifacts: {formatArtifactSummary(step.outputArtifactDefinitions)}
            </p>
            <div className="mt-4 flex flex-wrap gap-2">
              <Link params={{ stepType: step.stepType }} to="/workflow-steps/$stepType">
                <Button variant="secondary">
                  Open step
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Button>
              </Link>
            </div>
          </div>
        ))}
      </div>
    </PageFrame>
  );
}
