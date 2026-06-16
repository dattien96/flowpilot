"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.desktopBridgeFetch = void 0;
function getFlowpilotHttpBridge() {
    const candidate = globalThis;
    return candidate.window?.flowpilot;
}
const desktopBridgeFetch = async (input, init) => {
    const request = new Request(input, init);
    const bridge = getFlowpilotHttpBridge();
    if (!bridge?.requestHttp) {
        return fetch(request);
    }
    const headers = Object.fromEntries(request.headers.entries());
    const body = request.method === "GET" || request.method === "HEAD"
        ? undefined
        : await request.text();
    const response = await bridge.requestHttp({
        url: request.url,
        method: request.method,
        headers,
        body,
    });
    const nullBodyStatuses = new Set([101, 103, 204, 205, 304]);
    return new Response(nullBodyStatuses.has(response.status) ? null : response.body, {
        status: response.status,
        headers: response.headers,
    });
};
exports.desktopBridgeFetch = desktopBridgeFetch;
