import { randomUUID } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

import type { SupabaseClient } from "@supabase/supabase-js";

import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import {
  deriveStepPromptBase,
  isSupportedStepModel,
  REASONING_EFFORT_OPTIONS,
  type StepType,
  type WorkflowRunStartRequest,
} from "@/domain/model/entity/workflow-engine";
import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";

type WorkflowDefinitionRow = {
  id: string;
  name: string;
  project_id: string | null;
  provider_override: string | null;
  model_override: string | null;
  reasoning_effort_override: string | null;
};

type WorkflowStepRow = {
  id: string;
  step_type: string;
  order_index: number;
  is_enabled: boolean;
  provider_override: string | null;
  model_override: string | null;
  reasoning_effort_override: string | null;
  requires_approval: boolean;
};

type StepDefinitionRow = {
  step_type: string;
  name: string;
  description: string;
  prompt_base: string | null;
  required_mcps: string[] | null;
  required_skills: string[] | null;
  team_role: string | null;
  subagent: string | null;
  model: string;
  reasoning_effort: string | null;
};

type ArtifactDefinitionRow = {
  key: string;
  name: string;
  local_path_template: string;
  remote_path_template: string;
  default_file_name: string;
};

type ArtifactRunRow = {
  id: string;
  artifact_definition_key: string;
  local_path: string;
  title: string;
};

type WorkflowRunRow = {
  id: string;
  workflow_id: string;
  project_id: string;
};

type WorkflowRunStepExecutionRow = {
  id: string;
  workflow_run_id: string;
  workflow_step_id: string | null;
  execution_order_index: number;
  step_type: string;
  status: string;
  retry_count: number;
};

type DirectoryValidationResult = {
  path: string;
  usable: boolean;
  reason: string;
};

type ProjectSettingsRow = {
  default_provider: string | null;
  default_model: string | null;
  default_reasoning_effort: string | null;
};

type StepExecutionPlan = {
  workflowStepId: string;
  stepType: string;
  orderIndex: number;
  definition: StepDefinitionRow;
  providerKey: string;
  model: string;
  reasoningEffort: string | null;
  inputArtifactKeys: string[];
  outputArtifactKeys: string[];
};

async function getOrCreateSession({
  adminClient,
  localRunnerGateway,
  workflowRunId,
  stepRunId,
  providerKey,
  modelName,
  reasoningEffort,
  workingDirectory,
  subagent,
}: {
  adminClient: SupabaseClient;
  localRunnerGateway: LocalRunnerGateway;
  workflowRunId: string;
  stepRunId: string;
  providerKey: string;
  modelName: string;
  reasoningEffort: string | null;
  workingDirectory: string;
  subagent: string | null;
}) {
  let sessionRow = null;

  if (!subagent) {
    const { data } = await adminClient
      .from("workflow_run_sessions")
      .select("*")
      .eq("workflow_run_id", workflowRunId)
      .eq("status", "active")
      .filter("metadata_json->>is_main", "eq", "true")
      .maybeSingle();
    sessionRow = data;
  } else {
    const { data } = await adminClient
      .from("workflow_run_sessions")
      .select("*")
      .eq("workflow_run_id", workflowRunId)
      .eq("status", "active")
      .filter("metadata_json->>step_run_id", "eq", stepRunId)
      .maybeSingle();
    sessionRow = data;
  }

  let handle: { transportType: string; providerSessionId: string; processKey: string | null } | null = null;

  if (sessionRow && sessionRow.process_key) {
    handle = {
      transportType: sessionRow.transport_type,
      providerSessionId: sessionRow.provider_session_id,
      processKey: sessionRow.process_key,
    };
  }

  if (!handle) {
    const startResult = await localRunnerGateway.startSession({
      providerKey,
      modelName,
      reasoningEffort: reasoningEffort as any,
      workingDirectory,
      approvalMode: null,
      allowWrite: true,
    });

    handle = startResult;

    if (sessionRow) {
      await adminClient
        .from("workflow_run_sessions")
        .update({
          process_key: handle.processKey,
          provider_session_id: handle.providerSessionId,
          transport_type: handle.transportType,
          provider: providerKey,
          model: modelName,
        })
        .eq("id", sessionRow.id);
    } else {
      const metadata = !subagent ? { is_main: true } : { step_run_id: stepRunId };
      const { data: newRow, error: newRowErr } = await adminClient
        .from("workflow_run_sessions")
        .insert({
          workflow_run_id: workflowRunId,
          provider: providerKey,
          model: modelName,
          transport_type: handle.transportType,
          provider_session_id: handle.providerSessionId,
          process_key: handle.processKey,
          status: "active",
          metadata_json: metadata,
        })
        .select("*")
        .single();
      if (newRowErr) {
        throw new Error(`Failed to save session to DB: ${newRowErr.message}`);
      }
    }
  }

  return handle;
}

async function sendMessageWithRetry({
  adminClient,
  localRunnerGateway,
  workflowRunId,
  stepRunId,
  providerKey,
  modelName,
  reasoningEffort,
  workingDirectory,
  subagent,
  prompt,
  skillIds,
}: {
  adminClient: SupabaseClient;
  localRunnerGateway: LocalRunnerGateway;
  workflowRunId: string;
  stepRunId: string;
  providerKey: string;
  modelName: string;
  reasoningEffort: string | null;
  workingDirectory: string;
  subagent: string | null;
  prompt: string;
  skillIds: string[];
}) {
  try {
    let handle = await getOrCreateSession({
      adminClient,
      localRunnerGateway,
      workflowRunId,
      stepRunId,
      providerKey,
      modelName,
      reasoningEffort,
      workingDirectory,
      subagent,
    });

    try {
      const result = await localRunnerGateway.sendMessage({
        session: handle,
        prompt,
        skillIds,
        contextSourceIds: [],
      });
      return result;
    } catch (error) {
      const isSessionDead = error instanceof Error && 
        (error.message.includes("session") || error.message.includes("not found") || error.message.includes("expired"));
      if (isSessionDead) {
        if (!subagent) {
          await adminClient
            .from("workflow_run_sessions")
            .update({ process_key: null })
            .eq("workflow_run_id", workflowRunId)
            .filter("metadata_json->>is_main", "eq", "true");
        } else {
          await adminClient
            .from("workflow_run_sessions")
            .update({ process_key: null })
            .eq("workflow_run_id", workflowRunId)
            .filter("metadata_json->>step_run_id", "eq", stepRunId);
        }

        handle = await getOrCreateSession({
          adminClient,
          localRunnerGateway,
          workflowRunId,
          stepRunId,
          providerKey,
          modelName,
          reasoningEffort,
          workingDirectory,
          subagent,
        });

        const result = await localRunnerGateway.sendMessage({
          session: handle,
          prompt,
          skillIds,
          contextSourceIds: [],
        });
        return result;
      }
      throw error;
    }
  } catch (sessError) {
    const result = await localRunnerGateway.executePrompt({
      providerKey,
      modelName,
      reasoningEffort: reasoningEffort as any,
      prompt,
      skillIds,
      flowId: null,
      contextSourceIds: [],
      timeoutMs: 600000,
      workingDirectory,
      allowWrite: true,
    });
    return result;
  }
}

