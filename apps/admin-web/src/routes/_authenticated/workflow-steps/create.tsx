import { useEffect, useRef, useState } from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { ListArtifactDefinitionsUseCase } from "@/domain/usecase/workflow-engine/list-artifact-definitions-usecase";
import { SaveStepDefinitionUseCase } from "@/domain/usecase/workflow-engine/save-step-definition-usecase";
import { ArtifactDefinitionSelector } from "@/features/workflow-engine/artifact-definition-selector";
import {
  deriveStepPromptBase,
  REASONING_EFFORT_OPTIONS,
  STEP_MODEL_OPTIONS,
  type ArtifactDefinition,
  type McpAccessMode,
} from "@/domain/model/entity/workflow-engine";
import { useSupportedModels } from "@/presentation/hooks/use-supported-models";
import { useMemo } from "react";
import { integrationTypes } from "@/features/mcp/integration-config";

export const Route = createFileRoute("/_authenticated/workflow-steps/create")({
  component: CreateWorkflowStepPage,
});

const DEFAULT_STEP_MODEL =
  STEP_MODEL_OPTIONS.find((option) => option.value === "gpt-5.4")?.value ??
  STEP_MODEL_OPTIONS[0].value;
const DEFAULT_REASONING_EFFORT = "medium";

export function CreateWorkflowStepPage() {
  const navigate = useNavigate();
  const gatewayBundle = useRef(createGatewayBundle());
  const saveStepDefinitionUseCase = useRef(
    new SaveStepDefinitionUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const listArtifactDefinitionsUseCase = useRef(
    new ListArtifactDefinitionsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const { data: supportedModels } = useSupportedModels();
  const modelsList = useMemo(() => {
    if (!supportedModels || supportedModels.length === 0) return STEP_MODEL_OPTIONS;
    return supportedModels
      .filter((m) => m.isEnabled)
      .map((m) => ({ value: m.modelId, label: m.displayName }));
  }, [supportedModels]);

  const [saving, setSaving] = useState(false);
  const [loadingArtifactDefinitions, setLoadingArtifactDefinitions] = useState(true);
  const [artifactDefinitions, setArtifactDefinitions] = useState<ArtifactDefinition[]>([]);
  const [stepType, setStepType] = useState("");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [requiredMcps, setRequiredMcps] = useState("");
  const [mcpAccessMode, setMcpAccessMode] = useState<McpAccessMode>("read_only");
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
      try {
        const definitions = await listArtifactDefinitionsUseCase.current.execute();
        setArtifactDefinitions(definitions);
      } catch {
        setArtifactDefinitions([]);
      } finally {
        setLoadingArtifactDefinitions(false);
      }
    };

    void load();
  }, []);

  const save = async () => {
    if (!stepType || !name || !description) {
      window.alert("Step key, name, and description are required.");
      return;
    }
    const isSupported = modelsList.some((option) => option.value === model);
    if (!isSupported) {
      window.alert("Select a supported model.");
      return;
    }

    const parsedRequiredMcps = requiredMcps
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
    const unknownMcp = parsedRequiredMcps.find(
      (mcp) => !integrationTypes.includes(mcp as (typeof integrationTypes)[number]),
    );
    if (unknownMcp) {
      window.alert(`Unknown MCP type "${unknownMcp}". Use a known MCP key.`);
      return;
    }

    setSaving(true);
    try {
      const now = new Date().toISOString();
      await saveStepDefinitionUseCase.current.execute({
        stepType,
        name,
        description,
        promptBase: promptBase.trim() || deriveStepPromptBase({ stepType, name, description }),
        requiredMcps: parsedRequiredMcps,
        mcpAccessMode: parsedRequiredMcps.includes("google_drive") ? mcpAccessMode : "read_only",
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
        createdAt: now,
        updatedAt: now,
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
        {requiredMcps
          .split(",")
          .map((item) => item.trim())
          .includes("google_drive") ? (
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
