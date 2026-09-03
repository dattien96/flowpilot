"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const WorkflowsSettings_1 = require("./WorkflowsSettings");
// Flow/Workflow launches always run YOLO=true — the gate/approval/reinvoke machinery a
// workflow's own hub/cohort orchestration depends on is not robust against a paused
// approval mid-flow (a stalled/failed child, a missed approval render while another child
// is focused, the hub's own stall-timeout firing while a sibling is still legitimately
// working — Test A3's YOLO=off repro). YOLO=false is only supported in Normal Chat, which
// has no hub/cohort/gate layer to race against.
//
// A built-in workflow's mirror-sync insert never sets yolo_mode itself (it is a pure
// per-installation admin setting, not part of the pack schema — see
// supabase/migrations/20260720150000_default_flow_yolo_mode_true.sql), so a freshly
// mirrored "Review Loop" row could still be persisted with yolo_mode=false depending on
// when it was first synced. mapWorkflowToDraft normalizes this the moment such a row is
// loaded into the Settings edit form, regardless of what is actually stored.
function builtinWorkflow(overrides = {}) {
    return {
        id: "wf-review-loop",
        projectId: null,
        name: "Review Loop",
        description: "",
        isTemplate: false,
        providerOverride: null,
        modelOverride: null,
        reasoningEffortOverride: null,
        yoloMode: false,
        createdAt: "2026-07-20T00:00:00Z",
        updatedAt: "2026-07-20T00:00:00Z",
        isBuiltin: true,
        editable: false,
        cloneable: true,
        clonedFrom: null,
        packId: "flow-pack",
        packVersion: "1",
        packFlowId: "review-loop",
        packHash: "abc123",
        selectableIn: ["chat", "flow"],
        chatBaseline: false,
        chatSubModes: ["bug"],
        policyCap: 3,
        policyOnCap: "escalate",
        policyExtendBy: 2,
        policyExtendMax: 2,
        edges: [],
        ...overrides,
    };
}
(0, node_test_1.default)("mapWorkflowToDraft forces yoloMode=true even for a built-in workflow persisted with yoloMode=false", () => {
    const draft = (0, WorkflowsSettings_1.mapWorkflowToDraft)(builtinWorkflow({ yoloMode: false }));
    strict_1.default.equal(draft?.yoloMode, true);
});
(0, node_test_1.default)("mapWorkflowToDraft stays yoloMode=true for a workflow already persisted correctly", () => {
    const draft = (0, WorkflowsSettings_1.mapWorkflowToDraft)(builtinWorkflow({ yoloMode: true }));
    strict_1.default.equal(draft?.yoloMode, true);
});
(0, node_test_1.default)("mapWorkflowToDraft returns null for a null workflow (no-selection state unaffected)", () => {
    strict_1.default.equal((0, WorkflowsSettings_1.mapWorkflowToDraft)(null), null);
});
(0, node_test_1.default)("createEmptyWorkflowDraft defaults yoloMode=true for a brand-new workflow", () => {
    const draft = (0, WorkflowsSettings_1.createEmptyWorkflowDraft)([], "");
    strict_1.default.equal(draft.yoloMode, true);
});
(0, node_test_1.default)("createEmptyStepDraft defaults yoloMode=true for a brand-new step definition", () => {
    const draft = (0, WorkflowsSettings_1.createEmptyStepDraft)("");
    strict_1.default.equal(draft.yoloMode, true);
});