const SINGLE_STEP_RUNTIME_CREATED_BY = "flowpilot-runtime";
const SUPPORTED_REASONING_EFFORTS = new Set(REASONING_EFFORT_OPTIONS.map((option) => option.value));
const DEFAULT_MODEL = "gpt-5.4";
const DEFAULT_REASONING_EFFORT = "medium";

function resolveArtifactPath(
  template: string,
  context: {
    projectId: string;
    workflowId: string;
    workflowRunId: string;
    workflowRunStepId: string;
    stepType: string;
    artifactKey: string;
    defaultFileName: string;
  },
) {
  return template
    .replaceAll("{projectId}", context.projectId)
    .replaceAll("{workflowId}", context.workflowId)
    .replaceAll("{workflowRunId}", context.workflowRunId)
    .replaceAll("{workflowRunStepId}", context.workflowRunStepId)
    .replaceAll("{stepType}", context.stepType)
    .replaceAll("{artifactKey}", context.artifactKey)
    .replaceAll("{defaultFileName}", context.defaultFileName);
}

function normalizeAbsolutePath(workingDirectory: string, localPath: string) {
  return path.isAbsolute(localPath) ? localPath : path.join(workingDirectory, localPath);
}

async function insertLog(
  adminClient: SupabaseClient,
  workflowRunStepId: string,
  logLevel: "info" | "warn" | "error" | "debug",
  message: string,
) {
  const { error } = await adminClient.from("workflow_run_logs").insert({
    workflow_run_step_id: workflowRunStepId,
    log_level: logLevel,
    message,
  });

  if (error) {
    throw new Error(`Unable to write workflow run log: ${error.message}`);
  }
}

async function validateLocalDirectory(pathToValidate: string): Promise<DirectoryValidationResult> {
  const baseUrl = getLocalRunnerBaseUrl();
  let response: Response;

  try {
    response = await fetch(new URL("/directories/validate", baseUrl), {
      method: "POST",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify({ path: pathToValidate }),
    });
  } catch (error) {
    throw new Error(
      `Local runner is unreachable at ${baseUrl}. Start it with 'just runner-dev' or 'just dev', then retry workflow launch.`,
    );
  }

  if (!response.ok) {
    const message = await response.text();
    throw new Error(
      `Directory validation failed: ${message || `${response.status} ${response.statusText}`}`,
    );
  }

  return (await response.json()) as DirectoryValidationResult;
}

async function resolveWorkingDirectory(adminClient: SupabaseClient, projectId: string) {
  const { data, error } = await adminClient
    .from("project_workspace_bindings")
    .select("local_path")
    .eq("project_id", projectId)
    .order("created_at", { ascending: true });
  if (error) {
    throw new Error(`Unable to load project directory bindings: ${error.message}`);
  }

  const bindings = (data ?? []).map((row) => String(row.local_path));
  if (bindings.length === 0) {
    throw new Error(
      `This project has no directory bindings. Open /projects/${projectId}/directory-bindings and add a valid local path for this machine.`,
    );
  }

  const results = await Promise.all(bindings.map((binding) => validateLocalDirectory(binding)));
  const usableBinding = results.find((result) => result.usable);
  if (usableBinding) {
    return usableBinding.path;
  }

  const details = results.map((result) => `- ${result.path}: ${result.reason || "unusable path"}`);
  throw new Error(
    [
      "Unable to start execution because the local runner could not open any bound directory for this project.",
      "",
      "Checked bindings:",
      ...details,
    ].join("\n"),
  );
}

async function loadProjectDefaults(adminClient: SupabaseClient, projectId: string) {
  const { data, error } = await adminClient
    .from("projects")
    .select("default_provider, default_model, default_reasoning_effort")
    .eq("id", projectId)
    .maybeSingle();

  if (error) {
    throw new Error(`Unable to load project defaults: ${error.message}`);
  }

  return (data ?? {
    default_provider: null,
    default_model: null,
    default_reasoning_effort: null,
  }) as ProjectSettingsRow;
}

export function resolveProviderKeyFromModel(model: string) {
  if (model.startsWith("gpt-")) {
    return "codex";
  }
  if (model.startsWith("gemini-")) {
    return "gemini";
  }
  if (model.startsWith("claude-")) {
    return "claude";
  }

  throw new Error(`Model "${model}" is not supported by the workflow runner.`);
}

function buildPromptSection(title: string, lines: string[]) {
  return [`## ${title}`, ...lines, ""].join("\n");
}

