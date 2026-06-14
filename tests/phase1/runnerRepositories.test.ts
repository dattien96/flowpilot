import test from "node:test";
import assert from "node:assert/strict";

import { RunnerRuntimeConfigRepository } from "../../packages/flowpilot-client-core/src/data/runnerRuntimeConfigRepository";
import { HttpRunnerRepository } from "../../packages/flowpilot-client-core/src/data/runnerRepository";
import type { HttpClient } from "../../packages/flowpilot-client-core/src/data/http";

class FakeHttpClient implements HttpClient {
  constructor(
    private readonly handler: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>,
  ) {}

  request(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    return this.handler(input, init);
  }
}

test("RunnerRuntimeConfigRepository returns invalid-config error from the runner", async () => {
  const repository = new RunnerRuntimeConfigRepository(
    new FakeHttpClient(async () => new Response("Saved Supabase config is incomplete.", { status: 500 })),
    "http://127.0.0.1:4317",
  );

  const status = await repository.loadSupabaseRuntimeStatus();

  assert.equal(status.configured, false);
  assert.equal(status.runnerReachable, true);
  assert.equal(status.lastError, "Saved Supabase config is incomplete.");
});

test("RunnerRuntimeConfigRepository maps saved config payload", async () => {
  const repository = new RunnerRuntimeConfigRepository(
    new FakeHttpClient(async () =>
      new Response(
        JSON.stringify({
          apiUrl: "https://proj.supabase.co",
          anonKey: "anon",
          edgeFunctionUrl: "https://proj.supabase.co/functions/v1",
          hasServiceRoleKey: true,
        }),
        { status: 200 },
      )),
    "http://127.0.0.1:4317",
  );

  const status = await repository.loadSupabaseRuntimeStatus();

  assert.equal(status.configured, true);
  assert.equal(status.projectRef, "proj");
  assert.equal(status.edgeFunctionsReady, true);
});

test("HttpRunnerRepository maps health payload", async () => {
  const repository = new HttpRunnerRepository(
    new FakeHttpClient(async () =>
      new Response(
        JSON.stringify({
          cwd: "/workspace/app",
          os: "darwin",
          startedAt: "2026-06-14T10:00:00.000Z",
          version: "1.2.3",
        }),
        { status: 200 },
      )),
    "http://127.0.0.1:4317",
  );

  const health = await repository.loadHealth();

  assert.deepEqual(health, {
    cwd: "/workspace/app",
    os: "darwin",
    startedAt: "2026-06-14T10:00:00.000Z",
    version: "1.2.3",
  });
});
