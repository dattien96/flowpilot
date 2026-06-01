import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  Trash2,
  Plus,
  RefreshCw,
  CheckCircle2,
  ChevronRight,
  ChevronDown,
  Check,
  Terminal,
} from "lucide-react";
import { useEffect, useState } from "react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { Badge } from "@/presentation/components/ui/badge";

export const Route = createFileRoute("/_authenticated/settings/accounts")({
  component: AccountsPage,
});

interface ProviderAccount {
  id: string;
  provider_key: string;
  display_name: string;
  display_label: string;
  home_path: string;
  auth_store_path: string | null;
  slot_index: number;
  auth_status: "pending" | "connecting" | "connected" | "failed";
  is_active: boolean;
  created_at: string;
  last_authenticated_at: string | null;
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
}

const SUPPORTED_PROVIDERS = [
  { key: "codex", label: "Codex" },
  { key: "claude", label: "Claude" },
  { key: "gemini", label: "Gemini" },
];

function AccountsPage() {
  const queryClient = useQueryClient();
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [pendingLoginProviderKey, setPendingLoginProviderKey] = useState<
    string | null
  >(null);

  const { data, isLoading, error } = useQuery<{ accounts: ProviderAccount[] }>({
    queryKey: ["providerAccounts"],
    queryFn: async () => {
      const res = await fetch("/api/local-runner/provider-accounts");
      if (!res.ok) throw new Error("Failed to fetch accounts");
      return res.json();
    },
    refetchInterval: pendingLoginProviderKey ? 2000 : false,
  });

  const accounts = data?.accounts || [];
  const pendingProviderAccounts = pendingLoginProviderKey
    ? accounts.filter((a) => a.provider_key === pendingLoginProviderKey)
    : [];

  useEffect(() => {
    if (!pendingLoginProviderKey || pendingProviderAccounts.length === 0) {
      return;
    }

    const stillConnecting = pendingProviderAccounts.some(
      (account) => account.auth_status === "connecting",
    );
    if (stillConnecting) {
      return;
    }

    const hasConnectedNewAccount = pendingProviderAccounts.some(
      (account) =>
        account.slot_index > 0 && account.auth_status === "connected",
    );
    if (hasConnectedNewAccount) {
      setStatusMessage("New account detected. The UI refreshed automatically.");
      setErrorMessage(null);
    }

    setPendingLoginProviderKey(null);
  }, [pendingLoginProviderKey, pendingProviderAccounts]);

  const connectAccount = useMutation({
    mutationFn: async (providerKey: string) => {
      const res = await fetch("/api/local-runner/provider-accounts/connect", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ providerKey }),
      });
      if (!res.ok) {
        const error = await res.json();
        throw new Error(error.error || "Failed to connect");
      }
      return res.json();
    },
    onMutate: () => {
      setErrorMessage(null);
      setStatusMessage(null);
    },
    onSuccess: (_, providerKey) => {
      setPendingLoginProviderKey(providerKey);
      setStatusMessage(
        "Login terminal requested. Complete authentication in the opened terminal/browser. FlowPilot will refresh this list automatically when the new account is detected.",
      );
      queryClient.invalidateQueries({ queryKey: ["providerAccounts"] });
    },
    onError: (err) => {
      setPendingLoginProviderKey(null);
      setErrorMessage(
        err instanceof Error ? err.message : "Failed to connect account.",
      );
    },
  });

  const verifyAccount = useMutation({
    mutationFn: async (accountId: string) => {
      const res = await fetch("/api/local-runner/provider-accounts/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ accountId }),
      });
      if (!res.ok) {
        const error = await res.json().catch(() => null);
        throw new Error(error?.error || "Failed to verify");
      }
      return res.json();
    },
    onMutate: () => {
      setErrorMessage(null);
      setStatusMessage(null);
    },
    onSuccess: () => {
      setStatusMessage("Account verification completed.");
      queryClient.invalidateQueries({ queryKey: ["providerAccounts"] });
    },
    onError: (err) => {
      setErrorMessage(
        err instanceof Error ? err.message : "Failed to verify account.",
      );
    },
  });

  const activateAccount = useMutation({
    mutationFn: async (accountId: string) => {
      const res = await fetch("/api/local-runner/provider-accounts/activate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ accountId }),
      });
      if (!res.ok) {
        const error = await res.json().catch(() => null);
        throw new Error(error?.error || "Failed to activate");
      }
      return res.json();
    },
    onMutate: () => {
      setErrorMessage(null);
      setStatusMessage(null);
    },
    onSuccess: () => {
      setStatusMessage("Active account updated.");
      queryClient.invalidateQueries({ queryKey: ["providerAccounts"] });
    },
    onError: (err) => {
      setErrorMessage(
        err instanceof Error ? err.message : "Failed to activate account.",
      );
    },
  });

  const testAccount = useMutation({
    mutationFn: async (accountId: string) => {
      const res = await fetch("/api/local-runner/provider-accounts/test", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ accountId }),
      });
      if (!res.ok) {
        const error = await res.json().catch(() => null);
        throw new Error(error?.error || "Failed to open test terminal");
      }
      return res.json();
    },
    onMutate: () => {
      setErrorMessage(null);
      setStatusMessage(null);
    },
    onSuccess: () => {
      setStatusMessage(
        "Test terminal opened with the selected account environment.",
      );
      queryClient.invalidateQueries({ queryKey: ["providerAccounts"] });
    },
    onError: (err) => {
      setErrorMessage(
        err instanceof Error ? err.message : "Failed to open test terminal.",
      );
    },
  });

  const deleteAccount = useMutation({
    mutationFn: async (accountId: string) => {
      const res = await fetch(
        `/api/local-runner/provider-accounts/${accountId}`,
        {
          method: "DELETE",
        },
      );
      if (!res.ok) {
        const error = await res.json().catch(() => null);
        throw new Error(error?.error || "Failed to delete");
      }
    },
    onMutate: () => {
      setErrorMessage(null);
      setStatusMessage(null);
    },
    onSuccess: () => {
      setStatusMessage("Account removed.");
      queryClient.invalidateQueries({ queryKey: ["providerAccounts"] });
    },
    onError: (err) => {
      setErrorMessage(
        err instanceof Error ? err.message : "Failed to delete account.",
      );
    },
  });

  return (
    <PageFrame
      title="Provider Accounts"
      description="Manage multiple accounts for AI providers. Select an active account to use."
    >
      <div className="flex flex-col gap-6">
        {isLoading ? (
          <div className="rounded-xl border border-border bg-background/60 p-4 text-sm text-muted-foreground">
            Loading provider accounts...
          </div>
        ) : null}
        {error ? (
          <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
            {error instanceof Error
              ? error.message
              : "Failed to load provider accounts."}
          </div>
        ) : null}
        {statusMessage ? (
          <div className="rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-4 text-sm text-emerald-200">
            {statusMessage}
          </div>
        ) : null}
        {errorMessage ? (
          <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive">
            {errorMessage}
          </div>
        ) : null}
        {SUPPORTED_PROVIDERS.map((provider) => (
          <ProviderAccountGroup
            key={provider.key}
            provider={provider}
            accounts={accounts.filter((a) => a.provider_key === provider.key)}
            onConnect={() => connectAccount.mutate(provider.key)}
            onTest={(id) => testAccount.mutate(id)}
            onVerify={(id) => verifyAccount.mutate(id)}
            onActivate={(id) => activateAccount.mutate(id)}
            onDelete={(id) => deleteAccount.mutate(id)}
            isConnecting={
              connectAccount.isPending &&
              connectAccount.variables === provider.key
            }
          />
        ))}
      </div>
    </PageFrame>
  );
}