export function buildWorkflowStepPrompt({
  beginPrompt,
  inputArtifactPaths,
  model,
  outputArtifactPaths,
  promptBase,
  stepType,
  subagent,
  teamRole,
  workingDirectory,
  requiredSkills,
}: {
  beginPrompt: string;
  inputArtifactPaths: string[];
  model: string;
  outputArtifactPaths: string[];
  promptBase: string;
  stepType: string;
  subagent: string | null;
  teamRole: string | null;
  workingDirectory: string;
  requiredSkills: string[];
}) {
  const sections = [
    "# Workflow Step Execution",
    "",
    buildPromptSection("Execution Contract", [
      `- Step type: ${stepType}`,
      `- Model: ${model}`,
      `- Working directory: ${workingDirectory}`,
      `- Team role: ${teamRole ?? "not set"}`,
      `- Subagent: ${subagent ?? "not set"}`,
    ]),
    buildPromptSection("Prompt Base", [promptBase]),
    buildPromptSection("Begin Prompt", [beginPrompt]),
    buildPromptSection(
      "Input Artifacts",
      inputArtifactPaths.length === 0
        ? ["- None"]
        : inputArtifactPaths.map((inputPath) => `- ${inputPath}`),
    ),
    buildPromptSection(
      "Required Skills",
      requiredSkills.length === 0 ? ["- None"] : requiredSkills.map((skill) => `- ${skill}`),
    ),
    buildPromptSection(
      "Output Targets",
      outputArtifactPaths.length === 0
        ? ["- No artifact output configured for this step."]
        : outputArtifactPaths.map((outputPath) => `- ${outputPath}`),
    ),
    "Return the final result in Markdown and write any requested deliverables to the listed output targets when appropriate.",
  ];

  return sections.join("\n");
}

export function buildWorkflowStepFollowUpPrompt({
  followUpPrompt,
  inputArtifactPaths,
  outputArtifactPaths,
  workingDirectory,
}: {
  followUpPrompt: string;
  inputArtifactPaths: string[];
  outputArtifactPaths: string[];
  workingDirectory: string;
}) {
  const sections = [
    followUpPrompt.trim(),
    "",
    buildPromptSection(
      "Artifacts To Review",
      outputArtifactPaths.length === 0
        ? ["- None"]
        : outputArtifactPaths.map((outputPath) => `- ${outputPath}`),
    ),
    buildPromptSection(
      "Related Input Artifacts",
      inputArtifactPaths.length === 0
        ? ["- None"]
        : inputArtifactPaths.map((inputPath) => `- ${inputPath}`),
    ),
    buildPromptSection("Execution Context", [`- Working directory: ${workingDirectory}`]),
    "Revise the current artifact according to the follow-up prompt and return the updated final result in Markdown.",
  ];

  return sections.join("\n");
}

async function assertProjectMembership(
  adminClient: SupabaseClient,
  projectId: string,
  _email: string | null,
) {
  const { data, error } = await adminClient
    .from("projects")
    .select("id")
    .eq("id", projectId)
    .maybeSingle();

  if (error) {
    throw new Error(`Unable to verify project: ${error.message}`);
  }
  if (!data) {
    throw new Error("Project not found");
  }
}

async function loadWorkflowDefinition(
  adminClient: SupabaseClient,
  workflowId: string,
  projectId: string,
) {
  const { data, error } = await adminClient
    .from("workflows")
    .select("id, name, project_id, provider_override, model_override, reasoning_effort_override")
    .eq("id", workflowId)
    .or(`project_id.is.null,project_id.eq.${projectId}`)
    .maybeSingle();

  if (error) {
    throw new Error(`Unable to load workflow definition: ${error.message}`);
  }
  if (!data) {
    throw new Error("Workflow definition not found for this project.");
  }

  return data as WorkflowDefinitionRow;
}

async function loadWorkflowRun(adminClient: SupabaseClient, runId: string) {
  const { data, error } = await adminClient
    .from("workflow_runs")
    .select("id, workflow_id, project_id")
    .eq("id", runId)
    .maybeSingle();

  if (error) {
    throw new Error(`Unable to load workflow run: ${error.message}`);
  }
  if (!data) {
    throw new Error("Workflow run not found.");
  }

  return data as WorkflowRunRow;
}

async function loadWorkflowRunStep(adminClient: SupabaseClient, stepRunId: string) {
  const { data, error } = await adminClient
    .from("workflow_run_steps")
    .select("id, workflow_run_id, workflow_step_id, execution_order_index, step_type, status, retry_count")
    .eq("id", stepRunId)
    .maybeSingle();

  if (error) {
    throw new Error(`Unable to load workflow run step: ${error.message}`);
  }
  if (!data) {
    throw new Error("Workflow step run not found.");
  }

  return data as WorkflowRunStepExecutionRow;
}

async function loadWorkflowRunSteps(adminClient: SupabaseClient, runId: string) {
  const { data, error } = await adminClient
    .from("workflow_run_steps")
    .select("id, workflow_run_id, workflow_step_id, execution_order_index, step_type, status, retry_count")
    .eq("workflow_run_id", runId)
    .order("execution_order_index", { ascending: true });

  if (error) {
    throw new Error(`Unable to load workflow run steps: ${error.message}`);
  }

  return (data ?? []) as WorkflowRunStepExecutionRow[];
}

async function loadWorkflowSteps(adminClient: SupabaseClient, workflowId: string) {
  const { data, error } = await adminClient
    .from("workflow_steps")
    .select("id, step_type, order_index, is_enabled, provider_override, model_override, reasoning_effort_override, requires_approval")
    .eq("workflow_id", workflowId)
    .order("order_index", { ascending: true });

  if (error) {
    throw new Error(`Unable to load workflow steps: ${error.message}`);
  }

  return (data ?? []) as WorkflowStepRow[];
}

async function loadWorkflowStepById(adminClient: SupabaseClient, workflowStepId: string) {
  const { data, error } = await adminClient
    .from("workflow_steps")
    .select("id, step_type, order_index, is_enabled, provider_override, model_override, reasoning_effort_override, requires_approval")
    .eq("id", workflowStepId)
    .maybeSingle();

  if (error) {
    throw new Error(`Unable to load workflow step definition: ${error.message}`);
  }
  if (!data) {
    throw new Error("Workflow step definition not found.");
  }

  return data as WorkflowStepRow;
}

async function loadStepDefinitions(adminClient: SupabaseClient, stepTypes: string[]) {
  const { data, error } = await adminClient
    .from("step_definitions")
    .select("step_type, name, description, prompt_base, required_mcps, required_skills, team_role, subagent, model, reasoning_effort")
    .in("step_type", stepTypes);

  if (error) {
    throw new Error(`Unable to load step definitions: ${error.message}`);
  }

  return new Map(((data ?? []) as StepDefinitionRow[]).map((row) => [row.step_type, row]));
}

