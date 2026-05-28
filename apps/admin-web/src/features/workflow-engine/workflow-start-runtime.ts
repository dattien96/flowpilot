import { randomUUID } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

import type { SupabaseClient } from "@supabase/supabase-js";

import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type { LocalRunnerPromptExecutionResult } from "@/domain/model/entity/local-runner";
import {
  deriveStepPromptBase,
  isSupportedStepModel,
  REASONING_EFFORT_OPTIONS,
  type StepType,
  type WorkflowRunStartRequest,
} from "@/domain/model/entity/workflow-engine";
import {
  buildResultSummaryPrompt,
  RESULT_SUMMARY_STEP_NAME,
  RESULT_SUMMARY_STEP_TYPE,
  shouldAppendResultSummaryStep,
  type ResultSummarySourceStep,
} from "@/features/workflow-engine/workflow-result-summary";
import {
  INTERRUPTED_RUN_ERROR,
  isInterruptedWorkflowStep,
} from "@/features/workflow-engine/workflow-run-interruption";
import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";

type WorkflowDefinitionRow = {
  id: string;
  name: string;
  project_id: string | null;
  provider_override: string | null;
  model_override: string | null;
  reasoning_effort_override: string | null;
  session_idle_ttl_minutes?: number | null;
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
  error_message?: string | null;
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
  session_idle_ttl_minutes: number | null;
};

type StepExecutionPlan = {
  planKey: string;
  workflowStepId: string | null;
  stepType: string;
  orderIndex: number;
  definition: StepDefinitionRow;
  providerKey: string;
  model: string;
  reasoningEffort: string | null;
  inputArtifactKeys: string[];
  outputArtifactKeys: string[];
};

