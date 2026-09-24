// Phase1 test runtime hook (CP-59 Task-316 close-out): the compiled
// .phase1-tests store tests keep the `@/*` path aliases from the TypeScript
// sources; Node 18 cannot resolve them natively. Loaded via
// `node --require scripts/phase1-runtime.js --test …`, this maps:
//   @/<rel>                  → .phase1-tests/apps/desktop-flowpilot/src/<rel>.js
//   @flowpilot/client-core…  → packages/flowpilot-client-core/src/…
//   bare imports (zustand…)  → apps/desktop-flowpilot/node_modules fallback —
//     the compiled output lives outside the app tree so Node's walk-up never
//     reaches it. Original resolution is tried first; this is only a fallback.
// Additive infrastructure only — no test or production behavior changes.
"use strict";

const path = require("path");
const Module = require("module");

const ROOT = path.resolve(__dirname, "..");
const OUT = path.join(ROOT, ".phase1-tests");

const DESKTOP_SRC = path.join(OUT, "apps", "desktop-flowpilot", "src");
const DESKTOP_NM = path.join(ROOT, "apps", "desktop-flowpilot", "node_modules");
const CLIENT_CORE = path.join(ROOT, "packages", "flowpilot-client-core", "src");

const originalResolve = Module._resolveFilename;
Module._resolveFilename = function (request, ...rest) {
  // KR-005: compiled tests live under .phase1-tests — Node's walk-up can hit
  // the ROOT node_modules first and load a second React copy (hooks then die
  // with "Cannot read properties of null (reading 'useCallback')"). Pin the
  // React family + zustand to the app's own node_modules so renderer and
  // components always share one instance.
  if (
    request === "react" ||
    request === "react-dom" ||
    request.startsWith("react/") ||
    request.startsWith("react-dom/") ||
    request === "zustand" ||
    request.startsWith("zustand/") ||
    request === "scheduler"
  ) {
    try {
      const [parent, isMain, options] = rest;
      return originalResolve.call(this, request, parent, isMain, {
        ...(options || {}),
        paths: [DESKTOP_NM],
      });
    } catch {
      // fall through to normal resolution
    }
  }
  const candidates = [];
  if (request.startsWith("@/")) {
    const rel = request.slice(2);
    candidates.push(path.join(DESKTOP_SRC, rel + ".js"));
    candidates.push(path.join(DESKTOP_SRC, rel, "index.js"));
  } else if (request.startsWith("@flowpilot/client-core/")) {
    const rel = request.slice("@flowpilot/client-core/".length);
    candidates.push(path.join(OUT, "packages", "flowpilot-client-core", "src", rel + ".js"));
    candidates.push(path.join(OUT, "packages", "flowpilot-client-core", "src", rel, "index.js"));
  } else if (request === "@flowpilot/client-core") {
    candidates.push(path.join(OUT, "packages", "flowpilot-client-core", "src", "index.js"));
  }
  for (const candidate of candidates) {
    try {
      return originalResolve.call(this, candidate, ...rest);
    } catch {
      // try the next candidate
    }
  }
  try {
    return originalResolve.call(this, request, ...rest);
  } catch (err) {
    if (request.startsWith(".") || path.isAbsolute(request)) throw err;
    return require.resolve(request, { paths: [DESKTOP_NM, path.join(ROOT, "node_modules")] });
  }
};
