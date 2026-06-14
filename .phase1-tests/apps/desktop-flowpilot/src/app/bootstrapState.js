"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.resolveBootstrapIssue = resolveBootstrapIssue;
exports.resolveDesktopBootstrapState = resolveDesktopBootstrapState;
function resolveBootstrapIssue(runtimeStatus) {
    if (!runtimeStatus.runnerReachable) {
        return "runner-offline";
    }
    if (!runtimeStatus.configured) {
        return runtimeStatus.lastError ? "supabase-invalid" : "supabase-missing";
    }
    return null;
}
function resolveDesktopBootstrapState(bootstrap) {
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
