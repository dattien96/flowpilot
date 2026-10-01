// CA-1076: the step/workflow Model selects flatten every enabled
// ai_supported_models row (200+ entries) into one native <select> — models
// like devin/swe-2-high are buried deep and effectively unreachable.
// These helpers keep the option-build/group/filter logic dependency-light so
// it is unit-testable without pulling in @flowpilot/client-core (same
// constraint as stepModelVisibility.ts).

export interface StepModelOption {
  value: string;
  label: string;
  providerKey: string;
}

export interface StepModelOptionGroup {
  providerKey: string;
  providerLabel: string;
  options: StepModelOption[];
}

// Mirrors the providerKey union in
// packages/flowpilot-client-core/src/domain/adminModels.ts — kept in sync
// manually; unknown keys fall back to the raw key, capitalized.
const MODEL_PROVIDER_LABELS: Record<string, string> = {
  claude: "Claude",
  codex: "Codex",
  gemini: "Gemini",
  grok: "Grok",
  opencode: "OpenCode",
  devin: "Devin",
};

export function modelProviderLabel(providerKey: string): string {
  const key = providerKey.trim();
  if (!key) return "Other";
  return MODEL_PROVIDER_LABELS[key] ?? key.charAt(0).toUpperCase() + key.slice(1);
}

// buildModelOptions maps the enabled catalog rows into select options,
// preserving the table's sort_order. Disabled rows never surface.
export function buildModelOptions(
  models: Array<{ modelId: string; displayName: string; providerKey: string; isEnabled: boolean }>,
): StepModelOption[] {
  return models
    .filter((model) => model.isEnabled)
    .map((model) => ({ value: model.modelId, label: model.displayName, providerKey: model.providerKey }));
}

// filterModelOptions matches a query case-insensitively against the option's
// display label, model id, or provider key/label — "swe" finds
// "Devin SWE-2 (High)", "devin" finds every Devin row.
export function filterModelOptions(options: StepModelOption[], query: string): StepModelOption[] {
  const q = query.trim().toLowerCase();
  if (!q) return options;
  return options.filter((option) =>
    option.label.toLowerCase().includes(q) ||
    option.value.toLowerCase().includes(q) ||
    option.providerKey.toLowerCase().includes(q) ||
    modelProviderLabel(option.providerKey).toLowerCase().includes(q),
  );
}

// groupModelOptions folds a (filtered) option list into per-provider groups,
// preserving the incoming option order inside and across groups — first
// occurrence of a provider defines its group position.
export function groupModelOptions(options: StepModelOption[]): StepModelOptionGroup[] {
  const groups: StepModelOptionGroup[] = [];
  const byProvider = new Map<string, StepModelOptionGroup>();
  for (const option of options) {
    const key = option.providerKey.trim() || "other";
    let group = byProvider.get(key);
    if (!group) {
      group = { providerKey: key, providerLabel: modelProviderLabel(key), options: [] };
      byProvider.set(key, group);
      groups.push(group);
    }
    group.options.push(option);
  }
  return groups;
}
