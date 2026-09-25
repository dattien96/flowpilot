import test from "node:test";
import assert from "node:assert/strict";
import { useStore } from "./store";

// Task-446 (CP-87 P-5): machine-global quota routing settings round-trip
// through the runner-owned document — Desktop never writes provider-local
// files itself.

// Task-450 T-1: the machine-global document defaults to manual — auto
// rotation is opt-in, never ambient.
test("quota routing setting defaults Manual", async () => {
  await useStore.getState().loadQuotaRoutingSettings();
  assert.equal(useStore.getState().quotaRoutingSettings.mode, "manual");
});

test("settings edits mode priority and class model bindings", async () => {
  const store = useStore.getState();
  await store.loadQuotaRoutingSettings();
  // Default: manual mode (Task-446 T-2 — rotation never defaults to auto).
  assert.equal(useStore.getState().quotaRoutingSettings.mode, "manual");

  await useStore.getState().saveQuotaRoutingSettings({
    mode: "auto",
    providerPriority: ["devin", "grok", "claude"],
    modelBindings: [
      { providerKey: "devin", workloadClass: "coding", model: "devin/swe-2-high" },
      { providerKey: "grok", workloadClass: "high_reasoning", model: "grok-4.5" },
    ],
    headroomLowPercent: 15,
    telemetryTtlSeconds: 90,
    sameProviderCooldownSeconds: 30,
  });

  const saved = useStore.getState().quotaRoutingSettings;
  assert.equal(saved.mode, "auto");
  assert.deepEqual(saved.providerPriority, ["devin", "grok", "claude"]);
  assert.equal(saved.modelBindings?.length, 2);
  assert.equal(saved.modelBindings?.[0].model, "devin/swe-2-high");
  assert.equal(saved.modelBindings?.[1].workloadClass, "high_reasoning");
  assert.equal(saved.headroomLowPercent, 15);
  assert.equal(saved.telemetryTtlSeconds, 90);
  assert.equal(saved.sameProviderCooldownSeconds, 30);

  // Quota percentages are telemetry, not token balances — the settings shape
  // must not carry any token-budget field on account headroom.
  assert.equal("maxUsageTokens" in saved, false);
});
