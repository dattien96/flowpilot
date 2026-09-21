import type { ProjectWorkspaceBinding, SupportedModel } from "./adminModels";

export function resolveProviderKeyForModel(
  modelId: string,
  supportedModels: SupportedModel[],
): SupportedModel["providerKey"] | null {
  if (modelId.startsWith("gpt-")) return "codex";
  if (modelId.startsWith("gemini-") || modelId.startsWith("auto-gemini-")) {
    return "gemini";
  }
  if (modelId.startsWith("claude-")) return "claude";
  if (modelId.startsWith("grok-") || modelId === "grok-build") return "grok";
  if (modelId.startsWith("opencode/") || modelId.startsWith("opencode-go/")) return "opencode";
  if (modelId.startsWith("devin/")) return "devin";
  return (
    supportedModels.find((entry) => entry.modelId === modelId)?.providerKey ?? null
  );
}

export function assertValidProjectBindings(
  bindings: Array<Pick<ProjectWorkspaceBinding, "localPath">>,
): void {
  const normalized = bindings
    .map((binding) => binding.localPath.trim())
    .filter((path) => path.length > 0);

  if (normalized.length === 0) {
    throw new Error("Add at least one directory binding.");
  }

  const seen = new Set<string>();
  for (const path of normalized) {
    if (seen.has(path)) {
      throw new Error("Directory bindings must use unique paths.");
    }
    seen.add(path);
  }
}