type WorkflowSessionRecoveryMode = "resumed_thread" | "bootstrap_replay";

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
  idleTTLSeconds,
  resumeProviderSessionId,
  forceNewProviderSession,
  recoveryMode,
  recoveredFromSessionId,
  recoveredFromProviderSessionId,
  replayCheckpointCount,
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
  idleTTLSeconds?: number | null;
  resumeProviderSessionId?: string | null;
  forceNewProviderSession?: boolean;
  recoveryMode?: WorkflowSessionRecoveryMode | null;
  recoveredFromSessionId?: string | null;
  recoveredFromProviderSessionId?: string | null;
  replayCheckpointCount?: number | null;
}) {
  let sessionRow = null;

  if (!subagent) {
    const { data, error } = await adminClient
      .from("workflow_run_sessions")
      .select("*")
      .eq("workflow_run_id", workflowRunId)
      .filter("metadata_json->>is_main", "eq", "true")
      .order("started_at", { ascending: false })
      .limit(1)
      .maybeSingle();
    if (error) {
      throw new Error(`Failed to query main session: ${error.message}`);
    }
    sessionRow = data;
  } else {
    const { data, error } = await adminClient
      .from("workflow_run_sessions")
      .select("*")
      .eq("workflow_run_id", workflowRunId)
      .filter("metadata_json->>step_run_id", "eq", stepRunId)
      .order("started_at", { ascending: false })
      .limit(1)
      .maybeSingle();
    if (error) {
      throw new Error(`Failed to query isolated session: ${error.message}`);
    }
    sessionRow = data;
  }

  let previousCheckpoints: any[] = [];
  let resolvedRecoveryMode = recoveryMode ?? null;
  let resolvedRecoveredFromSessionId = recoveredFromSessionId ?? null;
  let resolvedRecoveredFromProviderSessionId =
    recoveredFromProviderSessionId ?? null;
  let resolvedReplayCheckpointCount = replayCheckpointCount ?? null;
  if (sessionRow) {
    if (sessionRow.status !== "active") {
      previousCheckpoints = sessionRow.metadata_json?.checkpoints || [];
      // It's a completed/terminated session. We can try to resume its provider thread.
      if (
        !forceNewProviderSession &&
        sessionRow.provider === providerKey &&
        sessionRow.model === modelName
      ) {
        resumeProviderSessionId = resumeProviderSessionId ?? sessionRow.provider_session_id;
        resolvedRecoveryMode = resolvedRecoveryMode ?? "resumed_thread";
        resolvedRecoveredFromSessionId =
          resolvedRecoveredFromSessionId ?? sessionRow.id;
        resolvedRecoveredFromProviderSessionId =
          resolvedRecoveredFromProviderSessionId ??
          sessionRow.provider_session_id;
        resolvedReplayCheckpointCount =
          resolvedReplayCheckpointCount ?? previousCheckpoints.length;
      }
      sessionRow = null;
    } else if (forceNewProviderSession || sessionRow.provider !== providerKey || sessionRow.model !== modelName) {
      previousCheckpoints = sessionRow.metadata_json?.checkpoints || [];
      await writeWorkflowSessionLog({
        adminClient,
        workflowRunStepId: stepRunId,
        logLevel: "warn",
        event: "session_replaced",
        details: {
          workflowRunId,
          stepRunId,
          sessionKind: subagent ? "subagent" : "main",
          provider: sessionRow.provider,
          model: sessionRow.model,
          requestedProvider: providerKey,
          requestedModel: modelName,
          providerSessionId: sessionRow.provider_session_id,
          processKey: sessionRow.process_key,
          reason: "provider_or_model_mismatch",
        },
      });
      if (sessionRow.process_key) {
        try {
          await localRunnerGateway.closeSession({
            transportType: sessionRow.transport_type,
            providerSessionId: sessionRow.provider_session_id,
            processKey: sessionRow.process_key,
          });
        } catch (closeErr) {
          console.error(`Failed to close incompatible session ${sessionRow.process_key}:`, closeErr);
        }
      }
      const { error: updateErr } = await adminClient
        .from("workflow_run_sessions")
        .update({
          status: "completed",
          completed_at: new Date().toISOString(),
        })
        .eq("id", sessionRow.id);
      if (updateErr) {
        throw new Error(`Failed to mark incompatible session as completed: ${updateErr.message}`);
      }
      sessionRow = null;
    }
  }

  let handle: WorkflowSessionHandle | null = null;

  if (sessionRow && sessionRow.process_key) {
    handle = {
      transportType: sessionRow.transport_type,
      providerSessionId: sessionRow.provider_session_id,
      processKey: sessionRow.process_key,
      processPid: sessionRow.process_pid ? Number(sessionRow.process_pid) : null,
      dbId: sessionRow.id,
    };
    await writeWorkflowSessionLog({
      adminClient,
      workflowRunStepId: stepRunId,
      logLevel: "info",
      event: "session_reused",
      details: {
        workflowRunId,
        stepRunId,
        sessionKind: subagent ? "subagent" : "main",
        provider: providerKey,
        model: modelName,
        transportType: sessionRow.transport_type,
        providerSessionId: sessionRow.provider_session_id,
        processKey: sessionRow.process_key,
      },
    });
  }

  if (!handle) {
    const startResult = await localRunnerGateway.startSession({
      providerKey,
      modelName,
      reasoningEffort: reasoningEffort as any,
      workingDirectory,
      approvalMode: null,
      allowWrite: true,
      idleTTLSeconds: idleTTLSeconds ?? undefined,
      resumeProviderSessionId: resumeProviderSessionId ?? undefined,
    });

    handle = startResult;

    try {
      if (sessionRow) {
        const { error: updateErr } = await adminClient
          .from("workflow_run_sessions")
          .update({
            process_key: handle.processKey,
            provider_session_id: handle.providerSessionId,
            transport_type: handle.transportType,
            provider: providerKey,
            model: modelName,
            process_pid: handle.processPid ?? null,
          })
          .eq("id", sessionRow.id);
        if (updateErr) {
          throw new Error(`Failed to update session in DB: ${updateErr.message}`);
        }
        (handle as WorkflowSessionHandle).dbId = sessionRow.id;
      } else {
        const metadata = !subagent ? { is_main: true } : { step_run_id: stepRunId };
        const recoveryMetadata = resolvedRecoveryMode
          ? {
              recovery: {
                mode: resolvedRecoveryMode,
                recoveredFromSessionId: resolvedRecoveredFromSessionId,
                recoveredFromProviderSessionId:
                  resolvedRecoveredFromProviderSessionId,
                replayCheckpointCount: resolvedReplayCheckpointCount,
              },
            }
          : {};
        const { data: newRow, error: newRowErr } = await adminClient
          .from("workflow_run_sessions")
          .insert({
            workflow_run_id: workflowRunId,
            provider: providerKey,
            model: modelName,
            transport_type: handle.transportType,
            provider_session_id: handle.providerSessionId,
            process_key: handle.processKey,
            process_pid: handle.processPid ?? null,
            status: "active",
            metadata_json: {
              ...metadata,
              checkpoints: previousCheckpoints,
              ...recoveryMetadata,
            },
          })
          .select("*")
          .single();
        if (newRowErr) {
          throw new Error(`Failed to save session to DB: ${newRowErr.message}`);
        }
        (handle as WorkflowSessionHandle).dbId = newRow.id;
      }
    } catch (dbErr) {
      if (handle.processKey) {
        await localRunnerGateway.closeSession({
          transportType: handle.transportType,
          providerSessionId: handle.providerSessionId,
          processKey: handle.processKey,
        }).catch(console.error);
      }
      throw dbErr;
    }

    await writeWorkflowSessionLog({
      adminClient,
      workflowRunStepId: stepRunId,
      logLevel: "info",
      event: "session_created",
      details: {
        workflowRunId,
        stepRunId,
        sessionKind: subagent ? "subagent" : "main",
        provider: providerKey,
        model: modelName,
        transportType: handle.transportType,
        providerSessionId: handle.providerSessionId,
        processKey: handle.processKey,
      },
    });
  }

  return handle;
}

type WorkflowSessionHandle = {
  transportType: string;
  providerSessionId: string;
  processKey: string | null;
  processPid?: number | null;
  dbId?: string;
};

