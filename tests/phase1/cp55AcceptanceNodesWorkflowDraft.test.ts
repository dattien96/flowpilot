import test from "node:test";
import assert from "node:assert/strict";

import type { Workflow } from "../../packages/flowpilot-client-core/src/domain/adminModels";
import {
  createEmptyWorkflowDraft,
  mapWorkflowToDraft,
} from "../../apps/desktop-flowpilot/src/components/settings/WorkflowsSettings";

// CP-55 P-1 (Task-263/CA-424 pass 3): companion to
// tests/phase1/cp55AcceptanceNodesRoundTrip.test.ts, which covers the
// SupabaseAdminRepository layer. This file covers the Settings UI draft
// layer (WorkflowsSettings.tsx) instead: acceptanceNodes is read-only/
// display-only there in P-1 (no editor), so the only way a save can avoid
// silently erasing a declared acceptance boundary is if the draft actually
// carries the loaded value forward — exactly like edges/policyCap already
// do (BUG-NOTE-CP42 #14). New file — no pre-existing test is modified.

function fullWorkflow(overrides: Partial<Workflow> = {}): Workflow {
  return {
    id: "wf-1",
    projectId: null,
    name: "Custom Flow",
    description: "",
    isTemplate: false,
    providerOverride: null,
    modelOverride: null,
    reasoningEffortOverride: null,
    yoloMode: true,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    isBuiltin: false,
    editable: true,
    cloneable: true,
    clonedFrom: null,
    packId: null,
    packVersion: null,
    packFlowId: null,
    packHash: null,
    selectableIn: [],
    chatBaseline: false,
    chatSubModes: [],
    policyCap: null,
    policyOnCap: null,
    policyExtendBy: null,
    policyExtendMax: null,
    edges: [],
    ...overrides,
  };
}

test("mapWorkflowToDraft carries acceptanceNodes through for a workflow that declares them", () => {
  const draft = mapWorkflowToDraft(fullWorkflow({ acceptanceNodes: ["validate"] }));
  assert.deepEqual(draft?.acceptanceNodes, ["validate"]);
});

test("mapWorkflowToDraft defaults acceptanceNodes to [] when the workflow omits the field", () => {
  const workflow = fullWorkflow();
  delete (workflow as { acceptanceNodes?: string[] }).acceptanceNodes;
  const draft = mapWorkflowToDraft(workflow);
  assert.deepEqual(draft?.acceptanceNodes, []);
});

test("createEmptyWorkflowDraft defaults acceptanceNodes to [] for a brand-new workflow", () => {
  const draft = createEmptyWorkflowDraft([], "");
  assert.deepEqual(draft.acceptanceNodes, []);
});
