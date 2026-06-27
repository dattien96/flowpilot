import { afterEach, describe, expect, it } from "vitest";

import {
  getLocalRunnerBaseUrl,
  getSupabaseAnonKey,
  getSupabaseEdgeFunctionUrl,
  getSupabaseUrl,
  hasSupabaseEnv,
} from "./app-env";

const originalEnv: NodeJS.ProcessEnv = process.env;

function resetEnv(values: Partial<NodeJS.ProcessEnv> = {}) {
  process.env = {
    ...originalEnv,
    ...values,
  } as NodeJS.ProcessEnv;

  for (const key of [
    "SUPABASE_API_URL",
    "SUPABASE_API_KEY",
    "SUPABASE_API_EDGE_FUNCTION_URL",
    "FLOWPILOT_RUNNER_URL",
    "FLOWPILOT_RUNNER_PORT",
  ]) {
    if (!(key in values)) {
      delete process.env[key];
    }
  }
}

afterEach(() => {
  process.env = originalEnv;
});

describe("getLocalRunnerBaseUrl", () => {
  it("derives the runner URL from FLOWPILOT_RUNNER_PORT", () => {
    resetEnv({
      FLOWPILOT_RUNNER_PORT: "4318",
    });

    expect(getLocalRunnerBaseUrl()).toBe("http://127.0.0.1:4318");
  });
});

describe("hasSupabaseEnv", () => {
  it("returns true only when all root Supabase variables are present", () => {
    resetEnv({
      SUPABASE_API_URL: "https://example.supabase.co",
      SUPABASE_API_KEY: "anon",
    });

    expect(hasSupabaseEnv()).toBe(true);
  });

  it("returns false when any required Supabase variable is missing", () => {
    resetEnv({
      SUPABASE_API_KEY: "anon",
    });

    expect(hasSupabaseEnv()).toBe(false);
  });
});

describe("supabase env getters", () => {
  it("reads root env values without NEXT_PUBLIC aliases", () => {
    resetEnv({
      SUPABASE_API_URL: "https://example.supabase.co",
      SUPABASE_API_KEY: "anon",
      SUPABASE_API_EDGE_FUNCTION_URL: "https://example.supabase.co/functions/v1",
    });

    expect(getSupabaseUrl()).toBe("https://example.supabase.co");
    expect(getSupabaseAnonKey()).toBe("anon");
    expect(getSupabaseEdgeFunctionUrl()).toBe("https://example.supabase.co/functions/v1");
  });
});
