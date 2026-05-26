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
            <Button variant="secondary" className="rounded-xl border border-border bg-background/50 hover:bg-muted/80 backdrop-blur-sm transition-all duration-300">
              Workflow definitions
            </Button>
          </Link>
          <Link to="/workflow-steps/create">
            <Button className="rounded-xl shadow-sm hover:shadow-md hover:opacity-90 transition-all duration-300">
              Create step
            </Button>
          </Link>
        </div>
      }
    >
      <div className="relative overflow-hidden rounded-[1.5rem] border border-border/60 bg-gradient-to-r from-violet-500/5 via-purple-500/5 to-blue-500/5 p-6 backdrop-blur-sm">
        <div className="absolute top-0 left-0 right-0 h-[1px] bg-gradient-to-r from-violet-500/30 via-purple-500/30 to-blue-500/30" />
        <p className="font-mono text-xs uppercase tracking-[0.28em] text-primary/80 font-semibold">
          Main Workflow Reference
        </p>
        <p className="mt-3 text-sm text-foreground/80 leading-relaxed font-medium">
          Idea - Business - Architecture - Tech Spec - Code Plan - TDD - Coding Implementation Checklist - Review
        </p>
      </div>

      <div className="rounded-[1.5rem] border border-border/60 bg-card/45 p-5 backdrop-blur-sm">
        <label className="space-y-2 text-sm flex flex-col md:flex-row md:items-center md:gap-4 md:space-y-0">
          <span className="font-semibold text-muted-foreground">Sort by</span>
          <select
            className="w-full rounded-xl border border-border/80 bg-background/50 px-4 py-2.5 md:max-w-xs focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all cursor-pointer font-medium"
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

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {sortedSteps.map((step) => {
          const isStandard = step.agentType === "standard";
          return (
            <div
              key={step.stepType}
              className="group relative flex flex-col justify-between overflow-hidden rounded-[1.8rem] border border-border/60 bg-gradient-to-b from-card/90 to-background/40 backdrop-blur-md p-6 shadow-sm hover:shadow-xl hover:border-primary/20 hover:-translate-y-1 transition-all duration-300"
            >
              {/* Dynamic top gradient line based on agent type */}
              <div 
                className={`absolute top-0 left-0 right-0 h-[3px] opacity-70 group-hover:opacity-100 transition-opacity bg-gradient-to-r ${
                  isStandard 
                    ? "from-violet-500/80 via-purple-500/80 to-blue-500/80" 
                    : "from-cyan-500/80 via-teal-500/80 to-emerald-500/80"
                }`} 
              />

              <div className="flex flex-col h-full">
                {/* Header */}
                <div className="flex items-start justify-between gap-3 mb-4">
                  <div className="space-y-1">
                    <h3 className="text-xl font-bold tracking-tight text-foreground group-hover:text-primary transition-colors duration-300">
                      {step.name}
                    </h3>
                    <p className="font-mono text-[10px] tracking-wider text-muted-foreground uppercase mt-1">
                      Key: {step.stepType}
                    </p>
                  </div>
                  <span className={`shrink-0 rounded-full px-2.5 py-0.5 text-[10px] font-semibold tracking-wider uppercase border ${
                    isStandard 
                      ? "bg-violet-500/10 text-violet-500 border-violet-500/20" 
                      : "bg-cyan-500/10 text-cyan-500 border-cyan-500/20"
                  }`}>
                    {step.agentType}
                  </span>
                </div>

                {/* Description */}
                <p className="text-sm text-muted-foreground leading-relaxed mb-5 line-clamp-3 flex-grow min-h-[3rem]">
                  {step.description}
                </p>

                {/* Metadata & Requirements */}
                <div className="space-y-3 border-t border-border/40 pt-4 mb-6 text-xs text-muted-foreground/90">
                  <p className="font-medium text-foreground/75 flex justify-between">
                    <span>Model:</span>
                    <span className="font-mono bg-muted/60 px-2 py-0.5 rounded text-[11px] text-foreground/90 border border-border/30">
                      {step.model}
                    </span>
                  </p>
                  <p className="font-semibold text-foreground/90">
                    Reasoning: {formatReasoningSummary(step.reasoningEffort)}
                  </p>
                  <p className="font-medium text-foreground/75">
                    MCPs: {step.requiredMcps.length > 0 ? step.requiredMcps.join(", ") : "None"}
                  </p>
                  <p className="font-medium text-foreground/75">
                    Skills: {step.requiredSkills.length > 0 ? step.requiredSkills.join(", ") : "None"}
                  </p>
                  
                  {/* Artifacts bindings */}
                  <div className="pt-2 border-t border-dashed border-border/30 space-y-1.5">
                    <p className="font-medium text-foreground/75">
                      Input artifacts: {formatArtifactSummary(step.inputArtifactDefinitions)}
                    </p>
                    <p className="font-medium text-foreground/75">
                      Output artifacts: {formatArtifactSummary(step.outputArtifactDefinitions)}
                    </p>
                  </div>
                </div>
              </div>

              {/* Actions Footer */}
              <div className="flex items-center justify-between pt-2 border-t border-border/40 mt-auto">
                <span className="text-[10px] font-mono text-muted-foreground/60">
                  Key: {step.stepType}
                </span>
                <Link params={{ stepType: step.stepType }} to="/workflow-steps/$stepType">
                  <Button size="sm" variant="secondary" className="rounded-xl bg-muted/60 hover:bg-primary hover:text-primary-foreground border border-border/40 transition-all duration-300 group/btn">
                    Open step
                    <ArrowRight className="ml-1.5 h-3.5 w-3.5 group-hover/btn:translate-x-1 transition-transform duration-300" />
                  </Button>
                </Link>
              </div>
            </div>
          );
        })}
      </div>
    </PageFrame>
  );
}
