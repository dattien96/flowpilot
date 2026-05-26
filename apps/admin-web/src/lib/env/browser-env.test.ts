import { afterEach, describe, expect, it } from "vitest";

import { getLocalRunnerBaseUrl } from "./browser-env";

const originalEnv = import.meta.env;

function setImportMetaEnv(values: Record<string, string | undefined>) {
  Object.defineProperty(import.meta, "env", {
    value: values,
    configurable: true,
  });
}

afterEach(() => {
  Object.defineProperty(import.meta, "env", {
    value: originalEnv,
    configurable: true,
  });
});

describe("getLocalRunnerBaseUrl", () => {
  it("defaults to 127.0.0.1 instead of localhost", () => {
    const nextEnv = { ...originalEnv } as Record<string, string | undefined>;
    delete nextEnv.VITE_LOCAL_RUNNER_URL;
    setImportMetaEnv(nextEnv);

    expect(getLocalRunnerBaseUrl()).toBe("http://127.0.0.1:4317");
  });
});
