"use client";

import { useMutation } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { ProjectWorkspaceBinding } from "@/domain/model/entity/project-workspace-binding";
import { StartWorkflowRunUseCase } from "@/domain/usecase/workflow-engine/start-workflow-run-usecase";
import { ensureProjectHasUsableBinding } from "@/features/projects/project-binding-launch-guard";
import { Badge } from "@/presentation/components/ui/badge";

type GenerationLaunchPanelProps = {
  projectId: string;
  stepType: string;
  title: string;
  description: string;
  buttonLabel: string;
  promptLabel: string;
  promptPlaceholder: string;
  promptHint: string;
  bindings: ProjectWorkspaceBinding[];
  tone?: "neutral" | "success" | "warning";
};

export function GenerationLaunchPanel({
  projectId,
  stepType,
  title,
  description,
  buttonLabel,
  promptLabel,
  promptPlaceholder,
  promptHint,
  bindings,
  tone = "neutral",
}: GenerationLaunchPanelProps) {
  const navigate = useNavigate();
  const gatewayBundle = useRef(createGatewayBundle());
  const startWorkflowRunUseCase = useRef(
    new StartWorkflowRunUseCase(gatewayBundle.current.workflowEngineGateway),
  );
  const [prompt, setPrompt] = useState("");

  const startGeneration = useMutation({
    mutationFn: async () => {
      const beginPrompt = prompt.trim();
      if (!beginPrompt) {
        throw new Error("Prompt is required.");
      }

      await ensureProjectHasUsableBinding(projectId, bindings);

      return startWorkflowRunUseCase.current.execute({
        projectId,
        startMode: "single-step",
        stepType,
        beginPrompt,
      });
    },
    onSuccess: async (run) => {
      await navigate({ to: "/workflow-runs/$runId", params: { runId: run.id } });
    },
  });

  return (
    <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            AI Generation
          </p>
          <h3 className="mt-3 text-2xl font-semibold tracking-tight">{title}</h3>
          <p className="mt-2 max-w-2xl text-sm text-muted-foreground">{description}</p>
        </div>
        <Badge tone={tone}>{stepType}</Badge>
      </div>

      <div className="mt-6 space-y-4">
        <label className="block space-y-2">
          <span className="text-sm font-medium">{promptLabel}</span>
          <textarea
            className="min-h-40 w-full rounded-[1.4rem] border border-border bg-card px-4 py-4 text-sm outline-none"
            placeholder={promptPlaceholder}
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
          />
        </label>

        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm text-muted-foreground">{promptHint}</p>
          <Button
            disabled={startGeneration.isPending}
            type="button"
            onClick={() => {
              void startGeneration.mutate();
            }}
          >
            {startGeneration.isPending ? "Launching..." : buttonLabel}
          </Button>
        </div>

        {startGeneration.error ? (
          <p className="text-sm text-danger">
            {startGeneration.error instanceof Error
              ? startGeneration.error.message
              : "Unable to launch generation."}
          </p>
        ) : null}
      </div>
    </section>
  );
}