async function updateWorkflowRunSessionById({
  adminClient,
  workflowRunId,
  stepRunId,
  subagent,
  updates,
  dbId,
}: {
  adminClient: SupabaseClient;
  workflowRunId: string;
  stepRunId: string;
  subagent: string | null;
  updates: Record<string, unknown>;
  dbId?: string;
}) {
  let query = adminClient.from("workflow_run_sessions").update(updates);
  if (dbId) {
    query = query.eq("id", dbId);
  } else {
    // Fallback for tests or legacy callers
    query = query.eq("workflow_run_id", workflowRunId);
    if (!subagent) {
      query = query.filter("metadata_json->>is_main", "eq", "true");
    } else {
      query = query.filter("metadata_json->>step_run_id", "eq", stepRunId);
    }
  }

  const { error } = await query;
  if (error) {
    throw new Error(`Failed to update workflow session state: ${error.message}`);
  }
}

export async function syncWorkflowRunSessionProviderSessionId({
  adminClient,
  workflowRunId,
  stepRunId,
  subagent,
  providerKey,
  modelName,
  handle,
  providerSessionId,
}: {
  adminClient: SupabaseClient;
  workflowRunId: string;
  stepRunId: string;
  subagent: string | null;
  providerKey: string;
  modelName: string;
  handle: WorkflowSessionHandle;
  providerSessionId: string | null | undefined;
}) {
  const nextProviderSessionId =
    providerSessionId && providerSessionId.trim().length > 0
      ? providerSessionId
      : handle.providerSessionId;

  try {
    await updateWorkflowRunSessionById({
      adminClient,
      workflowRunId,
      stepRunId,
      subagent,
      dbId: handle.dbId,
      updates: {
        provider: providerKey,
        model: modelName,
        transport_type: handle.transportType,
        provider_session_id: nextProviderSessionId,
        process_key: handle.processKey,
        process_pid: handle.processPid ?? null,
        status: "active",
      },
    });
  } catch (error) {
    console.error("Failed to persist provider session id for workflow session:", error);
  }
}

export async function deactivateWorkflowRunSession({
  adminClient,
  workflowRunId,
  stepRunId,
  subagent,
  localRunnerGateway,
  handle,
}: {
  adminClient: SupabaseClient;
  workflowRunId: string;
  stepRunId: string;
  subagent: string | null;
  localRunnerGateway: LocalRunnerGateway;
  handle: WorkflowSessionHandle | null;
}) {
  if (handle?.processKey) {
    try {
      await localRunnerGateway.closeSession({
        transportType: handle.transportType,
        providerSessionId: handle.providerSessionId,
        processKey: handle.processKey,
      });
    } catch (closeErr) {
      console.error(`Failed to close workflow session ${handle.processKey}:`, closeErr);
    }
  }

  await updateWorkflowRunSessionById({
    adminClient,
    workflowRunId,
    stepRunId,
    subagent,
    dbId: handle?.dbId,
    updates: {
      status: "completed",
      completed_at: new Date().toISOString(),
      process_key: null,
    },
  });

  await writeWorkflowSessionLog({
    adminClient,
    workflowRunStepId: stepRunId,
    logLevel: "info",
    event: "session_completed",
    details: {
      workflowRunId,
      stepRunId,
      sessionKind: subagent ? "subagent" : "main",
      providerSessionId: handle?.providerSessionId ?? null,
      processKey: handle?.processKey ?? null,
    },
  });
}

export async function finalizeWorkflowRunSessions(
  adminClient: SupabaseClient,
  localRunnerGateway: LocalRunnerGateway,
  workflowRunId: string,
  options?: {
    preserveMainSession?: boolean;
  },
) {
  try {
    const { data: sessions, error } = await adminClient
      .from("workflow_run_sessions")
      .select("*")
      .eq("workflow_run_id", workflowRunId)
      .eq("status", "active");

    if (error) {
      console.error(`Failed to fetch active sessions for cleanup: ${error.message}`);
      return;
    }

    if (!sessions || sessions.length === 0) {
      return;
    }

    for (const sessionRow of sessions) {
      const isMainSession =
        sessionRow.metadata_json?.is_main === true ||
        sessionRow.metadata_json?.is_main === "true";
      if (options?.preserveMainSession && isMainSession) {
        continue;
      }

      if (sessionRow.process_key) {
        try {
          await localRunnerGateway.closeSession({
            transportType: sessionRow.transport_type,
            providerSessionId: sessionRow.provider_session_id,
            processKey: sessionRow.process_key,
          });
        } catch (closeErr) {
          console.error(`Failed to close session ${sessionRow.process_key} in runner:`, closeErr);
        }
      }

      const { error: updateErr } = await adminClient
        .from("workflow_run_sessions")
        .update({
          status: "completed",
          completed_at: new Date().toISOString(),
          provider_session_id: null,
          process_key: null,
        })
        .eq("id", sessionRow.id);
      if (updateErr) {
        console.error(`Failed to update session status to completed: ${updateErr.message}`);
      }
    }
  } catch (err) {
    console.error("Error finalizing workflow run sessions:", err);
  }
}

