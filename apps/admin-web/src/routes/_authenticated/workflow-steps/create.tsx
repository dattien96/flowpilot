import { useState, useRef } from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { SaveStepDefinitionUseCase } from "@/domain/usecase/workflow-engine/save-step-definition-usecase";

export const Route = createFileRoute("/_authenticated/workflow-steps/create")({
  component: CreateWorkflowStepPage,
});

function CreateWorkflowStepPage() {
  const navigate = useNavigate();
  const gatewayBundle = useRef(createGatewayBundle());
  const saveStepDefinitionUseCase = useRef(
    new SaveStepDefinitionUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const [saving, setSaving] = useState(false);
  const [stepType, setStepType] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [requiredMcps, setRequiredMcps] = useState("");
  const [requiredSkills, setRequiredSkills] = useState("");
  const [agentType, setAgentType] = useState<"standard" | "autonomous">("standard");

  const save = async () => {
    if (!stepType || !name || !description) {
      window.alert("Step key, name, and description are required.");
      return;
    }

    setSaving(true);
    try {
      await saveStepDefinitionUseCase.current.execute({
        stepType,
        name,
        description,
        requiredMcps: requiredMcps
          .split(",")
          .map((item) => item.trim())
          .filter(Boolean),
        requiredSkills: requiredSkills
          .split(",")
          .map((item) => item.trim())
          .filter(Boolean),
        agentType,
      });
      await navigate({ to: "/workflow-steps" as never });
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to save step definition.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <PageFrame
      title="Create Step Definition"
      description="Add a reusable workflow step to the shared catalog so new workflows can pick and order it."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/workflow-steps">
            <Button variant="secondary">Back to steps</Button>
          </Link>
          <Button disabled={saving} onClick={() => void save()}>
            Save step
          </Button>
        </div>
      }
    >
      <div className="grid gap-4 rounded-[1.5rem] border border-border bg-background/60 p-5 md:grid-cols-2">
        <label className="space-y-2 text-sm">
          <span className="font-medium">Step key</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="example: security_review"
            value={stepType}
            onChange={(event) => setStepType(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Display name</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm md:col-span-2">
          <span className="font-medium">Description</span>
          <textarea
            className="min-h-28 w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Required MCPs</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="jira, figma"
            value={requiredMcps}
            onChange={(event) => setRequiredMcps(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Required skills</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="security_review_skill"
            value={requiredSkills}
            onChange={(event) => setRequiredSkills(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Agent type</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={agentType}
            onChange={(event) => setAgentType(event.target.value as "standard" | "autonomous")}
          >
            <option value="standard">standard</option>
            <option value="autonomous">autonomous</option>
          </select>
        </label>
      </div>
    </PageFrame>
  );
}
