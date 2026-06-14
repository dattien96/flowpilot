"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const node_test_1 = __importDefault(require("node:test"));
const strict_1 = __importDefault(require("node:assert/strict"));
const desktopBridgeHttp_1 = require("../../apps/desktop-flowpilot/src/auth/desktopBridgeHttp");
(0, node_test_1.default)("desktopBridgeFetch proxies renderer requests through the Electron bridge", async () => {
    const originalWindow = globalThis.window;
    const requests = [];
    globalThis.window = {
        flowpilot: {
            requestHttp: async (request) => {
                requests.push(request);
                return {
                    status: 200,
                    headers: [["content-type", "application/json"]],
                    body: JSON.stringify({ ok: true }),
                };
            },
        },
    };
    try {
        const response = await (0, desktopBridgeHttp_1.desktopBridgeFetch)("https://example.com/rest/v1/projects", {
            method: "POST",
            headers: {
                apikey: "anon",
                "content-type": "application/json",
            },
            body: JSON.stringify({ hello: "world" }),
        });
        strict_1.default.equal(response.status, 200);
        strict_1.default.deepEqual(await response.json(), { ok: true });
        strict_1.default.deepEqual(requests, [
            {
                url: "https://example.com/rest/v1/projects",
                method: "POST",
                headers: {
                    apikey: "anon",
                    "content-type": "application/json",
                },
                body: JSON.stringify({ hello: "world" }),
            },
        ]);
    }
    finally {
        globalThis.window = originalWindow;
    }
});