async function createSingleStepWorkflow(
  adminClient: SupabaseClient,
  {
    projectId,
    stepType,
  }: {
    projectId: string;
    stepType: string;
  },
) {
  const stepDefinitions = await loadStepDefinitions(adminClient, [stepType]);
  const definition = stepDefinitions.get(stepType);
  if (!definition) {
    throw new Error(`Step definition "${stepType}" could not be loaded.`);
  }

  const { data: workflowData, error: workflowError } = await adminClient
    .from("workflows")
    .insert({
      project_id: projectId,
      name: `Single Step: ${definition.name}`,
      description: `Runtime-generated single-step workflow for ${stepType}.`,
      is_template: false,
      provider_override: resolveProviderKeyFromModel(definition.model ?? DEFAULT_MODEL),
      model_override: definition.model ?? DEFAULT_MODEL,
      reasoning_effort_override: definition.reasoning_effort ?? DEFAULT_REASONING_EFFORT,
      created_by: SINGLE_STEP_RUNTIME_CREATED_BY,
    })
    .select("id, name, project_id, provider_override, model_override, reasoning_effort_override")
    .single();

  if (workflowError) {
    throw new Error(`Unable to create single-step workflow: ${workflowError.message}`);
  }

  const { data: workflowStepData, error: workflowStepError } = await adminClient
    .from("workflow_steps")
    .insert({
      workflow_id: workflowData.id,
      step_type: stepType,
      order_index: 0,
      is_enabled: true,
      provider_override: resolveProviderKeyFromModel(definition.model ?? DEFAULT_MODEL),
      model_override: definition.model ?? DEFAULT_MODEL,
      reasoning_effort_override: definition.reasoning_effort ?? DEFAULT_REASONING_EFFORT,
      requires_approval: false,
    })
    .select("id, step_type, order_index, is_enabled, provider_override, model_override, reasoning_effort_override, requires_approval")
    .single();

  if (workflowStepError) {
    throw new Error(`Unable to create single-step workflow step: ${workflowStepError.message}`);
  }

  return {
    workflow: workflowData as WorkflowDefinitionRow,
    workflowSteps: [workflowStepData as WorkflowStepRow],
  };
}

async function loadArtifactBindings(
  adminClient: SupabaseClient,
  table: "step_input_artifact_definitions" | "step_output_artifact_definitions",
  stepTypes: string[],
) {
  const { data, error } = await adminClient
    .from(table)
    .select("step_type, artifact_definition_key, order_index")
    .in("step_type", stepTypes)
    .order("order_index", { ascending: true });

  if (error) {
    throw new Error(`Unable to load ${table}: ${error.message}`);
  }

  const bindings = new Map<string, string[]>();
  for (const row of data ?? []) {
    const stepType = String(row.step_type);
    const current = bindings.get(stepType) ?? [];
    current.push(String(row.artifact_definition_key));
    bindings.set(stepType, current);
  }
  return bindings;
}

async function loadArtifactDefinitions(adminClient: SupabaseClient, keys: string[]) {
  if (keys.length === 0) {
    return new Map<string, ArtifactDefinitionRow>();
  }

  const { data, error } = await adminClient
    .from("artifact_definitions")
    .select("key, name, local_path_template, remote_path_template, default_file_name")
    .in("key", keys);

  if (error) {
    throw new Error(`Unable to load artifact definitions: ${error.message}`);
  }

  return new Map(((data ?? []) as ArtifactDefinitionRow[]).map((row) => [row.key, row]));
}

async function listExistingArtifacts(
  adminClient: SupabaseClient,
  workflowRunId: string,
  artifactKeys: string[],
) {
  if (artifactKeys.length === 0) {
    return [];
  }

  const { data, error } = await adminClient
    .from("artifact_runs")
    .select("id, artifact_definition_key, local_path, title")
    .eq("workflow_run_id", workflowRunId)
    .in("artifact_definition_key", artifactKeys);

  if (error) {
    throw new Error(`Unable to load input artifacts: ${error.message}`);
  }

  return (data ?? []) as ArtifactRunRow[];
}

function resolvePlannedStepExecution(
  step: WorkflowStepRow,
  workflow: WorkflowDefinitionRow,
  projectDefaults: ProjectSettingsRow,
  definition: StepDefinitionRow,
) {
  const resolvedReasoningEffort =
    step.reasoning_effort_override ||
    workflow.reasoning_effort_override ||
    projectDefaults.default_reasoning_effort ||
    DEFAULT_REASONING_EFFORT;
  if (resolvedReasoningEffort && !SUPPORTED_REASONING_EFFORTS.has(resolvedReasoningEffort)) {
    throw new Error(`Step ${definition.step_type} is configured with an unsupported reasoning effort.`);
  }
  const resolvedModel =
    step.model_override ||
    workflow.model_override ||
    projectDefaults.default_model ||
    definition.model ||
    DEFAULT_MODEL;

  if (!resolvedModel || !isSupportedStepModel(resolvedModel)) {
    throw new Error(`Step ${definition.step_type} is configured with an unsupported model.`);
  }

  const resolvedProvider = resolveProviderKeyFromModel(resolvedModel);

  return {
    model: resolvedModel,
    providerKey: resolvedProvider,
    reasoningEffort: resolvedReasoningEffort,
  };
}

