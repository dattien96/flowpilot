"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.stepDefinitionRequiresModel = void 0;
exports.createEmptyStepDraft = createEmptyStepDraft;
exports.mapWorkflowToDraft = mapWorkflowToDraft;
exports.createEmptyWorkflowDraft = createEmptyWorkflowDraft;
exports.WorkflowsSettings = WorkflowsSettings;
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const client_core_1 = require("@flowpilot/client-core");
const clientCore_1 = require("@/clientCore");
const settingsHelpers_1 = require("@/components/settings/settingsHelpers");
const fileArtifactConfig_1 = require("@/components/settings/fileArtifactConfig");
const createRunnerClient_1 = require("@/client/createRunnerClient");
const stepModelVisibility_1 = require("@/components/settings/stepModelVisibility");
Object.defineProperty(exports, "stepDefinitionRequiresModel", { enumerable: true, get: function () { return stepModelVisibility_1.stepDefinitionRequiresModel; } });
// Task-196 (CP-44 P-7): the selectable context-source ids, limited to what
// the runner's ContextSourceRegistry actually supports (Task-191/192/195) —
// never free text. This list is a manually-synced descriptor (CP-44 Q-1
// option a); the Go registry stays the validation authority: an id here that
// drifts out of sync with the registry fails flow load fast (Task-194 T-2),
// it does not silently run without it. mcp.driver is backed by a production
// Google Drive adapter (Task-204) — it reads from the project-level Google
// Drive account connected in Google Drive setup. This editor only enables the
// source; when no legacy default file id is stored, the run asks for the file
// URL/id at runtime instead of forcing a settings edit for every run.
// Task-229 (CP-05-06 Q-3, resolved 2026-07-13): jira.issue and jira.sprint
// are two independent sources, not one source with a "target mode" — mirrors
// mcp.driver: the run asks for the issue key / sprint at runtime when
// enabled, instead of forcing a settings edit for every run.
const contextSourceOptions = [
    { id: "canonical.head", label: "Canonical Head" },
    { id: "feature.history", label: "Feature History" },
    { id: "change.contract", label: "Change Contract" },
    { id: "source.dependence", label: "Source Dependence (impact)" },
    { id: "chat.summary", label: "Chat Summary" },
    { id: "source.excerpt", label: "Source Excerpt" },
    { id: "mcp.driver", label: "MCP Driver (Google Drive)" },
    { id: "jira.issue", label: "Jira Issue" },
    { id: "jira.sprint", label: "Jira Sprint" },
    { id: "firebase.crashlytics", label: "Firebase Crashlytics" },
];
const DEFAULT_MODEL = "gpt-5.4";
const DEFAULT_REASONING = "medium";
const REASONING_OPTIONS = [
    { value: "low", label: "Low" },
    { value: "medium", label: "Medium" },
    { value: "high", label: "High" },
    { value: "xhigh", label: "XHigh" },
];
// The 3 outcome statuses a hub.inline node's flow_control call can actually
// produce (mapped via reviewOutcomeFace() in the runner). A plain
// agent.delegate node only ever completes as "done" - it has no way to
// produce "continue"/"escalate" itself. A native <select> here (not a
// datalist "suggestion" input) avoids a real browser quirk: an <input
// list=...> only filters the datalist to entries matching what's ALREADY
// typed, so once a field held "done", the other two options never showed.
const WORKFLOW_EDGE_WHEN_OPTIONS = ["done", "continue", "escalate"];
function deriveStepPromptBase(input) {
    const title = input.name.trim() || input.stepType.trim() || "workflow step";
    const summary = input.description.trim() || `Execute the ${title} step.`;
    return `Execute the ${title} step.\n\n${summary}`;
}
// Flow/Workflow launches (single-step and multi-step alike) always run YOLO=true — the
// gate/approval/reinvoke machinery a workflow's own hub/cohort orchestration depends on is
// not robust against a paused approval card mid-flow (a stalled/failed child, a missed
// approval render while another child is focused, the hub's own stall-timeout firing while
// a sibling is still legitimately working). YOLO=false is only supported in Normal Chat,
// which has no hub/cohort/gate layer to race against. Every yoloMode default and mapper
// below is pinned to true; the corresponding settings checkboxes are shown checked+disabled
// (see renderStepDefinitionForm / the workflow create+edit forms) rather than removed, so the
// UI still explains why the option is unavailable here.
function createEmptyStepDraft(modelId) {
    return {
        stepType: "",
        name: "",
        description: "",
        promptBase: "",
        requiredMcps: [],
        mcpAccessMode: "read_only",
        requiredSkills: [],
        teamRole: null,
        subagent: null,
        model: null,
        reasoningEffort: DEFAULT_REASONING,
        yoloMode: true,
        agentType: "standard",
        nodeId: null,
        behaviorId: null,
        agentRef: null,
        nodeLifecycle: null,
        joinMode: null,
        cohort: null,
        promptTemplateRef: null,
        contextRef: null,
        contextSources: [],
        artifactBindings: [],
        createdAt: "",
        updatedAt: "",
    };
}
function mapWorkflowToDraft(workflow) {
    if (!workflow)
        return null;
    return {
        id: workflow.id,
        projectId: workflow.projectId,
        name: workflow.name,
        description: workflow.description,
        isTemplate: workflow.isTemplate,
        modelOverride: workflow.modelOverride ?? "",
        reasoningEffortOverride: workflow.reasoningEffortOverride ?? DEFAULT_REASONING,
        // Force true regardless of what is persisted — see the comment above createEmptyStepDraft.
        // A legacy workflow saved with yoloMode=false is normalized the moment it is loaded into
        // the edit draft, so simply opening it (even without touching the checkbox) corrects it
        // on next save.
        yoloMode: true,
        policyCap: workflow.policyCap,
        policyOnCap: workflow.policyOnCap,
        policyExtendBy: workflow.policyExtendBy,
        policyExtendMax: workflow.policyExtendMax,
        edges: workflow.edges,
        acceptanceNodes: workflow.acceptanceNodes ?? [],
    };
}
function createEmptyWorkflowDraft(projects, modelId) {
    return {
        projectId: projects[0]?.id ?? null,
        name: "New Workflow",
        description: "",
        isTemplate: false,
        modelOverride: "",
        reasoningEffortOverride: DEFAULT_REASONING,
        yoloMode: true,
        policyCap: null,
        policyOnCap: null,
        policyExtendBy: null,
        policyExtendMax: null,
        edges: [],
        acceptanceNodes: [],
    };
}
// normalizeWorkflowEdgesSnapshot stabilizes edges for dirty comparison
// (BUG-274: order is non-semantic for save enablement).
function normalizeWorkflowEdgesSnapshot(edges) {
    return [...(edges ?? [])]
        .map((edge) => ({
        from: edge.from ?? "",
        to: edge.to ?? "",
        kind: edge.kind ?? "",
        when: edge.when ?? "",
    }))
        .sort((a, b) => {
        const ak = `${a.from}\0${a.to}\0${a.kind}\0${a.when}`;
        const bk = `${b.from}\0${b.to}\0${b.kind}\0${b.when}`;
        return ak.localeCompare(bk);
    });
}
function normalizeWorkflowSnapshot(draft, steps) {
    if (!draft)
        return "";
    return JSON.stringify({
        projectId: draft.projectId ?? null,
        name: draft.name,
        description: draft.description,
        isTemplate: draft.isTemplate,
        modelOverride: draft.modelOverride ?? "",
        reasoningEffortOverride: draft.reasoningEffortOverride ?? "",
        yoloMode: draft.yoloMode,
        // BUG-274: edges + policy must participate in dirty fingerprint or
        // edges-only edits leave Save Workflow disabled.
        policyCap: draft.policyCap ?? null,
        policyOnCap: draft.policyOnCap ?? null,
        policyExtendBy: draft.policyExtendBy ?? null,
        policyExtendMax: draft.policyExtendMax ?? null,
        edges: normalizeWorkflowEdgesSnapshot(draft.edges),
        // Read-only in this UI (no editor mutates it), included for the same
        // reason edges/policy* are: a dirty-fingerprint that silently ignored a
        // real field of the draft would be quietly wrong if that ever changed.
        acceptanceNodes: [...draft.acceptanceNodes].sort(),
        steps: steps.map((step, index) => ({
            stepType: step.stepType,
            orderIndex: index,
            isEnabled: step.isEnabled,
            requiresApproval: step.requiresApproval,
        })),
    });
}
function normalizeStepSnapshot(step) {
    if (!step)
        return "";
    return JSON.stringify({
        stepType: step.stepType,
        name: step.name,
        description: step.description,
        promptBase: step.promptBase ?? "",
        requiredMcps: [...step.requiredMcps].sort(),
        mcpAccessMode: step.mcpAccessMode,
        requiredSkills: [...step.requiredSkills].sort(),
        teamRole: step.teamRole ?? "",
        subagent: step.subagent ?? "",
        model: step.model,
        reasoningEffort: step.reasoningEffort ?? "",
        yoloMode: step.yoloMode,
        agentType: step.agentType,
        nodeId: step.nodeId ?? "",
        behaviorId: step.behaviorId ?? "",
        agentRef: step.agentRef ?? "",
        nodeLifecycle: step.nodeLifecycle ?? "",
        joinMode: step.joinMode ?? "",
        cohort: step.cohort ?? "",
        promptTemplateRef: step.promptTemplateRef ?? "",
        contextRef: step.contextRef ?? "",
        artifactBindings: [...step.artifactBindings]
            .map((binding) => `${binding.direction}:${binding.artifactInstanceId}:${binding.required}`)
            .sort(),
    });
}
function buildModelOptions(models) {
    const enabledModels = models.filter((model) => model.isEnabled);
    return enabledModels.length > 0
        ? enabledModels.map((model) => ({ value: model.modelId, label: model.displayName }))
        : [{ value: DEFAULT_MODEL, label: DEFAULT_MODEL }];
}
function toggleString(list, value) {
    return list.includes(value) ? list.filter((item) => item !== value) : [...list, value];
}
// A step whose Behavior ID requires an agent (agent.delegate) but has no
// Agent ref set resolves at runtime to flowNodeAgentName(node) === "" — the
// executor logs "entry node has no resolvable agent; skipped" and the node
// never spawns, with no error surfaced to the user anywhere in the UI. This
// mirrors validateFlowGraph's same check (which only ever runs at Save
// Workflow time, and only once a flow has edges) at the point the step
// itself is actually authored, so the gap can't be saved in the first place.
function stepDefinitionAgentRefIssue(behaviorId, agentRef) {
    const requiresAgent = client_core_1.FLOW_BEHAVIOR_OPTIONS.find((option) => option.id === behaviorId)?.requiresAgent ?? false;
    if (requiresAgent && !agentRef?.trim()) {
        return `Behavior "${behaviorId}" requires an Agent ref — set one before saving this step.`;
    }
    return null;
}
// CP-45/SD-23 Task-200 D-7 (simplified v1 type-compat rule): a
// `context.produce` step's only meaningful artifact slot is the
// `context_artifact.v1` instance it outputs (resolveArtifactBoundContextSources
// reads OUTPUT bindings of that type — artifact_type_registry.go); any other
// step consumes/produces non-context artifact types only (e.g.
// `file_artifact.v1` — resolveInputArtifactPrompt explicitly skips
// context_artifact bindings). This keeps the picker from ever offering a
// type-incompatible instance, without needing a fuller per-slot type-contract
// system.
// direction (Task-233): telegram.v1 is an OUTPUT-only "action" artifact — it
// has no meaningful INPUT semantics (there is nothing to read back from a
// sent Telegram message), so it must never appear in an INPUT picker.
function compatibleArtifactInstancesFor(behaviorId, instances, direction = "output") {
    const isContextStep = behaviorId === "context.produce";
    return instances.filter((instance) => {
        if (instance.artifactTypeId === "telegram.v1")
            return direction === "output";
        return isContextStep ? instance.artifactTypeId === "context_artifact.v1" : instance.artifactTypeId !== "context_artifact.v1";
    });
}
function artifactInstanceLabel(instance, id) {
    if (!instance)
        return id;
    return instance.isBuiltin ? `${instance.name} (built-in)` : instance.name;
}
function toTitleCase(value) {
    return value
        .split("_")
        .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
        .join(" ");
}
// CP-45/SD-23 Task-199: the Flow settings "Artifacts" tab. Types are a
// read-only catalog (system-owned, Task-197); instances split into built-in
// (isBuiltin=true, seeded, read-only — shown with a "built-in" tag,
// mirroring how a built-in Workflow is read-only) and user-authored
// (isBuiltin=false, full CRUD here). Step binding happens in the Steps tab
// (Task-200), not here — this page only authors reusable instances.
function ArtifactsTabContent(props) {
    const { artifactTypes, artifactInstances, artifactInstanceView, artifactInstanceDraft, busy, onCreateNew, onSelect, onSave, onDelete, setArtifactInstanceView, setArtifactInstanceDraft, } = props;
    const draftType = artifactTypes.find((type) => type.id === artifactInstanceDraft?.artifactTypeId);
    const isFileType = artifactInstanceDraft?.artifactTypeId === "file_artifact.v1";
    const isContextType = artifactInstanceDraft?.artifactTypeId === "context_artifact.v1";
    const isTelegramType = artifactInstanceDraft?.artifactTypeId === "telegram.v1";
    const draftTelegramChatId = isTelegramType ? (artifactInstanceDraft?.configJson.chatId ?? "") : "";
    const draftTelegramTemplate = isTelegramType
        ? (artifactInstanceDraft?.configJson.messageTemplate ?? "")
        : "";
    const draftPaths = isFileType
        ? (artifactInstanceDraft?.configJson.paths ?? []).join("\n")
        : "";
    const draftConfig = artifactInstanceDraft?.configJson;
    const structureEnabled = isFileType && (0, fileArtifactConfig_1.isFileArtifactStructureEnabled)(draftConfig);
    const draftFileStructure = isFileType ? (0, fileArtifactConfig_1.parseFileArtifactStructure)(draftConfig) : null;
    // When structure is off, keep sections text empty so enabling does not
    // silently inject coding-memo titles (defaults apply on Save or preset button).
    const draftStructureSections = structureEnabled
        ? (0, fileArtifactConfig_1.sectionsToTextarea)(draftFileStructure?.sections ?? [])
        : "";
    const draftSources = isContextType
        ? (artifactInstanceDraft?.configJson.sources ?? [])
        : [];
    if (artifactInstanceView === "create" && artifactInstanceDraft) {
        return ((0, jsx_runtime_1.jsxs)("div", { className: "settings-subpanel", children: [(0, jsx_runtime_1.jsx)("div", { className: "settings-actions", children: (0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", onClick: () => {
                            setArtifactInstanceView("list");
                            setArtifactInstanceDraft(null);
                        }, type: "button", children: "\u2190 Back" }) }), (0, jsx_runtime_1.jsx)("h3", { children: artifactInstanceDraft.id ? "Edit Artifact Instance" : "New Artifact Instance" }), artifactInstanceDraft.isBuiltin ? ((0, jsx_runtime_1.jsxs)("div", { className: "artifact-info-panel", children: [(0, jsx_runtime_1.jsx)("strong", { children: "Built-in artifact instance" }), (0, jsx_runtime_1.jsx)("p", { children: "Built-in instances are seeded by the system and cannot be edited here." })] })) : null, (0, jsx_runtime_1.jsxs)("div", { className: "settings-grid", children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Artifact Type" }), (0, jsx_runtime_1.jsx)("input", { disabled: true, value: draftType?.id ?? artifactInstanceDraft.artifactTypeId })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Name" }), (0, jsx_runtime_1.jsx)("input", { disabled: artifactInstanceDraft.isBuiltin, onChange: (event) => setArtifactInstanceDraft({ ...artifactInstanceDraft, name: event.target.value }), value: artifactInstanceDraft.name })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("span", { children: "Description" }), (0, jsx_runtime_1.jsx)("textarea", { disabled: artifactInstanceDraft.isBuiltin, onChange: (event) => setArtifactInstanceDraft({ ...artifactInstanceDraft, description: event.target.value }), value: artifactInstanceDraft.description })] }), isContextType ? ((0, jsx_runtime_1.jsxs)("div", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("span", { children: "Context Sources" }), (0, jsx_runtime_1.jsxs)("small", { className: "settings-field-help", children: ["Primary authoring path (CP-45): sources live on this ", (0, jsx_runtime_1.jsx)("code", { children: "context_artifact.v1" }), " instance. Bind the instance on a ", (0, jsx_runtime_1.jsx)("code", { children: "context.produce" }), " step as Artifact Output \u2014 do not use raw Step-level Context Sources chips."] }), (0, jsx_runtime_1.jsx)("div", { className: "workflow-chips", children: contextSourceOptions.map((option) => {
                                        const checked = draftSources.includes(option.id);
                                        return ((0, jsx_runtime_1.jsxs)("label", { className: "settings-checkbox", children: [(0, jsx_runtime_1.jsx)("input", { checked: checked, disabled: artifactInstanceDraft.isBuiltin, onChange: () => setArtifactInstanceDraft({
                                                        ...artifactInstanceDraft,
                                                        configJson: { ...artifactInstanceDraft.configJson, sources: toggleString(draftSources, option.id) },
                                                    }), type: "checkbox" }), (0, jsx_runtime_1.jsx)("span", { children: option.label })] }, option.id));
                                    }) }), draftSources.includes("mcp.driver") ? ((0, jsx_runtime_1.jsx)("small", { children: "Uses the Google Drive account selected in Google Drive setup. The file URL or ID is chosen at runtime." })) : null, draftSources.includes("jira.issue") || draftSources.includes("jira.sprint") ? ((0, jsx_runtime_1.jsx)("small", { children: "Uses the Jira MCP connected in MCP Servers settings. The issue key or sprint is chosen at runtime \u2014 the run asks before it starts, and the AI reads only the selected issue/sprint via the Jira MCP tools." })) : null, draftSources.includes("firebase.crashlytics") ? ((0, jsx_runtime_1.jsx)("small", { children: "Uses the Firebase MCP connected in MCP Servers settings. The crash issue id is chosen at runtime \u2014 the run asks before it starts, and the AI reads only the selected crash via the Firebase Crashlytics MCP tools." })) : null] })) : null, isFileType ? ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("span", { children: "File Paths (one per line, workspace-relative)" }), (0, jsx_runtime_1.jsxs)("small", { className: "settings-field-help", children: ["Workspace-relative path(s) this instance designates (Task-223 / BUG-276).", " ", (0, jsx_runtime_1.jsx)("strong", { children: "Output" }), " = write contract (agent must create/update these files; gate reprompts if missing). ", (0, jsx_runtime_1.jsx)("strong", { children: "Input" }), " = path mention only \u2014 agent is told to open/read those paths with tools; file body is not pasted into the prompt (unlike context packages)."] }), (0, jsx_runtime_1.jsx)("textarea", { disabled: artifactInstanceDraft.isBuiltin, onChange: (event) => setArtifactInstanceDraft({
                                                ...artifactInstanceDraft,
                                                configJson: {
                                                    ...artifactInstanceDraft.configJson,
                                                    paths: event.target.value.split("\n").map((line) => line.trim()).filter(Boolean),
                                                },
                                            }), placeholder: "docs/coder-summary.md", value: draftPaths })] }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-field-full", style: { display: "grid", gap: "8px" }, children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-checkbox", children: [(0, jsx_runtime_1.jsx)("input", { checked: structureEnabled, disabled: artifactInstanceDraft.isBuiltin, onChange: (event) => {
                                                        const enabled = event.target.checked;
                                                        setArtifactInstanceDraft({
                                                            ...artifactInstanceDraft,
                                                            configJson: (0, fileArtifactConfig_1.withFileArtifactStructure)(artifactInstanceDraft.configJson, enabled, 
                                                            // Enable with empty sections; Save or "Use coding memo" fills defaults.
                                                            "", { fillDefaultWhenEmpty: false }),
                                                        });
                                                    }, type: "checkbox" }), (0, jsx_runtime_1.jsx)("span", { children: "Require markdown sections (OUTPUT template + gate)" })] }), (0, jsx_runtime_1.jsxs)("small", { className: "settings-field-help", children: ["Task-225: optional per-instance structure. When enabled and this instance is bound as a", " ", (0, jsx_runtime_1.jsx)("strong", { children: "required Output" }), ", the agent prompt lists these section titles and the flow gate checks they exist as ATX headings after the file is written (flexible level/casing). Leave off for testing/planning/raw notes (existence-only). Does not affect Input path-only behavior."] }), structureEnabled ? ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("span", { children: "Section titles (one per line)" }), (0, jsx_runtime_1.jsx)("textarea", { disabled: artifactInstanceDraft.isBuiltin, onChange: (event) => setArtifactInstanceDraft({
                                                                ...artifactInstanceDraft,
                                                                configJson: (0, fileArtifactConfig_1.withFileArtifactStructure)(artifactInstanceDraft.configJson, true, event.target.value),
                                                            }), placeholder: "What\nWhy\nBaseline", rows: 4, value: draftStructureSections })] }), !artifactInstanceDraft.isBuiltin ? ((0, jsx_runtime_1.jsx)("div", { className: "settings-actions", children: (0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", onClick: () => setArtifactInstanceDraft({
                                                            ...artifactInstanceDraft,
                                                            configJson: (0, fileArtifactConfig_1.withFileArtifactStructure)(artifactInstanceDraft.configJson, true, (0, fileArtifactConfig_1.sectionsToTextarea)([...fileArtifactConfig_1.DEFAULT_CODING_MEMO_SECTIONS])),
                                                        }), type: "button", children: "Use coding memo (What / Why / Baseline)" }) })) : null] })) : null] })] })) : null, isTelegramType ? ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("span", { children: "Telegram Chat/Channel ID" }), (0, jsx_runtime_1.jsxs)("small", { className: "settings-field-help", children: ["The Telegram chat or channel id this instance sends to. Bind this instance as a step's", " ", (0, jsx_runtime_1.jsx)("strong", { children: "Output only" }), " (Task-233) \u2014 the bound step must send a notification here via the Telegram MCP tool before finishing; the flow gate verifies a real message was sent."] }), (0, jsx_runtime_1.jsx)("input", { disabled: artifactInstanceDraft.isBuiltin, onChange: (event) => setArtifactInstanceDraft({
                                                ...artifactInstanceDraft,
                                                configJson: { ...artifactInstanceDraft.configJson, chatId: event.target.value.trim() },
                                            }), placeholder: "-100123456789", value: draftTelegramChatId })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("span", { children: "Message Template (optional)" }), (0, jsx_runtime_1.jsx)("small", { className: "settings-field-help", children: "Optional template for the notification text (CP-05-05 Q-5). Leave blank to let the agent summarize the run's outcome freely." }), (0, jsx_runtime_1.jsx)("textarea", { disabled: artifactInstanceDraft.isBuiltin, onChange: (event) => setArtifactInstanceDraft({
                                                ...artifactInstanceDraft,
                                                configJson: { ...artifactInstanceDraft.configJson, messageTemplate: event.target.value },
                                            }), placeholder: "Run finished: {{status}}\nSummary: ...", rows: 3, value: draftTelegramTemplate })] })] })) : null] }), !artifactInstanceDraft.isBuiltin ? ((0, jsx_runtime_1.jsx)("div", { className: "settings-actions", children: (0, jsx_runtime_1.jsx)("button", { className: "primary-btn", disabled: busy || !artifactInstanceDraft.name.trim(), onClick: onSave, type: "button", children: "Save Artifact Instance" }) })) : null] }));
    }
    return ((0, jsx_runtime_1.jsxs)("div", { className: "settings-two-column", children: [(0, jsx_runtime_1.jsxs)("div", { className: "settings-subpanel", children: [(0, jsx_runtime_1.jsx)("h3", { children: "Artifact Types (built-in catalog)" }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-list", children: [artifactTypes.map((type) => ((0, jsx_runtime_1.jsxs)("div", { className: "settings-list-item static", children: [(0, jsx_runtime_1.jsx)("strong", { children: type.id }), (0, jsx_runtime_1.jsxs)("span", { children: [type.category, " \u00B7 ", type.status] }), type.id === "file_artifact.v1" ? ((0, jsx_runtime_1.jsxs)("small", { children: ["Path-designated file artifact: bind as ", (0, jsx_runtime_1.jsx)("strong", { children: "Output" }), " so the step must write those path(s); bind as ", (0, jsx_runtime_1.jsx)("strong", { children: "Input" }), " so the step reads them (e.g. coder \u2192 review). Optional", " ", (0, jsx_runtime_1.jsx)("strong", { children: "markdown sections" }), " on the instance configure OUTPUT template + structure gate (Task-225)."] })) : null, type.id === "context_artifact.v1" ? ((0, jsx_runtime_1.jsx)("small", { children: "Context package producer: configure sources on the instance, then bind as step Artifact I/O." })) : null, type.id === "telegram.v1" ? ((0, jsx_runtime_1.jsxs)("small", { children: ["Send-only notification action (Task-233): bind as a step's ", (0, jsx_runtime_1.jsx)("strong", { children: "Output only" }), " \u2014 the step must send a Telegram message via the MCP tool before finishing; the flow gate verifies a real message_id, not a file."] })) : null] }, type.id))), artifactTypes.length === 0 ? (0, jsx_runtime_1.jsx)("div", { className: "settings-list-empty", children: "No artifact types." }) : null] }), (0, jsx_runtime_1.jsx)("div", { className: "settings-actions", style: { marginTop: "12px" }, children: artifactTypes.map((type) => ((0, jsx_runtime_1.jsxs)("button", { className: "secondary-btn", onClick: () => onCreateNew(type.id), type: "button", children: ["+ New ", type.id, " instance"] }, type.id))) })] }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-subpanel", children: [(0, jsx_runtime_1.jsx)("h3", { children: "Artifact Instances" }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-list", children: [artifactInstances.map((instance) => ((0, jsx_runtime_1.jsxs)("button", { className: "settings-list-item", onClick: () => onSelect(instance), type: "button", children: [(0, jsx_runtime_1.jsxs)("strong", { children: [instance.name, instance.isBuiltin ? " (built-in)" : ""] }), (0, jsx_runtime_1.jsx)("span", { children: instance.artifactTypeId }), !instance.isBuiltin ? ((0, jsx_runtime_1.jsx)("button", { "aria-label": `Delete ${instance.name}`, className: "workflow-chip-remove", onClick: (event) => {
                                            event.stopPropagation();
                                            void onDelete(instance.id);
                                        }, type: "button", children: "\u00D7" })) : null] }, instance.id))), artifactInstances.length === 0 ? (0, jsx_runtime_1.jsx)("div", { className: "settings-list-empty", children: "No artifact instances yet." }) : null] })] })] }));
}
function WorkflowsSettings() {
    const [tab, setTab] = (0, react_1.useState)("workflows");
    const [workflowView, setWorkflowView] = (0, react_1.useState)("list");
    const [stepView, setStepView] = (0, react_1.useState)("list");
    const [projects, setProjects] = (0, react_1.useState)([]);
    const [models, setModels] = (0, react_1.useState)([]);
    const [workflows, setWorkflows] = (0, react_1.useState)([]);
    const [workflowSteps, setWorkflowSteps] = (0, react_1.useState)([]);
    const [stepDefinitions, setStepDefinitions] = (0, react_1.useState)([]);
    // CP-45/SD-23: the generic typed-artifact framework's catalog + instances.
    // artifactTypes is read-only (system-owned, Task-197);
    // artifactInstances mixes built-in (isBuiltin=true, seeded, read-only) and
    // user-authored rows, managed from the new "Artifacts" tab (Task-199).
    const [artifactTypes, setArtifactTypes] = (0, react_1.useState)([]);
    const [artifactInstances, setArtifactInstances] = (0, react_1.useState)([]);
    const [artifactInstanceDraft, setArtifactInstanceDraft] = (0, react_1.useState)(null);
    const [artifactInstanceView, setArtifactInstanceView] = (0, react_1.useState)("list");
    // Built-in workflows are read-only (see workflowDetailReadOnly below) and
    // can never be edited to drop a step, so a step definition still listed by
    // any built-in workflow can't be deleted either — doing so would leave
    // that (undeletable, uneditable) workflow with a dangling stepType it can
    // never resolve at runtime. Recomputed on every refresh() from the small
    // set of built-in workflows' own step lists.
    const [stepTypesUsedByBuiltin, setStepTypesUsedByBuiltin] = (0, react_1.useState)(new Set());
    const [selectedWorkflowId, setSelectedWorkflowId] = (0, react_1.useState)("");
    const [selectedStepType, setSelectedStepType] = (0, react_1.useState)("");
    const [detailWorkflowStepType, setDetailWorkflowStepType] = (0, react_1.useState)("");
    const [workflowDraft, setWorkflowDraft] = (0, react_1.useState)(null);
    const [workflowInitialSnapshot, setWorkflowInitialSnapshot] = (0, react_1.useState)("");
    const [stepDraft, setStepDraft] = (0, react_1.useState)(null);
    const [stepInitialSnapshot, setStepInitialSnapshot] = (0, react_1.useState)("");
    const [createWorkflowDraft, setCreateWorkflowDraft] = (0, react_1.useState)(() => createEmptyWorkflowDraft([], DEFAULT_MODEL));
    const [createWorkflowSteps, setCreateWorkflowSteps] = (0, react_1.useState)([]);
    const [createWorkflowStepType, setCreateWorkflowStepType] = (0, react_1.useState)("");
    const [createStepDraft, setCreateStepDraft] = (0, react_1.useState)(() => createEmptyStepDraft(DEFAULT_MODEL));
    // Owner finding (2026-07-06): step-definitions are edited in the Steps tab,
    // independent of any one workflow, so there's no workflow.projectId to
    // resolve a real target-project cwd from automatically. This lets the user
    // explicitly pick which project's agents to preview/select for Agent ref —
    // "" (Workspace global) loads only built-in + provider-home agents (no
    // project-local .claude/agents, since there's no fixed cwd for those).
    // Whatever gets picked here is a real, working agent at runtime: Go's
    // spawnChildRun falls back to agentCatalog.listAgents(cwd) — cwd = the
    // RUN's own workspaceCwd at that time — whenever resolvePackAgentDefinition
    // (built-ins only) doesn't recognize the name, so a project-local agent
    // works as long as the flow actually runs against that same project later.
    const [agentPreviewProjectId, setAgentPreviewProjectId] = (0, react_1.useState)("");
    const [agentOptions, setAgentOptions] = (0, react_1.useState)([]);
    const [busy, setBusy] = (0, react_1.useState)(false);
    const [message, setMessage] = (0, react_1.useState)(null);
    const [deleteTarget, setDeleteTarget] = (0, react_1.useState)(null);
    const [deleteConfirmationText, setDeleteConfirmationText] = (0, react_1.useState)("");
    const [pickerModal, setPickerModal] = (0, react_1.useState)(null);
    // Multi-select delete: entered via long-press on a Workflow/Step Registry
    // row instead of a mode toggle. `bulkSelect` is null outside select mode;
    // once set, every row in that same list renders a checkbox and a tap
    // toggles selection instead of opening the row for edit.
    const [bulkSelect, setBulkSelect] = (0, react_1.useState)(null);
    const longPressTimerRef = (0, react_1.useRef)(null);
    const longPressFiredRef = (0, react_1.useRef)(false);
    // Built-in flow cloning (CP-42/Task-179): a built-in (isBuiltin=true)
    // workflow is read-only in this screen — cloneTarget drives the "name your
    // copy" modal that creates an editable, non-builtin copy.
    const [cloneTarget, setCloneTarget] = (0, react_1.useState)(null);
    const [cloneName, setCloneName] = (0, react_1.useState)("");
    // Workflow step cards default to collapsed; expansion is tracked per
    // source+step id so detail and create editors don't share state.
    const [expandedStepKeys, setExpandedStepKeys] = (0, react_1.useState)(new Set());
    // Task-189 slice 4: the visual canvas is a second view over the SAME
    // `edges` array as the form-list editor (no separate edge state — that's
    // what keeps the two in sync). Node x/y is purely a view-layout concern:
    // it is never sent to saveWorkflow/saveNewWorkflow and has no backing
    // column, so it can't violate the "node data lives only in
    // step_definitions" contract — it just remembers where a node box was
    // last dragged to, keyed by `${source}:${nodeKey}`.
    const [canvasPositions, setCanvasPositions] = (0, react_1.useState)({});
    const [draggingCanvasNode, setDraggingCanvasNode] = (0, react_1.useState)(null);
    const [pendingCanvasEdgeFrom, setPendingCanvasEdgeFrom] = (0, react_1.useState)(null);
    const toggleStepExpanded = (key) => {
        setExpandedStepKeys((current) => {
            const next = new Set(current);
            if (next.has(key)) {
                next.delete(key);
            }
            else {
                next.add(key);
            }
            return next;
        });
    };
    const clearLongPressTimer = () => {
        if (longPressTimerRef.current !== null) {
            window.clearTimeout(longPressTimerRef.current);
            longPressTimerRef.current = null;
        }
    };
    // Long-press (works for touch and mouse via Pointer Events) enters select
    // mode and selects the pressed row; a short tap afterwards falls through
    // to the row's normal click handler. `longPressFiredRef` lets that click
    // handler tell the two apart, since a pointerup after a fired long-press
    // still dispatches a click. If select mode is already active, long-pressing
    // another row adds it to the existing selection instead of resetting it —
    // once inside select mode a plain tap already toggles rows one at a time,
    // but a user may still long-press out of habit and shouldn't lose progress.
    const startLongPress = (kind, id) => {
        clearLongPressTimer();
        longPressFiredRef.current = false;
        longPressTimerRef.current = window.setTimeout(() => {
            longPressFiredRef.current = true;
            setBulkSelect((current) => {
                if (current && current.kind === kind) {
                    const next = new Set(current.ids);
                    next.add(id);
                    return { ...current, ids: next };
                }
                return { kind, ids: new Set([id]) };
            });
        }, 500);
    };
    const consumeLongPressClick = () => {
        if (!longPressFiredRef.current)
            return false;
        longPressFiredRef.current = false;
        return true;
    };
    const toggleBulkSelected = (id) => {
        setBulkSelect((current) => {
            if (!current)
                return current;
            const next = new Set(current.ids);
            if (next.has(id)) {
                next.delete(id);
            }
            else {
                next.add(id);
            }
            return { ...current, ids: next };
        });
    };
    const exitBulkSelect = () => setBulkSelect(null);
    const selectedWorkflow = (0, react_1.useMemo)(() => workflows.find((item) => item.id === selectedWorkflowId) ?? null, [workflows, selectedWorkflowId]);
    // BUG-NOTE-CP42 #8: the detail copy already says a built-in can't be
    // edited and Save/Delete are hidden for it, but the actual form fields
    // below had no disabled binding at all — a user could type changes into a
    // built-in workflow's Name/Description/Model/etc. that could never be
    // saved, undercutting the read-only UX the copy promises.
    const workflowDetailReadOnly = selectedWorkflow?.editable === false;
    const selectedStep = (0, react_1.useMemo)(() => stepDefinitions.find((item) => item.stepType === selectedStepType) ?? null, [stepDefinitions, selectedStepType]);
    const modelOptions = (0, react_1.useMemo)(() => buildModelOptions(models), [models]);
    const availableCreateWorkflowSteps = (0, react_1.useMemo)(() => stepDefinitions.filter((definition) => !createWorkflowSteps.some((step) => step.stepType === definition.stepType)), [createWorkflowSteps, stepDefinitions]);
    const workflowDirty = normalizeWorkflowSnapshot(workflowDraft, workflowSteps) !== workflowInitialSnapshot;
    const stepDirty = normalizeStepSnapshot(stepDraft) !== stepInitialSnapshot;
    const syncWorkflowSelection = (workflowId, nextWorkflows = workflows, nextWorkflowSteps, nextStepDefinitions = stepDefinitions) => {
        setSelectedWorkflowId(workflowId);
        const nextSelectedWorkflow = nextWorkflows.find((item) => item.id === workflowId) ?? null;
        const nextDetailWorkflowStepType = nextStepDefinitions.find((definition) => !nextWorkflowSteps.some((step) => step.stepType === definition.stepType))?.stepType ?? "";
        setDetailWorkflowStepType(nextDetailWorkflowStepType);
        setWorkflowSteps(nextWorkflowSteps);
        const nextWorkflowDraft = mapWorkflowToDraft(nextSelectedWorkflow);
        setWorkflowDraft(nextWorkflowDraft);
        setWorkflowInitialSnapshot(normalizeWorkflowSnapshot(nextWorkflowDraft, nextWorkflowSteps));
    };
    const selectStepDefinition = (stepType, definitions = stepDefinitions) => {
        setSelectedStepType(stepType);
        const nextSelectedStep = definitions.find((item) => item.stepType === stepType) ?? null;
        // Force true regardless of what is persisted — see the comment above createEmptyStepDraft.
        const nextStepDraft = nextSelectedStep ? { ...nextSelectedStep, yoloMode: true } : null;
        setStepDraft(nextStepDraft);
        setStepInitialSnapshot(normalizeStepSnapshot(nextStepDraft));
    };
    const selectWorkflowDefinition = async (workflowId) => {
        const admin = await (0, clientCore_1.getAdminUseCases)();
        const nextWorkflowSteps = workflowId
            ? await admin.workflows.listWorkflowSteps(workflowId)
            : [];
        syncWorkflowSelection(workflowId, workflows, nextWorkflowSteps, stepDefinitions);
    };
    const refresh = async (workflowId, stepType, options) => {
        try {
            const admin = await (0, clientCore_1.getAdminUseCases)();
            const [nextProjects, nextWorkflows, nextStepDefinitions, nextModels, nextArtifactTypes, nextArtifactInstances,] = await Promise.all([
                admin.projects.listProjects(),
                admin.workflows.listWorkflows(),
                admin.workflows.listStepDefinitions(),
                admin.providers.listSupportedModels(),
                admin.workflows.listArtifactTypes(),
                admin.workflows.listArtifactInstances(),
            ]);
            const enabledModels = nextModels.filter((model) => model.isEnabled);
            const defaultModel = enabledModels[0]?.modelId ?? DEFAULT_MODEL;
            const resolvedWorkflowId = workflowId && nextWorkflows.some((item) => item.id === workflowId)
                ? workflowId
                : nextWorkflows[0]?.id ?? "";
            // A cloned workflow's own steps carry a freshly namespaced step_type
            // (BUG-262), distinct from the source's. A caller (e.g. cloneSelected)
            // may still pass the PREVIOUSLY selected step's type, left over from
            // before the switch — validating that against the full cross-workflow
            // catalog (nextStepDefinitions) let a stale, unrelated-workflow
            // step_type pass through as "valid" whenever it happened to also exist
            // elsewhere in the catalog, silently loading (and, on Save, mutating)
            // that OTHER workflow's step instead of one actually belonging to
            // resolvedWorkflowId. Must validate against this workflow's own steps.
            const nextWorkflowSteps = resolvedWorkflowId
                ? await admin.workflows.listWorkflowSteps(resolvedWorkflowId)
                : [];
            const resolvedStepType = stepType && nextWorkflowSteps.some((item) => item.stepType === stepType)
                ? stepType
                : nextWorkflowSteps[0]?.stepType ?? "";
            const builtinWorkflows = nextWorkflows.filter((workflow) => workflow.isBuiltin);
            const builtinStepLists = await Promise.all(builtinWorkflows.map((workflow) => admin.workflows.listWorkflowSteps(workflow.id)));
            const nextStepTypesUsedByBuiltin = new Set(builtinStepLists.flat().map((step) => step.stepType));
            setProjects(nextProjects);
            setWorkflows(nextWorkflows);
            setStepDefinitions(nextStepDefinitions);
            setModels(enabledModels);
            setArtifactTypes(nextArtifactTypes);
            setArtifactInstances(nextArtifactInstances);
            setStepTypesUsedByBuiltin(nextStepTypesUsedByBuiltin);
            if (!options?.preserveCreateDrafts) {
                setCreateWorkflowDraft(createEmptyWorkflowDraft(nextProjects, defaultModel));
                setCreateWorkflowSteps([]);
                setCreateStepDraft(createEmptyStepDraft(defaultModel));
            }
            syncWorkflowSelection(resolvedWorkflowId, nextWorkflows, nextWorkflowSteps, nextStepDefinitions);
            selectStepDefinition(resolvedStepType, nextStepDefinitions);
        }
        catch (error) {
            setMessage((0, settingsHelpers_1.toErrorMessage)(error, "Unable to load workflows and step definitions."));
        }
    };
    (0, react_1.useEffect)(() => {
        void refresh();
    }, []);
    (0, react_1.useEffect)(() => {
        const cwd = projects.find((project) => project.id === agentPreviewProjectId)?.directoryPath ?? undefined;
        let cancelled = false;
        const client = (0, createRunnerClient_1.createRunnerClient)();
        if (!client.listAgents) {
            setAgentOptions([]);
            return;
        }
        client
            .listAgents(cwd)
            .then((agents) => {
            if (!cancelled)
                setAgentOptions(agents);
        })
            .catch(() => {
            if (!cancelled)
                setAgentOptions([]);
        });
        return () => {
            cancelled = true;
        };
    }, [agentPreviewProjectId, projects]);
    (0, react_1.useEffect)(() => {
        if (workflowView === "list" &&
            tab === "workflows") {
            const availableDetailStepTypes = stepDefinitions.filter((definition) => !workflowSteps.some((step) => step.stepType === definition.stepType));
            if (availableDetailStepTypes.length > 0 &&
                !availableDetailStepTypes.some((item) => item.stepType === detailWorkflowStepType)) {
                setDetailWorkflowStepType(availableDetailStepTypes[0].stepType);
            }
            if (availableDetailStepTypes.length === 0 && detailWorkflowStepType !== "") {
                setDetailWorkflowStepType("");
            }
        }
    }, [detailWorkflowStepType, stepDefinitions, tab, workflowSteps, workflowView]);
    (0, react_1.useEffect)(() => {
        if (workflowView === "create" &&
            availableCreateWorkflowSteps.length > 0 &&
            !availableCreateWorkflowSteps.some((item) => item.stepType === createWorkflowStepType)) {
            setCreateWorkflowStepType(availableCreateWorkflowSteps[0].stepType);
        }
        if (workflowView === "create" && availableCreateWorkflowSteps.length === 0) {
            setCreateWorkflowStepType("");
        }
    }, [availableCreateWorkflowSteps, createWorkflowStepType, workflowView]);
    const startCreateWorkflow = () => {
        setCreateWorkflowDraft(createEmptyWorkflowDraft(projects, ""));
        setCreateWorkflowSteps([]);
        setCreateWorkflowStepType(stepDefinitions[0]?.stepType ?? "");
        setWorkflowView("create");
        setMessage(null);
    };
    const startCreateStep = () => {
        setCreateStepDraft(createEmptyStepDraft(""));
        setStepView("create");
        setMessage(null);
    };
    const updateWorkflowStep = (index, patch) => {
        setWorkflowSteps((current) => current.map((item, currentIndex) => currentIndex === index ? { ...item, ...patch } : item));
    };
    const updateCreateWorkflowStep = (index, patch) => {
        setCreateWorkflowSteps((current) => current.map((item, currentIndex) => currentIndex === index ? { ...item, ...patch } : item));
    };
    const moveWorkflowStep = (index, direction, source) => {
        const current = source === "detail" ? workflowSteps : createWorkflowSteps;
        const nextIndex = direction === "up" ? index - 1 : index + 1;
        if (nextIndex < 0 || nextIndex >= current.length)
            return;
        const updated = [...current];
        const temp = updated[index];
        updated[index] = updated[nextIndex];
        updated[nextIndex] = temp;
        const normalized = updated.map((step, orderIndex) => ({ ...step, orderIndex }));
        if (source === "detail") {
            setWorkflowSteps(normalized);
        }
        else {
            setCreateWorkflowSteps(normalized);
        }
    };
    const removeWorkflowStep = (index, source) => {
        const next = (source === "detail" ? workflowSteps : createWorkflowSteps)
            .filter((_, currentIndex) => currentIndex !== index)
            .map((step, orderIndex) => ({ ...step, orderIndex }));
        if (source === "detail") {
            setWorkflowSteps(next);
        }
        else {
            setCreateWorkflowSteps(next);
        }
    };
    const addWorkflowStep = (source) => {
        const chosenStepType = source === "detail"
            ? detailWorkflowStepType
            : createWorkflowStepType;
        if (!chosenStepType)
            return;
        const target = source === "detail" ? workflowSteps : createWorkflowSteps;
        const nextStep = {
            id: `${chosenStepType}-${Date.now()}`,
            workflowId: source === "detail" ? selectedWorkflowId : "__new__",
            stepType: chosenStepType,
            orderIndex: target.length,
            isEnabled: true,
            requiresApproval: true,
            createdAt: "",
            updatedAt: "",
        };
        if (source === "detail") {
            setWorkflowSteps((current) => [...current, nextStep]);
        }
        else {
            setCreateWorkflowSteps((current) => [...current, nextStep]);
        }
    };
    // Task-189: an edge's from/to references a node's `nodeId` (the flow-graph
    // identity resolved onto FlowNode.ID at runtime — recordFromWorkflowRow in
    // supabase_workflow_flow_store.go only sets it when the step_definition's
    // node_id is non-empty, with NO fallback to step_type), not the step's
    // `stepType`. A step whose step_definition has no Node ID set can't be
    // referenced by an edge yet; it's filtered out here and flagged by the
    // edges-editor validation (Task-189 slice 3) instead of silently producing
    // an edge that can never resolve to a real node.
    const workflowEdgeNodeOptions = (steps) => {
        const ids = steps
            .map((step) => stepDefinitions.find((definition) => definition.stepType === step.stepType)?.nodeId)
            .filter((id) => Boolean(id));
        return Array.from(new Set(ids));
    };
    // BUG-282: a step's flow dependency is no longer persisted anywhere on the
    // step definition — topology lives only on the workflow's own edges
    // (workflows.edges_json), and the Go runner derives each node's dependency
    // from those edges at flow-load time (forwardEdgeSources, flow_executor.go).
    // There is therefore nothing edge-derived to write back onto step_definitions
    // when a workflow saves, so the former persistEdgeDerivedDependsOn /
    // computeDependsOnByStepType helpers (and their BUG-262 shared-step guard)
    // are gone; a step reused across flows can no longer carry another flow's
    // node ids because it carries no topology at all.
    // Task-189: the flow graph's edges (Workflow.edges/edges_json) were already
    // round-tripped by saveWorkflow/saveNewWorkflow (BUG-NOTE-CP42 #14 passes
    // them through unchanged), but no UI control ever wrote to them — a new
    // workflow always saved with edges: [] and could never advance past its
    // entry node. These three helpers are the edges-editor equivalent of
    // addWorkflowStep/updateWorkflowStep/removeWorkflowStep above.
    const addWorkflowEdge = (source) => {
        const nodeOptions = workflowEdgeNodeOptions(source === "detail" ? workflowSteps : createWorkflowSteps);
        const nextEdge = {
            from: nodeOptions[0] ?? "",
            to: nodeOptions[1] ?? client_core_1.FLOW_EDGE_TERMINALS[0],
            when: "done",
            kind: "forward",
        };
        if (source === "detail") {
            setWorkflowDraft((current) => (current ? { ...current, edges: [...current.edges, nextEdge] } : current));
        }
        else {
            setCreateWorkflowDraft((current) => ({ ...current, edges: [...current.edges, nextEdge] }));
        }
    };
    const updateWorkflowEdge = (index, patch, source) => {
        const apply = (edges) => edges.map((edge, currentIndex) => (currentIndex === index ? { ...edge, ...patch } : edge));
        if (source === "detail") {
            setWorkflowDraft((current) => (current ? { ...current, edges: apply(current.edges) } : current));
        }
        else {
            setCreateWorkflowDraft((current) => ({ ...current, edges: apply(current.edges) }));
        }
    };
    const removeWorkflowEdge = (index, source) => {
        const apply = (edges) => edges.filter((_, currentIndex) => currentIndex !== index);
        if (source === "detail") {
            setWorkflowDraft((current) => (current ? { ...current, edges: apply(current.edges) } : current));
        }
        else {
            setCreateWorkflowDraft((current) => ({ ...current, edges: apply(current.edges) }));
        }
    };
    // Task-189 slice 4: canvas equivalent of addWorkflowEdge, but with an
    // explicit from/to (the two nodes the user connected by clicking their
    // connector dots in sequence) instead of defaulting to the first two node
    // options.
    const addWorkflowEdgeBetween = (from, to, source) => {
        const nextEdge = { from, to, when: "done", kind: "forward" };
        if (source === "detail") {
            setWorkflowDraft((current) => (current ? { ...current, edges: [...current.edges, nextEdge] } : current));
        }
        else {
            setCreateWorkflowDraft((current) => ({ ...current, edges: [...current.edges, nextEdge] }));
        }
    };
    // Default grid layout for any canvas node that hasn't been manually
    // dragged yet, keyed by `${source}:${nodeKey}` so detail/create canvases
    // (and re-renders after adding/removing nodes) never collide.
    const canvasLayoutFor = (source, keys) => {
        const layout = {};
        keys.forEach((key, index) => {
            const stored = canvasPositions[`${source}:${key}`];
            if (stored) {
                layout[key] = stored;
                return;
            }
            const column = index % 3;
            const row = Math.floor(index / 3);
            layout[key] = { x: 70 + column * 190, y: 50 + row * 110 };
        });
        return layout;
    };
    const handleCanvasNodePointerDown = (source, key) => (event) => {
        event.preventDefault();
        event.currentTarget.setPointerCapture(event.pointerId);
        setDraggingCanvasNode({ source, key });
    };
    const handleCanvasSurfacePointerMove = (source) => (event) => {
        if (!draggingCanvasNode || draggingCanvasNode.source !== source)
            return;
        const rect = event.currentTarget.getBoundingClientRect();
        const x = Math.max(30, event.clientX - rect.left);
        const y = Math.max(24, event.clientY - rect.top);
        setCanvasPositions((current) => ({ ...current, [`${source}:${draggingCanvasNode.key}`]: { x, y } }));
    };
    const handleCanvasSurfacePointerUp = () => setDraggingCanvasNode(null);
    // Clicking a node's connector dot arms a pending "from" node; clicking a
    // second (different) node's dot completes the edge. Clicking the same
    // node again, or starting a fresh click while a different source's canvas
    // is armed, resets the pending state instead of connecting.
    const handleCanvasConnectorClick = (source, key) => {
        if (!pendingCanvasEdgeFrom || pendingCanvasEdgeFrom.source !== source) {
            setPendingCanvasEdgeFrom({ source, nodeId: key });
            return;
        }
        if (pendingCanvasEdgeFrom.nodeId === key) {
            setPendingCanvasEdgeFrom(null);
            return;
        }
        addWorkflowEdgeBetween(pendingCanvasEdgeFrom.nodeId, key, source);
        setPendingCanvasEdgeFrom(null);
    };
    const saveWorkflow = async () => {
        if (!workflowDraft)
            return;
        if (!workflowDraft.modelOverride) {
            setMessage("Model is required.");
            return;
        }
        // Task-189 slice 3: only enforce flow-graph correctness once the user has
        // actually started wiring edges — a plain linear/legacy workflow with no
        // edges never intended to run as a flow-engine graph at all (it relies on
        // order_index sequencing), so it must not be newly blocked by these checks.
        if (workflowDraft.edges.length > 0) {
            const issues = (0, client_core_1.validateFlowGraph)(workflowSteps, stepDefinitions, workflowDraft.edges);
            if (issues.length > 0) {
                setMessage(issues.join(" "));
                return;
            }
        }
        setBusy(true);
        setMessage(null);
        try {
            const admin = await (0, clientCore_1.getAdminUseCases)();
            const saved = await admin.workflows.saveWorkflow({
                id: workflowDraft.id,
                projectId: workflowDraft.projectId,
                name: workflowDraft.name,
                description: workflowDraft.description,
                isTemplate: workflowDraft.isTemplate,
                modelOverride: workflowDraft.modelOverride,
                reasoningEffortOverride: workflowDraft.reasoningEffortOverride,
                yoloMode: workflowDraft.yoloMode,
                // BUG-NOTE-CP42 #14: pass these through unchanged so a plain edit
                // (e.g. renaming) doesn't null out the workflow's existing policy
                // caps and edge graph — saveWorkflow's repository has no notion of
                // "field omitted, leave unchanged."
                policyCap: workflowDraft.policyCap,
                policyOnCap: workflowDraft.policyOnCap,
                policyExtendBy: workflowDraft.policyExtendBy,
                policyExtendMax: workflowDraft.policyExtendMax,
                edges: workflowDraft.edges,
                // CP-55 P-1 (Task-263/CA-424 pass 3): no editor writes this today —
                // sent back unchanged, same as edges/policy* above, so a plain save
                // (e.g. a rename) can't silently erase a declared acceptance
                // boundary the way an omitted field would (saveWorkflow's repository
                // always overwrites every column it's given).
                acceptanceNodes: workflowDraft.acceptanceNodes,
                steps: workflowSteps.map((step, orderIndex) => ({ ...step, orderIndex })),
            });
            // BUG-282: the workflow's edges are the only home for flow topology, and
            // they were just persisted above. Nothing further to sync onto the step
            // definitions.
            await refresh(saved.id, selectedStepType, { preserveCreateDrafts: true });
            setMessage("Workflow saved.");
        }
        catch (error) {
            setMessage((0, settingsHelpers_1.toErrorMessage)(error, "Unable to save workflow."));
        }
        finally {
            setBusy(false);
        }
    };
    const saveNewWorkflow = async () => {
        if (!createWorkflowDraft.modelOverride) {
            setMessage("Model is required.");
            return;
        }
        if (createWorkflowDraft.edges.length > 0) {
            const issues = (0, client_core_1.validateFlowGraph)(createWorkflowSteps, stepDefinitions, createWorkflowDraft.edges);
            if (issues.length > 0) {
                setMessage(issues.join(" "));
                return;
            }
        }
        setBusy(true);
        setMessage(null);
        try {
            const admin = await (0, clientCore_1.getAdminUseCases)();
            const saved = await admin.workflows.saveWorkflow({
                projectId: createWorkflowDraft.projectId,
                name: createWorkflowDraft.name.trim() || "New Workflow",
                description: createWorkflowDraft.description,
                isTemplate: createWorkflowDraft.isTemplate,
                modelOverride: createWorkflowDraft.modelOverride,
                reasoningEffortOverride: createWorkflowDraft.reasoningEffortOverride ?? DEFAULT_REASONING,
                yoloMode: createWorkflowDraft.yoloMode,
                policyCap: createWorkflowDraft.policyCap,
                policyOnCap: createWorkflowDraft.policyOnCap,
                policyExtendBy: createWorkflowDraft.policyExtendBy,
                policyExtendMax: createWorkflowDraft.policyExtendMax,
                edges: createWorkflowDraft.edges,
                // See the equivalent comment in saveWorkflow above — a brand-new
                // workflow simply has none declared yet (createEmptyWorkflowDraft
                // defaults to []), so this sends the same empty set the DB default
                // already produces; it exists for symmetry with saveWorkflow, not
                // because create needs different behavior.
                acceptanceNodes: createWorkflowDraft.acceptanceNodes,
                steps: createWorkflowSteps.map((step, orderIndex) => ({ ...step, orderIndex })),
            });
            // BUG-282: flow topology lives only on the workflow's edges (persisted
            // above); nothing further to sync onto the step definitions.
            setWorkflowView("list");
            await refresh(saved.id, selectedStepType);
            setMessage("Workflow created.");
        }
        catch (error) {
            setMessage((0, settingsHelpers_1.toErrorMessage)(error, "Unable to create workflow."));
        }
        finally {
            setBusy(false);
        }
    };
    const saveStepDefinition = async () => {
        if (!stepDraft)
            return;
        const requiresModel = (0, stepModelVisibility_1.stepDefinitionRequiresModel)(stepDraft.behaviorId);
        if (requiresModel && !stepDraft.model) {
            setMessage("Model is required.");
            return;
        }
        const agentRefIssue = stepDefinitionAgentRefIssue(stepDraft.behaviorId, stepDraft.agentRef);
        if (agentRefIssue) {
            setMessage(agentRefIssue);
            return;
        }
        setBusy(true);
        setMessage(null);
        try {
            const admin = await (0, clientCore_1.getAdminUseCases)();
            const saved = await admin.workflows.saveStepDefinition({
                ...stepDraft,
                // A behavior that doesn't need a model (inline/control) must not
                // silently persist a stale value left over from before the user
                // switched Behavior ID — the fields are hidden in the form, so there
                // is no UI left to clear them manually.
                model: requiresModel ? stepDraft.model : null,
                reasoningEffort: requiresModel ? stepDraft.reasoningEffort : null,
                promptBase: stepDraft.promptBase?.trim() ||
                    deriveStepPromptBase({
                        stepType: stepDraft.stepType,
                        name: stepDraft.name,
                        description: stepDraft.description,
                    }),
            });
            await refresh(selectedWorkflowId, saved.stepType, { preserveCreateDrafts: true });
            setMessage("Step definition saved.");
        }
        catch (error) {
            setMessage((0, settingsHelpers_1.toErrorMessage)(error, "Unable to save step definition."));
        }
        finally {
            setBusy(false);
        }
    };
    const saveNewStepDefinition = async () => {
        if (!createStepDraft.stepType.trim()) {
            setMessage("Step key is required.");
            return;
        }
        const requiresModel = (0, stepModelVisibility_1.stepDefinitionRequiresModel)(createStepDraft.behaviorId);
        if (requiresModel && !createStepDraft.model) {
            setMessage("Model is required.");
            return;
        }
        const agentRefIssue = stepDefinitionAgentRefIssue(createStepDraft.behaviorId, createStepDraft.agentRef);
        if (agentRefIssue) {
            setMessage(agentRefIssue);
            return;
        }
        setBusy(true);
        setMessage(null);
        try {
            const admin = await (0, clientCore_1.getAdminUseCases)();
            const saved = await admin.workflows.saveStepDefinition({
                ...createStepDraft,
                stepType: createStepDraft.stepType.trim(),
                name: createStepDraft.name.trim() || createStepDraft.stepType.trim(),
                description: createStepDraft.description.trim(),
                model: requiresModel ? createStepDraft.model : null,
                reasoningEffort: requiresModel ? createStepDraft.reasoningEffort : null,
                promptBase: createStepDraft.promptBase?.trim() ||
                    deriveStepPromptBase({
                        stepType: createStepDraft.stepType.trim(),
                        name: createStepDraft.name,
                        description: createStepDraft.description,
                    }),
            });
            setStepView("list");
            await refresh(selectedWorkflowId, saved.stepType);
            setMessage("Step definition created.");
        }
        catch (error) {
            setMessage((0, settingsHelpers_1.toErrorMessage)(error, "Unable to create step definition."));
        }
        finally {
            setBusy(false);
        }
    };
    const openDeleteWorkflowConfirm = (ids, labels) => {
        if (ids.length === 0)
            return;
        setDeleteTarget({ kind: "workflow", ids, labels });
        setDeleteConfirmationText("");
    };
    // Steps have no FK-cascade from workflow_steps back to step_definitions
    // (BUG-236: a workflow's step list is a pure relation), so deleting a step
    // definition that's still referenced by a workflow would leave that
    // workflow with a dangling stepType it can never resolve at runtime. Look
    // up every workflow currently using any of the given step types and fold
    // them into the same confirmation so they're deleted together.
    const openDeleteStepConfirm = async (ids, labels) => {
        if (ids.length === 0)
            return;
        // A step still listed by a built-in workflow can't be deleted at all —
        // that workflow is read-only, so there's no cascade that could clear it
        // out first. Block before the usage lookup instead of only relying on
        // the disabled row/button, since bulk selections can mix deletable and
        // blocked steps.
        const blockedIds = ids.filter((id) => stepTypesUsedByBuiltin.has(id));
        if (blockedIds.length > 0) {
            const blockedLabels = blockedIds.map((id) => stepDefinitions.find((step) => step.stepType === id)?.name ?? id);
            setMessage(`Can't delete ${blockedLabels.join(", ")}: still used by a built-in workflow, which can't be edited.`);
            return;
        }
        setBusy(true);
        setMessage(null);
        try {
            const admin = await (0, clientCore_1.getAdminUseCases)();
            const usages = await admin.workflows.listWorkflowsUsingSteps(ids);
            const cascadeWorkflowIds = Array.from(new Set(usages.map((usage) => usage.workflowId)));
            const cascadeWorkflows = cascadeWorkflowIds.map((workflowId) => ({
                id: workflowId,
                name: workflows.find((workflow) => workflow.id === workflowId)?.name ?? workflowId,
            }));
            setDeleteTarget({ kind: "step", ids, labels, cascadeWorkflows });
            setDeleteConfirmationText("");
        }
        catch (error) {
            setMessage((0, settingsHelpers_1.toErrorMessage)(error, "Unable to check step usage before delete."));
        }
        finally {
            setBusy(false);
        }
    };
    const deleteSelected = async () => {
        if (!deleteTarget)
            return;
        setBusy(true);
        setMessage(null);
        try {
            const admin = await (0, clientCore_1.getAdminUseCases)();
            if (deleteTarget.kind === "workflow") {
                // Independent rows — run the deletes concurrently instead of one at
                // a time so a bulk delete of N workflows takes roughly as long as
                // the slowest single delete, not N times that.
                await Promise.all(deleteTarget.ids.map((id) => admin.workflows.deleteWorkflow(id)));
                setDeleteTarget(null);
                setDeleteConfirmationText("");
                exitBulkSelect();
                await refresh(undefined, selectedStepType);
                setMessage(deleteTarget.ids.length > 1 ? `${deleteTarget.ids.length} workflows deleted.` : "Workflow deleted.");
            }
            else {
                // Cascade workflows must be gone before the step definitions they
                // reference are deleted, but within each phase the rows are
                // independent, so fan each phase out concurrently rather than
                // serializing every single delete.
                await Promise.all(deleteTarget.cascadeWorkflows.map((cascadeWorkflow) => admin.workflows.deleteWorkflow(cascadeWorkflow.id)));
                await Promise.all(deleteTarget.ids.map((id) => admin.workflows.deleteStepDefinition(id)));
                setDeleteTarget(null);
                setDeleteConfirmationText("");
                exitBulkSelect();
                await refresh(undefined, undefined);
                const cascadeNote = deleteTarget.cascadeWorkflows.length > 0
                    ? ` (also removed ${deleteTarget.cascadeWorkflows.length} workflow(s) that used it)`
                    : "";
                setMessage((deleteTarget.ids.length > 1
                    ? `${deleteTarget.ids.length} step definitions deleted.`
                    : "Step definition deleted.") + cascadeNote);
            }
        }
        catch (error) {
            setMessage((0, settingsHelpers_1.toErrorMessage)(error, "Unable to delete item."));
        }
        finally {
            setBusy(false);
        }
    };
    const cloneSelected = async () => {
        if (!cloneTarget)
            return;
        setBusy(true);
        setMessage(null);
        try {
            const admin = await (0, clientCore_1.getAdminUseCases)();
            const cloned = await admin.workflows.cloneWorkflow(cloneTarget.workflowId, cloneName.trim() || `${cloneTarget.sourceName} (copy)`);
            setCloneTarget(null);
            setCloneName("");
            await refresh(cloned.id, selectedStepType, { preserveCreateDrafts: true });
            setMessage("Workflow cloned. You can now edit the copy.");
        }
        catch (error) {
            setMessage((0, settingsHelpers_1.toErrorMessage)(error, "Unable to clone workflow."));
        }
        finally {
            setBusy(false);
        }
    };
    const renderWorkflowStepsEditor = (steps, source, selectedStepTypeValue, onSelectedStepTypeChange) => {
        const availableSteps = source === "detail"
            ? stepDefinitions.filter((definition) => !steps.some((step) => step.stepType === definition.stepType))
            : availableCreateWorkflowSteps;
        // A built-in workflow's steps can't be persisted (Save is hidden for it),
        // so block the mutating actions here too rather than let the user add/
        // remove steps that silently go nowhere.
        const readOnly = source === "detail" && selectedWorkflow?.editable === false;
        return ((0, jsx_runtime_1.jsxs)("div", { className: "settings-subpanel workflow-steps-panel", children: [(0, jsx_runtime_1.jsx)("div", { className: "project-panel-head", children: (0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("strong", { children: "Workflow Steps" }), (0, jsx_runtime_1.jsx)("p", { className: "project-muted-copy", children: readOnly
                                    ? "Read-only: clone this workflow to edit its steps."
                                    : "Add, remove, and reorder the reusable steps that make up this workflow." })] }) }), (0, jsx_runtime_1.jsxs)("div", { className: "workflow-step-add-row", children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-field workflow-step-add-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Add step" }), (0, jsx_runtime_1.jsxs)("select", { disabled: readOnly, onChange: (event) => onSelectedStepTypeChange(event.target.value), value: selectedStepTypeValue, children: [availableSteps.length === 0 ? (0, jsx_runtime_1.jsx)("option", { value: "", children: "No more steps available" }) : null, availableSteps.map((definition) => ((0, jsx_runtime_1.jsx)("option", { value: definition.stepType, children: definition.name }, definition.stepType)))] })] }), (0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", disabled: readOnly || !selectedStepTypeValue, onClick: () => addWorkflowStep(source), type: "button", children: "Add Step" })] }), (0, jsx_runtime_1.jsx)("div", { className: "settings-list", children: steps.length === 0 ? ((0, jsx_runtime_1.jsx)("div", { className: "settings-empty", children: "No steps added yet." })) : (steps.map((step, index) => {
                        const stepKey = `${source}:${step.id}`;
                        const isExpanded = expandedStepKeys.has(stepKey);
                        const definition = stepDefinitions.find((item) => item.stepType === step.stepType);
                        return ((0, jsx_runtime_1.jsxs)("div", { className: "settings-list-item static workflow-step-card", children: [(0, jsx_runtime_1.jsxs)("div", { className: "workflow-step-card-head", children: [(0, jsx_runtime_1.jsxs)("button", { "aria-expanded": isExpanded, className: "workflow-step-card-toggle", onClick: () => toggleStepExpanded(stepKey), type: "button", children: [(0, jsx_runtime_1.jsx)("span", { className: `workflow-step-card-chevron ${isExpanded ? "expanded" : ""}`, children: "\u25B8" }), (0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsxs)("strong", { children: [index + 1, ".", " ", definition?.nodeId || definition?.agentRef || definition?.name || step.stepType] }), (0, jsx_runtime_1.jsx)("span", { children: step.stepType })] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-inline-actions", children: [(0, jsx_runtime_1.jsx)("button", { className: "secondary-btn project-icon-btn", disabled: readOnly || index === 0, onClick: () => moveWorkflowStep(index, "up", source), title: "Move up", type: "button", children: "^" }), (0, jsx_runtime_1.jsx)("button", { className: "secondary-btn project-icon-btn", disabled: readOnly || index === steps.length - 1, onClick: () => moveWorkflowStep(index, "down", source), title: "Move down", type: "button", children: "v" }), (0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", disabled: readOnly, onClick: () => removeWorkflowStep(index, source), type: "button", children: "Remove" })] })] }), isExpanded ? ((0, jsx_runtime_1.jsx)(jsx_runtime_1.Fragment, { children: (0, jsx_runtime_1.jsxs)("div", { className: "settings-grid workflow-step-grid", children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-checkbox", children: [(0, jsx_runtime_1.jsx)("input", { checked: step.isEnabled, disabled: readOnly, onChange: (event) => source === "detail"
                                                            ? updateWorkflowStep(index, { isEnabled: event.target.checked })
                                                            : updateCreateWorkflowStep(index, { isEnabled: event.target.checked }), type: "checkbox" }), (0, jsx_runtime_1.jsx)("span", { children: "Enabled" })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-checkbox", children: [(0, jsx_runtime_1.jsx)("input", { checked: step.requiresApproval, disabled: readOnly, onChange: (event) => source === "detail"
                                                            ? updateWorkflowStep(index, { requiresApproval: event.target.checked })
                                                            : updateCreateWorkflowStep(index, {
                                                                requiresApproval: event.target.checked,
                                                            }), type: "checkbox" }), (0, jsx_runtime_1.jsx)("span", { children: "Requires approval" })] })] }) })) : null] }, `${step.stepType}-${index}`));
                    })) })] }));
    };
    // Task-189 slice 2: the form-list edge editor. `steps` is this workflow's
    // OWN step list (workflowSteps for "detail", createWorkflowSteps for
    // "create") so the from/to pickers only ever offer nodes that are actually
    // part of this flow. A canvas-based graph editor (Task-189 slice 4) will be
    // added as a second, synchronized view over the same `edges` array — not a
    // replacement for this one.
    const renderWorkflowEdgesEditor = (edges, steps, source) => {
        const readOnly = source === "detail" && selectedWorkflow?.editable === false;
        const nodeOptions = workflowEdgeNodeOptions(steps);
        const toOptions = [...nodeOptions, ...client_core_1.FLOW_EDGE_TERMINALS];
        const nodesMissingId = steps.filter((step) => !stepDefinitions.find((definition) => definition.stepType === step.stepType)?.nodeId);
        return ((0, jsx_runtime_1.jsxs)("div", { className: "settings-subpanel workflow-edges-panel", children: [(0, jsx_runtime_1.jsx)("div", { className: "project-panel-head", children: (0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("strong", { children: "Flow Edges" }), (0, jsx_runtime_1.jsx)("p", { className: "project-muted-copy", children: readOnly
                                    ? "Read-only: clone this workflow to edit its edges."
                                    : "Wire the flow graph: which node follows which, and under what outcome. " +
                                        "Without at least one edge, this flow's steps run in isolation and the " +
                                        "engine cannot advance past the entry node." })] }) }), nodesMissingId.length > 0 ? ((0, jsx_runtime_1.jsxs)("div", { className: "settings-warning", children: [nodesMissingId.length, " step(s) have no Node ID set on their step type \u2014 set a Node ID (in the step type's catalog entry) before they can be used as an edge endpoint:", " ", nodesMissingId.map((step) => step.stepType).join(", ")] })) : null, (0, jsx_runtime_1.jsx)("div", { className: "settings-list", children: edges.length === 0 ? ((0, jsx_runtime_1.jsx)("div", { className: "settings-empty", children: "No edges yet \u2014 add one below." })) : (edges.map((edge, index) => ((0, jsx_runtime_1.jsxs)("div", { className: "settings-list-item static workflow-edge-row", children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "From" }), (0, jsx_runtime_1.jsxs)("select", { disabled: readOnly, onChange: (event) => updateWorkflowEdge(index, { from: event.target.value }, source), value: edge.from, children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "(select a node)" }), nodeOptions.map((id) => ((0, jsx_runtime_1.jsx)("option", { value: id, children: id }, id)))] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "To" }), (0, jsx_runtime_1.jsxs)("select", { disabled: readOnly, onChange: (event) => updateWorkflowEdge(index, { to: event.target.value }, source), value: edge.to, children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "(select a node or terminal)" }), toOptions.map((id) => ((0, jsx_runtime_1.jsx)("option", { value: id, children: client_core_1.FLOW_EDGE_TERMINALS.includes(id) ? `(terminal) ${id}` : id }, id)))] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "When (outcome status)" }), (0, jsx_runtime_1.jsxs)("select", { disabled: readOnly, onChange: (event) => updateWorkflowEdge(index, { when: event.target.value }, source), value: edge.when, children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "(select an outcome)" }), WORKFLOW_EDGE_WHEN_OPTIONS.map((option) => ((0, jsx_runtime_1.jsx)("option", { value: option, children: option }, option))), edge.when && !WORKFLOW_EDGE_WHEN_OPTIONS.includes(edge.when) ? ((0, jsx_runtime_1.jsxs)("option", { value: edge.when, children: [edge.when, " (current value)"] })) : null] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Kind" }), (0, jsx_runtime_1.jsxs)("select", { disabled: readOnly, onChange: (event) => updateWorkflowEdge(index, { kind: event.target.value }, source), value: edge.kind, children: [(0, jsx_runtime_1.jsx)("option", { value: "forward", children: "Forward" }), (0, jsx_runtime_1.jsx)("option", { value: "back", children: "Back (loop)" })] })] }), (0, jsx_runtime_1.jsx)("div", { className: "settings-inline-actions", children: (0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", disabled: readOnly, onClick: () => removeWorkflowEdge(index, source), type: "button", children: "Remove" }) })] }, index)))) }), (0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", disabled: readOnly || nodeOptions.length === 0, onClick: () => addWorkflowEdge(source), type: "button", children: "+ Add Edge" })] }));
    };
    // Task-189 slice 4: the visual canvas. It reads/writes the SAME
    // `edges` array as renderWorkflowEdgesEditor above (passed in verbatim) —
    // there is no separate canvas-edge state, so editing an edge's "when" in
    // the form-list is reflected here immediately and vice versa. Only node
    // *position* is canvas-local view state (canvasPositions), never
    // persisted.
    const renderWorkflowFlowCanvas = (edges, steps, source) => {
        const readOnly = source === "detail" && selectedWorkflow?.editable === false;
        const nodeOptions = workflowEdgeNodeOptions(steps);
        const nodeKeys = [...nodeOptions, ...client_core_1.FLOW_EDGE_TERMINALS];
        const layout = canvasLayoutFor(source, nodeKeys);
        const pending = pendingCanvasEdgeFrom?.source === source ? pendingCanvasEdgeFrom : null;
        return ((0, jsx_runtime_1.jsxs)("div", { className: "settings-subpanel workflow-canvas-panel", children: [(0, jsx_runtime_1.jsx)("div", { className: "project-panel-head", children: (0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("strong", { children: "Flow Canvas" }), (0, jsx_runtime_1.jsx)("p", { className: "project-muted-copy", children: "Drag a node to arrange it. Click a node's dot, then another node's dot, to draw an edge between them \u2014 it's added to the Flow Edges list above." })] }) }), pending ? ((0, jsx_runtime_1.jsxs)("div", { className: "settings-warning", children: ["Drawing an edge from \"", pending.nodeId, "\" \u2014 click another node's dot to connect it, or", " ", (0, jsx_runtime_1.jsx)("button", { className: "link-btn", onClick: () => setPendingCanvasEdgeFrom(null), type: "button", children: "cancel" }), "."] })) : null, nodeOptions.length === 0 ? ((0, jsx_runtime_1.jsx)("div", { className: "settings-empty", children: "No nodes with a Node ID yet \u2014 set a Node ID on a step type before it can appear here." })) : ((0, jsx_runtime_1.jsxs)("div", { className: "workflow-canvas-surface", onPointerLeave: handleCanvasSurfacePointerUp, onPointerMove: handleCanvasSurfacePointerMove(source), onPointerUp: handleCanvasSurfacePointerUp, children: [(0, jsx_runtime_1.jsxs)("svg", { className: "workflow-canvas-edges", children: [(0, jsx_runtime_1.jsx)("defs", { children: (0, jsx_runtime_1.jsx)("marker", { id: `workflow-canvas-arrow-${source}`, markerHeight: "8", markerWidth: "8", orient: "auto", refX: "7", refY: "4", children: (0, jsx_runtime_1.jsx)("path", { className: "workflow-canvas-arrowhead", d: "M0,0 L8,4 L0,8 z" }) }) }), edges.map((edge, index) => {
                                    const from = layout[edge.from];
                                    const to = layout[edge.to];
                                    if (!from || !to)
                                        return null;
                                    return ((0, jsx_runtime_1.jsxs)("g", { children: [(0, jsx_runtime_1.jsx)("line", { className: edge.kind === "back" ? "workflow-canvas-edge back" : "workflow-canvas-edge", markerEnd: `url(#workflow-canvas-arrow-${source})`, x1: from.x, y1: from.y, x2: to.x, y2: to.y }), (0, jsx_runtime_1.jsx)("text", { className: "workflow-canvas-edge-label", x: (from.x + to.x) / 2, y: (from.y + to.y) / 2 - 6, children: edge.when })] }, index));
                                })] }), nodeKeys.map((key) => {
                            const isTerminal = client_core_1.FLOW_EDGE_TERMINALS.includes(key);
                            const position = layout[key];
                            return ((0, jsx_runtime_1.jsxs)("div", { className: [
                                    "workflow-canvas-node",
                                    isTerminal ? "terminal" : "",
                                    pending?.nodeId === key ? "connecting" : "",
                                ]
                                    .filter(Boolean)
                                    .join(" "), onPointerDown: handleCanvasNodePointerDown(source, key), style: { left: position.x, top: position.y }, children: [(0, jsx_runtime_1.jsx)("span", { children: key }), (0, jsx_runtime_1.jsx)("button", { className: "workflow-canvas-connector", disabled: readOnly, onClick: () => handleCanvasConnectorClick(source, key), title: "Click, then click another node, to draw an edge", type: "button", children: "\u25CF" })] }, key));
                        })] }))] }));
    };
    const renderStepDefinitionForm = (draft, onChange, mode) => {
        const requiredSkillsText = draft.requiredSkills.join(", ");
        return ((0, jsx_runtime_1.jsxs)("div", { className: "settings-grid", children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Step key" }), (0, jsx_runtime_1.jsx)("input", { disabled: mode === "detail", onChange: (event) => {
                                const nextStepType = event.target.value;
                                // Node ID auto-follows the Step key while it's untouched (still
                                // empty, or still equal to the previous Step key value) — Step
                                // key is only ever editable at creation time (disabled above in
                                // "detail" mode), so this only ever runs while a new
                                // step-definition is being drafted. Once the user types their
                                // own Node ID, it stops matching the old Step key and this
                                // no-ops, leaving their choice alone.
                                const nodeIdFollowsStepType = !draft.nodeId || draft.nodeId === draft.stepType;
                                onChange({
                                    ...draft,
                                    stepType: nextStepType,
                                    nodeId: nodeIdFollowsStepType ? nextStepType : draft.nodeId,
                                });
                            }, value: draft.stepType })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Display name" }), (0, jsx_runtime_1.jsx)("input", { onChange: (event) => onChange({ ...draft, name: event.target.value }), value: draft.name })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("span", { children: "Description" }), (0, jsx_runtime_1.jsx)("textarea", { onChange: (event) => onChange({ ...draft, description: event.target.value }), value: draft.description })] }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsxs)("div", { className: "workflow-chip-row", children: [(0, jsx_runtime_1.jsx)("span", { children: "Required MCPs" }), (0, jsx_runtime_1.jsx)("button", { className: "secondary-btn workflow-chip-add-btn", onClick: () => setPickerModal({ kind: "mcp", mode }), type: "button", children: "+ Add" })] }), draft.requiredMcps.length === 0 ? ((0, jsx_runtime_1.jsx)("div", { className: "workflow-chip-empty", children: "None selected" })) : ((0, jsx_runtime_1.jsx)("div", { className: "workflow-chips", children: draft.requiredMcps.map((mcp) => ((0, jsx_runtime_1.jsxs)("span", { className: "workflow-chip", children: [(0, jsx_runtime_1.jsx)("span", { children: toTitleCase(mcp) }), (0, jsx_runtime_1.jsx)("button", { "aria-label": `Remove ${mcp}`, className: "workflow-chip-remove", onClick: () => onChange({ ...draft, requiredMcps: draft.requiredMcps.filter((m) => m !== mcp) }), type: "button", children: "\u00D7" })] }, mcp))) }))] }), draft.requiredMcps.includes("google_drive") ? ((0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Google Drive access" }), (0, jsx_runtime_1.jsxs)("select", { onChange: (event) => onChange({
                                ...draft,
                                mcpAccessMode: event.target.value === "read_write" ? "read_write" : "read_only",
                            }), value: draft.mcpAccessMode, children: [(0, jsx_runtime_1.jsx)("option", { value: "read_only", children: "Read only" }), (0, jsx_runtime_1.jsx)("option", { value: "read_write", children: "Read + write" })] })] })) : null, (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Required skills" }), (0, jsx_runtime_1.jsx)("input", { onChange: (event) => onChange({
                                ...draft,
                                requiredSkills: event.target.value
                                    .split(",")
                                    .map((item) => item.trim())
                                    .filter(Boolean),
                            }), placeholder: "planner-skill, coding-skill", value: requiredSkillsText })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Team role" }), (0, jsx_runtime_1.jsx)("input", { onChange: (event) => onChange({ ...draft, teamRole: event.target.value || null }), value: draft.teamRole ?? "" })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("span", { children: "Prompt base" }), (0, jsx_runtime_1.jsx)("textarea", { onChange: (event) => onChange({ ...draft, promptBase: event.target.value }), placeholder: "Describe the execution intent for this step.", value: draft.promptBase ?? "" })] }), (0, stepModelVisibility_1.stepDefinitionRequiresModel)(draft.behaviorId) ? ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Model" }), (0, jsx_runtime_1.jsxs)("select", { onChange: (event) => onChange({ ...draft, model: event.target.value || null }), value: draft.model ?? "", children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "Select a model..." }), modelOptions.map((model) => ((0, jsx_runtime_1.jsx)("option", { value: model.value, children: model.label }, model.value)))] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Reasoning effort" }), (0, jsx_runtime_1.jsx)("select", { onChange: (event) => onChange({ ...draft, reasoningEffort: event.target.value }), value: draft.reasoningEffort ?? DEFAULT_REASONING, children: REASONING_OPTIONS.map((option) => ((0, jsx_runtime_1.jsx)("option", { value: option.value, children: option.label }, option.value))) })] })] })) : (
                // Inline/control behaviors (context.produce, command.validate,
                // artifact.audit_draft, telegram.notify, ...) are Go-deterministic
                // and never spawn a provider turn, so Model/Reasoning effort do not
                // apply — hidden rather than forcing a meaningless choice.
                (0, jsx_runtime_1.jsxs)("div", { className: "settings-field settings-field-full project-muted-copy", children: ["Model / Reasoning effort not applicable \u2014 Behavior \"", draft.behaviorId, "\" runs inline, with no provider turn."] })), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Behavior ID" }), (0, jsx_runtime_1.jsxs)("select", { onChange: (event) => onChange({ ...draft, behaviorId: event.target.value || null }), value: draft.behaviorId ?? "", children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "(none / inherit from step type)" }), client_core_1.FLOW_BEHAVIOR_OPTIONS.map((b) => ((0, jsx_runtime_1.jsx)("option", { value: b.id, children: b.label }, b.id)))] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Preview agents for project" }), (0, jsx_runtime_1.jsxs)("select", { onChange: (event) => setAgentPreviewProjectId(event.target.value), value: agentPreviewProjectId, children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "Workspace global (built-in + provider-home only)" }), projects.map((project) => ((0, jsx_runtime_1.jsx)("option", { value: project.id, children: project.name }, project.id)))] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Agent ref" }), (0, jsx_runtime_1.jsxs)("select", { onChange: (event) => onChange({ ...draft, agentRef: event.target.value || null }), value: draft.agentRef ?? "", children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "(none)" }), agentOptions.map((agent) => {
                                    // Prefer the full path so same-named agents across sources
                                    // (project .claude/agents vs provider-home .codex/agents) stay
                                    // distinct. Built-in flow-pack agents have no path, so they fall
                                    // back to their bare name. Runtime resolution (spawnChildRun)
                                    // matches either form, so both work.
                                    const value = agent.path || agent.name;
                                    return ((0, jsx_runtime_1.jsxs)("option", { value: value, children: [agent.name, " (", agent.source, ")"] }, value));
                                }), draft.agentRef && !agentOptions.some((agent) => (agent.path || agent.name) === draft.agentRef) ? ((0, jsx_runtime_1.jsxs)("option", { value: draft.agentRef, children: [draft.agentRef, " (current value, not in this project's list)"] })) : null] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Lifecycle" }), (0, jsx_runtime_1.jsxs)("select", { onChange: (event) => onChange({ ...draft, nodeLifecycle: event.target.value || null }), value: draft.nodeLifecycle ?? "", children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "Default (reinvoke)" }), (0, jsx_runtime_1.jsx)("option", { value: "reinvoke", children: "Reinvoke" }), (0, jsx_runtime_1.jsx)("option", { value: "spawn", children: "Spawn new" }), (0, jsx_runtime_1.jsx)("option", { value: "once", children: "Once" })] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Prompt template ref" }), (0, jsx_runtime_1.jsx)("input", { onChange: (event) => onChange({ ...draft, promptTemplateRef: event.target.value || null }), placeholder: "prompts/example.md", value: draft.promptTemplateRef ?? "" })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Context ref" }), (0, jsx_runtime_1.jsx)("input", { onChange: (event) => onChange({ ...draft, contextRef: event.target.value || null }), placeholder: "contexts/example.yaml", value: draft.contextRef ?? "" })] }), draft.contextSources.length > 0 ? ((0, jsx_runtime_1.jsxs)("div", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("div", { className: "workflow-chip-row", children: (0, jsx_runtime_1.jsx)("span", { children: "Legacy context sources (clear only)" }) }), (0, jsx_runtime_1.jsxs)("small", { className: "settings-field-help", children: ["Transition data from CP-44/Task-196. Prefer Artifacts tab \u2192", " ", (0, jsx_runtime_1.jsx)("code", { children: "context_artifact.v1" }), " instance sources, then bind under Artifact Inputs/Outputs. Runner still applies these raw ids only when no artifact binding supplies sources (SD-23 D-6). Clear by removing chips if you no longer need them."] }), (0, jsx_runtime_1.jsx)("div", { className: "workflow-chips", children: draft.contextSources.map((sourceId) => ((0, jsx_runtime_1.jsxs)("span", { className: "workflow-chip", children: [(0, jsx_runtime_1.jsx)("span", { children: contextSourceOptions.find((option) => option.id === sourceId)?.label ?? sourceId }), (0, jsx_runtime_1.jsx)("button", { "aria-label": `Remove legacy ${sourceId}`, className: "workflow-chip-remove", onClick: () => onChange({
                                            ...draft,
                                            contextSources: draft.contextSources.filter((id) => id !== sourceId),
                                        }), type: "button", children: "\u00D7" })] }, sourceId))) })] })) : null, ["output", "input"].map((direction) => {
                    const bindings = draft.artifactBindings.filter((b) => b.direction === direction);
                    return ((0, jsx_runtime_1.jsxs)("div", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsxs)("div", { className: "workflow-chip-row", children: [(0, jsx_runtime_1.jsxs)("span", { children: ["Artifact ", direction === "input" ? "Inputs" : "Outputs"] }), (0, jsx_runtime_1.jsx)("button", { className: "secondary-btn workflow-chip-add-btn", onClick: () => setPickerModal({
                                            kind: direction === "input" ? "artifact-binding-input" : "artifact-binding-output",
                                            mode,
                                        }), type: "button", children: "+ Add" })] }), (0, jsx_runtime_1.jsx)("small", { className: "settings-field-help", children: direction === "output"
                                    ? "Output: this step must produce the bound file_artifact path(s) (write contract)."
                                    : "Input: prompt mentions bound file path(s); agent reads via tools (no full-file paste)." }), bindings.length === 0 ? ((0, jsx_runtime_1.jsx)("div", { className: "workflow-chip-empty", children: "None bound" })) : ((0, jsx_runtime_1.jsx)("div", { className: "workflow-chips", children: bindings.map((binding) => ((0, jsx_runtime_1.jsxs)("span", { className: "workflow-chip", children: [(0, jsx_runtime_1.jsx)("span", { children: artifactInstanceLabel(artifactInstances.find((i) => i.id === binding.artifactInstanceId), binding.artifactInstanceId) }), (0, jsx_runtime_1.jsx)("button", { "aria-label": `Remove ${binding.artifactInstanceId}`, className: "workflow-chip-remove", onClick: () => onChange({
                                                ...draft,
                                                artifactBindings: draft.artifactBindings.filter((b) => !(b.direction === direction && b.artifactInstanceId === binding.artifactInstanceId)),
                                            }), type: "button", children: "\u00D7" })] }, binding.artifactInstanceId))) }))] }, direction));
                }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-checkbox settings-field-full", title: "Flow/Workflow launches always run YOLO \u2014 the gate/approval machinery here is not robust against a paused approval mid-flow. Use Normal Chat for gated (YOLO=off) runs.", children: [(0, jsx_runtime_1.jsx)("input", { checked: true, disabled: true, type: "checkbox" }), (0, jsx_runtime_1.jsx)("span", { children: "YOLO for single-step runs (always on for Flow mode)" })] })] }));
    };
    const renderPickerModal = () => {
        if (!pickerModal)
            return null;
        const isDetail = pickerModal.mode === "detail";
        const draftSource = isDetail ? stepDraft : createStepDraft;
        const updateDraft = (patch) => {
            if (isDetail) {
                setStepDraft((d) => (d ? { ...d, ...patch } : d));
            }
            else {
                setCreateStepDraft((d) => ({ ...d, ...patch }));
            }
        };
        const title = pickerModal.kind === "context-source"
            ? "Context Sources"
            : pickerModal.kind === "artifact-binding-input"
                ? "Artifact Inputs"
                : pickerModal.kind === "artifact-binding-output"
                    ? "Artifact Outputs"
                    : "Required MCPs";
        return ((0, jsx_runtime_1.jsx)("div", { className: "settings-modal-backdrop", role: "presentation", children: (0, jsx_runtime_1.jsxs)("div", { className: "settings-modal", children: [(0, jsx_runtime_1.jsxs)("div", { className: "project-create-head", children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { className: "settings-eyebrow", children: "Select" }), (0, jsx_runtime_1.jsx)("h3", { children: title })] }), (0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", onClick: () => setPickerModal(null), type: "button", children: "Done" })] }), pickerModal.kind === "mcp" ? ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)("div", { className: "workflow-choice-grid workflow-picker-grid", children: settingsHelpers_1.integrationTypes.map((mcpType) => ((0, jsx_runtime_1.jsxs)("label", { className: "workflow-choice-card", children: [(0, jsx_runtime_1.jsx)("input", { checked: draftSource?.requiredMcps.includes(mcpType) ?? false, onChange: () => {
                                                if (!draftSource)
                                                    return;
                                                const nextMcps = toggleString(draftSource.requiredMcps, mcpType);
                                                updateDraft({
                                                    requiredMcps: nextMcps,
                                                    mcpAccessMode: mcpType === "google_drive" ||
                                                        draftSource.requiredMcps.includes("google_drive")
                                                        ? draftSource.mcpAccessMode
                                                        : "read_only",
                                                });
                                            }, type: "checkbox" }), (0, jsx_runtime_1.jsxs)("span", { children: [(0, jsx_runtime_1.jsx)("strong", { children: toTitleCase(mcpType) }), (0, jsx_runtime_1.jsx)("small", { children: mcpType })] })] }, mcpType))) }), draftSource?.requiredMcps.includes("google_drive") ? ((0, jsx_runtime_1.jsxs)("label", { className: "settings-field workflow-picker-sub", children: [(0, jsx_runtime_1.jsx)("span", { children: "Google Drive access" }), (0, jsx_runtime_1.jsxs)("select", { onChange: (event) => updateDraft({
                                            mcpAccessMode: event.target.value === "read_write" ? "read_write" : "read_only",
                                        }), value: draftSource.mcpAccessMode, children: [(0, jsx_runtime_1.jsx)("option", { value: "read_only", children: "Read only" }), (0, jsx_runtime_1.jsx)("option", { value: "read_write", children: "Read + write" })] })] })) : null] })) : pickerModal.kind === "context-source" ? ((0, jsx_runtime_1.jsx)("div", { className: "workflow-choice-grid workflow-picker-grid", children: contextSourceOptions.map((option) => ((0, jsx_runtime_1.jsxs)("label", { className: "workflow-choice-card", children: [(0, jsx_runtime_1.jsx)("input", { checked: draftSource?.contextSources.includes(option.id) ?? false, onChange: () => {
                                        if (!draftSource)
                                            return;
                                        updateDraft({ contextSources: toggleString(draftSource.contextSources, option.id) });
                                    }, type: "checkbox" }), (0, jsx_runtime_1.jsxs)("span", { children: [(0, jsx_runtime_1.jsx)("strong", { children: option.label }), (0, jsx_runtime_1.jsx)("small", { children: option.id })] })] }, option.id))) })) : pickerModal.kind === "artifact-binding-input" || pickerModal.kind === "artifact-binding-output" ? (
                    // CP-45/SD-23 Task-200: options are filtered to instances whose
                    // type is compatible with this step's behavior (D-7), never the
                    // full unfiltered instance list.
                    (() => {
                        const direction = pickerModal.kind === "artifact-binding-input" ? "input" : "output";
                        const options = compatibleArtifactInstancesFor(draftSource?.behaviorId, artifactInstances, direction);
                        if (options.length === 0) {
                            return ((0, jsx_runtime_1.jsx)("div", { className: "settings-empty", children: "No compatible artifact instances yet \u2014 create one in the Artifacts tab." }));
                        }
                        return ((0, jsx_runtime_1.jsx)("div", { className: "workflow-choice-grid workflow-picker-grid", children: options.map((instance) => {
                                const bound = draftSource?.artifactBindings.some((b) => b.direction === direction && b.artifactInstanceId === instance.id);
                                return ((0, jsx_runtime_1.jsxs)("label", { className: "workflow-choice-card", children: [(0, jsx_runtime_1.jsx)("input", { checked: bound ?? false, onChange: () => {
                                                if (!draftSource)
                                                    return;
                                                const existing = draftSource.artifactBindings.filter((b) => !(b.direction === direction && b.artifactInstanceId === instance.id));
                                                const next = bound
                                                    ? existing
                                                    : [
                                                        ...existing,
                                                        {
                                                            id: "",
                                                            direction,
                                                            slotName: "",
                                                            artifactInstanceId: instance.id,
                                                            required: true,
                                                            position: draftSource.artifactBindings.filter((b) => b.direction === direction).length,
                                                            createdAt: "",
                                                        },
                                                    ];
                                                updateDraft({ artifactBindings: next });
                                            }, type: "checkbox" }), (0, jsx_runtime_1.jsxs)("span", { children: [(0, jsx_runtime_1.jsx)("strong", { children: artifactInstanceLabel(instance, instance.id) }), (0, jsx_runtime_1.jsx)("small", { children: instance.artifactTypeId })] })] }, instance.id));
                            }) }));
                    })()) : null] }) }));
    };
    return ((0, jsx_runtime_1.jsxs)("section", { className: "settings-panel", children: [(0, jsx_runtime_1.jsxs)("div", { className: "settings-panel-head", children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { className: "settings-eyebrow", children: "Workflows/Steps" }), (0, jsx_runtime_1.jsx)("h2", { children: "Workflows/Steps" }), (0, jsx_runtime_1.jsx)("p", { children: "Manage workflow definitions and reusable step definitions in separate create and detail views." })] }), (0, jsx_runtime_1.jsxs)("div", { className: "header-tabs", children: [(0, jsx_runtime_1.jsx)("button", { className: `header-tab ${tab === "workflows" ? "active" : ""}`, onClick: () => {
                                    setTab("workflows");
                                    exitBulkSelect();
                                }, type: "button", children: "Workflows" }), (0, jsx_runtime_1.jsx)("button", { className: `header-tab ${tab === "steps" ? "active" : ""}`, onClick: () => {
                                    setTab("steps");
                                    exitBulkSelect();
                                }, type: "button", children: "Steps" }), (0, jsx_runtime_1.jsx)("button", { className: `header-tab ${tab === "artifacts" ? "active" : ""}`, onClick: () => {
                                    setTab("artifacts");
                                    exitBulkSelect();
                                }, type: "button", children: "Artifacts" })] })] }), message ? (0, jsx_runtime_1.jsx)("div", { className: `settings-feedback ${message.toLowerCase().includes("unable") ? "error" : ""}`, children: message }) : null, tab === "artifacts" ? ((0, jsx_runtime_1.jsx)(ArtifactsTabContent, { artifactInstanceDraft: artifactInstanceDraft, artifactInstances: artifactInstances, artifactInstanceView: artifactInstanceView, artifactTypes: artifactTypes, busy: busy, onCreateNew: (artifactTypeId) => {
                    setArtifactInstanceDraft({
                        id: "",
                        projectId: null,
                        artifactTypeId,
                        name: "",
                        description: "",
                        configJson: artifactTypeId === "file_artifact.v1"
                            ? (0, fileArtifactConfig_1.emptyFileArtifactConfig)()
                            : artifactTypeId === "telegram.v1"
                                ? { chatId: "", messageTemplate: "" }
                                : { sources: [] },
                        isBuiltin: false,
                        status: "active",
                        createdAt: "",
                        updatedAt: "",
                    });
                    setArtifactInstanceView("create");
                }, onDelete: async (instanceId) => {
                    setBusy(true);
                    try {
                        const admin = await (0, clientCore_1.getAdminUseCases)();
                        await admin.workflows.deleteArtifactInstance(instanceId);
                        await refresh(selectedWorkflowId, selectedStepType, { preserveCreateDrafts: true });
                        setMessage("Artifact instance deleted.");
                    }
                    catch (error) {
                        setMessage((0, settingsHelpers_1.toErrorMessage)(error, "Unable to delete artifact instance."));
                    }
                    finally {
                        setBusy(false);
                    }
                }, onSave: async () => {
                    if (!artifactInstanceDraft)
                        return;
                    setBusy(true);
                    try {
                        const admin = await (0, clientCore_1.getAdminUseCases)();
                        const draftToSave = artifactInstanceDraft.artifactTypeId === "file_artifact.v1"
                            ? {
                                ...artifactInstanceDraft,
                                configJson: (0, fileArtifactConfig_1.normalizeFileArtifactConfigForSave)(artifactInstanceDraft.configJson),
                            }
                            : artifactInstanceDraft;
                        await admin.workflows.saveArtifactInstance(draftToSave);
                        await refresh(selectedWorkflowId, selectedStepType, { preserveCreateDrafts: true });
                        setArtifactInstanceView("list");
                        setArtifactInstanceDraft(null);
                        setMessage("Artifact instance saved.");
                    }
                    catch (error) {
                        setMessage((0, settingsHelpers_1.toErrorMessage)(error, "Unable to save artifact instance."));
                    }
                    finally {
                        setBusy(false);
                    }
                }, onSelect: (instance) => {
                    setArtifactInstanceDraft(instance);
                    setArtifactInstanceView("create");
                }, setArtifactInstanceDraft: setArtifactInstanceDraft, setArtifactInstanceView: setArtifactInstanceView })) : tab === "workflows" ? (workflowView === "create" ? ((0, jsx_runtime_1.jsxs)("div", { className: "workflow-editor-column", children: [(0, jsx_runtime_1.jsxs)("div", { className: "settings-subpanel", children: [(0, jsx_runtime_1.jsx)("div", { className: "project-create-back-row", children: (0, jsx_runtime_1.jsx)("button", { "aria-label": "Back to workflows", className: "secondary-btn project-icon-btn", onClick: () => setWorkflowView("list"), title: "Back to workflows", type: "button", children: "<" }) }), (0, jsx_runtime_1.jsxs)("div", { className: "project-create-head", children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { className: "settings-eyebrow", children: "Create Workflow" }), (0, jsx_runtime_1.jsx)("h3", { children: "New Workflow" }), (0, jsx_runtime_1.jsx)("p", { className: "project-muted-copy", children: "Create first, then return to the list/detail layout for future edits." })] }), (0, jsx_runtime_1.jsx)("button", { className: "primary-btn", disabled: busy || !createWorkflowDraft.name.trim(), onClick: () => void saveNewWorkflow(), type: "button", children: "Save Workflow" })] }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-grid", children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Owner project" }), (0, jsx_runtime_1.jsxs)("select", { onChange: (event) => setCreateWorkflowDraft((current) => ({
                                                    ...current,
                                                    projectId: event.target.value || null,
                                                })), value: createWorkflowDraft.projectId ?? "", children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "Workspace global" }), projects.map((project) => ((0, jsx_runtime_1.jsx)("option", { value: project.id, children: project.name }, project.id)))] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Name" }), (0, jsx_runtime_1.jsx)("input", { onChange: (event) => setCreateWorkflowDraft((current) => ({
                                                    ...current,
                                                    name: event.target.value,
                                                })), value: createWorkflowDraft.name })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("span", { children: "Description" }), (0, jsx_runtime_1.jsx)("textarea", { onChange: (event) => setCreateWorkflowDraft((current) => ({
                                                    ...current,
                                                    description: event.target.value,
                                                })), value: createWorkflowDraft.description })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Model override" }), (0, jsx_runtime_1.jsxs)("select", { onChange: (event) => setCreateWorkflowDraft((current) => ({
                                                    ...current,
                                                    modelOverride: event.target.value,
                                                })), value: createWorkflowDraft.modelOverride ?? "", children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "Select a model..." }), modelOptions.map((model) => ((0, jsx_runtime_1.jsx)("option", { value: model.value, children: model.label }, model.value)))] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Reasoning effort" }), (0, jsx_runtime_1.jsx)("select", { onChange: (event) => setCreateWorkflowDraft((current) => ({
                                                    ...current,
                                                    reasoningEffortOverride: event.target.value,
                                                })), value: createWorkflowDraft.reasoningEffortOverride ?? DEFAULT_REASONING, children: REASONING_OPTIONS.map((option) => ((0, jsx_runtime_1.jsx)("option", { value: option.value, children: option.label }, option.value))) })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-checkbox settings-field-full", title: "Flow/Workflow launches always run YOLO \u2014 the gate/approval machinery here is not robust against a paused approval mid-flow. Use Normal Chat for gated (YOLO=off) runs.", children: [(0, jsx_runtime_1.jsx)("input", { checked: true, disabled: true, type: "checkbox" }), (0, jsx_runtime_1.jsx)("span", { children: "YOLO mode (always on for Flow mode)" })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Cap (max rounds before blocking)" }), (0, jsx_runtime_1.jsx)("input", { min: 1, onChange: (event) => setCreateWorkflowDraft((current) => ({
                                                    ...current,
                                                    policyCap: event.target.value ? Number(event.target.value) : null,
                                                })), placeholder: "3 (default)", type: "number", value: createWorkflowDraft.policyCap ?? "" })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Extend by (rounds added when a cap is extended)" }), (0, jsx_runtime_1.jsx)("input", { min: 1, onChange: (event) => setCreateWorkflowDraft((current) => ({
                                                    ...current,
                                                    policyExtendBy: event.target.value ? Number(event.target.value) : null,
                                                })), placeholder: "2 (default)", type: "number", value: createWorkflowDraft.policyExtendBy ?? "" })] })] })] }), renderWorkflowStepsEditor(createWorkflowSteps, "create", createWorkflowStepType, setCreateWorkflowStepType), renderWorkflowEdgesEditor(createWorkflowDraft.edges, createWorkflowSteps, "create"), renderWorkflowFlowCanvas(createWorkflowDraft.edges, createWorkflowSteps, "create")] })) : ((0, jsx_runtime_1.jsxs)("div", { className: "project-layout", children: [(0, jsx_runtime_1.jsxs)("aside", { className: "settings-subpanel project-registry-panel", children: [(0, jsx_runtime_1.jsxs)("div", { className: "project-registry-head", children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { className: "settings-eyebrow", children: "Workflow Registry" }), (0, jsx_runtime_1.jsx)("h3", { children: "Definitions" }), (0, jsx_runtime_1.jsx)("p", { className: "project-muted-copy", children: "Select a workflow on the left to edit it on the right." })] }), (0, jsx_runtime_1.jsx)("button", { "aria-label": "Create workflow", className: "primary-btn project-create-fab", onClick: startCreateWorkflow, type: "button", children: "+" })] }), bulkSelect?.kind === "workflow" ? ((0, jsx_runtime_1.jsxs)("div", { className: "project-bulk-bar", children: [(0, jsx_runtime_1.jsxs)("span", { children: [bulkSelect.ids.size, " selected"] }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-inline-actions", children: [(0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", onClick: exitBulkSelect, type: "button", children: "Cancel" }), (0, jsx_runtime_1.jsxs)("button", { className: "secondary-btn danger-btn", disabled: bulkSelect.ids.size === 0, onClick: () => {
                                                    const ids = Array.from(bulkSelect.ids);
                                                    openDeleteWorkflowConfirm(ids, ids.map((id) => workflows.find((workflow) => workflow.id === id)?.name ?? id));
                                                }, type: "button", children: ["Delete ", bulkSelect.ids.size] })] })] })) : null, (0, jsx_runtime_1.jsx)("div", { className: "settings-list", children: workflows.length === 0 ? ((0, jsx_runtime_1.jsx)("div", { className: "settings-empty", children: "No workflows found. Use the + button to create one." })) : (workflows.map((workflow) => {
                                    const bulkModeActive = bulkSelect?.kind === "workflow";
                                    const isBulkSelected = bulkModeActive && bulkSelect.ids.has(workflow.id);
                                    // Built-ins can't be deleted (Delete is disabled for them
                                    // below too), so long-press on one is a no-op instead of
                                    // starting a select mode that can only ever fail to delete.
                                    const longPressSelectable = workflow.editable !== false;
                                    return ((0, jsx_runtime_1.jsxs)("button", { className: `settings-list-item ${workflow.id === selectedWorkflowId && !bulkModeActive ? "active" : ""} ${isBulkSelected ? "bulk-selected" : ""} ${bulkModeActive ? "checkable" : ""} ${!longPressSelectable ? "not-deletable" : ""}`, onClick: () => {
                                            if (consumeLongPressClick())
                                                return;
                                            if (bulkModeActive) {
                                                if (longPressSelectable)
                                                    toggleBulkSelected(workflow.id);
                                                return;
                                            }
                                            void selectWorkflowDefinition(workflow.id);
                                            setMessage(null);
                                        }, onPointerCancel: clearLongPressTimer, onPointerDown: () => {
                                            if (longPressSelectable)
                                                startLongPress("workflow", workflow.id);
                                        }, onPointerLeave: clearLongPressTimer, onPointerUp: clearLongPressTimer, title: longPressSelectable ? undefined : "Built-in workflows can't be deleted.", type: "button", children: [bulkModeActive ? ((0, jsx_runtime_1.jsx)("input", { "aria-hidden": "true", checked: isBulkSelected, className: "settings-bulk-checkbox", disabled: !longPressSelectable, readOnly: true, tabIndex: -1, type: "checkbox" })) : null, (0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsxs)("strong", { children: [workflow.name, workflow.isBuiltin ? (0, jsx_runtime_1.jsx)("span", { className: "settings-badge", children: "Built-in" }) : null] }), (0, jsx_runtime_1.jsxs)("span", { children: [workflow.projectId
                                                                ? projects.find((project) => project.id === workflow.projectId)?.name ??
                                                                    workflow.projectId
                                                                : "Workspace global", " ", "/ ", (0, settingsHelpers_1.formatTimestamp)(workflow.updatedAt)] }), workflow.isBuiltin ? ((0, jsx_runtime_1.jsxs)("span", { className: "settings-list-item-meta", children: [workflow.packId ?? "unknown pack", workflow.packVersion ? ` v${workflow.packVersion}` : "", workflow.selectableIn.length > 0
                                                                ? ` · selectable in: ${workflow.selectableIn.join(", ")}`
                                                                : ""] })) : null] })] }, workflow.id));
                                })) })] }), (0, jsx_runtime_1.jsx)("div", { className: "project-detail-column", children: workflowDraft ? ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsxs)("div", { className: "settings-subpanel project-detail-panel", children: [(0, jsx_runtime_1.jsxs)("div", { className: "project-create-head", children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsxs)("div", { className: "settings-eyebrow", children: ["Workflow Detail", selectedWorkflow?.isBuiltin ? (0, jsx_runtime_1.jsx)("span", { className: "settings-badge", children: "Built-in" }) : null] }), (0, jsx_runtime_1.jsx)("h3", { children: workflowDraft.name || "Untitled Workflow" }), (0, jsx_runtime_1.jsx)("p", { className: "project-muted-copy", children: selectedWorkflow?.editable === false
                                                                ? "This is a built-in template and cannot be edited directly. Clone it to make changes."
                                                                : "Edit definition fields here. Save only becomes active after a change." }), selectedWorkflow?.isBuiltin ? ((0, jsx_runtime_1.jsxs)("p", { className: "project-muted-copy settings-list-item-meta", children: ["Pack: ", selectedWorkflow.packId ?? "unknown", selectedWorkflow.packVersion ? ` v${selectedWorkflow.packVersion}` : "", " \u00B7 Selectable in: ", selectedWorkflow.selectableIn.length > 0 ? selectedWorkflow.selectableIn.join(", ") : "none", selectedWorkflow.chatBaseline ? " (chat baseline, always on)" : ""] })) : null, workflowDraft.acceptanceNodes.length > 0 ? ((0, jsx_runtime_1.jsxs)("p", { className: "project-muted-copy settings-list-item-meta", children: ["Acceptance nodes (read-only in this UI): ", workflowDraft.acceptanceNodes.join(", ")] })) : null] }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-inline-actions", children: [selectedWorkflow?.cloneable !== false ? ((0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", onClick: () => {
                                                                setCloneTarget({ workflowId: selectedWorkflowId, sourceName: workflowDraft.name || "Untitled Workflow" });
                                                                setCloneName(`${workflowDraft.name || "Untitled Workflow"} (copy)`);
                                                            }, type: "button", children: "Clone" })) : null, (0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", disabled: selectedWorkflow?.editable === false, onClick: () => openDeleteWorkflowConfirm([selectedWorkflowId], [workflowDraft.name || "this workflow"]), title: selectedWorkflow?.editable === false
                                                                ? "Built-in workflows can't be deleted."
                                                                : undefined, type: "button", children: "Delete" }), (0, jsx_runtime_1.jsx)("button", { className: "primary-btn", disabled: busy || !workflowDirty, onClick: () => void saveWorkflow(), type: "button", children: "Save Workflow" })] })] }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-grid", children: [(0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Owner project" }), (0, jsx_runtime_1.jsxs)("select", { disabled: workflowDetailReadOnly, onChange: (event) => setWorkflowDraft((current) => current
                                                                ? { ...current, projectId: event.target.value || null }
                                                                : current), value: workflowDraft.projectId ?? "", children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "Workspace global" }), projects.map((project) => ((0, jsx_runtime_1.jsx)("option", { value: project.id, children: project.name }, project.id)))] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Name" }), (0, jsx_runtime_1.jsx)("input", { disabled: workflowDetailReadOnly, onChange: (event) => setWorkflowDraft((current) => current ? { ...current, name: event.target.value } : current), value: workflowDraft.name })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("span", { children: "Description" }), (0, jsx_runtime_1.jsx)("textarea", { disabled: workflowDetailReadOnly, onChange: (event) => setWorkflowDraft((current) => current ? { ...current, description: event.target.value } : current) })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Model override" }), (0, jsx_runtime_1.jsxs)("select", { onChange: (event) => setWorkflowDraft((current) => current
                                                                ? { ...current, modelOverride: event.target.value }
                                                                : current), value: workflowDraft.modelOverride ?? "", children: [(0, jsx_runtime_1.jsx)("option", { value: "", children: "Select a model..." }), modelOptions.map((model) => ((0, jsx_runtime_1.jsx)("option", { value: model.value, children: model.label }, model.value)))] })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Reasoning effort" }), (0, jsx_runtime_1.jsx)("select", { onChange: (event) => setWorkflowDraft((current) => current
                                                                ? {
                                                                    ...current,
                                                                    reasoningEffortOverride: event.target.value,
                                                                }
                                                                : current), value: workflowDraft.reasoningEffortOverride ?? DEFAULT_REASONING, children: REASONING_OPTIONS.map((option) => ((0, jsx_runtime_1.jsx)("option", { value: option.value, children: option.label }, option.value))) })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-checkbox settings-field-full", title: "Flow/Workflow launches always run YOLO \u2014 the gate/approval machinery here is not robust against a paused approval mid-flow. Use Normal Chat for gated (YOLO=off) runs.", children: [(0, jsx_runtime_1.jsx)("input", { checked: true, disabled: true, type: "checkbox" }), (0, jsx_runtime_1.jsx)("span", { children: "YOLO mode (always on for Flow mode)" })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Cap (max rounds before blocking)" }), (0, jsx_runtime_1.jsx)("input", { disabled: workflowDetailReadOnly, min: 1, onChange: (event) => setWorkflowDraft((current) => current
                                                                ? { ...current, policyCap: event.target.value ? Number(event.target.value) : null }
                                                                : current), placeholder: "3 (default)", type: "number", value: workflowDraft.policyCap ?? "" })] }), (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Extend by (rounds added when a cap is extended)" }), (0, jsx_runtime_1.jsx)("input", { disabled: workflowDetailReadOnly, min: 1, onChange: (event) => setWorkflowDraft((current) => current
                                                                ? {
                                                                    ...current,
                                                                    policyExtendBy: event.target.value ? Number(event.target.value) : null,
                                                                }
                                                                : current), placeholder: "2 (default)", type: "number", value: workflowDraft.policyExtendBy ?? "" })] })] })] }), renderWorkflowStepsEditor(workflowSteps, "detail", detailWorkflowStepType, setDetailWorkflowStepType), renderWorkflowEdgesEditor(workflowDraft.edges, workflowSteps, "detail"), renderWorkflowFlowCanvas(workflowDraft.edges, workflowSteps, "detail")] })) : ((0, jsx_runtime_1.jsx)("div", { className: "settings-subpanel", children: (0, jsx_runtime_1.jsx)("div", { className: "settings-empty", children: "Select a workflow to inspect its detail page." }) })) })] }))) : stepView === "create" ? ((0, jsx_runtime_1.jsx)("div", { className: "workflow-editor-column", children: (0, jsx_runtime_1.jsxs)("div", { className: "settings-subpanel", children: [(0, jsx_runtime_1.jsx)("div", { className: "project-create-back-row", children: (0, jsx_runtime_1.jsx)("button", { "aria-label": "Back to steps", className: "secondary-btn project-icon-btn", onClick: () => setStepView("list"), title: "Back to steps", type: "button", children: "<" }) }), (0, jsx_runtime_1.jsxs)("div", { className: "project-create-head", children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { className: "settings-eyebrow", children: "Create Step" }), (0, jsx_runtime_1.jsx)("h3", { children: "New Step Definition" }), (0, jsx_runtime_1.jsx)("p", { className: "project-muted-copy", children: "Create a reusable step first, then attach it from the workflow editor." })] }), (0, jsx_runtime_1.jsx)("button", { className: "primary-btn", disabled: busy || !createStepDraft.stepType.trim() || !createStepDraft.name.trim(), onClick: () => void saveNewStepDefinition(), type: "button", children: "Save Step" })] }), renderStepDefinitionForm(createStepDraft, setCreateStepDraft, "create")] }) })) : ((0, jsx_runtime_1.jsxs)("div", { className: "project-layout", children: [(0, jsx_runtime_1.jsxs)("aside", { className: "settings-subpanel project-registry-panel", children: [(0, jsx_runtime_1.jsxs)("div", { className: "project-registry-head", children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { className: "settings-eyebrow", children: "Step Registry" }), (0, jsx_runtime_1.jsx)("h3", { children: "Definitions" }), (0, jsx_runtime_1.jsx)("p", { className: "project-muted-copy", children: "Reusable execution units for workflows and direct single-step runs." })] }), (0, jsx_runtime_1.jsx)("button", { "aria-label": "Create step definition", className: "primary-btn project-create-fab", onClick: startCreateStep, type: "button", children: "+" })] }), bulkSelect?.kind === "step" ? ((0, jsx_runtime_1.jsxs)("div", { className: "project-bulk-bar", children: [(0, jsx_runtime_1.jsxs)("span", { children: [bulkSelect.ids.size, " selected"] }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-inline-actions", children: [(0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", onClick: exitBulkSelect, type: "button", children: "Cancel" }), (0, jsx_runtime_1.jsxs)("button", { className: "secondary-btn danger-btn", disabled: bulkSelect.ids.size === 0, onClick: () => {
                                                    const ids = Array.from(bulkSelect.ids);
                                                    void openDeleteStepConfirm(ids, ids.map((id) => stepDefinitions.find((step) => step.stepType === id)?.name ?? id));
                                                }, type: "button", children: ["Delete ", bulkSelect.ids.size] })] })] })) : null, (0, jsx_runtime_1.jsx)("div", { className: "settings-list", children: stepDefinitions.length === 0 ? ((0, jsx_runtime_1.jsx)("div", { className: "settings-empty", children: "No step definitions found. Use the + button to create one." })) : (stepDefinitions.map((step) => {
                                    const bulkModeActive = bulkSelect?.kind === "step";
                                    const isBulkSelected = bulkModeActive && bulkSelect.ids.has(step.stepType);
                                    // A step still listed by a built-in workflow can't be
                                    // deleted (that workflow is read-only), so it can't be
                                    // long-pressed into a select mode that can only fail.
                                    const longPressSelectable = !stepTypesUsedByBuiltin.has(step.stepType);
                                    return ((0, jsx_runtime_1.jsxs)("button", { className: `settings-list-item ${step.stepType === selectedStepType && !bulkModeActive ? "active" : ""} ${isBulkSelected ? "bulk-selected" : ""} ${bulkModeActive ? "checkable" : ""} ${!longPressSelectable ? "not-deletable" : ""}`, onClick: () => {
                                            if (consumeLongPressClick())
                                                return;
                                            if (bulkModeActive) {
                                                if (longPressSelectable)
                                                    toggleBulkSelected(step.stepType);
                                                return;
                                            }
                                            selectStepDefinition(step.stepType);
                                            setMessage(null);
                                        }, onPointerCancel: clearLongPressTimer, onPointerDown: () => {
                                            if (longPressSelectable)
                                                startLongPress("step", step.stepType);
                                        }, onPointerLeave: clearLongPressTimer, onPointerUp: clearLongPressTimer, title: longPressSelectable ? undefined : "Used by a built-in workflow — can't be deleted.", type: "button", children: [bulkModeActive ? ((0, jsx_runtime_1.jsx)("input", { "aria-hidden": "true", checked: isBulkSelected, className: "settings-bulk-checkbox", disabled: !longPressSelectable, readOnly: true, tabIndex: -1, type: "checkbox" })) : null, (0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("strong", { children: step.name }), (0, jsx_runtime_1.jsx)("span", { children: (0, stepModelVisibility_1.stepDefinitionListSubtitle)(step) })] })] }, step.stepType));
                                })) })] }), (0, jsx_runtime_1.jsx)("div", { className: "project-detail-column", children: stepDraft ? ((0, jsx_runtime_1.jsxs)("div", { className: "settings-subpanel project-detail-panel", children: [(0, jsx_runtime_1.jsxs)("div", { className: "project-create-head", children: [(0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { className: "settings-eyebrow", children: "Step Detail" }), (0, jsx_runtime_1.jsx)("h3", { children: stepDraft.name || stepDraft.stepType || "Untitled Step" }), (0, jsx_runtime_1.jsx)("p", { className: "project-muted-copy", children: "Edit runtime contract fields here. Save only becomes active after a change." })] }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-inline-actions", children: [(0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", disabled: stepTypesUsedByBuiltin.has(stepDraft.stepType), onClick: () => void openDeleteStepConfirm([stepDraft.stepType], [stepDraft.name || stepDraft.stepType]), title: stepTypesUsedByBuiltin.has(stepDraft.stepType)
                                                        ? "Used by a built-in workflow — can't be deleted."
                                                        : undefined, type: "button", children: "Delete" }), (0, jsx_runtime_1.jsx)("button", { className: "primary-btn", disabled: busy || !stepDirty, onClick: () => void saveStepDefinition(), type: "button", children: "Save Step" })] })] }), renderStepDefinitionForm(stepDraft, setStepDraft, "detail")] })) : ((0, jsx_runtime_1.jsx)("div", { className: "settings-subpanel", children: (0, jsx_runtime_1.jsx)("div", { className: "settings-empty", children: "Select a step definition to inspect its detail page." }) })) })] })), renderPickerModal(), cloneTarget ? ((0, jsx_runtime_1.jsx)("div", { className: "settings-modal-backdrop", role: "presentation", children: (0, jsx_runtime_1.jsxs)("div", { className: "settings-modal", children: [(0, jsx_runtime_1.jsx)("div", { className: "project-create-head", children: (0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { className: "settings-eyebrow", children: "Clone" }), (0, jsx_runtime_1.jsx)("h3", { children: "Clone Workflow" }), (0, jsx_runtime_1.jsxs)("p", { className: "project-muted-copy", children: ["Creates an editable copy of ", (0, jsx_runtime_1.jsx)("strong", { children: cloneTarget.sourceName }), ". The original stays unchanged."] })] }) }), (0, jsx_runtime_1.jsx)("div", { className: "settings-grid", children: (0, jsx_runtime_1.jsxs)("label", { className: "settings-field settings-field-full", children: [(0, jsx_runtime_1.jsx)("span", { children: "New workflow name" }), (0, jsx_runtime_1.jsx)("input", { onChange: (event) => setCloneName(event.target.value), value: cloneName })] }) }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-actions", children: [(0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", onClick: () => {
                                        setCloneTarget(null);
                                        setCloneName("");
                                    }, type: "button", children: "Cancel" }), (0, jsx_runtime_1.jsx)("button", { className: "primary-btn", disabled: busy || !cloneName.trim(), onClick: () => void cloneSelected(), type: "button", children: "Clone Workflow" })] })] }) })) : null, deleteTarget ? ((0, jsx_runtime_1.jsx)("div", { className: "settings-modal-backdrop", role: "presentation", children: (0, jsx_runtime_1.jsxs)("div", { className: "settings-modal project-delete-modal", children: [(0, jsx_runtime_1.jsx)("div", { className: "project-create-head", children: (0, jsx_runtime_1.jsxs)("div", { children: [(0, jsx_runtime_1.jsx)("div", { className: "settings-eyebrow", children: "Delete" }), (0, jsx_runtime_1.jsx)("h3", { children: deleteTarget.kind === "workflow"
                                            ? deleteTarget.ids.length > 1
                                                ? `Delete ${deleteTarget.ids.length} Workflows`
                                                : "Delete Workflow"
                                            : deleteTarget.ids.length > 1
                                                ? `Delete ${deleteTarget.ids.length} Step Definitions`
                                                : "Delete Step Definition" }), (0, jsx_runtime_1.jsxs)("p", { className: "project-muted-copy", children: ["Type ", (0, jsx_runtime_1.jsx)("code", { children: "delete" }), " to confirm deleting", " ", deleteTarget.labels.length > 1 ? ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [deleteTarget.labels.length, " items"] })) : ((0, jsx_runtime_1.jsx)("strong", { children: deleteTarget.labels[0] })), "."] }), deleteTarget.labels.length > 1 ? ((0, jsx_runtime_1.jsx)("p", { className: "project-muted-copy settings-list-item-meta", children: deleteTarget.labels.join(", ") })) : null, deleteTarget.kind === "step" && deleteTarget.cascadeWorkflows.length > 0 ? ((0, jsx_runtime_1.jsxs)("div", { className: "settings-warning", children: ["Still used by ", deleteTarget.cascadeWorkflows.length, " workflow", deleteTarget.cascadeWorkflows.length > 1 ? "s" : "", " \u2014 deleting", " ", deleteTarget.ids.length > 1 ? "these steps" : "this step", " will also delete", " ", deleteTarget.cascadeWorkflows.map((workflow) => workflow.name).join(", "), "."] })) : null] }) }), (0, jsx_runtime_1.jsx)("div", { className: "settings-grid", children: (0, jsx_runtime_1.jsxs)("label", { className: "settings-field", children: [(0, jsx_runtime_1.jsx)("span", { children: "Confirmation" }), (0, jsx_runtime_1.jsx)("input", { onChange: (event) => setDeleteConfirmationText(event.target.value), value: deleteConfirmationText })] }) }), (0, jsx_runtime_1.jsxs)("div", { className: "settings-actions", children: [(0, jsx_runtime_1.jsx)("button", { className: "secondary-btn", onClick: () => {
                                        setDeleteTarget(null);
                                        setDeleteConfirmationText("");
                                    }, type: "button", children: "Cancel" }), (0, jsx_runtime_1.jsx)("button", { className: "primary-btn", disabled: busy || deleteConfirmationText !== "delete", onClick: () => void deleteSelected(), type: "button", children: deleteTarget.kind === "workflow"
                                        ? deleteTarget.ids.length > 1
                                            ? `Delete ${deleteTarget.ids.length} Workflows`
                                            : "Delete Workflow"
                                        : deleteTarget.ids.length > 1
                                            ? `Delete ${deleteTarget.ids.length} Steps`
                                            : "Delete Step" })] })] }) })) : null, busy ? ((0, jsx_runtime_1.jsx)("div", { className: "settings-modal-backdrop workflow-settings-loading-modal-backdrop", role: "presentation", children: (0, jsx_runtime_1.jsxs)("div", { "aria-live": "polite", "aria-modal": "true", className: "settings-modal workflow-settings-loading-modal", role: "status", children: [(0, jsx_runtime_1.jsx)("span", { className: "workflow-settings-loading-dot" }), (0, jsx_runtime_1.jsx)("strong", { children: "Syncing changes\u2026" }), (0, jsx_runtime_1.jsx)("p", { children: "Saving updates and refreshing the latest list." })] }) })) : null] }));
}
