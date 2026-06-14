import test from "node:test";
import assert from "node:assert/strict";

import { RunnerAdminRepository } from "../../packages/flowpilot-client-core/src/data/runnerAdminRepository";
import type { HttpClient } from "../../packages/flowpilot-client-core/src/data/http";

class RecordingHttpClient implements HttpClient {
  public requests: Array<{ url: string; init?: RequestInit }> = [];

  constructor(
    private readonly responseFactory: (url: string, init?: RequestInit) => Response | Promise<Response>,
  ) {}

  async request(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const url = String(input);
    this.requests.push({ url, init });
    return this.responseFactory(url, init);
  }
}

test("RunnerAdminRepository validates directory paths through runner endpoint", async () => {
  const httpClient = new RecordingHttpClient(async () =>
    new Response(JSON.stringify({ path: "/workspace/app", usable: true, reason: "" }), {
      status: 200,
    }),
  );

  const repository = new RunnerAdminRepository(httpClient, "http://127.0.0.1:4317");
  const result = await repository.validatePath("/workspace/app");

  assert.deepEqual(result, { path: "/workspace/app", usable: true, reason: "" });
  assert.equal(httpClient.requests[0]?.url, "http://127.0.0.1:4317/directories/validate");
  assert.equal(httpClient.requests[0]?.init?.method, "POST");
});

test("RunnerAdminRepository starts provider auth through runner endpoint", async () => {
  const httpClient = new RecordingHttpClient(async () => new Response("", { status: 200 }));
  const repository = new RunnerAdminRepository(httpClient, "http://127.0.0.1:4317");

  await repository.authenticateProvider("codex");

  assert.equal(httpClient.requests[0]?.url, "http://127.0.0.1:4317/providers/auth");
  assert.equal(httpClient.requests[0]?.init?.method, "POST");
});
