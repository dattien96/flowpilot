import { useEffect, useRef, useState } from "react";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { ListStepDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-step-definitions-usecase";
import type { StepDefinition } from "@/domain/model/entity/workflow-engine";

export const Route = createFileRoute("/_authenticated/workflow-steps")({
  component: WorkflowStepsPage,
});

function WorkflowStepsPage() {
  const location = useLocation();
  const gatewayBundle = useRef(createGatewayBundle());
  const listStepDefinitionsUseCase = useRef(
    new ListStepDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const [steps, setSteps] = useState<StepDefinition[]>([]);

  useEffect(() => {
    void listStepDefinitionsUseCase.current.execute().then(setSteps);
  }, []);

  if (location.pathname !== "/workflow-steps") {
    return <Outlet />;
  }

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
      <div className="grid gap-4 xl:grid-cols-2">
        {steps.map((step) => (
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
          </div>
        ))}
      </div>
    </PageFrame>
  );
}