async function appendSessionCheckpoint(
  adminClient: SupabaseClient,
  dbId: string,
  checkpoint: {
    promptPath: string;
    outputContentPath: string;
    artifactOutputPaths?: string[];
  },
) {
  const { data, error } = await adminClient
    .from("workflow_run_sessions")
    .select("metadata_json")
    .eq("id", dbId)
    .single();
  if (error) {
    throw new Error(`Failed to read session checkpoints: ${error.message}`);
  }

  const metadata = data?.metadata_json || {};
  const checkpoints = Array.isArray(metadata.checkpoints) ? metadata.checkpoints : [];
  checkpoints.push({
    promptPath: checkpoint.promptPath,
    outputContentPath: checkpoint.outputContentPath,
    artifactOutputPaths: (checkpoint.artifactOutputPaths ?? []).filter(
      (value) => value.trim().length > 0,
    ),
  });

  const { error: updateError } = await adminClient
    .from("workflow_run_sessions")
    .update({ metadata_json: { ...metadata, checkpoints } })
    .eq("id", dbId);
  if (updateError) {
    throw new Error(`Failed to write session checkpoints: ${updateError.message}`);
  }
}

function buildBootstrapPrompt(
  checkpoints: Array<{
    prompt?: string;
    output?: string;
    artifactPaths?: string[];
    promptPath?: string;
    outputContentPath?: string;
    artifactOutputPaths?: string[];
  }>,
  newPrompt: string,
) {
  if (!checkpoints || checkpoints.length === 0) return newPrompt;
  const recentCheckpoints = checkpoints.slice(-5);
  const visibleCheckpoints = recentCheckpoints.filter((checkpoint) => {
    const promptPath = checkpoint.promptPath?.trim() ?? "";
    const outputContentPath = checkpoint.outputContentPath?.trim() ?? "";
    const artifactOutputPaths = Array.isArray(checkpoint.artifactOutputPaths)
      ? checkpoint.artifactOutputPaths.filter(
          (value) => typeof value === "string" && value.trim().length > 0,
        )
      : [];

    return Boolean(promptPath || outputContentPath || artifactOutputPaths.length > 0);
  });

  if (visibleCheckpoints.length === 0) {
    return newPrompt;
  }

  const parts = [
    "# Previous Conversation Context",
    `(Carry only the latest ${visibleCheckpoints.length} prompt context entries from the previous session.)`,
  ];
  visibleCheckpoints.forEach((checkpoint, i) => {
    const promptPath = checkpoint.promptPath?.trim() ?? "";
    const outputContentPath = checkpoint.outputContentPath?.trim() ?? "";
    const artifactOutputPaths = Array.isArray(checkpoint.artifactOutputPaths)
      ? checkpoint.artifactOutputPaths.filter(
          (value) => typeof value === "string" && value.trim().length > 0,
        )
      : [];

    if (promptPath) {
      parts.push(`## Prompt Path ${i + 1}\n- ${promptPath}`);
    }

    if (outputContentPath) {
      parts.push(`## Output Content Path ${i + 1}\n- ${outputContentPath}`);
    }

    if (artifactOutputPaths.length > 0) {
      parts.push(
        `## Artifact Output Path ${i + 1}\n${artifactOutputPaths.map((artifactPath) => `- ${artifactPath}`).join("\n")}`,
      );
    }
  });
  parts.push(`# Current Request\n${newPrompt}`);
  return parts.join("\n\n");
}

function isThreadMissingOutput(outputMarkdown: string | null | undefined) {
  const normalized = (outputMarkdown ?? "").trim().toLowerCase();
  if (!normalized) {
    return false;
  }

  return (
    normalized.includes("session not found for thread_id") ||
    normalized.includes("session not found for thread id") ||
    normalized.includes("thread not found")
  );
}

type WorkflowSessionSendResult = LocalRunnerPromptExecutionResult & {
  actualPromptText: string;
  sessionDbId?: string;
};

