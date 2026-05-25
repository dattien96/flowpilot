type WorkflowArtifactInput = {
  artifactId: string;
  title: string;
  projectId: string;
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
  promptText: string;
  stdoutText: string;
  stderrText: string;
  commandText: string;
};

const STORAGE_KEY = "flowpilot:workflow-artifacts";
const memoryStore = new Map<string, WorkflowArtifactManifest>();

function hasLocalStorage() {
  try {
    return typeof localStorage !== "undefined";
  } catch {
    return false;
  }
}

function readArtifacts() {
  if (hasLocalStorage()) {
    try {
      const payload = localStorage.getItem(STORAGE_KEY);
      if (payload) {
        const parsed = JSON.parse(payload) as WorkflowArtifactManifest[];
        if (Array.isArray(parsed)) {
          return parsed;
        }
      }
    } catch {
      // Fall through to the in-memory store when browser persistence fails.
    }
  }

  return Array.from(memoryStore.values());
}

function writeArtifacts(artifacts: WorkflowArtifactManifest[]) {
  if (hasLocalStorage()) {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(artifacts));
    } catch {
      // Fall back to the in-memory store when browser persistence is unavailable.
    }
  }

  memoryStore.clear();
  for (const artifact of artifacts) {
    memoryStore.set(artifact.artifactId, artifact);
  }
}

async function digestSha256(contents: string) {
  const bytes = new TextEncoder().encode(contents);
  const subtle = globalThis.crypto?.subtle;

  if (subtle) {
    const digest = await subtle.digest("SHA-256", bytes);
    return Array.from(new Uint8Array(digest), (value) =>
      value.toString(16).padStart(2, "0"),
    ).join("");
  }

  let hash = 0;
  for (const char of contents) {
    hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  }

  return hash.toString(16).padStart(8, "0");
}

export async function saveWorkflowArtifact(input: WorkflowArtifactInput) {
  const createdAt = new Date().toISOString();
  const basePath = [
    ".flowpilot",
    "artifacts",
    sanitize(input.projectId),
    sanitize(input.workflowRunId),
    sanitize(input.workflowStepKey),
    sanitize(input.artifactId),
  ].join("/");

  const contentPath = `${basePath}/content.md`;
  const promptPath = `${basePath}/prompt.md`;
  const stdoutPath = `${basePath}/stdout.txt`;
  const stderrPath = `${basePath}/stderr.txt`;
  const commandPath = `${basePath}/command.txt`;
  const manifestPath = `${basePath}/manifest.json`;
  const contentMarkdown = input.contentMarkdown.trim();
  const checksum = await digestSha256(contentMarkdown);
  const artifact: WorkflowArtifactManifest = {
    artifactId: input.artifactId,
    title: input.title,
    sourceKind: input.sourceKind ?? "workflow_output",
    projectId: input.projectId,
    workflowRunId: input.workflowRunId,
    workflowStepKey: input.workflowStepKey,
    providerKey: input.providerKey,
    localPath: basePath,
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
    promptText: input.promptText ?? "",
    stdoutText: input.stdoutText ?? "",
    stderrText: input.stderrText ?? "",
    commandText: input.commandText ?? "",
  };

  const artifacts = readArtifacts().filter((item) => item.artifactId !== artifact.artifactId);
  artifacts.unshift(artifact);
  writeArtifacts(artifacts);
}

export function sanitize(value: string) {
  const trimmed = value.trim();
  const segments = trimmed.split(/[\\/]/).filter(Boolean);
  const leaf = segments.at(-1) ?? "";

  if (!leaf || /^\.+$/.test(leaf)) {
    return "unassigned";
  }

  return leaf
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
