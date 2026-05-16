import crypto from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";

import { getWorkspaceRoot } from "@/lib/env/app-env";

type WorkflowArtifactInput = {
  artifactId: string;
  title: string;
  projectId: string;
  featureId: string;
  workflowRunId: string;
  workflowStepKey: string;
  providerKey: string;
  contentMarkdown: string;
  promptText?: string;
  stdoutText?: string;
  stderrText?: string;
  commandText?: string;
  sourceKind?: string;
};

type WorkflowArtifactManifest = {
  artifactId: string;
  title: string;
  sourceKind: string;
  projectId: string;
  featureId: string;
  workflowRunId: string;
  workflowStepKey: string;
  providerKey: string;
  localPath: string;
  remotePath: string;
  remoteUrl: string;
  syncStatus: "local_only" | "syncing" | "synced" | "failed";
  createdAt: string;
  updatedAt: string;
  contentMarkdown: string;
  previewMarkdown: string;
  manifestPath: string;
  promptPath: string;
  stdoutPath: string;
  stderrPath: string;
  commandPath: string;
  contentPath: string;
  checksum: string;
};

export async function saveWorkflowArtifact(input: WorkflowArtifactInput) {
  const workspaceRoot = getWorkspaceRoot();
  const createdAt = new Date().toISOString();
  const baseDir = path.join(
    workspaceRoot,
    ".flowpilot",
    "artifacts",
    sanitize(input.projectId),
    sanitize(input.featureId),
    sanitize(input.workflowRunId),
    sanitize(input.workflowStepKey),
    sanitize(input.artifactId),
  );

  const contentPath = path.join(baseDir, "content.md");
  const promptPath = path.join(baseDir, "prompt.md");
  const stdoutPath = path.join(baseDir, "stdout.txt");
  const stderrPath = path.join(baseDir, "stderr.txt");
  const commandPath = path.join(baseDir, "command.txt");
  const manifestPath = path.join(baseDir, "manifest.json");
  const contentMarkdown = input.contentMarkdown.trim();
  const checksum = crypto.createHash("sha256").update(contentMarkdown).digest("hex");

  await fs.mkdir(baseDir, { recursive: true });
  await fs.writeFile(contentPath, input.contentMarkdown, "utf8");
  await fs.writeFile(promptPath, input.promptText ?? "", "utf8");
  await fs.writeFile(stdoutPath, input.stdoutText ?? "", "utf8");
  await fs.writeFile(stderrPath, input.stderrText ?? "", "utf8");
  await fs.writeFile(commandPath, input.commandText ?? "", "utf8");

  const manifest: WorkflowArtifactManifest = {
    artifactId: input.artifactId,
    title: input.title,
    sourceKind: input.sourceKind ?? "workflow_output",
    projectId: input.projectId,
    featureId: input.featureId,
    workflowRunId: input.workflowRunId,
    workflowStepKey: input.workflowStepKey,
    providerKey: input.providerKey,
    localPath: baseDir,
    remotePath: "",
    remoteUrl: "",
    syncStatus: "local_only",
    createdAt,
    updatedAt: createdAt,
    contentMarkdown: input.contentMarkdown,
    previewMarkdown: preview(input.contentMarkdown),
    manifestPath,
    promptPath,
    stdoutPath,
    stderrPath,
    commandPath,
    contentPath,
    checksum,
  };

  await fs.writeFile(manifestPath, JSON.stringify(manifest, null, 2), "utf8");
}

export function sanitize(value: string) {
  const trimmed = path.basename(value.trim().split(/[\\/]/).filter(Boolean).at(-1) ?? "");
  if (!trimmed) {
    return "unassigned";
  }

  return trimmed
    .replaceAll("/", "_")
    .replaceAll("\\", "_")
    .replaceAll(":", "_")
    .replaceAll(" ", "_")
    .replace(/[^\w.-]/g, "_")
    .replace(/^\.+$/, "unassigned");
}

function preview(contents: string) {
  const trimmed = contents.trim();
  if (trimmed.length <= 240) {
    return trimmed;
  }

  return `${trimmed.slice(0, 240)}\n...[truncated]`;
}
