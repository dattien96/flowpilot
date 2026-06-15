import test from "node:test";
import assert from "node:assert/strict";

import { desktopBridgeFetch } from "../../apps/desktop-flowpilot/src/auth/desktopBridgeHttp";

test("desktopBridgeFetch proxies renderer requests through the Electron bridge", async () => {
  const originalWindow = (globalThis as { window?: unknown }).window;
  const requests: Array<{
    url: string;
    method?: string;
    headers?: Record<string, string>;
    body?: string;
  }> = [];

  (globalThis as { window?: unknown }).window = {
    flowpilot: {
      requestHttp: async (request: {
        url: string;
        method?: string;
        headers?: Record<string, string>;
        body?: string;
      }) => {
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
    const response = await desktopBridgeFetch("https://example.com/rest/v1/projects", {
      method: "POST",
      headers: {
        apikey: "anon",
        "content-type": "application/json",
      },
      body: JSON.stringify({ hello: "world" }),
    });

    assert.equal(response.status, 200);
    assert.deepEqual(await response.json(), { ok: true });
    assert.deepEqual(requests, [
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
  } finally {
    (globalThis as { window?: unknown }).window = originalWindow;
  }
});
