"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.DRIFT_PATH_MARKER = void 0;
exports.parseDriftedPaths = parseDriftedPaths;
exports.awaitingUserDriftState = awaitingUserDriftState;
/** Task-309: frozen-contract scope drift parsing (mirrors gate_hook.go:863 / TUI parseDriftedPaths). */
exports.DRIFT_PATH_MARKER = "wrote outside the frozen contract's declared paths:";
function parseDriftedPaths(gate) {
    const idx = gate.indexOf(exports.DRIFT_PATH_MARKER);
    if (idx < 0)
        return null;
    let rest = gate.slice(idx + exports.DRIFT_PATH_MARKER.length).trim();
    if (rest.endsWith("."))
        rest = rest.slice(0, -1).trim();
    if (!rest)
        return null;
    const parts = rest
        .split(",")
        .map((p) => p.trim().replace(/\.$/, "").trim())
        .filter((p) => p.length > 0);
    return parts.length > 0 ? parts : null;
}
function awaitingUserDriftState(loopState) {
    const stalled = loopState.blockReason === "member_stalled";
    const isCap = loopState.blockReason === "cap";
    const driftedPaths = !stalled && !isCap ? parseDriftedPaths(loopState.gateReason ?? "") : null;
    const isDrift = driftedPaths !== null && driftedPaths.length > 0;
    return {
        driftedPaths,
        isDrift,
        stalled,
        isCap,
        retryIsPrimary: !isDrift,
    };
}
