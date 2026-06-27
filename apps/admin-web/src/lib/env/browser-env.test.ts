import { afterEach, describe, expect, it } from "vitest";

import { getLocalRunnerBaseUrl } from "./browser-env";

const originalEnv = import.meta.env;
const envGlobal = globalThis as typeof globalThis & {
  __FLOWPILOT_VITE_ENV__?: Record<string, string | undefined>;
};

function setImportMetaEnv(values: Record<string, string | undefined>) {
  envGlobal.__FLOWPILOT_VITE_ENV__ = values;
  Object.defineProperty(import.meta, "env", {
    value: values,
    configurable: true,
  });
}

afterEach(() => {
  delete envGlobal.__FLOWPILOT_VITE_ENV__;
  Object.defineProperty(import.meta, "env", {
    value: originalEnv,
    configurable: true,
  });
});

describe("getLocalRunnerBaseUrl", () => {
	it("defaults to 127.0.0.1 instead of localhost", () => {
		setImportMetaEnv({});

		expect(getLocalRunnerBaseUrl()).toBe("http://127.0.0.1:4317");
	});

	it("derives the runner URL from FLOWPILOT_RUNNER_PORT", () => {
		setImportMetaEnv({ FLOWPILOT_RUNNER_PORT: "4318" });

		expect(getLocalRunnerBaseUrl()).toBe("http://127.0.0.1:4318");
	});
});
