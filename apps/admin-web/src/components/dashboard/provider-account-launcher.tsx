import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Terminal, PlugZap } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Badge } from "@/presentation/components/ui/badge";

type ProviderAccount = {
  id: string;
  provider_key: string;
  display_name: string;
  display_label: string;
  home_path: string;
  auth_store_path: string | null;
  slot_index: number;
  auth_status: "pending" | "connecting" | "connected" | "failed";
  is_active: boolean;
  account_email: string | null;
  account_name: string | null;
  usage_summary: string | null;
  remaining_5h_percent: number | null;
  remaining_7d_percent: number | null;
  remaining_5h_reset_at: string | null;
  remaining_7d_reset_at: string | null;
  usage_source: "provider_api" | "unavailable";
  access_token_expires_at: string | null;
  refresh_token_expires_at: string | null;
  refresh_token_expiry_note: string | null;
  usage_detail_lines: Array<{
    label: string;
    remaining_percent: number;
    reset_at: string | null;
  }>;
};

const PROVIDERS = [
  { key: "codex", label: "Codex" },
  { key: "claude", label: "Claude" },
  { key: "gemini", label: "Gemini" },
] as const;

function formatResetAt(value: string | null) {
  if (!value) {
    return null;
  }

  try {
    return new Intl.DateTimeFormat(undefined, {
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    }).format(new Date(value));
  } catch {
    return null;
  }
}

