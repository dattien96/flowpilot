"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.SCENARIO_NAMES = exports.MOCK_SKILLS = exports.MOCK_STEPS = exports.MOCK_WORKFLOWS = exports.MOCK_PROJECTS = void 0;
exports.mockArtifacts = mockArtifacts;
exports.scriptFor = scriptFor;
// ============================================================================
// Static fixtures + scenario scripts for the Phase 1 mock MVP (04-01 Part A).
// Everything here is fake; no backend, Codex, or Supabase is involved.
// ============================================================================
exports.MOCK_PROJECTS = [
    { id: "proj-web", name: "Acme Web App", path: "/Users/dev/acme-web" },
    { id: "proj-android", name: "Acme Android", path: "/Users/dev/acme-android" },
];
exports.MOCK_WORKFLOWS = {
    "proj-web": [
        { id: "wf-feature", projectId: "proj-web", name: "Implement Feature", description: "Plan → code → test → summarize." },
        { id: "wf-bugfix", projectId: "proj-web", name: "Fix Bug", description: "Reproduce → patch → verify." },
    ],
    "proj-android": [
        { id: "wf-screen", projectId: "proj-android", name: "Build Screen", description: "Compose UI → wire VM → test." },
    ],
};
exports.MOCK_STEPS = {
    "wf-feature": [
        { id: "step-plan", workflowId: "wf-feature", name: "Plan", order: 1, defaultSkill: "architect" },
        { id: "step-code", workflowId: "wf-feature", name: "Implement", order: 2, defaultSkill: "coder" },
        { id: "step-test", workflowId: "wf-feature", name: "Write Tests", order: 3 },
        { id: "step-sum", workflowId: "wf-feature", name: "Summarize", order: 4 },
    ],
    "wf-bugfix": [
        { id: "bug-repro", workflowId: "wf-bugfix", name: "Reproduce", order: 1 },
        { id: "bug-patch", workflowId: "wf-bugfix", name: "Patch", order: 2, defaultSkill: "coder" },
    ],
    "wf-screen": [
        { id: "scr-ui", workflowId: "wf-screen", name: "Compose UI", order: 1 },
        { id: "scr-vm", workflowId: "wf-screen", name: "Wire ViewModel", order: 2 },
    ],
};
exports.MOCK_SKILLS = [
    { name: "architect", description: "High-level planning & design", source: "flowpilot" },
    { name: "coder", description: "Implementation-focused", source: "flowpilot" },
    { name: "reviewer", description: "Critical code review", source: "flowpilot" },
    { name: "test-writer", description: "Generates tests", source: "workspace" },
];
function mockArtifacts(runId) {
    return [
        { id: `${runId}-final`, runId, kind: "final_response", name: "final-response.md", preview: "Implemented the feature and added tests.", createdAt: "2026-06-12T10:00:00Z" },
        { id: `${runId}-diff`, runId, kind: "diff_snapshot", name: "changes.diff", preview: "3 files changed", createdAt: "2026-06-12T10:00:01Z" },
        { id: `${runId}-sum`, runId, kind: "summary", name: "summary.md", preview: "Short run summary", createdAt: "2026-06-12T10:00:02Z" },
    ];
}
exports.SCENARIO_NAMES = [
    "normal",
    "approval-required",
    "question-required",
    "tool-heavy",
    "file-changes",
    "failed",
    "reconnect/replay",
];
const NORMAL = [
    { kind: "delta", delay: 250, text: "Sure — let me work through this step.\n\n" },
    {
        kind: "token_usage",
        delay: 120,
        tokenUsage: {
            last: { cachedInputTokens: 320, inputTokens: 2400, outputTokens: 680, reasoningOutputTokens: 140, totalTokens: 3540 },
            total: { cachedInputTokens: 320, inputTokens: 2400, outputTokens: 680, reasoningOutputTokens: 140, totalTokens: 3540 },
            modelContextWindow: 200_000,
        },
    },
    { kind: "delta", delay: 350, text: "I reviewed the relevant files and the approach looks sound. " },
    { kind: "delta", delay: 350, text: "Proceeding with the implementation now." },
    { kind: "completed", delay: 400, finalMessage: "Done. The change is implemented and the step is complete." },
];
const TOOL_HEAVY = [
    { kind: "delta", delay: 200, text: "Investigating the codebase...\n" },
    { kind: "tool_started", delay: 250, toolName: "grep", input: { pattern: "useAuth(", glob: "**/*.tsx" } },
    { kind: "tool_completed", delay: 450, toolName: "grep", status: "success", output: { matches: 7 } },
    { kind: "tool_started", delay: 200, toolName: "read_file", input: { path: "src/auth/useAuth.tsx" } },
    { kind: "tool_completed", delay: 400, toolName: "read_file", status: "success", output: { lines: 142 } },
    { kind: "tool_started", delay: 200, toolName: "run_tests", input: { suite: "auth" } },
    { kind: "tool_completed", delay: 700, toolName: "run_tests", status: "failed", output: { failed: 1, passed: 23 } },
    { kind: "delta", delay: 300, text: "One test failed; fixing it.\n" },
    { kind: "tool_started", delay: 200, toolName: "run_tests", input: { suite: "auth" } },
    { kind: "tool_completed", delay: 600, toolName: "run_tests", status: "success", output: { passed: 24 } },
    { kind: "completed", delay: 350, finalMessage: "All 24 auth tests pass." },
];
const FILE_CHANGES = [
    { kind: "delta", delay: 250, text: "Applying the edits across the module.\n" },
    { kind: "file_changed", delay: 300, path: "src/features/cart/cartSlice.ts", changeType: "modified" },
    { kind: "file_changed", delay: 250, path: "src/features/cart/Cart.tsx", changeType: "modified" },
    { kind: "file_changed", delay: 250, path: "src/features/cart/cart.test.ts", changeType: "created" },
    { kind: "file_changed", delay: 250, path: "src/features/cart/legacyCart.ts", changeType: "deleted" },
    { kind: "completed", delay: 350, finalMessage: "Refactored the cart module: 3 changed, 1 created, 1 removed." },
];
const FAILED = [
    { kind: "delta", delay: 250, text: "Attempting the operation...\n" },
    { kind: "tool_started", delay: 250, toolName: "build", input: { target: "web" } },
    { kind: "tool_completed", delay: 600, toolName: "build", status: "failed", output: { code: 1 } },
    { kind: "failed", delay: 350, error: "Build failed: type error in cartSlice.ts:42. (recoverable — you can resend)", recoverable: true },
];
const RECONNECT_PARTIAL = [
    { kind: "delta", delay: 250, text: "Starting the long task...\n" },
    { kind: "tool_started", delay: 250, toolName: "index_repo", input: { files: 1200 } },
    { kind: "tool_completed", delay: 500, toolName: "index_repo", status: "success", output: { indexed: 1200 } },
    { kind: "delta", delay: 300, text: "Halfway through analysis" },
    { kind: "disconnect", delay: 300, error: "stream disconnected (mock) — click Reconnect to replay" },
];
// Full happy path streamed (fast) on reconnect to visibly rebuild the timeline.
const RECONNECT_REPLAY = [
    { kind: "delta", delay: 90, text: "Starting the long task...\n" },
    { kind: "tool_started", delay: 90, toolName: "index_repo", input: { files: 1200 } },
    { kind: "tool_completed", delay: 120, toolName: "index_repo", status: "success", output: { indexed: 1200 } },
    { kind: "delta", delay: 90, text: "Halfway through analysis" },
    { kind: "delta", delay: 120, text: " ... analysis complete.\n" },
    { kind: "completed", delay: 200, finalMessage: "Recovered after reconnect; task finished." },
];
function approvalScript() {
    return [
        { kind: "delta", delay: 250, text: "I need to run a shell command to apply the migration.\n" },
        {
            kind: "await_approval",
            delay: 300,
            details: {
                command: "rm -rf ./dist && npm run migrate",
                cwd: "/Users/dev/acme-web",
                reason: "Runs a database migration after clearing the build output.",
                decisions: [
                    { value: "approve", label: "Approve" },
                    { value: "approve_for_session", label: "Approve for session" },
                    { value: "deny", label: "Deny" },
                ],
            },
            onApprove: [
                { kind: "tool_started", delay: 200, toolName: "shell", input: { cmd: "npm run migrate" } },
                { kind: "tool_completed", delay: 600, toolName: "shell", status: "success", output: { code: 0 } },
                { kind: "completed", delay: 300, finalMessage: "Migration applied successfully." },
            ],
            onDeny: [
                { kind: "tool_completed", delay: 150, toolName: "shell", status: "cancelled", output: { reason: "denied by user" } },
                { kind: "completed", delay: 250, finalMessage: "Command was denied; I stopped without running it." },
            ],
        },
    ];
}
function questionScript() {
    return [
        { kind: "delta", delay: 250, text: "Before I continue I need a decision from you.\n" },
        {
            kind: "await_question",
            delay: 300,
            prompt: "Which styling approach should I use for the new component?",
            multiSelect: false,
            options: [
                { label: "Tailwind utility classes", description: "Matches the rest of the app", value: "tailwind" },
                { label: "CSS Modules", description: "Scoped, but new pattern here", value: "css-modules" },
                { label: "Styled-components", description: "Runtime CSS-in-JS", value: "styled" },
            ],
            onAnswer: (choice) => {
                const picked = Array.isArray(choice) ? choice.join(", ") : choice;
                return [
                    { kind: "delta", delay: 250, text: `Got it — using "${picked}".\n` },
                    { kind: "file_changed", delay: 300, path: "src/components/NewWidget.tsx", changeType: "created" },
                    { kind: "completed", delay: 300, finalMessage: `Component created using ${picked}.` },
                ];
            },
        },
    ];
}
function scriptFor(scenario, opts) {
    switch (scenario) {
        case "normal":
            return NORMAL;
        case "tool-heavy":
            return TOOL_HEAVY;
        case "file-changes":
            return FILE_CHANGES;
        case "failed":
            return FAILED;
        case "approval-required":
            return approvalScript();
        case "question-required":
            return questionScript();
        case "reconnect/replay":
            return opts?.replay ? RECONNECT_REPLAY : RECONNECT_PARTIAL;
        default:
            return NORMAL;
    }
}
