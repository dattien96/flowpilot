import { useEffect, useRef, useState } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { ArtifactDefinition, StepDefinition } from "@/domain/model/entity/workflow-engine";
import { ListArtifactDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-artifact-definitions-usecase";
import { ListStepDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-step-definitions-usecase";
import { SaveStepDefinitionUseCase } from "@/domain/usecase/workflow-engine/save-step-definition-usecase";
import { ArtifactDefinitionSelector } from "@/features/workflow-engine/artifact-definition-selector";
import { integrationTypes, toTitleCase } from "@/features/mcp/integration-config";

export const Route = createFileRoute("/_authenticated/workflow-steps/$stepType")({
  component: WorkflowStepDetailPage,
});

function firstAvailableMcpType(selectedMcps: string[]) {
  return integrationTypes.find((type) => !selectedMcps.includes(type)) ?? "";
}

export function WorkflowStepDetailPage() {
  const { stepType } = Route.useParams();
  const gatewayBundle = useRef(createGatewayBundle());
  const listStepDefinitionsUseCase = useRef(
    new ListStepDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const listArtifactDefinitionsUseCase = useRef(
    new ListArtifactDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const saveStepDefinitionUseCase = useRef(
    new SaveStepDefinitionUseCase(gatewayBundle.current.workflowEngineGateway)
  );

  const [loading, setLoading] = useState(true);
  const [loadingArtifactDefinitions, setLoadingArtifactDefinitions] = useState(true);
  const [saving, setSaving] = useState(false);
  const [step, setStep] = useState<StepDefinition | null>(null);
  const [artifactDefinitions, setArtifactDefinitions] = useState<ArtifactDefinition[]>([]);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [requiredMcps, setRequiredMcps] = useState<string[]>([]);
  const [selectedMcpType, setSelectedMcpType] = useState(integrationTypes[0] ?? "");
  const [requiredSkills, setRequiredSkills] = useState("");
  const [inputArtifactDefinitions, setInputArtifactDefinitions] = useState<string[]>([]);
  const [outputArtifactDefinitions, setOutputArtifactDefinitions] = useState<string[]>([]);
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
        setRequiredMcps(match?.requiredMcps ?? []);
        setSelectedMcpType(firstAvailableMcpType(match?.requiredMcps ?? []));
        setRequiredSkills(match?.requiredSkills.join(", ") ?? "");
        setInputArtifactDefinitions(match?.inputArtifactDefinitions ?? []);
        setOutputArtifactDefinitions(match?.outputArtifactDefinitions ?? []);
        setAgentType(match?.agentType ?? "standard");
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, [stepType]);

  useEffect(() => {
    const loadArtifactDefinitions = async () => {
      try {
        const definitions = await listArtifactDefinitionsUseCase.current.execute();
        setArtifactDefinitions(definitions);
      } catch {
        setArtifactDefinitions([]);
      } finally {
        setLoadingArtifactDefinitions(false);
      }
    };

    void loadArtifactDefinitions();
  }, []);

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
        requiredMcps,
        requiredSkills: requiredSkills
          .split(",")
          .map((item) => item.trim())
          .filter(Boolean),
        inputArtifactDefinitions,
        outputArtifactDefinitions,
        agentType,
      });
      setStep(saved);
      setName(saved.name);
      setDescription(saved.description);
      setRequiredMcps(saved.requiredMcps);
      setSelectedMcpType(firstAvailableMcpType(saved.requiredMcps));
      setRequiredSkills(saved.requiredSkills.join(", "));
      setInputArtifactDefinitions(saved.inputArtifactDefinitions ?? []);
      setOutputArtifactDefinitions(saved.outputArtifactDefinitions ?? []);
      setAgentType(saved.agentType);
      window.alert("Step definition saved.");
    } catch (error) {
      window.alert(error instanceof Error ? error.message : "Unable to save step definition.");
    } finally {
      setSaving(false);
    }
  };

  const addRequiredMcp = () => {
    if (!selectedMcpType || requiredMcps.includes(selectedMcpType)) {
      return;
    }

    setRequiredMcps((current) => [...current, selectedMcpType]);
    setSelectedMcpType(firstAvailableMcpType([...requiredMcps, selectedMcpType]));
  };

  const removeRequiredMcp = (mcpType: string) => {
    const nextRequiredMcps = requiredMcps.filter((current) => current !== mcpType);
    setRequiredMcps(nextRequiredMcps);
    setSelectedMcpType(firstAvailableMcpType(nextRequiredMcps));
  };

  const availableMcpOptions = integrationTypes.filter((type) => !requiredMcps.includes(type));

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
          <div className="space-y-3 rounded-2xl border border-border bg-card p-3">
            <div className="flex flex-wrap items-end gap-3">
              <label className="min-w-[220px] flex-1 space-y-2 text-sm">
                <span className="font-medium">Available MCP</span>
                <select
                  className="w-full rounded-2xl border border-border bg-background px-4 py-3"
                  value={selectedMcpType}
                  onChange={(event) => setSelectedMcpType(event.target.value)}
                >
                  {availableMcpOptions.length === 0 ? (
                    <option value="">No more MCP types available</option>
                  ) : null}
                  {availableMcpOptions.map((mcpType) => (
                    <option key={mcpType} value={mcpType}>
                      {toTitleCase(mcpType)}
                    </option>
                  ))}
                </select>
              </label>
              <Button
                disabled={!selectedMcpType || availableMcpOptions.length === 0}
                type="button"
                variant="secondary"
                onClick={addRequiredMcp}
              >
                Add MCP
              </Button>
            </div>
            <div className="flex flex-wrap gap-2">
              {requiredMcps.length === 0 ? (
                <p className="text-sm text-muted-foreground">No required MCPs selected.</p>
              ) : null}
              {requiredMcps.map((mcpType) => (
                <span
                  key={mcpType}
                  className="inline-flex items-center gap-2 rounded-full border border-border bg-background px-3 py-1 text-xs font-medium"
                >
                  {toTitleCase(mcpType)}
                  <button
                    className="text-muted-foreground hover:text-foreground"
                    type="button"
                    onClick={() => removeRequiredMcp(mcpType)}
                  >
                    Remove
                  </button>
                </span>
              ))}
            </div>
          </div>
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
        <div className="space-y-2 md:col-span-2">
          <ArtifactDefinitionSelector
            label="Input artifact definitions"
            description="Select the artifact definitions this step requires before it can run."
            selectedArtifactKeys={inputArtifactDefinitions}
            artifactDefinitions={artifactDefinitions}
            onChange={setInputArtifactDefinitions}
          />
          <ArtifactDefinitionSelector
            label="Output artifact definitions"
            description="Select the artifact definitions this step may generate when it completes."
            selectedArtifactKeys={outputArtifactDefinitions}
            artifactDefinitions={artifactDefinitions}
            onChange={setOutputArtifactDefinitions}
          />
        </div>
        {loadingArtifactDefinitions ? (
          <p className="text-sm text-muted-foreground md:col-span-2">
            Loading artifact definitions...
          </p>
        ) : null}
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