function ProviderAccountGroup({
  provider,
  accounts,
  onConnect,
  onTest,
  onVerify,
  onActivate,
  onDelete,
  isConnecting,
}: {
  provider: { key: string; label: string };
  accounts: ProviderAccount[];
  onConnect: () => void;
  onTest: (id: string) => void;
  onVerify: (id: string) => void;
  onActivate: (id: string) => void;
  onDelete: (id: string) => void;
  isConnecting: boolean;
}) {
  const [expanded, setExpanded] = useState(true);

  return (
    <div className="rounded-[1.4rem] border border-border bg-card/80 overflow-hidden">
      <div
        className="p-5 flex items-center justify-between cursor-pointer hover:bg-muted/20 transition-colors"
        onClick={() => setExpanded(!expanded)}
      >
        <div className="flex items-center gap-3">
          <div className="flex items-center justify-center w-8 h-8 rounded-full bg-muted">
            {expanded ? (
              <ChevronDown className="w-5 h-5 text-muted-foreground" />
            ) : (
              <ChevronRight className="w-5 h-5 text-muted-foreground" />
            )}
          </div>
          <div>
            <h3 className="text-lg font-semibold">{provider.label}</h3>
            <p className="text-sm text-muted-foreground">
              {accounts.length} account(s)
            </p>
          </div>
        </div>
        <Button
          variant="secondary"
          className="text-sm px-3 py-1.5"
          onClick={(e) => {
            e.stopPropagation();
            onConnect();
          }}
          disabled={isConnecting}
        >
          {isConnecting ? (
            <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
          ) : (
            <Plus className="mr-2 h-4 w-4" />
          )}
          Connect New Account
        </Button>
      </div>

      {expanded && (
        <div className="border-t border-border p-5 flex flex-col gap-3">
          {accounts.length === 0 ? (
            <div className="text-center py-6 text-sm text-muted-foreground italic bg-background/50 rounded-xl border border-dashed">
              No accounts connected for {provider.label}
            </div>
          ) : (
            accounts.map((account) => (
              <div
                key={account.id}
                className={`flex items-center justify-between p-4 rounded-xl border transition-colors ${
                  account.is_active
                    ? "border-primary bg-primary/5"
                    : "border-border bg-background"
                }`}
              >
                <div className="flex items-center gap-4">
                  {account.is_active && (
                    <div className="w-6 h-6 rounded-full bg-primary flex items-center justify-center">
                      <Check className="w-4 h-4 text-primary-foreground" />
                    </div>
                  )}
                  <div>
                    <div className="flex items-center gap-2">
                      <p className="font-mono text-sm font-semibold">
                        {account.display_label}
                      </p>
                      <Badge
                        tone={
                          account.auth_status === "connected"
                            ? "success"
                            : account.auth_status === "connecting"
                              ? "neutral"
                              : "danger"
                        }
                      >
                        {account.auth_status}
                      </Badge>
                      {account.account_name &&
                      account.account_email &&
                      account.account_name !== account.account_email ? (
                        <span className="text-xs text-muted-foreground">
                          {account.account_name}
                        </span>
                      ) : null}
                    </div>
                    <p
                      className="text-xs text-muted-foreground mt-1 truncate max-w-[300px]"
                      title={account.auth_store_path ?? account.home_path}
                    >
                      Auth store: {account.auth_store_path ?? account.home_path}
                    </p>
                    <p
                      className="text-xs text-muted-foreground mt-1 truncate max-w-[300px]"
                      title={account.home_path}
                    >
                      Exec home: {account.home_path}
                    </p>
                    {account.usage_summary ? (
                      <p className="text-xs text-muted-foreground mt-1">
                        Plan: {account.usage_summary}
                      </p>
                    ) : null}
                    {account.access_token_expires_at ? (
                      <p className="text-xs text-muted-foreground mt-1">
                        Access token expires:{" "}
                        {formatResetAt(account.access_token_expires_at)}
                      </p>
                    ) : null}
                    {account.refresh_token_expires_at ? (
                      <p className="text-xs text-muted-foreground mt-1">
                        Refresh token expires:{" "}
                        {formatResetAt(account.refresh_token_expires_at)}
                      </p>
                    ) : null}
                    {account.refresh_token_expiry_note ? (
                      <p className="text-xs text-muted-foreground mt-1">
                        Refresh token expiry:{" "}
                        {account.refresh_token_expiry_note}
                      </p>
                    ) : null}
                    {account.usage_detail_lines.map((line) => (
                      <p
                        key={`${account.id}-${line.label}`}
                        className="text-xs text-muted-foreground mt-1"
                      >
                        {line.label}: {line.remaining_percent}%
                        {formatResetAt(line.reset_at)
                          ? ` · resets ${formatResetAt(line.reset_at)}`
                          : ""}
                      </p>
                    ))}
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  {account.auth_status === "connected" &&
                    !account.is_active && (
                      <Button
                        variant="secondary"
                        className="text-sm px-3 py-1.5"
                        onClick={() => onActivate(account.id)}
                      >
                        <CheckCircle2 className="w-4 h-4 mr-2" />
                        Set Active
                      </Button>
                    )}
                  <Button
                    variant="secondary"
                    className="text-sm px-3 py-1.5"
                    onClick={() => onTest(account.id)}
                  >
                    <Terminal className="w-4 h-4 mr-2" />
                    Test
                  </Button>
                  <Button
                    variant="secondary"
                    className="text-sm px-3 py-1.5"
                    onClick={() => onVerify(account.id)}
                  >
                    <RefreshCw className="w-4 h-4 mr-2" />
                    Verify
                  </Button>
                  <Button
                    variant="ghost"
                    className="p-2 text-destructive hover:text-destructive hover:bg-destructive/10"
                    onClick={() => {
                      if (confirm("Delete this account?")) {
                        onDelete(account.id);
                      }
                    }}
                  >
                    <Trash2 className="w-4 h-4" />
                  </Button>
                </div>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
}

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
