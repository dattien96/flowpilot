import type { DesktopBootstrapState, SupabaseRuntimeStatus } from "@flowpilot/client-core";

export type BootstrapIssue =
  | "runner-offline"
  | "supabase-missing"
  | "supabase-invalid"
  | null;

export type AppBootstrapView =
  | "authenticated-chat"
  | "login"
  | "settings";

export interface ResolvedBootstrapState {
  issue: BootstrapIssue;
  view: AppBootstrapView;
  preferredSettingsSection: "supabase" | "runner";
}

export function resolveBootstrapIssue(
  runtimeStatus: SupabaseRuntimeStatus,
): BootstrapIssue {
  if (!runtimeStatus.runnerReachable) {
    return "runner-offline";
  }
  if (!runtimeStatus.configured) {
    return runtimeStatus.lastError ? "supabase-invalid" : "supabase-missing";
  }
  return null;
}

export function resolveDesktopBootstrapState(
  bootstrap: DesktopBootstrapState,
): ResolvedBootstrapState {
  const issue = resolveBootstrapIssue(bootstrap.runtimeStatus);
  if (issue === "runner-offline") {
    return {
      issue,
      view: "settings",
      preferredSettingsSection: "runner",
    };
  }
  if (issue === "supabase-missing" || issue === "supabase-invalid") {
    return {
      issue,
      view: "settings",
      preferredSettingsSection: "supabase",
    };
  }
  if (bootstrap.session) {
    return {
      issue: null,
      view: "authenticated-chat",
      preferredSettingsSection: "supabase",
    };
  }
  return {
    issue: null,
    view: "login",
    preferredSettingsSection: "supabase",
  };
}