async function createArtifactOutputs({
  adminClient,
  artifactDefinitions,
  outputArtifactKeys,
  outputMarkdown,
  promptText,
  projectId,
  stepRunId,
  stepType,
  stderrText,
  stdoutText,
  workflowId,
  workflowRunId,
  workingDirectory,
  commandText,
  providerKey,
}: {
  adminClient: SupabaseClient;
  artifactDefinitions: Map<string, ArtifactDefinitionRow>;
  outputArtifactKeys: string[];
  outputMarkdown: string;
  promptText: string;
  projectId: string;
  stepRunId: string;
  stepType: string;
  stderrText: string;
  stdoutText: string;
  workflowId: string;
  workflowRunId: string;
  workingDirectory: string;
  commandText: string;
  providerKey: string;
}) {
  if (outputArtifactKeys.length === 0) {
    return null;
  }

  const now = new Date().toISOString();
  const rows = [];
  for (const artifactKey of outputArtifactKeys) {
    const definition = artifactDefinitions.get(artifactKey);
    if (!definition) {
      throw new Error(`Artifact definition "${artifactKey}" is missing.`);
    }

    const context = {
      projectId,
      workflowId,
      workflowRunId,
      workflowRunStepId: stepRunId,
      stepType,
      artifactKey,
      defaultFileName: definition.default_file_name || definition.name,
    };

    const localPath = resolveArtifactPath(definition.local_path_template, context);
    const absoluteOutputPath = normalizeAbsolutePath(workingDirectory, localPath);
    const artifactDirectory = path.dirname(absoluteOutputPath);
    const artifactId = randomUUID();
    const snapshotDirectory = path.join(artifactDirectory, ".snapshots", artifactId);
    await mkdir(path.dirname(absoluteOutputPath), { recursive: true });
    await mkdir(snapshotDirectory, { recursive: true });
    await writeFile(absoluteOutputPath, outputMarkdown, "utf8");

    const absoluteSnapshotContentPath = path.join(
      snapshotDirectory,
      path.basename(absoluteOutputPath),
    );
    const absolutePromptPath = path.join(snapshotDirectory, "prompt.md");
    const absoluteStdoutPath = path.join(snapshotDirectory, "stdout.txt");
    const absoluteStderrPath = path.join(snapshotDirectory, "stderr.txt");
    const absoluteCommandPath = path.join(snapshotDirectory, "command.txt");
    await writeFile(absoluteSnapshotContentPath, outputMarkdown, "utf8");
    await writeFile(absolutePromptPath, promptText, "utf8");
    await writeFile(absoluteStdoutPath, stdoutText, "utf8");
    await writeFile(absoluteStderrPath, stderrText, "utf8");
    await writeFile(absoluteCommandPath, commandText, "utf8");

    const manifest = {
      artifactId,
      title: definition.default_file_name || definition.name,
      sourceKind: "workflow_output",
      projectId,
      featureId: stepType,
      workflowRunId,
      workflowStepKey: stepType,
      providerKey,
      localPath: snapshotDirectory,
      remotePath: resolveArtifactPath(definition.remote_path_template, context),
      remoteUrl: "",
      syncStatus: "local_only",
      createdAt: now,
      updatedAt: now,
      promptPath: absolutePromptPath,
      stdoutPath: absoluteStdoutPath,
      stderrPath: absoluteStderrPath,
      commandPath: absoluteCommandPath,
      contentPath: absoluteSnapshotContentPath,
    };

    const absoluteManifestPath = path.join(snapshotDirectory, "manifest.json");
    await writeFile(absoluteManifestPath, JSON.stringify(manifest, null, 2), "utf8");

    rows.push({
      id: artifactId,
      artifact_definition_key: artifactKey,
      project_id: projectId,
      workflow_id: workflowId,
      workflow_run_id: workflowRunId,
      workflow_run_step_id: stepRunId,
      title: definition.default_file_name || definition.name,
      local_path: localPath,
      remote_path: resolveArtifactPath(definition.remote_path_template, context),
      remote_url: "",
      sync_status: "local_only",
      created_at: now,
      updated_at: now,
    });
  }

  const { data, error } = await adminClient.from("artifact_runs").insert(rows).select("id");
  if (error) {
    throw new Error(`Unable to create artifact outputs: ${error.message}`);
  }

  return data?.[0]?.id ? String(data[0].id) : null;
}

