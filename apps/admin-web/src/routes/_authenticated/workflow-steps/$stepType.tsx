import { useEffect, useRef, useState } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { StepDefinition } from "@/domain/model/entity/workflow-engine";
import { ListStepDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-step-definitions-usecase";
import { SaveStepDefinitionUseCase } from "@/domain/usecase/workflow-engine/save-step-definition-usecase";

export const Route = createFileRoute("/_authenticated/workflow-steps/$stepType")({
  component: WorkflowStepDetailPage,
});

export function WorkflowStepDetailPage() {
  const { stepType } = Route.useParams();
  const gatewayBundle = useRef(createGatewayBundle());
  const listStepDefinitionsUseCase = useRef(
    new ListStepDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const saveStepDefinitionUseCase = useRef(
    new SaveStepDefinitionUseCase(gatewayBundle.current.workflowEngineGateway)
  );

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [step, setStep] = useState<StepDefinition | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [requiredMcps, setRequiredMcps] = useState("");
  const [requiredSkills, setRequiredSkills] = useState("");
  const [agentType, setAgentType] = useState<"standard" | "autonomous">("standard");

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const definitions = await listStepDefinitionsUseCase.current.execute();
        const match = definitions.find((definition) => definition.stepType === stepType) ?? null;
        setStep(match);
        setName(match?.name ?? "");
        setDescription(match?.description ?? "");
        setRequiredMcps(match?.requiredMcps.join(", ") ?? "");
        setRequiredSkills(match?.requiredSkills.join(", ") ?? "");
        setAgentType(match?.agentType ?? "standard");
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, [stepType]);

  const save = async () => {
    if (!step || !name || !description) {
      window.alert("Step name and description are required.");
      return;
    }

    setSaving(true);
    try {
      const saved = await saveStepDefinitionUseCase.current.execute({
        ...step,
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
      setStep(saved);
      setName(saved.name);
      setDescription(saved.description);
      setRequiredMcps(saved.requiredMcps.join(", "));
      setRequiredSkills(saved.requiredSkills.join(", "));
      setAgentType(saved.agentType);
      window.alert("Step definition saved.");
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to save step definition.");
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <PageFrame title="Step Definition" description="Loading step definition..." />;
  }

  if (!step) {
    return (
      <PageFrame
        title="Step Definition"
        description="The requested step definition could not be found."
        actions={
          <Link to="/workflow-steps">
            <Button variant="secondary">Back to steps</Button>
          </Link>
        }
      />
    );
  }

  return (
    <PageFrame
      title={step.name}
      description="Review and update a reusable workflow step definition."
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
      <div className="grid gap-4 rounded-[1.5rem] border border-border bg-background/60 p-5 md:grid-cols-3">
        <div className="rounded-2xl border border-border bg-card px-4 py-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Step key
          </p>
          <p className="mt-2 text-sm font-medium">{step.stepType}</p>
        </div>
        <div className="rounded-2xl border border-border bg-card px-4 py-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Created
          </p>
          <p className="mt-2 text-sm font-medium">{new Date(step.createdAt).toLocaleString()}</p>
        </div>
        <div className="rounded-2xl border border-border bg-card px-4 py-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Updated
          </p>
          <p className="mt-2 text-sm font-medium">{new Date(step.updatedAt).toLocaleString()}</p>
        </div>
      </div>

      <div className="grid gap-4 rounded-[1.5rem] border border-border bg-background/60 p-5 md:grid-cols-2">
        <label className="space-y-2 text-sm">
          <span className="font-medium">Step key</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-muted-foreground"
            disabled
            value={step.stepType}
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
