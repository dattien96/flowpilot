"use strict";
// ============================================================================
// The client<->runner contract (04-01 "The contract (key output)").
//
// This is the ONE artifact Phase 1 must lock. Both MockRunnerClient (Part A) and
// HttpWsRunnerClient (Part B) implement `RunnerClient` against these types; the
// renderer only ever talks to `RunnerClient`. The `ProviderEventDTO` union is the
// serialized form of the shared `ProviderEvent` union in 04-Detailed-Coding-Plan.md.
// Keep this in sync with the Go runner's event DTOs.
// ============================================================================
Object.defineProperty(exports, "__esModule", { value: true });
exports.HANDOFF_PROMPT_PREFIX = exports.CHAT_POSTURES = void 0;
/** Posture labels/descriptions for the composer tabs + setup modal. */
exports.CHAT_POSTURES = [
    { key: "scan", label: "Scan", hint: "Read-only exploration — reads auto-approve, writes auto-deny." },
    { key: "plan", label: "Plan", hint: "Read-only planning — reads auto-approve, writes auto-deny." },
    { key: "code", label: "Code", hint: "Normal approvals / YOLO — full tool access." },
    { key: "non", label: "Non", hint: "No posture — keeps your last model choice across restarts." },
];
// ---- CP-59 chat SSOT (Task-316) -------------------------------------------
/** Mirrors the runner's handoff marker — parity pinned in the TUI runner suite. */
exports.HANDOFF_PROMPT_PREFIX = "[FlowPilot cross-provider chat handoff]";