export async function submitWorkflowStepFollowUpRuntime({
  adminClient,
  localRunnerGateway,
  stepId,
  comment,
}: {
  adminClient: SupabaseClient;
  localRunnerGateway: Pick<
    LocalRunnerGateway,
    "executePrompt" | "listMcpBackends" | "startSession" | "sendMessage" | "closeSession"
  >;
  stepId: string;
  comment: string;
}) {
  const trimmedComment = comment.trim();
  if (!trimmedComment) {
    throw new Error("Follow-up prompt is required.");
  }

  const stepRun = await loadWorkflowRunStep(adminClient, stepId);
  if (stepRun.status !== "DONE" && stepRun.status !== "WAITING_USER_APPROVAL") {
    throw new Error("Follow-up is only supported for completed steps.");
  }

  const allRunSteps = await loadWorkflowRunSteps(adminClient, stepRun.workflow_run_id);
  const latestExecutableStep = [...allRunSteps]
    .filter((step) => step.status !== "SKIPPED")
    .sort((left, right) => left.execution_order_index - right.execution_order_index)
    .at(-1);

  if (!latestExecutableStep || latestExecutableStep.id !== stepRun.id) {
    throw new Error("Follow-up is only supported for the latest workflow step in the run.");
  }

  if (!stepRun.workflow_step_id) {
    throw new Error("Workflow step definition is missing for this run step.");
  }

  const run = await loadWorkflowRun(adminClient, stepRun.workflow_run_id);
  const projectDefaults = await loadProjectDefaults(adminClient, run.project_id);
  const workflow = await loadWorkflowDefinition(adminClient, run.workflow_id, run.project_id);
  const workflowStep = await loadWorkflowStepById(adminClient, stepRun.workflow_step_id);
  const stepDefinitions = await loadStepDefinitions(adminClient, [stepRun.step_type]);
  const definition = stepDefinitions.get(stepRun.step_type);
  if (!definition) {
    throw new Error(`Step definition "${stepRun.step_type}" could not be loaded.`);
  }

  const inputBindings = await loadArtifactBindings(adminClient, "step_input_artifact_definitions", [stepRun.step_type]);
  const outputBindings = await loadArtifactBindings(adminClient, "step_output_artifact_definitions", [stepRun.step_type]);
  const inputArtifactKeys = inputBindings.get(stepRun.step_type) ?? [];
  const outputArtifactKeys = outputBindings.get(stepRun.step_type) ?? [];
  const artifactDefinitionKeys = Array.from(new Set([...inputArtifactKeys, ...outputArtifactKeys]));
  const artifactDefinitions = await loadArtifactDefinitions(adminClient, artifactDefinitionKeys);
  const workingDirectory = await resolveWorkingDirectory(adminClient, run.project_id);
  const installedBackends = await localRunnerGateway.listMcpBackends();
  const usableMcpKeys = new Set(
    installedBackends
      .filter((backend) => backend.installed || backend.state === "installed" || backend.state === "launcher_available")
      .flatMap((backend) => [backend.key, backend.providerType])
      .map((value) => value.toLowerCase()),
  );

  const missingMcps = (definition.required_mcps ?? []).filter(
    (mcp) => !usableMcpKeys.has(String(mcp).toLowerCase()),
  );
  if (missingMcps.length > 0) {
    throw new Error(`Missing required MCPs for ${stepRun.step_type}: ${missingMcps.join(", ")}`);
  }

  const existingArtifacts = await listExistingArtifacts(
    adminClient,
    run.id,
    artifactDefinitionKeys,
  );
  const artifactsByKey = new Map(existingArtifacts.map((artifact) => [artifact.artifact_definition_key, artifact]));
  const missingArtifacts = inputArtifactKeys.filter((key) => !artifactsByKey.has(key));
  if (missingArtifacts.length > 0) {
    throw new Error(
      `Step ${stepRun.step_type} is missing required input artifacts: ${missingArtifacts.join(", ")}`,
    );
  }

  const inputArtifactPaths = inputArtifactKeys.map((artifactKey) =>
    normalizeAbsolutePath(workingDirectory, artifactsByKey.get(artifactKey)!.local_path),
  );
  const outputArtifactPaths = outputArtifactKeys.map((artifactKey) => {
    const definitionRow = artifactDefinitions.get(artifactKey);
    if (!definitionRow) {
      throw new Error(`Artifact definition "${artifactKey}" is missing.`);
    }

    return normalizeAbsolutePath(
      workingDirectory,
      resolveArtifactPath(definitionRow.local_path_template, {
        projectId: run.project_id,
        workflowId: run.workflow_id,
        workflowRunId: run.id,
        workflowRunStepId: stepRun.id,
        stepType: stepRun.step_type,
        artifactKey,
        defaultFileName: definitionRow.default_file_name || definitionRow.name,
      }),
    );
  });

  const execution = resolvePlannedStepExecution(workflowStep, workflow, projectDefaults, definition);
  const finalPrompt = buildWorkflowStepFollowUpPrompt({
    followUpPrompt: trimmedComment,
    inputArtifactPaths,
    outputArtifactPaths,
    workingDirectory,
  });

  const decisionTimestamp = new Date().toISOString();
  await adminClient
    .from("workflow_runs")
    .update({
      status: "RUNNING",
      finished_at: null,
      error_message: null,
    })
    .eq("id", run.id);

  await adminClient
    .from("workflow_run_steps")
    .update({
      status: "RUNNING",
      retry_count: stepRun.retry_count + 1,
      rejection_note: trimmedComment,
      started_at: decisionTimestamp,
      finished_at: null,
      error_message: null,
    })
    .eq("id", stepRun.id);

  await insertLog(
    adminClient,
    stepRun.id,
    "info",
    `approval_decision:${JSON.stringify({
      id: randomUUID(),
      approvalId: `approval_${stepRun.id}`,
      workflowRunId: run.id,
      workflowStepId: stepRun.id,
      aiOutputId: null,
      decision: "changes_requested",
      reviewerId: null,
      comment: trimmedComment,
      createdAt: decisionTimestamp,
    })}`,
  );
  await insertLog(adminClient, stepRun.id, "info", `Follow-up prompt: ${trimmedComment}`);
  await insertLog(adminClient, stepRun.id, "info", `Launching follow-up for ${stepRun.step_type} in ${workingDirectory}.`);

  try {
    const result = await sendMessageWithRetry({
      adminClient,
      localRunnerGateway: localRunnerGateway as any,
      workflowRunId: run.id,
      stepRunId: stepRun.id,
      providerKey: execution.providerKey,
      modelName: execution.model,
      reasoningEffort: execution.reasoningEffort ?? null,
      workingDirectory,
      subagent: definition.subagent ?? null,
      prompt: finalPrompt,
      skillIds: definition.required_skills ?? [],
    });

    if (result.status !== "success") {
      throw new Error(result.errorMessage || `Local runner execution failed for ${stepRun.step_type}.`);
    }

    const artifactRunId = await createArtifactOutputs({
      adminClient,
      artifactDefinitions,
      outputArtifactKeys,
      outputMarkdown: result.outputMarkdown,
      promptText: finalPrompt,
      projectId: run.project_id,
      stepRunId: stepRun.id,
      stepType: stepRun.step_type,
      stderrText: result.stderrSummary,
      stdoutText: result.stdoutSummary,
      workflowId: run.workflow_id,
      workflowRunId: run.id,
      workingDirectory,
      commandText: result.command,
      providerKey: execution.providerKey,
    });

    await adminClient
      .from("workflow_run_steps")
      .update({
        status: "DONE",
        started_at: result.startedAt,
        finished_at: result.completedAt,
        artifact_run_id: artifactRunId,
        rejection_note: null,
        error_message: null,
      })
      .eq("id", stepRun.id);
    await insertLog(adminClient, stepRun.id, "info", `Command: ${result.command}`);
    if (result.stdoutSummary) {
      await insertLog(adminClient, stepRun.id, "debug", result.stdoutSummary);
    }
    if (result.stderrSummary) {
      await insertLog(adminClient, stepRun.id, "warn", result.stderrSummary);
    }
    await insertLog(adminClient, stepRun.id, "info", `Completed follow-up for ${stepRun.step_type} with model ${execution.model}.`);

    await adminClient
      .from("workflow_runs")
      .update({
        status: "DONE",
        finished_at: new Date().toISOString(),
        error_message: null,
      })
      .eq("id", run.id);
  } catch (error) {
    const message = error instanceof Error ? error.message : `Follow-up execution failed for ${stepRun.step_type}.`;
    await adminClient
      .from("workflow_run_steps")
      .update({
        status: "FAILED",
        finished_at: new Date().toISOString(),
        error_message: message,
      })
      .eq("id", stepRun.id);
    await insertLog(adminClient, stepRun.id, "error", message);
    await adminClient
      .from("workflow_runs")
      .update({
        status: "FAILED",
        finished_at: new Date().toISOString(),
        error_message: message,
      })
      .eq("id", run.id);
    throw error;
  }

  const { data: updatedStep, error: updatedStepError } = await adminClient
    .from("workflow_run_steps")
    .select("*")
    .eq("id", stepRun.id)
    .single();

  if (updatedStepError) {
    throw new Error(`Unable to load updated workflow step: ${updatedStepError.message}`);
  }

  return updatedStep;
}

