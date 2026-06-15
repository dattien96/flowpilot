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
