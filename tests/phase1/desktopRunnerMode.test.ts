import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { createRunnerClient, resolveRunnerUrlFromSources, runnerModeLabel } from "../../apps/desktop-flowpilot/src/client/createRunnerClient";
import { MockRunnerClient } from "../../apps/desktop-flowpilot/src/client/MockRunnerClient";
import { HttpWsRunnerClient } from "../../apps/desktop-flowpilot/src/client/HttpWsRunnerClient";
import { RUNNER_URL } from "../../apps/desktop-flowpilot/src/config";

function withRunnerEnv(
  env: Record<string, string | undefined>,
  run: () => void,
): void {
  const candidate = globalThis as typeof globalThis & {
    __FLOWPILOT_VITE_ENV__?: Record<string, string | undefined>;
  };
  const previousViteEnv = candidate.__FLOWPILOT_VITE_ENV__;
  const previousRunnerUrl = process.env.VITE_RUNNER_URL;
  const previousLocalRunnerUrl = process.env.VITE_LOCAL_RUNNER_URL;
  const previousUseRunner = process.env.VITE_USE_RUNNER;

  candidate.__FLOWPILOT_VITE_ENV__ = env;

  if (env.VITE_RUNNER_URL === undefined) delete process.env.VITE_RUNNER_URL;
  else process.env.VITE_RUNNER_URL = env.VITE_RUNNER_URL;

  if (env.VITE_LOCAL_RUNNER_URL === undefined) delete process.env.VITE_LOCAL_RUNNER_URL;
  else process.env.VITE_LOCAL_RUNNER_URL = env.VITE_LOCAL_RUNNER_URL;

  if (env.VITE_USE_RUNNER === undefined) delete process.env.VITE_USE_RUNNER;
  else process.env.VITE_USE_RUNNER = env.VITE_USE_RUNNER;

  try {
    run();
  } finally {
    if (previousViteEnv === undefined) delete candidate.__FLOWPILOT_VITE_ENV__;
    else candidate.__FLOWPILOT_VITE_ENV__ = previousViteEnv;

    if (previousRunnerUrl === undefined) delete process.env.VITE_RUNNER_URL;
    else process.env.VITE_RUNNER_URL = previousRunnerUrl;

    if (previousLocalRunnerUrl === undefined) delete process.env.VITE_LOCAL_RUNNER_URL;
    else process.env.VITE_LOCAL_RUNNER_URL = previousLocalRunnerUrl;

    if (previousUseRunner === undefined) delete process.env.VITE_USE_RUNNER;
    else process.env.VITE_USE_RUNNER = previousUseRunner;
  }
}

test("resolveRunnerUrlFromSources prefers renderer Vite env over process env", () => {
  const url = resolveRunnerUrlFromSources(
    { VITE_RUNNER_URL: "http://renderer-runner:4000" },
    { VITE_RUNNER_URL: "http://process-runner:5000", VITE_USE_RUNNER: "true" },
  );

  assert.equal(url, "http://renderer-runner:4000");
});

test("resolveRunnerUrlFromSources accepts VITE_LOCAL_RUNNER_URL", () => {
  const url = resolveRunnerUrlFromSources(
    { VITE_LOCAL_RUNNER_URL: "http://local-runner:4318" },
    {},
  );

  assert.equal(url, "http://local-runner:4318");
});

test("resolveRunnerUrlFromSources falls back to process env when renderer env is missing", () => {
  const url = resolveRunnerUrlFromSources({}, { VITE_USE_RUNNER: "true" });

  assert.equal(url, RUNNER_URL);
});

test("createRunnerClient uses HttpWsRunnerClient when the renderer Vite env enables the runner", () => {
  withRunnerEnv({ VITE_USE_RUNNER: "true", VITE_RUNNER_URL: undefined }, () => {
    const client = createRunnerClient();

    assert.ok(client instanceof HttpWsRunnerClient);
    assert.equal(runnerModeLabel(), `runner ${RUNNER_URL}`);
  });
});

test("createRunnerClient uses MockRunnerClient when neither env source enables the runner", () => {
  withRunnerEnv({ VITE_USE_RUNNER: undefined, VITE_RUNNER_URL: undefined }, () => {
    const client = createRunnerClient();

    assert.ok(client instanceof MockRunnerClient);
    assert.equal(runnerModeLabel(), "mock");
  });
});

test("desktop main publishes Vite env before importing App and store modules", () => {
  const mainSource = readFileSync(
    resolve(__dirname, "../../../apps/desktop-flowpilot/src/main.tsx"),
    "utf8",
  );

  assert.equal(mainSource.includes('import { App } from "@/App"'), false);
  assert.ok(mainSource.indexOf("__FLOWPILOT_VITE_ENV__") < mainSource.indexOf('import("@/App")'));
});