export async function runWorkflowStartRuntime({
  adminClient,
  localRunnerGateway,
  request,
  user,
}: {
  adminClient: SupabaseClient;
  localRunnerGateway: Pick<
    LocalRunnerGateway,
    "executePrompt" | "listMcpBackends" | "startSession" | "sendMessage" | "closeSession"
  >;
  request: WorkflowRunStartRequest;
  user: { id: string; email: string | null };
}) {
  await assertProjectMembership(adminClient, request.projectId, user.email);
  const projectDefaults = await loadProjectDefaults(adminClient, request.projectId);
  const executionSource =
    request.startMode === "single-step"
      ? await createSingleStepWorkflow(adminClient, {
          projectId: request.projectId,
          stepType: request.stepType,
        })
      : {
          workflow: await loadWorkflowDefinition(adminClient, request.workflowId, request.projectId),
          workflowSteps: await loadWorkflowSteps(adminClient, request.workflowId),
        };
  const { workflow, workflowSteps } = executionSource;
  if (workflowSteps.length === 0) {
    throw new Error("This workflow has no steps to execute.");
  }

  const stepTypes = Array.from(new Set(workflowSteps.map((step) => step.step_type)));
  const stepDefinitions = await loadStepDefinitions(adminClient, stepTypes);
  const inputBindings = await loadArtifactBindings(adminClient, "step_input_artifact_definitions", stepTypes);
  const outputBindings = await loadArtifactBindings(adminClient, "step_output_artifact_definitions", stepTypes);
  const artifactDefinitionKeys = Array.from(
    new Set(
      [...inputBindings.values(), ...outputBindings.values()].flatMap((keys) => keys),
    ),
  );
  const artifactDefinitions = await loadArtifactDefinitions(adminClient, artifactDefinitionKeys);
  const workingDirectory = await resolveWorkingDirectory(adminClient, request.projectId);
  const installedBackends = await localRunnerGateway.listMcpBackends();
  const usableMcpKeys = new Set(
    installedBackends
      .filter((backend) => backend.installed || backend.state === "installed" || backend.state === "launcher_available")
      .flatMap((backend) => [backend.key, backend.providerType])
      .map((value) => value.toLowerCase()),
  );

  const stepPlans = workflowSteps
    .filter((step) => step.is_enabled)
    .map((step) => {
      const definition = stepDefinitions.get(step.step_type);
      if (!definition) {
        throw new Error(`Step definition "${step.step_type}" could not be loaded.`);
      }

      return {
        workflowStepId: step.id,
        stepType: step.step_type,
        orderIndex: step.order_index,
        definition,
        ...resolvePlannedStepExecution(step, workflow, projectDefaults, definition),
        inputArtifactKeys: inputBindings.get(step.step_type) ?? [],
        outputArtifactKeys: outputBindings.get(step.step_type) ?? [],
      } satisfies StepExecutionPlan;
    });

  const firstStepModel =
    stepPlans[0]?.model || workflow.model_override || projectDefaults.default_model || DEFAULT_MODEL;
  const firstStepProvider = resolveProviderKeyFromModel(firstStepModel);
  const firstStepReasoningEffort =
    stepPlans[0]?.reasoningEffort ||
    workflow.reasoning_effort_override ||
    projectDefaults.default_reasoning_effort ||
    DEFAULT_REASONING_EFFORT;
  const { data: runRow, error: runError } = await adminClient
    .from("workflow_runs")
    .insert({
      workflow_id: workflow.id,
      project_id: request.projectId,
      status: "RUNNING",
      provider: firstStepProvider,
      model: firstStepModel,
      reasoning_effort: firstStepReasoningEffort,
      yolo_mode: false,
      started_by: user.email ?? user.id,
    })
    .select("*")
    .single();

  if (runError) {
    throw new Error(`Unable to create workflow run: ${runError.message}`);
  }

  const { data: insertedSteps, error: stepInsertError } = await adminClient
    .from("workflow_run_steps")
    .insert(
      workflowSteps.map((step) => ({
        workflow_run_id: runRow.id,
        workflow_step_id: step.id,
        execution_order_index: step.order_index,
        step_type: step.step_type,
        status: step.is_enabled ? "PENDING" : "SKIPPED",
        retry_count: 0,
      })),
    )
    .select("id, workflow_step_id, step_type, status");

  if (stepInsertError) {
    throw new Error(`Unable to create workflow run steps: ${stepInsertError.message}`);
  }

  const stepRunIds = new Map(
    (insertedSteps ?? []).map((row) => [String(row.workflow_step_id), String(row.id)]),
  );

  let activeStepRunId: string | null = null;

  try {
    for (const stepPlan of stepPlans) {
      const stepRunId = stepRunIds.get(stepPlan.workflowStepId);
      if (!stepRunId) {
        throw new Error(`Workflow run step for ${stepPlan.stepType} was not created.`);
      }
      activeStepRunId = stepRunId;

      const missingMcps = (stepPlan.definition.required_mcps ?? []).filter(
        (mcp) => !usableMcpKeys.has(String(mcp).toLowerCase()),
      );
      if (missingMcps.length > 0) {
        await adminClient
          .from("workflow_run_steps")
          .update({
            status: "FAILED",
            started_at: new Date().toISOString(),
            finished_at: new Date().toISOString(),
            error_message: `Missing MCPs: ${missingMcps.join(", ")}`,
          })
          .eq("id", stepRunId);
        await insertLog(
          adminClient,
          stepRunId,
          "error",
          `Execution stopped before provider launch. Missing MCPs: ${missingMcps.join(", ")}.`,
        );
        throw new Error(`Missing required MCPs for ${stepPlan.stepType}: ${missingMcps.join(", ")}`);
      }

      const existingArtifacts = await listExistingArtifacts(
        adminClient,
        String(runRow.id),
        stepPlan.inputArtifactKeys,
      );
      const artifactsByKey = new Map(existingArtifacts.map((artifact) => [artifact.artifact_definition_key, artifact]));
      const missingArtifacts = stepPlan.inputArtifactKeys.filter((key) => !artifactsByKey.has(key));
      if (missingArtifacts.length > 0) {
        throw new Error(
          `Step ${stepPlan.stepType} is missing required input artifacts: ${missingArtifacts.join(", ")}`,
        );
      }

      const inputArtifactPaths = stepPlan.inputArtifactKeys.map((artifactKey) =>
        normalizeAbsolutePath(workingDirectory, artifactsByKey.get(artifactKey)!.local_path),
      );
      const outputArtifactPaths = stepPlan.outputArtifactKeys.map((artifactKey) => {
        const definition = artifactDefinitions.get(artifactKey);
        if (!definition) {
          throw new Error(`Artifact definition "${artifactKey}" is missing.`);
        }
        return normalizeAbsolutePath(
          workingDirectory,
          resolveArtifactPath(definition.local_path_template, {
            projectId: request.projectId,
            workflowId: workflow.id,
            workflowRunId: String(runRow.id),
            workflowRunStepId: stepRunId,
            stepType: stepPlan.stepType,
            artifactKey,
            defaultFileName: definition.default_file_name || definition.name,
          }),
        );
      });

      const promptBase = stepPlan.definition.prompt_base?.trim()
        ? stepPlan.definition.prompt_base.trim()
        : deriveStepPromptBase({
            stepType: stepPlan.definition.step_type as StepType,
            name: stepPlan.definition.name,
            description: stepPlan.definition.description,
          });
      const finalPrompt = buildWorkflowStepPrompt({
        beginPrompt: request.beginPrompt,
        inputArtifactPaths,
        model: stepPlan.model,
        outputArtifactPaths,
        promptBase,
        stepType: stepPlan.stepType,
        subagent: stepPlan.definition.subagent,
        teamRole: stepPlan.definition.team_role,
        workingDirectory,
        requiredSkills: stepPlan.definition.required_skills ?? [],
      });

      await adminClient
        .from("workflow_run_steps")
        .update({
          status: "RUNNING",
          started_at: new Date().toISOString(),
          error_message: null,
        })
        .eq("id", stepRunId);
      await insertLog(adminClient, stepRunId, "info", `Launching ${stepPlan.stepType} in ${workingDirectory}.`);
      await insertLog(adminClient, stepRunId, "info", `Begin prompt: ${request.beginPrompt}`);

      const result = await sendMessageWithRetry({
        adminClient,
        localRunnerGateway: localRunnerGateway as any,
        workflowRunId: String(runRow.id),
        stepRunId,
        providerKey: stepPlan.providerKey,
        modelName: stepPlan.model,
        reasoningEffort: stepPlan.reasoningEffort ?? null,
        workingDirectory,
        subagent: stepPlan.definition.subagent ?? null,
        prompt: finalPrompt,
        skillIds: stepPlan.definition.required_skills ?? [],
      });

      if (result.status !== "success") {
        throw new Error(result.errorMessage || `Local runner execution failed for ${stepPlan.stepType}.`);
      }

      const artifactRunId = await createArtifactOutputs({
        adminClient,
        artifactDefinitions,
        outputArtifactKeys: stepPlan.outputArtifactKeys,
        outputMarkdown: result.outputMarkdown,
        promptText: finalPrompt,
        projectId: request.projectId,
        stepRunId,
        stepType: stepPlan.stepType,
        stderrText: result.stderrSummary,
        stdoutText: result.stdoutSummary,
        workflowId: workflow.id,
        workflowRunId: String(runRow.id),
        workingDirectory,
        commandText: result.command,
        providerKey: stepPlan.providerKey,
      });

      await adminClient
        .from("workflow_run_steps")
        .update({
          status: "DONE",
          started_at: result.startedAt,
          finished_at: result.completedAt,
          artifact_run_id: artifactRunId,
          error_message: null,
        })
        .eq("id", stepRunId);
      await insertLog(adminClient, stepRunId, "info", `Command: ${result.command}`);
      if (result.stdoutSummary) {
        await insertLog(adminClient, stepRunId, "debug", result.stdoutSummary);
      }
      if (result.stderrSummary) {
        await insertLog(adminClient, stepRunId, "warn", result.stderrSummary);
      }
      await insertLog(adminClient, stepRunId, "info", `Completed ${stepPlan.stepType} with model ${stepPlan.model}.`);
    }

    const finishedAt = new Date().toISOString();
    const { data: completedRun, error: completedRunError } = await adminClient
      .from("workflow_runs")
      .update({
        status: "DONE",
        finished_at: finishedAt,
        error_message: null,
      })
      .eq("id", runRow.id)
      .select("*")
      .single();

    if (completedRunError) {
      throw new Error(`Unable to finalize workflow run: ${completedRunError.message}`);
    }

    return completedRun;
  } catch (error) {
    const message = error instanceof Error ? error.message : "Workflow execution failed.";
    if (activeStepRunId) {
      await adminClient
        .from("workflow_run_steps")
        .update({
          status: "FAILED",
          finished_at: new Date().toISOString(),
          error_message: message,
        })
        .eq("id", activeStepRunId);
      await insertLog(adminClient, activeStepRunId, "error", message);
    }
    await adminClient
      .from("workflow_runs")
      .update({
        status: "FAILED",
        finished_at: new Date().toISOString(),
        error_message: message,
      })
      .eq("id", runRow.id);
    throw error;
  }
}