export async function sendMessageWithRetry({
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
  idleTTLSeconds,
  forceNewProviderSession,
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
  idleTTLSeconds?: number | null;
  forceNewProviderSession?: boolean;
}): Promise<WorkflowSessionSendResult> {
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
    idleTTLSeconds,
    forceNewProviderSession,
  });

  let attempt = 1;
  const maxAttempts = 3;
  let currentPrompt = prompt;

  while (attempt <= maxAttempts) {
    try {
      const result = await localRunnerGateway.sendMessage({
        session: handle,
        prompt: currentPrompt,
        skillIds,
        contextSourceIds: [],
        idleTTLSeconds,
      });

      if (isThreadMissingOutput(result.outputMarkdown)) {
        throw new Error(`provider error: ${result.outputMarkdown}`);
      }

      await syncWorkflowRunSessionProviderSessionId({
        adminClient,
        workflowRunId,
        stepRunId,
        subagent,
        providerKey,
        modelName,
        handle,
        providerSessionId: result.providerSessionId ?? null,
      });

      await writeWorkflowSessionLog({
        adminClient,
        workflowRunStepId: stepRunId,
        logLevel: "info",
        event: "session_message_sent",
        details: {
          workflowRunId,
          stepRunId,
          sessionKind: subagent ? "subagent" : "main",
          provider: providerKey,
          model: modelName,
          providerSessionId: result.providerSessionId ?? handle.providerSessionId,
          processKey: handle.processKey,
        },
      });

      return {
        ...result,
        actualPromptText: currentPrompt,
        sessionDbId: handle.dbId,
      };
    } catch (error) {
      const msg = String((error as any).message || "").toLowerCase();
      const isSessionDead = (error as any).code === "session_dead";
      const isThreadMissing =
        msg.includes("provider error:") &&
        (msg.includes("thread") || msg.includes("not found") || msg.includes("invalid"));

      if (attempt >= maxAttempts) {
        throw error;
      }

      if (isSessionDead) {
        await writeWorkflowSessionLog({
          adminClient,
          workflowRunStepId: stepRunId,
          logLevel: "warn",
          event: "session_dead",
          details: {
            workflowRunId,
            stepRunId,
            sessionKind: subagent ? "subagent" : "main",
            provider: providerKey,
            model: modelName,
            providerSessionId: handle?.providerSessionId ?? null,
            processKey: handle?.processKey ?? null,
          },
        });
        await deactivateWorkflowRunSession({
          adminClient,
          workflowRunId,
          stepRunId,
          subagent,
          localRunnerGateway,
          handle,
        });

        // First fallback: same-machine resume
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
          idleTTLSeconds,
          resumeProviderSessionId: handle?.providerSessionId ?? null,
          recoveryMode: "resumed_thread",
          recoveredFromSessionId: handle?.dbId ?? null,
          recoveredFromProviderSessionId: handle?.providerSessionId ?? null,
        });
        attempt++;
        continue;
      }

      if (isThreadMissing) {
        // Second fallback: cross-machine bootstrap
        await writeWorkflowSessionLog({
          adminClient,
          workflowRunStepId: stepRunId,
          logLevel: "warn",
          event: "session_bootstrap_replay",
          details: { reason: "thread_not_found" },
        });

        const { data: sessionRow } = await adminClient
          .from("workflow_run_sessions")
          .select("metadata_json")
          .eq("id", handle.dbId)
          .single();

        const checkpoints = sessionRow?.metadata_json?.checkpoints || [];
        currentPrompt = buildBootstrapPrompt(checkpoints, prompt);

        await deactivateWorkflowRunSession({
          adminClient,
          workflowRunId,
          stepRunId,
          subagent,
          localRunnerGateway,
          handle,
        });

        // Force new session without resumeProviderSessionId
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
          idleTTLSeconds,
          forceNewProviderSession: true,
          recoveryMode: "bootstrap_replay",
          recoveredFromSessionId: handle?.dbId ?? null,
          recoveredFromProviderSessionId: handle?.providerSessionId ?? null,
          replayCheckpointCount: checkpoints.length,
        });
        attempt++;
        continue;
      }

      throw error;
    }
  }

  throw new Error("sendMessageWithRetry exceeded max attempts");
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

type LocalWorkflowOutputArtifactSnapshot = {
  artifactId: string;
  snapshotDirectory: string;
  manifestPath: string;
  contentPath: string;
};

