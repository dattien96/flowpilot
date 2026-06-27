type EnvSource = Record<string, string | undefined>;
type EnvGlobal = typeof globalThis & {
  __FLOWPILOT_VITE_ENV__?: EnvSource;
};

function currentEnv(): EnvSource {
  const candidate = globalThis as EnvGlobal;
  if (candidate.__FLOWPILOT_VITE_ENV__) {
    return candidate.__FLOWPILOT_VITE_ENV__;
  }
  return (typeof process !== "undefined" ? process.env : {}) as EnvSource;
}

function trimTrailingSlash(value: string): string {
  return value.replace(/\/+$/, "");
}

export const ADMIN_WEB_URL = trimTrailingSlash(
  currentEnv().VITE_ADMIN_WEB_URL ?? "http://localhost:3002",
);

export const RUNNER_URL = trimTrailingSlash(
  currentEnv().VITE_RUNNER_URL ??
    currentEnv().VITE_LOCAL_RUNNER_URL ??
    "http://127.0.0.1:4317",
);

export const LIBRETRANSLATE_URL = trimTrailingSlash(
  currentEnv().FLOWPILOT_LIBRETRANSLATE_URL ??
    (currentEnv().FLOWPILOT_LIBRETRANSLATE_PORT
      ? `http://127.0.0.1:${currentEnv().FLOWPILOT_LIBRETRANSLATE_PORT}`
      : "http://127.0.0.1:5001"),
);
