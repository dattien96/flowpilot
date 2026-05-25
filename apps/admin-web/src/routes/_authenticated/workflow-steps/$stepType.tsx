import { useEffect, useRef, useState } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import {
  deriveStepPromptBase,
  isSupportedStepModel,
  REASONING_EFFORT_OPTIONS,
  STEP_MODEL_OPTIONS,
  type ArtifactDefinition,
  type StepDefinition,
} from "@/domain/model/entity/workflow-engine";
import { ListArtifactDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-artifact-definitions-usecase";
import { ListStepDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-step-definitions-usecase";
import { SaveStepDefinitionUseCase } from "@/domain/usecase/workflow-engine/save-step-definition-usecase";
import { ArtifactDefinitionSelector } from "@/features/workflow-engine/artifact-definition-selector";
import { integrationTypes, toTitleCase } from "@/features/mcp/integration-config";

export const Route = createFileRoute("/_authenticated/workflow-steps/$stepType")({
  component: WorkflowStepDetailPage,
});

const DEFAULT_STEP_MODEL =
  STEP_MODEL_OPTIONS.find((option) => option.value === "gpt-5.4")?.value ??
  STEP_MODEL_OPTIONS[0].value;
const DEFAULT_REASONING_EFFORT = "medium";

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
  const [selectedMcpType, setSelectedMcpType] = useState<string>(integrationTypes[0] ?? "");
  const [requiredSkills, setRequiredSkills] = useState("");
  const [teamRole, setTeamRole] = useState("");
  const [promptBase, setPromptBase] = useState("");
  const [subagent, setSubagent] = useState("");
  const [model, setModel] = useState<string>(DEFAULT_STEP_MODEL);
  const [reasoningEffort, setReasoningEffort] = useState(DEFAULT_REASONING_EFFORT);
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
        setTeamRole(match?.teamRole ?? "");
        setPromptBase(match?.promptBase ?? "");
        setSubagent(match?.subagent ?? "");
        setModel(match?.model ?? DEFAULT_STEP_MODEL);
        setReasoningEffort(match?.reasoningEffort ?? DEFAULT_REASONING_EFFORT);
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
    if (!isSupportedStepModel(model)) {
      window.alert("Select a supported model.");
      return;
    }

    setSaving(true);
    try {
      const saved = await saveStepDefinitionUseCase.current.execute({
        ...step,
        name,
        description,
        promptBase: promptBase.trim() || deriveStepPromptBase({ stepType: step.stepType, name, description }),
        requiredMcps,
        requiredSkills: requiredSkills
          .split(",")
          .map((item) => item.trim())
          .filter(Boolean),
        teamRole: teamRole.trim() || null,
        subagent: subagent.trim() || null,
        model,
        reasoningEffort: reasoningEffort || DEFAULT_REASONING_EFFORT,
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
      setTeamRole(saved.teamRole ?? "");
      setPromptBase(saved.promptBase ?? "");
      setSubagent(saved.subagent ?? "");
      setModel(saved.model ?? DEFAULT_STEP_MODEL);
      setReasoningEffort(saved.reasoningEffort ?? DEFAULT_REASONING_EFFORT);
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
        <label className="space-y-2 text-sm">
          <span className="font-medium">Team role</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="design_lead"
            value={teamRole}
            onChange={(event) => setTeamRole(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm md:col-span-2">
          <span className="font-medium">Prompt base</span>
          <textarea
            className="min-h-28 w-full rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="Describe the execution intent for this step."
            value={promptBase}
            onChange={(event) => setPromptBase(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Subagent</span>
          <input
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            placeholder="codex-reviewer"
            value={subagent}
            onChange={(event) => setSubagent(event.target.value)}
          />
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Model</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={model}
            onChange={(event) => setModel(event.target.value)}
          >
            {STEP_MODEL_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Reasoning effort</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={reasoningEffort}
            onChange={(event) => setReasoningEffort(event.target.value)}
          >
            {REASONING_EFFORT_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
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
