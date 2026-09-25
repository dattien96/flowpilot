import test from "node:test";
import assert from "node:assert/strict";
import { applyEvent, useStore } from "./store";
import type { ProviderEventDTO } from "../types/contract";

// Task-444 (CP-86 P-5) — the renderer's ingest of the two new CP-86 event
// types. context_pressure(aware) and provider_compacted land as an inline
// notice on the focused run; the aware mark clears when a later usage event
// drops below the tier or the leg rotates; compaction stays pinned for the
// leg. Additive-only.

const BASE = {
  id: "e1",
  workflowRunId: "run-1",
  providerSessionId: "leg-1",
  providerKey: "codex",
  seq: 1,
  occurredAt: "2026-01-01T00:00:00Z",
} as const;

function usageEvt(seq: number, used: number, window: number, session = "leg-1"): ProviderEventDTO {
  return {
    ...BASE,
    seq,
    providerSessionId: session,
    type: "token_usage_updated",
    tokenUsage: {
      total: { cachedInputTokens: 0, inputTokens: used, outputTokens: 0, reasoningOutputTokens: 0, totalTokens: used },
      modelContextWindow: window,
    },
  };
}

test("context_pressure aware event lands as an inline notice", () => {
  useStore.setState({ contextNotice: undefined });
  useStore.setState((s) =>
    applyEvent(s, {
      ...BASE,
      type: "context_pressure",
      contextPressure: { tier: "aware", ratio: 0.83, usedTokens: 166000, windowTokens: 200000, legId: "leg-1" },
    }),
  );
  const n = useStore.getState().contextNotice;
  assert.equal(n?.kind, "pressure_aware");
  assert.equal(n?.ratio, 0.83);
});

test("provider_compacted lands as a pinned compacted notice with prev→cur", () => {
  useStore.setState({ contextNotice: undefined });
  useStore.setState((s) =>
    applyEvent(s, {
      ...BASE,
      type: "provider_compacted",
      contextPressure: { tier: "aware", ratio: 0.09, usedTokens: 18000, windowTokens: 200000, prevTokens: 190000, legId: "leg-1" },
    }),
  );
  const n = useStore.getState().contextNotice;
  assert.equal(n?.kind, "provider_compacted");
  assert.equal(n?.prev, 190000);
  assert.equal(n?.cur, 18000);
});

test("aware notice clears when usage drops below the tier; compacted stays pinned", () => {
  useStore.setState({ contextNotice: undefined });
  useStore.setState((s) =>
    applyEvent(s, {
      ...BASE,
      type: "context_pressure",
      contextPressure: { tier: "aware", ratio: 0.83, usedTokens: 166000, windowTokens: 200000, legId: "leg-1" },
    }),
  );
  useStore.setState((s) => applyEvent(s, usageEvt(2, 100000, 200000)));
  assert.equal(useStore.getState().contextNotice, undefined, "aware cleared on drop below tier");

  useStore.setState((s) =>
    applyEvent(s, {
      ...BASE,
      type: "provider_compacted",
      contextPressure: { tier: "aware", ratio: 0.09, usedTokens: 18000, windowTokens: 200000, prevTokens: 190000, legId: "leg-1" },
    }),
  );
  useStore.setState((s) => applyEvent(s, usageEvt(3, 20000, 200000)));
  assert.equal(useStore.getState().contextNotice?.kind, "provider_compacted", "compacted is a fact — stays pinned");
});

test("leg rotation clears the stale notice", () => {
  useStore.setState({ contextNotice: undefined });
  useStore.setState((s) =>
    applyEvent(s, {
      ...BASE,
      type: "provider_compacted",
      contextPressure: { tier: "aware", ratio: 0.09, usedTokens: 18000, windowTokens: 200000, prevTokens: 190000, legId: "leg-1" },
    }),
  );
  // First event on the new leg (leg-2) — old leg's marks must not bleed over.
  useStore.setState((s) => applyEvent(s, usageEvt(4, 5000, 200000, "leg-2")));
  assert.equal(useStore.getState().contextNotice, undefined);
});
