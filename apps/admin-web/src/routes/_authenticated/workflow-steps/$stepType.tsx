import { useEffect, useRef, useState } from "react";
import { createFileRoute, Link } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import {
  deriveStepPromptBase,
  REASONING_EFFORT_OPTIONS,
  STEP_MODEL_OPTIONS,
  type ArtifactDefinition,
  type McpAccessMode,
  type StepDefinition,
} from "@/domain/model/entity/workflow-engine";
import { useSupportedModels } from "@/presentation/hooks/use-supported-models";
import { useMemo } from "react";
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

  const { data: supportedModels } = useSupportedModels();
  const modelsList = useMemo(() => {
    if (!supportedModels || supportedModels.length === 0) return STEP_MODEL_OPTIONS;
    return supportedModels
      .filter((m) => m.isEnabled)
      .map((m) => ({ value: m.modelId, label: m.displayName }));
  }, [supportedModels]);

  const [loading, setLoading] = useState(true);
  const [loadingArtifactDefinitions, setLoadingArtifactDefinitions] = useState(true);
  const [saving, setSaving] = useState(false);
  const [step, setStep] = useState<StepDefinition | null>(null);
  const [artifactDefinitions, setArtifactDefinitions] = useState<ArtifactDefinition[]>([]);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [requiredMcps, setRequiredMcps] = useState<string[]>([]);
  const [mcpAccessMode, setMcpAccessMode] = useState<McpAccessMode>("read_only");
  const [selectedMcpType, setSelectedMcpType] = useState<string>(integrationTypes[0] ?? "");
  const [requiredSkills, setRequiredSkills] = useState("");
  const [teamRole, setTeamRole] = useState("");
  const [promptBase, setPromptBase] = useState("");
  const [subagent, setSubagent] = useState("");
  const [model, setModel] = useState<string>(DEFAULT_STEP_MODEL);
  const [reasoningEffort, setReasoningEffort] = useState(DEFAULT_REASONING_EFFORT);
  const [yoloMode, setYoloMode] = useState(false);
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
        setMcpAccessMode(match?.mcpAccessMode ?? "read_only");
        setSelectedMcpType(firstAvailableMcpType(match?.requiredMcps ?? []));
        setRequiredSkills(match?.requiredSkills.join(", ") ?? "");
        setTeamRole(match?.teamRole ?? "");
        setPromptBase(match?.promptBase ?? "");
        setSubagent(match?.subagent ?? "");
        setModel(match?.model ?? DEFAULT_STEP_MODEL);
        setReasoningEffort(match?.reasoningEffort ?? DEFAULT_REASONING_EFFORT);
        setYoloMode(match?.yoloMode ?? false);
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
    const isSupported = modelsList.some((option) => option.value === model);
    if (!isSupported) {
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
        mcpAccessMode: requiredMcps.includes("google_drive") ? mcpAccessMode : "read_only",
        requiredSkills: requiredSkills
          .split(",")
          .map((item) => item.trim())
          .filter(Boolean),
        teamRole: teamRole.trim() || null,
        subagent: subagent.trim() || null,
        model,
        reasoningEffort: reasoningEffort || DEFAULT_REASONING_EFFORT,
        yoloMode,
        inputArtifactDefinitions,
        outputArtifactDefinitions,
        agentType,
      });
      setStep(saved);
      setName(saved.name);
      setDescription(saved.description);
      setRequiredMcps(saved.requiredMcps);
      setMcpAccessMode(saved.mcpAccessMode ?? "read_only");
      setSelectedMcpType(firstAvailableMcpType(saved.requiredMcps));
      setRequiredSkills(saved.requiredSkills.join(", "));
      setTeamRole(saved.teamRole ?? "");
      setPromptBase(saved.promptBase ?? "");
      setSubagent(saved.subagent ?? "");
      setModel(saved.model ?? DEFAULT_STEP_MODEL);
      setReasoningEffort(saved.reasoningEffort ?? DEFAULT_REASONING_EFFORT);
      setYoloMode(saved.yoloMode);
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
    if (mcpType === "google_drive") {
      setMcpAccessMode("read_only");
    }
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
            {requiredMcps.includes("google_drive") ? (
              <label className="space-y-2 text-sm">
                <span className="font-medium">Google Drive access</span>
                <select
                  className="w-full rounded-2xl border border-border bg-card px-4 py-3"
                  value={mcpAccessMode}
                  onChange={(event) => setMcpAccessMode(event.target.value as McpAccessMode)}
                >
                  <option value="read_only">Read only</option>
                  <option value="read_write">Read + write</option>
                </select>
              </label>
            ) : null}
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
          <p className="text-[11px] text-muted-foreground mt-1 leading-normal">
            Specifying a subagent configures an isolated execution session for this step. Leave empty to reuse the main workflow session.
          </p>
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Model</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={model}
            onChange={(event) => setModel(event.target.value)}
          >
            {modelsList.map((option) => (
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
        <label className="flex items-start gap-3 rounded-2xl border border-border bg-card px-4 py-3 text-sm">
          <input
            checked={yoloMode}
            className="mt-1"
            type="checkbox"
            onChange={(event) => setYoloMode(event.target.checked)}
          />
          <span>
            <span className="block font-medium">YOLO for single-step runs</span>
            <span className="block text-xs text-muted-foreground">
              Used only when this step is launched directly. Workflows use workflow YOLO.
            </span>
          </span>
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