export async function createLocalWorkflowOutputArtifactSnapshot({
  outputMarkdown,
  promptText,
  actualPromptText = promptText,
  projectId,
  stepType,
  stderrText,
  stdoutText,
  workflowRunId,
  workingDirectory,
  commandText,
  providerKey,
  title = "Response.md",
}: {
  outputMarkdown: string;
  promptText: string;
  actualPromptText?: string;
  projectId: string;
  stepType: string;
  stderrText: string;
  stdoutText: string;
  workflowRunId: string;
  workingDirectory: string;
  commandText: string;
  providerKey: string;
  title?: string;
}): Promise<LocalWorkflowOutputArtifactSnapshot> {
  const artifactId = randomUUID();
  const snapshotDirectory = path.join(
    workingDirectory,
    ".flowpilot",
    "artifacts",
    projectId,
    workflowRunId,
    stepType,
    ".snapshots",
    artifactId,
  );
  const contentPath = path.join(snapshotDirectory, title);
  const promptPath = path.join(snapshotDirectory, "prompt.md");
  const actualPromptPath = path.join(snapshotDirectory, "actual-prompt.md");
  const stdoutPath = path.join(snapshotDirectory, "stdout.txt");
  const stderrPath = path.join(snapshotDirectory, "stderr.txt");
  const commandPath = path.join(snapshotDirectory, "command.txt");
  const manifestPath = path.join(snapshotDirectory, "manifest.json");
  const now = new Date().toISOString();

  await mkdir(snapshotDirectory, { recursive: true });
  await writeFile(contentPath, outputMarkdown, "utf8");
  await writeFile(promptPath, promptText, "utf8");
  await writeFile(actualPromptPath, actualPromptText, "utf8");
  await writeFile(stdoutPath, stdoutText, "utf8");
  await writeFile(stderrPath, stderrText, "utf8");
  await writeFile(commandPath, commandText, "utf8");
  await writeFile(
    manifestPath,
    JSON.stringify(
      {
        artifactId,
        title,
        sourceKind: "workflow_output",
        projectId,
        featureId: stepType,
        workflowRunId,
        workflowStepKey: stepType,
        providerKey,
        localPath: snapshotDirectory,
        remotePath: "",
        remoteUrl: "",
        syncStatus: "local_only",
        createdAt: now,
        updatedAt: now,
        promptPath,
        actualPromptPath,
        stdoutPath,
        stderrPath,
        commandPath,
        contentPath,
      },
      null,
      2,
    ),
    "utf8",
  );

  return {
    artifactId,
    snapshotDirectory,
    manifestPath,
    contentPath,
  };
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

function buildAiOutputLogMessage(outputMarkdown: string) {
  return `ai_output:${outputMarkdown}`;
}

function buildWorkflowSessionLogMessage(event: string, details: Record<string, unknown>) {
  return `session_event:${JSON.stringify({ event, ...details })}`;
}

async function writeWorkflowSessionLog({
  adminClient,
  workflowRunStepId,
  logLevel,
  event,
  details,
}: {
  adminClient: SupabaseClient;
  workflowRunStepId: string;
  logLevel: "info" | "warn" | "error" | "debug";
  event: string;
  details: Record<string, unknown>;
}) {
  try {
    await insertLog(
      adminClient,
      workflowRunStepId,
      logLevel,
      buildWorkflowSessionLogMessage(event, details),
    );
  } catch (error) {
    console.error("Failed to write workflow session log:", error);
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
    .select("default_provider, default_model, default_reasoning_effort, session_idle_ttl_minutes")
    .eq("id", projectId)
    .maybeSingle();

  if (error) {
    throw new Error(`Unable to load project defaults: ${error.message}`);
  }

  return (data ?? {
    default_provider: null,
    default_model: null,
    default_reasoning_effort: null,
    session_idle_ttl_minutes: null,
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

function buildOptionalPromptSection(title: string, lines: string[]) {
  return lines.length > 0 ? buildPromptSection(title, lines) : null;
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
    buildOptionalPromptSection(
      "Artifacts To Review",
      outputArtifactPaths.map((outputPath) => `- ${outputPath}`),
    ),
    buildOptionalPromptSection(
      "Related Input Artifacts",
      inputArtifactPaths.map((inputPath) => `- ${inputPath}`),
    ),
    buildPromptSection("Execution Context", [`- Working directory: ${workingDirectory}`]),
    "Revise the current artifact according to the follow-up prompt and return the updated final result in Markdown.",
  ];

  return sections.filter(Boolean).join("\n");
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
    .select(
      "id, workflow_run_id, workflow_step_id, execution_order_index, step_type, status, retry_count, error_message",
    )
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

function createBuiltInResultSummaryDefinition({
  workflow,
  projectDefaults,
}: {
  workflow: WorkflowDefinitionRow;
  projectDefaults: ProjectSettingsRow;
}): StepDefinitionRow {
  return {
    step_type: RESULT_SUMMARY_STEP_TYPE,
    name: RESULT_SUMMARY_STEP_NAME,
    description: "Runtime-generated final workflow summary step.",
    prompt_base: null,
    required_mcps: [],
    required_skills: [],
    team_role: null,
    subagent: null,
    model: workflow.model_override || projectDefaults.default_model || DEFAULT_MODEL,
    reasoning_effort:
      workflow.reasoning_effort_override ||
      projectDefaults.default_reasoning_effort ||
      DEFAULT_REASONING_EFFORT,
  };
}

async function ensureBuiltInResultSummaryStepDefinition({
  adminClient,
  workflow,
  projectDefaults,
}: {
  adminClient: SupabaseClient;
  workflow: WorkflowDefinitionRow;
  projectDefaults: ProjectSettingsRow;
}) {
  const definition = createBuiltInResultSummaryDefinition({
    workflow,
    projectDefaults,
  });

  const { error } = await adminClient
    .from("step_definitions")
    .upsert(
      {
        step_type: RESULT_SUMMARY_STEP_TYPE,
        name: definition.name,
        description: definition.description,
        prompt_base: definition.prompt_base,
        required_mcps: definition.required_mcps ?? [],
        required_skills: definition.required_skills ?? [],
        team_role: definition.team_role,
        subagent: definition.subagent,
        model: definition.model,
        reasoning_effort: definition.reasoning_effort,
        agent_type: "standard",
      },
      { onConflict: "step_type" },
    );

  if (error) {
    throw new Error(`Unable to ensure built-in step definition "${RESULT_SUMMARY_STEP_TYPE}": ${error.message}`);
  }

  return definition;
}

function resolveBuiltInStepExecution(
  workflow: WorkflowDefinitionRow,
  projectDefaults: ProjectSettingsRow,
  definition: StepDefinitionRow,
) {
  const resolvedReasoningEffort =
    definition.reasoning_effort ||
    workflow.reasoning_effort_override ||
    projectDefaults.default_reasoning_effort ||
    DEFAULT_REASONING_EFFORT;
  if (resolvedReasoningEffort && !SUPPORTED_REASONING_EFFORTS.has(resolvedReasoningEffort)) {
    throw new Error(`Step ${definition.step_type} is configured with an unsupported reasoning effort.`);
  }

  const resolvedModel =
    definition.model ||
    workflow.model_override ||
    projectDefaults.default_model ||
    DEFAULT_MODEL;
  if (!resolvedModel || !isSupportedStepModel(resolvedModel)) {
    throw new Error(`Step ${definition.step_type} is configured with an unsupported model.`);
  }

  return {
    model: resolvedModel,
    providerKey: resolveProviderKeyFromModel(resolvedModel),
    reasoningEffort: resolvedReasoningEffort,
  };
}

export async function createArtifactOutputs({
  adminClient,
  artifactDefinitions,
  outputArtifactKeys,
  outputMarkdown,
  promptText,
  actualPromptText = promptText,
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
  actualPromptText?: string;
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
  const checkpointArtifactOutputPaths: string[] = [];
  let checkpointPromptPath = "";
  let checkpointOutputContentPath = "";

  if (outputArtifactKeys.length === 0) {
    const snapshot = await createLocalWorkflowOutputArtifactSnapshot({
      outputMarkdown,
      promptText,
      actualPromptText,
      projectId,
      stepType,
      stderrText,
      stdoutText,
      workflowRunId,
      workingDirectory,
      commandText,
      providerKey,
    });
    return {
      artifactRunId: null,
      checkpoint: {
        promptPath: path.join(snapshot.snapshotDirectory, "prompt.md"),
        outputContentPath: snapshot.contentPath,
        artifactOutputPaths: [] as string[],
      },
    };
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
    checkpointArtifactOutputPaths.push(absoluteOutputPath);
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
    const snapshotLocalPath = path.join(
      path.dirname(localPath),
      ".snapshots",
      artifactId,
      path.basename(localPath),
    );
    const absolutePromptPath = path.join(snapshotDirectory, "prompt.md");
    const absoluteActualPromptPath = path.join(snapshotDirectory, "actual-prompt.md");
    const absoluteStdoutPath = path.join(snapshotDirectory, "stdout.txt");
    const absoluteStderrPath = path.join(snapshotDirectory, "stderr.txt");
    const absoluteCommandPath = path.join(snapshotDirectory, "command.txt");
    await writeFile(absoluteSnapshotContentPath, outputMarkdown, "utf8");
    await writeFile(absolutePromptPath, promptText, "utf8");
    await writeFile(absoluteActualPromptPath, actualPromptText, "utf8");
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
      actualPromptPath: absoluteActualPromptPath,
      stdoutPath: absoluteStdoutPath,
      stderrPath: absoluteStderrPath,
      commandPath: absoluteCommandPath,
      contentPath: absoluteSnapshotContentPath,
    };

    const absoluteManifestPath = path.join(snapshotDirectory, "manifest.json");
    await writeFile(absoluteManifestPath, JSON.stringify(manifest, null, 2), "utf8");

    if (!checkpointPromptPath) {
      checkpointPromptPath = absolutePromptPath;
      checkpointOutputContentPath = absoluteSnapshotContentPath;
    }

    rows.push({
      id: artifactId,
      artifact_definition_key: artifactKey,
      project_id: projectId,
      workflow_id: workflowId,
      workflow_run_id: workflowRunId,
      workflow_run_step_id: stepRunId,
      title: definition.default_file_name || definition.name,
      local_path: snapshotLocalPath,
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

  return {
    artifactRunId: data?.[0]?.id ? String(data[0].id) : null,
    checkpoint: {
      promptPath: checkpointPromptPath,
      outputContentPath: checkpointOutputContentPath,
      artifactOutputPaths: checkpointArtifactOutputPaths,
    },
  };
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
  const canReplayInterruptedFailedStep = isInterruptedWorkflowStep({
    status: stepRun.status,
    errorMessage: stepRun.error_message,
  });
  if (
    stepRun.status !== "DONE" &&
    stepRun.status !== "WAITING_USER_APPROVAL" &&
    !canReplayInterruptedFailedStep
  ) {
    throw new Error("Follow-up is only supported for completed or interrupted steps.");
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
      idleTTLSeconds: projectDefaults.session_idle_ttl_minutes ? projectDefaults.session_idle_ttl_minutes * 60 : undefined,
      forceNewProviderSession: canReplayInterruptedFailedStep,
    });

    if (result.status !== "success") {
      throw new Error(result.errorMessage || `Local runner execution failed for ${stepRun.step_type}.`);
    }

    await insertLog(
      adminClient,
      stepRun.id,
      "debug",
      buildAiOutputLogMessage(result.outputMarkdown),
    );

    const artifactOutputResult = await createArtifactOutputs({
      adminClient,
      artifactDefinitions,
      outputArtifactKeys,
      outputMarkdown: result.outputMarkdown,
      promptText: finalPrompt,
      actualPromptText: result.actualPromptText,
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
    const artifactRunId = artifactOutputResult.artifactRunId;

    if (result.sessionDbId) {
      await appendSessionCheckpoint(
        adminClient,
        result.sessionDbId,
        artifactOutputResult.checkpoint,
      );
    }

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

  const enabledWorkflowSteps = workflowSteps.filter((step) => step.is_enabled);
  const shouldAddResultSummary = shouldAppendResultSummaryStep(enabledWorkflowSteps);
  if (shouldAddResultSummary) {
    const builtInDefinition = await ensureBuiltInResultSummaryStepDefinition({
      adminClient,
      workflow,
      projectDefaults,
    });
    stepDefinitions.set(RESULT_SUMMARY_STEP_TYPE, builtInDefinition);
  }
  const stepPlans = enabledWorkflowSteps
    .map((step) => {
      const definition = stepDefinitions.get(step.step_type);
      if (!definition) {
        throw new Error(`Step definition "${step.step_type}" could not be loaded.`);
      }

      return {
        planKey: step.id,
        workflowStepId: step.id,
        stepType: step.step_type,
        orderIndex: step.order_index,
        definition,
        ...resolvePlannedStepExecution(step, workflow, projectDefaults, definition),
        inputArtifactKeys: inputBindings.get(step.step_type) ?? [],
        outputArtifactKeys: outputBindings.get(step.step_type) ?? [],
      } satisfies StepExecutionPlan;
    });

  if (shouldAddResultSummary) {
    const definition = stepDefinitions.get(RESULT_SUMMARY_STEP_TYPE);
    if (!definition) {
      throw new Error(`Step definition "${RESULT_SUMMARY_STEP_TYPE}" could not be loaded.`);
    }
    const nextOrderIndex =
      enabledWorkflowSteps.reduce(
        (highest, step) => Math.max(highest, step.order_index),
        -1,
      ) + 1;

    stepPlans.push({
      planKey: `__builtin_${RESULT_SUMMARY_STEP_TYPE}`,
      workflowStepId: null,
      stepType: RESULT_SUMMARY_STEP_TYPE,
      orderIndex: nextOrderIndex,
      definition,
      ...resolveBuiltInStepExecution(workflow, projectDefaults, definition),
      inputArtifactKeys: [],
      outputArtifactKeys: [],
    });
  }

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
      stepPlans.map((step) => ({
        workflow_run_id: runRow.id,
        workflow_step_id: step.workflowStepId,
        execution_order_index: step.orderIndex,
        step_type: step.stepType,
        status: "PENDING",
        retry_count: 0,
      })),
    )
    .select("id, workflow_step_id, step_type, execution_order_index, status");

  if (stepInsertError) {
    throw new Error(`Unable to create workflow run steps: ${stepInsertError.message}`);
  }

  const stepRunIds = new Map<string, string>();
  for (const row of insertedSteps ?? []) {
    const matchedPlan = stepPlans.find((step) => {
      if (step.workflowStepId) {
        return step.workflowStepId === String(row.workflow_step_id);
      }

      return (
        row.workflow_step_id == null &&
        step.stepType === String(row.step_type) &&
        step.orderIndex === Number(row.execution_order_index ?? 0)
      );
    });

    if (matchedPlan) {
      stepRunIds.set(matchedPlan.planKey, String(row.id));
    }
  }

  const executeWorkflow = async () => {
    let activeStepRunId: string | null = null;
    const completedSummarySteps: ResultSummarySourceStep[] = [];

    try {
    for (const stepPlan of stepPlans) {
      const stepRunId = stepRunIds.get(stepPlan.planKey);
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

      const finalPrompt = stepPlan.stepType === RESULT_SUMMARY_STEP_TYPE
        ? buildResultSummaryPrompt({
            beginPrompt: request.beginPrompt,
            workflowName: workflow.name,
            workingDirectory,
            steps: completedSummarySteps,
          })
        : (() => {
            const promptBase = stepPlan.definition.prompt_base?.trim()
              ? stepPlan.definition.prompt_base.trim()
              : deriveStepPromptBase({
                  stepType: stepPlan.definition.step_type as StepType,
                  name: stepPlan.definition.name,
                  description: stepPlan.definition.description,
                });

            return buildWorkflowStepPrompt({
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
          })();

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
        idleTTLSeconds: projectDefaults.session_idle_ttl_minutes ? projectDefaults.session_idle_ttl_minutes * 60 : undefined,
      });

      if (result.status !== "success") {
        throw new Error(result.errorMessage || `Local runner execution failed for ${stepPlan.stepType}.`);
      }

      await insertLog(
        adminClient,
        stepRunId,
        "debug",
        buildAiOutputLogMessage(result.outputMarkdown),
      );

      const artifactOutputResult = await createArtifactOutputs({
        adminClient,
        artifactDefinitions,
        outputArtifactKeys: stepPlan.outputArtifactKeys,
        outputMarkdown: result.outputMarkdown,
        promptText: finalPrompt,
        actualPromptText: result.actualPromptText,
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
      const artifactRunId = artifactOutputResult.artifactRunId;

      if (result.sessionDbId) {
        await appendSessionCheckpoint(
          adminClient,
          result.sessionDbId,
          artifactOutputResult.checkpoint,
        );
      }

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

      if (stepPlan.stepType !== RESULT_SUMMARY_STEP_TYPE) {
        completedSummarySteps.push({
          stepName: stepPlan.definition.name,
          stepType: stepPlan.stepType,
          outputMarkdown: result.outputMarkdown,
          artifactOutputPaths: artifactOutputResult.checkpoint.artifactOutputPaths,
          startedAt: result.startedAt,
          completedAt: result.completedAt,
        });
      }
    }

    const finishedAt = new Date().toISOString();
    await finalizeWorkflowRunSessions(
      adminClient,
      localRunnerGateway as any,
      String(runRow.id),
      { preserveMainSession: true },
    );

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
  } catch (error) {
    const message = error instanceof Error ? error.message : "Workflow execution failed.";
    await finalizeWorkflowRunSessions(
      adminClient,
      localRunnerGateway as any,
      String(runRow.id),
    );

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
  }
  };

  void executeWorkflow().catch((err) => {
    console.error("Failed to execute background workflow:", err);
  });

  return runRow;
}
