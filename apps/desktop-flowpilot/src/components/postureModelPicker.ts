/**
 * BUG-348: posture setup modal model picker (mirrors TUI mode_setup_modal.go).
 *
 * TUI rule: the setup modal has NO provider picker — picking a model
 * auto-infers the provider via providerForModel, and picking "(inherit)"
 * clears model + provider + reasoning (applyModalPickerSelection). Reasoning
 * options come from the picked model's catalog efforts, falling back to the
 * full default list; reasoning is disabled while no model is picked
 * (isReasoningEnabled).
 *
 * This module is the pure, React-free equivalent so node:test can cover it.
 */
import type { LocalRunnerProvider, SupportedModel } from "@flowpilot/client-core";

export interface PostureModelOption {
  providerKey: string;
  modelId: string;
  displayName: string;
}

export interface PostureModelGroup {
  providerKey: string;
  options: PostureModelOption[];
}

/** Mirrors TUI reasoningEffortOptions (helpers.go) — fallback when the model has no catalog. */
export const DEFAULT_POSTURE_REASONING_OPTIONS = ["minimal", "low", "medium", "high", "xhigh", "max"] as const;

function norm(id: string): string {
  return id.trim().toLowerCase();
}

/**
 * Mirrors TUI providerForModel: the provider key that owns a model ID, or
 * undefined. Supported registry first, runner-detected models second.
 */
export function inferPostureProvider(
  supported: SupportedModel[],
  detected: LocalRunnerProvider[],
  modelId: string,
): string | undefined {
  const want = norm(modelId);
  if (!want) return undefined;
  for (const m of supported) {
    if (norm(m.modelId) === want) return m.providerKey;
  }
  for (const p of detected) {
    for (const m of p.models ?? []) {
      if (norm(m.id) === want && p.key) return p.key;
    }
  }
  return undefined;
}

export interface PostureModelPick {
  /** Empty string = "(inherit)", mirrors TUI applyModalPickerSelection. */
  modelId: string;
  provider?: string;
  reasoningEffort?: string;
}

/**
 * Mirrors TUI applyModalPickerSelection ("model" kind): inherit clears
 * model + provider + reasoning; otherwise sets the model and auto-infers
 * the provider (cleared when the model is unknown, exactly like TUI).
 */
export function applyPostureModelPick(
  supported: SupportedModel[],
  detected: LocalRunnerProvider[],
  modelId: string,
): PostureModelPick {
  if (!norm(modelId)) return { modelId: "", provider: undefined, reasoningEffort: undefined };
  return { modelId: modelId.trim(), provider: inferPostureProvider(supported, detected, modelId), reasoningEffort: undefined };
}

/**
 * Mirrors TUI allModelsAcrossProviders: enabled registry models grouped by
 * provider, then runner-detected models not already listed (case-insensitive
 * dedupe), then the currently-pinned model as fallback so the select always
 * shows the draft value. The "(inherit)" row is rendered by the component.
 */
export function buildPostureModelGroups(
  supported: SupportedModel[],
  detected: LocalRunnerProvider[],
  currentModelId?: string,
  currentProvider?: string,
): PostureModelGroup[] {
  const seen = new Set<string>();
  const groups = new Map<string, PostureModelOption[]>();
  const push = (providerKey: string, modelId: string, displayName: string) => {
    const id = modelId.trim();
    if (!id || !providerKey) return;
    const key = norm(id);
    if (seen.has(key)) return;
    seen.add(key);
    const list = groups.get(providerKey) ?? [];
    list.push({ providerKey, modelId: id, displayName: displayName.trim() || id });
    groups.set(providerKey, list);
  };
  for (const m of supported) {
    if (!m.isEnabled) continue;
    push(m.providerKey, m.modelId, m.displayName);
  }
  for (const p of detected) {
    for (const m of p.models ?? []) {
      push(p.key, m.id, m.displayName);
    }
  }
  const cur = (currentModelId ?? "").trim();
  if (cur && !seen.has(norm(cur))) {
    // Mirrors TUI: the current value is always listed, falling back to the
    // pinned provider (else an "other" group) when the model is unknown.
    push(inferPostureProvider(supported, detected, cur) || (currentProvider ?? "").trim() || "other", cur, cur);
  }
  return [...groups.entries()].map(([providerKey, options]) => ({ providerKey, options }));
}

/**
 * Mirrors TUI reasoningOptionsForModel + isReasoningEnabled: null while no
 * model is picked (reasoning disabled); the model's catalog efforts when
 * known; otherwise the full default list.
 */
export function postureReasoningOptions(
  supported: SupportedModel[],
  modelId: string,
): string[] | null {
  if (!norm(modelId)) return null;
  const want = norm(modelId);
  for (const m of supported) {
    if (norm(m.modelId) === want && (m.supportedReasoningEfforts?.length ?? 0) > 0) {
      return m.supportedReasoningEfforts!.map((e) => e.trim().toLowerCase()).filter(Boolean);
    }
  }
  return [...DEFAULT_POSTURE_REASONING_OPTIONS];
}
