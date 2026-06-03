import { getBrowserSupabaseClient } from "@/data/datasource/supabase/client";
import { HttpLocalRunnerGateway } from "@/data/repository/local-runner/http-local-runner-gateway";
import { createDemoGatewayBundle } from "@/data/repository/demo/demo-gateway-bundle";
import { createSupabaseGatewayBundle } from "@/data/repository/supabase/supabase-gateway-bundle";
import { InMemoryAiOrchestrationGateway } from "@/data/repository/demo/in-memory-ai-orchestration-gateway";
import { SupabaseAiOrchestrationGateway } from "@/data/repository/supabase/supabase-ai-orchestration-gateway";
import { SupabaseWorkflowEngineGateway } from "@/data/repository/supabase/supabase-workflow-engine-gateway";
import { InMemoryWorkflowEngineGateway } from "@/data/repository/demo/in-memory-workflow-engine-gateway";
import { LocalFirstWorkflowGateway } from "@/data/repository/local-first/local-first-workflow-gateway";
import { getLocalRunnerBaseUrl } from "@/lib/env/browser-env";
import { SupabaseUserFavoriteGateway } from "@/data/repository/supabase/supabase-user-favorite-gateway";
import { InMemoryUserFavoriteGateway } from "@/data/repository/demo/in-memory-user-favorite-gateway";
import { loadSupabaseRuntimeStatus } from "@/lib/supabase/runtime-config";

type BrowserGatewayBundle = Awaited<ReturnType<typeof createResolvedGatewayBundle>>;

let resolvedBundlePromise: Promise<BrowserGatewayBundle> | null = null;

export function resetBrowserGatewayBundle() {
  resolvedBundlePromise = null;
}

export function createGatewayBundle(): BrowserGatewayBundle {
  return {
    projectGateway: lazyGateway("projectGateway"),
    integrationGateway: lazyGateway("integrationGateway"),
    contextSourceGateway: lazyGateway("contextSourceGateway"),
    workflowGateway: lazyGateway("workflowGateway"),
    teamGateway: lazyGateway("teamGateway"),
    workflowExecutor: lazyGateway("workflowExecutor"),
    workflowEngineGateway: lazyGateway("workflowEngineGateway"),
    aiOrchestrationGateway: lazyGateway("aiOrchestrationGateway"),
    userFavoriteGateway: lazyGateway("userFavoriteGateway"),
    localRunnerGateway: new HttpLocalRunnerGateway(getLocalRunnerBaseUrl()),
  } as BrowserGatewayBundle;
}

async function resolveGatewayBundle() {
  resolvedBundlePromise ??= createResolvedGatewayBundle();
  return resolvedBundlePromise;
}

function lazyGateway<TKey extends keyof BrowserGatewayBundle>(key: TKey) {
  return new Proxy(
    {},
    {
      get(_target, property) {
        if (typeof property !== "string") {
          return undefined;
        }
        return async (...args: unknown[]) => {
          const bundle = await resolveGatewayBundle();
          const gateway = bundle[key] as Record<string, unknown>;
          const method = gateway[property];
          if (typeof method !== "function") {
            throw new Error(`Gateway method ${String(key)}.${property} is not available.`);
          }
          return method.apply(gateway, args);
        };
      },
    },
  ) as BrowserGatewayBundle[TKey];
}

async function createResolvedGatewayBundle() {
  const localRunnerGateway = new HttpLocalRunnerGateway(
    getLocalRunnerBaseUrl(),
  );

  const runtimeStatus = await loadSupabaseRuntimeStatus();
  if (runtimeStatus.configured) {
    const supabaseClient = await getBrowserSupabaseClient();
    const bundle = createSupabaseGatewayBundle(supabaseClient);
    return {
      ...bundle,
      workflowGateway: new LocalFirstWorkflowGateway(
        bundle.workflowGateway,
        localRunnerGateway,
      ),
      workflowEngineGateway: new SupabaseWorkflowEngineGateway(supabaseClient),
      aiOrchestrationGateway: new SupabaseAiOrchestrationGateway(
        supabaseClient,
      ),
      userFavoriteGateway: new SupabaseUserFavoriteGateway(supabaseClient),
      localRunnerGateway,
    };
  }

  return {
    ...createDemoGatewayBundle(),
    workflowEngineGateway: new InMemoryWorkflowEngineGateway(),
    aiOrchestrationGateway: new InMemoryAiOrchestrationGateway(),
    userFavoriteGateway: new InMemoryUserFavoriteGateway(),
    localRunnerGateway,
  };
}