export function ProviderAccountLauncher() {
  const [selectedProvider, setSelectedProvider] = useState<string>("codex");
  const [selectedAccountId, setSelectedAccountId] = useState<string>("");
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const { data, isLoading, error } = useQuery<{ accounts: ProviderAccount[] }>({
    queryKey: ["providerAccounts", "dashboard-launcher"],
    queryFn: async () => {
      const res = await fetch("/api/local-runner/provider-accounts");
      if (!res.ok) throw new Error("Failed to fetch provider accounts");
      return res.json();
    },
  });

  const connectedAccounts = useMemo(
    () =>
      (data?.accounts ?? []).filter(
        (account) => account.auth_status === "connected",
      ),
    [data?.accounts],
  );

  const providerOptions = useMemo(
    () =>
      PROVIDERS.filter((provider) =>
        connectedAccounts.some(
          (account) => account.provider_key === provider.key,
        ),
      ),
    [connectedAccounts],
  );

  const providerAccounts = useMemo(
    () =>
      connectedAccounts.filter(
        (account) => account.provider_key === selectedProvider,
      ),
    [connectedAccounts, selectedProvider],
  );

  useEffect(() => {
    if (providerOptions.length === 0) {
      if (selectedProvider !== "codex") {
        setSelectedProvider("codex");
      }
      setSelectedAccountId("");
      return;
    }

    const providerStillExists = providerOptions.some(
      (provider) => provider.key === selectedProvider,
    );
    if (!providerStillExists) {
      setSelectedProvider(providerOptions[0]?.key ?? "codex");
      return;
    }

    const selectedAccountStillExists = providerAccounts.some(
      (account) => account.id === selectedAccountId,
    );
    if (!selectedAccountStillExists) {
      const activeAccount =
        providerAccounts.find((account) => account.is_active) ??
        providerAccounts[0] ??
        null;
      setSelectedAccountId(activeAccount?.id ?? "");
    }
  }, [providerAccounts, providerOptions, selectedAccountId, selectedProvider]);

  const selectedAccount =
    providerAccounts.find((account) => account.id === selectedAccountId) ??
    null;

  const launchTerminal = useMutation({
    mutationFn: async (accountId: string) => {
      const res = await fetch("/api/local-runner/provider-accounts/test", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ accountId }),
      });
      if (!res.ok) {
        const error = await res.json().catch(() => null);
        throw new Error(error?.error || "Failed to open terminal");
      }
      return res.json();
    },
    onMutate: () => {
      setErrorMessage(null);
      setStatusMessage(null);
    },
    onSuccess: () => {
      setStatusMessage(
        "New terminal opened with the selected account and current FlowPilot workspace.",
      );
    },
    onError: (err) => {
      setErrorMessage(
        err instanceof Error ? err.message : "Failed to open account terminal.",
      );
    },
  });

  return (
    <div className="rounded-[1.5rem] border border-border bg-background/60 p-6 md:col-span-3">
      <div className="flex items-center gap-2 mb-4">
        <PlugZap className="w-5 h-5 text-primary" />
        <h2 className="text-xl font-semibold">Provider Terminal Launcher</h2>
      </div>

      <p className="text-sm text-muted-foreground mb-5">
        Pick any connected account and open a new terminal already bound to that
        provider account and the current FlowPilot workspace.
      </p>

      {isLoading ? (
        <div className="rounded-xl border border-border/70 bg-background/50 p-4 text-sm text-muted-foreground">
          Loading connected accounts...
        </div>
      ) : null}

      {error ? (
        <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
          {error instanceof Error
            ? error.message
            : "Failed to load connected accounts."}
        </div>
      ) : null}

      {statusMessage ? (
        <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-200 mb-4">
          {statusMessage}
        </div>
      ) : null}

      {errorMessage ? (
        <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive mb-4">
          {errorMessage}
        </div>
      ) : null}

      {!isLoading && connectedAccounts.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border/60 bg-background/40 p-6 text-sm text-muted-foreground">
          No connected provider accounts found. Connect accounts in Settings
          first.
        </div>
      ) : null}

      {connectedAccounts.length > 0 ? (
        <div className="grid gap-4 lg:grid-cols-[220px_minmax(0,1fr)_auto]">
          <label className="flex flex-col gap-2">
            <span className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">
              Provider
            </span>
            <select
              className="rounded-xl border border-border/70 bg-background px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-primary/40"
              value={selectedProvider}
              onChange={(event) => {
                setSelectedProvider(event.target.value);
                setSelectedAccountId("");
                setStatusMessage(null);
                setErrorMessage(null);
              }}
            >
              {providerOptions.map((provider) => (
                <option key={provider.key} value={provider.key}>
                  {provider.label}
                </option>
              ))}
            </select>
          </label>

          <label className="flex flex-col gap-2">
            <span className="text-xs font-semibold uppercase tracking-widest text-muted-foreground">
              Account
            </span>
            <select
              className="rounded-xl border border-border/70 bg-background px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-primary/40"
              value={selectedAccountId}
              onChange={(event) => {
                setSelectedAccountId(event.target.value);
                setStatusMessage(null);
                setErrorMessage(null);
              }}
            >
              {providerAccounts.map((account) => (
                <option key={account.id} value={account.id}>
                  {account.display_label}
                  {account.is_active ? " (active)" : ""}
                </option>
              ))}
            </select>
          </label>

          <div className="flex items-end">
            <Button
              className="rounded-xl min-w-[180px]"
              onClick={() => {
                if (selectedAccountId) {
                  launchTerminal.mutate(selectedAccountId);
                }
              }}
              disabled={!selectedAccountId || launchTerminal.isPending}
            >
              <Terminal className="w-4 h-4 mr-2" />
              {launchTerminal.isPending ? "Opening..." : "Open Terminal"}
            </Button>
          </div>
        </div>
      ) : null}

      {selectedAccount ? (
        <div className="mt-5 rounded-[1.25rem] border border-border/70 bg-background/40 p-4">
          <div className="flex items-center gap-3 mb-2">
            <p className="font-mono text-sm font-semibold">
              {selectedAccount.display_label}
            </p>
            <Badge tone="success">connected</Badge>
            {selectedAccount.is_active ? (
              <Badge tone="neutral">active</Badge>
            ) : null}
          </div>
          <p className="text-xs text-muted-foreground">
            Auth store:{" "}
            {selectedAccount.auth_store_path ?? selectedAccount.home_path}
          </p>
          <p className="text-xs text-muted-foreground mt-1">
            Exec home: {selectedAccount.home_path}
          </p>
          {selectedAccount.usage_summary ? (
            <p className="text-xs text-muted-foreground mt-1">
              Plan: {selectedAccount.usage_summary}
            </p>
          ) : null}
          {selectedAccount.access_token_expires_at ? (
            <p className="text-xs text-muted-foreground mt-1">
              Access token expires:{" "}
              {formatResetAt(selectedAccount.access_token_expires_at)}
            </p>
          ) : null}
          {selectedAccount.refresh_token_expires_at ? (
            <p className="text-xs text-muted-foreground mt-1">
              Refresh token expires:{" "}
              {formatResetAt(selectedAccount.refresh_token_expires_at)}
            </p>
          ) : null}
          {selectedAccount.refresh_token_expiry_note ? (
            <p className="text-xs text-muted-foreground mt-1">
              Refresh token expiry: {selectedAccount.refresh_token_expiry_note}
            </p>
          ) : null}
          {selectedAccount.usage_detail_lines.map((line) => (
            <div
              key={`${selectedAccount.id}-${line.label}`}
              className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mt-2 bg-muted/20 p-2.5 rounded-lg border border-border/40 hover:bg-muted/30 transition-colors"
            >
              <span className="text-xs text-muted-foreground font-medium">
                {line.label}: {line.remaining_percent}%
                {formatResetAt(line.reset_at)
                  ? ` · resets ${formatResetAt(line.reset_at)}`
                  : ""}
              </span>
              <div className="w-full sm:w-32 bg-muted-foreground/10 border border-border/10 rounded-full h-2 overflow-hidden shadow-inner flex-shrink-0 relative">
                <div
                  className={`h-full rounded-full transition-all duration-500 ease-out ${
                    line.remaining_percent > 50
                      ? "bg-gradient-to-r from-emerald-500 to-teal-400"
                      : line.remaining_percent > 20
                        ? "bg-gradient-to-r from-amber-500 to-yellow-400"
                        : "bg-gradient-to-r from-rose-500 to-red-400"
                  }`}
                  style={{ width: `${line.remaining_percent}%` }}
                />
              </div>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}
