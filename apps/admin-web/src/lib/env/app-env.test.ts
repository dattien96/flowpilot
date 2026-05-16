import { afterEach, describe, expect, it } from "vitest";

import {
  getSupabaseAnonKey,
  getSupabaseServiceRoleKey,
  getSupabaseUrl,
  hasSupabaseEnv,
} from "./app-env";

const originalEnv = process.env;

function resetEnv(values: NodeJS.ProcessEnv = {}) {
  process.env = {
    ...originalEnv,
    ...values,
  };

  for (const key of [
    "SUPABASE_API_URL",
    "SUPABASE_API_KEY",
    "SUPABASE_SERVICE_ROLE_KEY",
  ]) {
    if (!(key in values)) {
      delete process.env[key];
    }
  }
}

afterEach(() => {
  process.env = originalEnv;
});

describe("hasSupabaseEnv", () => {
  it("returns true only when all root Supabase variables are present", () => {
    resetEnv({
      SUPABASE_API_URL: "https://example.supabase.co",
      SUPABASE_API_KEY: "anon",
      SUPABASE_SERVICE_ROLE_KEY: "service",
    });

    expect(hasSupabaseEnv()).toBe(true);
  });

  it("returns false when any required Supabase variable is missing", () => {
    resetEnv({
      SUPABASE_API_URL: "https://example.supabase.co",
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
      SUPABASE_SERVICE_ROLE_KEY: "service",
    });

    expect(getSupabaseUrl()).toBe("https://example.supabase.co");
    expect(getSupabaseAnonKey()).toBe("anon");
  });

  it("throws a clear error when SUPABASE_SERVICE_ROLE_KEY is missing", () => {
    resetEnv({
      SUPABASE_API_URL: "https://example.supabase.co",
      SUPABASE_API_KEY: "anon",
    });

    expect(() => getSupabaseServiceRoleKey()).toThrow(
      "Missing required environment variable: SUPABASE_SERVICE_ROLE_KEY",
    );
  });
});
