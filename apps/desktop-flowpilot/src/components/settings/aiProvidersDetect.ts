import type { SupportedModel } from "@flowpilot/client-core";

// Task-438: pure helpers for the Detect Models flow — kept free of React and
// repository types so the stale-disable decision is unit-testable.

export interface DetectedModelIdentity {
  id: string;
}

/**
 * staleDetectedModelRows returns the registered rows of `providerKey` that a
 * fresh detection pass reports as gone: previously `source: "detected"` rows
 * whose modelId is absent from the detected set. Rows are DISABLED (never
 * deleted — user edits and run history stay), and only enabled ones are
 * returned so a second detect on the same catalog is a no-op.
 *
 * Manually-added rows (`source` !== "detected") are never touched: a model the
 * user typed in by hand must not be switched off just because the provider's
 * catalog did not list it.
 */
export function staleDetectedModelRows(
  registered: readonly SupportedModel[],
  detected: readonly DetectedModelIdentity[],
  providerKey: string,
): SupportedModel[] {
  const detectedIds = new Set(detected.map((m) => m.id));
  return registered.filter(
    (row) =>
      row.providerKey === providerKey &&
      row.source === "detected" &&
      row.isEnabled &&
      !detectedIds.has(row.modelId),
  );
}
