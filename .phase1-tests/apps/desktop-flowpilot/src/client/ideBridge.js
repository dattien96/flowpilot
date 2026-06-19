"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ideBridge = void 0;
// Prefers the Electron preload bridge (whose main-side handler invokes the real
// IDE CLI in Part B — code -g / cursor / studio / xed); falls back to console.log
// so the renderer also works in a plain browser tab during UI development. This
// renderer code is unchanged between Part A and Part B — only the main-side handler
// swapped from a stub to real invocation.
exports.ideBridge = {
    async openInIde(file, line) {
        const bridge = window;
        if (bridge.flowpilot?.openInIde) {
            await bridge.flowpilot.openInIde(file, line);
            return;
        }
        // eslint-disable-next-line no-console
        console.log("[IdeBridge stub:renderer] open in IDE:", { file, line });
    },
    async openExternal(url) {
        const bridge = window;
        if (bridge.flowpilot?.openExternal) {
            await bridge.flowpilot.openExternal(url);
            return;
        }
        // Browser fallback (renderer running in a plain tab during dev).
        window.open(url, "_blank", "noopener");
    },
};
