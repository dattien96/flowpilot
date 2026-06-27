import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";
import type { IncomingMessage, ServerResponse } from "node:http";
import type { Plugin, ViteDevServer } from "vite";

async function readRequestBody(req: IncomingMessage) {
  const chunks: Buffer[] = [];
  for await (const chunk of req) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  }

  return Buffer.concat(chunks);
}

function toFetchHeaders(req: IncomingMessage) {
  const headers = new Headers();
  for (const [key, value] of Object.entries(req.headers)) {
    if (Array.isArray(value)) {
      for (const item of value) {
        headers.append(key, item);
      }
      continue;
    }
    if (value) {
      headers.set(key, value);
    }
  }

  return headers;
}

async function toFetchRequest(req: IncomingMessage) {
  const method = req.method ?? "GET";
  const host = req.headers.host ?? "localhost";
  const url = `http://${host}${req.url ?? "/"}`;
  const headers = toFetchHeaders(req);
  const init: RequestInit = {
    method,
    headers,
  };

  if (method !== "GET" && method !== "HEAD") {
    const body = await readRequestBody(req);
    init.body = body.length > 0 ? body : undefined;
  }

  return new Request(url, init);
}

async function writeFetchResponse(res: ServerResponse, response: Response) {
  res.statusCode = response.status;
  response.headers.forEach((value, key) => {
    res.setHeader(key, value);
  });

  const body = await response.arrayBuffer();
  res.end(Buffer.from(body));
}

async function ensureNodeWebSocket() {
  if (typeof globalThis.WebSocket !== "undefined") {
    return;
  }

  const wsModule = await import("ws");
  globalThis.WebSocket = wsModule.WebSocket as unknown as typeof WebSocket;
}

import fs from "node:fs";

interface RouteInfo {
  filePath: string;
  pattern: RegExp;
  paramNames: string[];
  segmentCount: number;
  staticSegmentCount: number;
}

function scanRoutes(dir: string, baseDir: string = dir): RouteInfo[] {
  const routes: RouteInfo[] = [];
  if (!fs.existsSync(dir)) return routes;

  const entries = fs.readdirSync(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      routes.push(...scanRoutes(fullPath, baseDir));
    } else if (entry.name === "route.ts" || entry.name === "route.js") {
      const relPath = path.relative(baseDir, dir).replace(/\\/g, "/");
      const segments = relPath ? relPath.split("/") : [];
      const paramNames: string[] = [];
      const regexParts = segments.map((seg) => {
        if (seg.startsWith("[") && seg.endsWith("]")) {
          const paramName = seg.slice(1, -1);
          paramNames.push(paramName);
          return "([^/]+)";
        }
        return seg.replace(/[-\/\\^$*+?.()|[\]{}]/g, "\\$&");
      });
      const pattern = new RegExp(`^/api/${regexParts.join("/")}$`);
      const projectRelPath =
        "/src/app/api/" + (relPath ? relPath + "/" : "") + entry.name;
      routes.push({
        filePath: projectRelPath,
        pattern,
        paramNames,
        segmentCount: segments.length,
        staticSegmentCount: segments.length - paramNames.length,
      });
    }
  }
  return routes;
}

function flowPilotApiRuntime(): Plugin {
  return {
    name: "flowpilot-api-runtime",
    configureServer(server: ViteDevServer) {
      server.middlewares.use(async (req, res, next) => {
        const pathname = new URL(req.url || "", "http://localhost").pathname;
        if (!pathname?.startsWith("/api/")) {
          next();
          return;
        }

        const apiDir = path.resolve(__dirname, "./src/app/api");
        const routes = scanRoutes(apiDir).sort((left, right) => {
          if (left.staticSegmentCount !== right.staticSegmentCount) {
            return right.staticSegmentCount - left.staticSegmentCount;
          }
          if (left.segmentCount !== right.segmentCount) {
            return right.segmentCount - left.segmentCount;
          }
          return left.paramNames.length - right.paramNames.length;
        });

        let matchedRoute: RouteInfo | null = null;
        const params: Record<string, string> = {};

        for (const route of routes) {
          const match = pathname.match(route.pattern);
          if (match) {
            matchedRoute = route;
            route.paramNames.forEach((name, index) => {
              params[name] = match[index + 1];
            });
            break;
          }
        }

        if (!matchedRoute) {
          next();
          return;
        }

        const method = req.method ?? "GET";
        try {
          await ensureNodeWebSocket();
          const route = await server.ssrLoadModule(matchedRoute.filePath);
          const handler = route[method];

          if (typeof handler !== "function") {
            res.statusCode = 405;
            res.setHeader("content-type", "application/json");
            res.end(JSON.stringify({ error: `Method ${method} not allowed` }));
            return;
          }

          const request = await toFetchRequest(req);
          const context = {
            params: Promise.resolve(params),
          };

          const response = await handler(request, context);
          await writeFetchResponse(res, response);
        } catch (error) {
          if (
            error &&
            typeof error === "object" &&
            ("url" in error || error.constructor.name === "RedirectError")
          ) {
            const redirectUrl = (error as any).url;
            res.statusCode = 302;
            res.setHeader("location", redirectUrl);
            res.end();
            return;
          }

          res.statusCode = 500;
          res.setHeader("content-type", "application/json");
          res.end(
            JSON.stringify({
              error:
                error instanceof Error
                  ? error.message
                  : "API execution failed.",
            }),
          );
        }
      });
    },
  };
}

const envDir = path.resolve(__dirname, "../..");

export default defineConfig(({ mode }) => {
  Object.assign(process.env, loadEnv(mode, envDir, ""));
  const adminWebPort = Number(process.env.FLOWPILOT_ADMIN_WEB_PORT ?? "3002");

  return {
    envDir,
    envPrefix: [
      "VITE_",
      "FLOWPILOT_RUNNER_PORT",
      "FLOWPILOT_RUNNER_URL",
      "SUPABASE_API_URL",
      "SUPABASE_API_KEY",
      "SUPABASE_API_EDGE_FUNCTION_URL",
    ],
    plugins: [flowPilotApiRuntime(), react()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
        "next/server": path.resolve(__dirname, "./src/lib/shims/next-shim.ts"),
        "next/navigation": path.resolve(
          __dirname,
          "./src/lib/shims/next-shim.ts",
        ),
      },
    },
    server: {
      host: "0.0.0.0",
      port: adminWebPort,
      strictPort: true,
      watch: {
        usePolling: true,
        interval: 300,
      },
      hmr: {
        host: "localhost",
        clientPort: adminWebPort,
      },
    },
  };
});
