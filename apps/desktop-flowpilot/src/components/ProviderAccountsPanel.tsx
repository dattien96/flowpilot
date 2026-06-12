import { useEffect, useMemo, useState } from "react";
import { useStore } from "@/state/store";
import type { ProviderAccountSummary } from "@/types/contract";

const PROVIDERS = [
  { key: "claude", label: "Claude" },
  { key: "codex", label: "Codex" },
  { key: "gemini", label: "Gemini" },
] as const;

function formatDateTime(value: string | null): string | null {
  if (!value) return null;
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

function formatAuthLabel(account: ProviderAccountSummary): string {
  if (account.accountEmail) return account.accountEmail;
  if (account.accountName) return account.accountName;
  return account.displayLabel || account.displayName;
}

function usageTone(percent: number): "ok" | "warn" | "err" {
  if (percent > 50) return "ok";
  if (percent > 20) return "warn";
  return "err";
}

function compactUsageLines(account: ProviderAccountSummary): ProviderAccountSummary["usageDetailLines"] {
  if (account.providerKey !== "gemini") {
    return account.usageDetailLines;
  }

  return account.usageDetailLines.filter((line) => line.label.includes("3.1"));
}

export function ProviderAccountsPanel(): React.ReactElement | null {
  const client = useStore((s) => s.client);
  const providerAccounts = useStore((s) => s.providerAccounts);
  const loadProviderAccounts = useStore((s) => s.loadProviderAccounts);
  const [openProviderKey, setOpenProviderKey] = useState<string | null>(null);
  const [expandedAccountIds, setExpandedAccountIds] = useState<Record<string, boolean>>({});
  const [busyAccountId, setBusyAccountId] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const providerGroups = useMemo(() => {
    return PROVIDERS.map((provider) => {
      const accounts = providerAccounts.filter((account) => account.providerKey === provider.key);
      const connectedAccounts = accounts.filter((account) => account.authStatus === "connected");
      const pinnedAccount = connectedAccounts.find((account) => account.isActive) ?? connectedAccounts[0] ?? null;
      return {
        ...provider,
        accounts,
        connectedAccounts,
        pinnedAccount,
        visible: pinnedAccount !== null,
      };
    }).filter((group) => group.visible);
  }, [providerAccounts]);

  useEffect(() => {
    if (!openProviderKey) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpenProviderKey(null);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [openProviderKey]);

  if (providerGroups.length === 0) {
    return null;
  }

  const activeGroup = providerGroups.find((group) => group.key === openProviderKey) ?? null;

  const toggleExpanded = (accountId: string) => {
    setExpandedAccountIds((current) => ({ ...current, [accountId]: !current[accountId] }));
  };

  const openGroupModal = (providerKey: string, focusAccountId?: string) => {
    setOpenProviderKey(providerKey);
    setMessage(null);
    setError(null);
    if (focusAccountId) {
      setExpandedAccountIds((current) => ({ ...current, [focusAccountId]: true }));
    }
  };

  const runAccountAction = async (accountId: string, action: () => Promise<void>, successMessage: string) => {
    setBusyAccountId(accountId);
    setMessage(null);
    setError(null);
    try {
      await action();
      setMessage(successMessage);
      await loadProviderAccounts();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Account action failed.");
    } finally {
      setBusyAccountId(null);
    }
  };

  const refreshAccounts = async () => {
    setRefreshing(true);
    setMessage(null);
    setError(null);
    try {
      await loadProviderAccounts();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to refresh accounts.");
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <>
      <section className="accounts-panel">
        <div className="accounts-title-row">
          <div className="accounts-title">Accounts</div>
          <button
            type="button"
            className="account-mini-btn icon"
            onClick={() => void refreshAccounts()}
            disabled={refreshing}
            title="Refresh account limits"
            aria-label="Refresh account limits"
          >
            {refreshing ? "…" : "↻"}
          </button>
        </div>
        <div className="accounts-list">
          {providerGroups.map((group) => {
            const pinned = group.pinnedAccount;
            if (!pinned) return null;
            const lines = compactUsageLines(pinned);
            return (
              <div key={group.key} className="account-provider-card">
                <div className="account-provider-head">
                  <div className="account-provider-label">{group.label}</div>
                  <button
                    type="button"
                    className="account-mini-btn"
                    onClick={() => openGroupModal(group.key, pinned.id)}
                    title={`Show ${group.label} accounts`}
                  >
                    All
                  </button>
                </div>
                <button
                  type="button"
                  className="account-pin"
                  onClick={() => openGroupModal(group.key, pinned.id)}
                >
                  <div className="account-pin-head">
                    <span className="account-pin-label">{formatAuthLabel(pinned)}</span>
                    <span className="account-chip">Active</span>
                  </div>
                  {pinned.accountName && pinned.accountEmail && pinned.accountName !== pinned.accountEmail ? (
                    <div className="account-pin-sub">{pinned.accountName}</div>
                  ) : null}
                  {lines.length > 0 ? (
                    <div className="account-bars">
                      {lines.map((line) => (
                        <div key={`${pinned.id}-${line.label}`} className="account-bar-row">
                          <div className="account-bar-meta">
                            <span>{line.label}</span>
                            <span>{line.remainingPercent}%</span>
                          </div>
                          <div className="meter">
                            <div className={`meter-fill ${usageTone(line.remainingPercent)}`} style={{ width: `${line.remainingPercent}%` }} />
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : pinned.usageSummary ? (
                    <div className="account-pin-sub">{pinned.usageSummary}</div>
                  ) : null}
                </button>
              </div>
            );
          })}
        </div>
      </section>

      {activeGroup ? (
        <div className="modal-backdrop" onClick={() => setOpenProviderKey(null)}>
          <div className="modal account-modal" onClick={(event) => event.stopPropagation()}>
            <div className="modal-head">
              <div>
                <div className="modal-title">{activeGroup.label} Accounts</div>
                <div className="modal-subtitle">{activeGroup.accounts.length} account(s)</div>
              </div>
              <button type="button" className="modal-close" onClick={() => setOpenProviderKey(null)}>
                Close
              </button>
            </div>

            {message ? <div className="panel-message ok">{message}</div> : null}
            {error ? <div className="panel-message err">{error}</div> : null}

            <div className="account-modal-list">
              {activeGroup.accounts.map((account) => {
                const expanded = expandedAccountIds[account.id] ?? account.isActive;
                const busy = busyAccountId === account.id;
                return (
                  <div key={account.id} className={`account-detail-card ${account.isActive ? "active" : ""}`}>
                    <button type="button" className="account-detail-summary" onClick={() => toggleExpanded(account.id)}>
                      <div className="account-detail-main">
                        <div className="account-detail-title-row">
                          <span className="account-detail-title">{formatAuthLabel(account)}</span>
                          <span className={`account-state ${account.authStatus}`}>{account.authStatus}</span>
                          {account.isActive ? <span className="account-chip">Active</span> : null}
                        </div>
                        {account.accountName && account.accountEmail && account.accountName !== account.accountEmail ? (
                          <div className="account-detail-sub">{account.accountName}</div>
                        ) : null}
                      </div>
                      <span className="account-expand-indicator">{expanded ? "Hide" : "Show"}</span>
                    </button>

                    {expanded ? (
                      <div className="account-detail-body">
                        <div className="account-detail-lines">
                          <div>Auth store: {account.authStorePath ?? account.homePath}</div>
                          <div>Exec home: {account.homePath}</div>
                          {account.usageSummary ? <div>Plan: {account.usageSummary}</div> : null}
                          {account.accessTokenExpiresAt ? <div>Access token expires: {formatDateTime(account.accessTokenExpiresAt)}</div> : null}
                          {account.refreshTokenExpiresAt ? <div>Refresh token expires: {formatDateTime(account.refreshTokenExpiresAt)}</div> : null}
                          {account.refreshTokenExpiryNote ? <div>Refresh token expiry: {account.refreshTokenExpiryNote}</div> : null}
                        </div>

                        {account.usageDetailLines.length > 0 ? (
                          <div className="account-bars details">
                            {account.usageDetailLines.map((line) => (
                              <div key={`${account.id}-detail-${line.label}`} className="account-bar-row">
                                <div className="account-bar-meta">
                                  <span>
                                    {line.label}: {line.remainingPercent}%
                                  </span>
                                  <span>{formatDateTime(line.resetAt) ? `resets ${formatDateTime(line.resetAt)}` : ""}</span>
                                </div>
                                <div className="meter">
                                  <div className={`meter-fill ${usageTone(line.remainingPercent)}`} style={{ width: `${line.remainingPercent}%` }} />
                                </div>
                              </div>
                            ))}
                          </div>
                        ) : null}

                        <div className="account-detail-actions">
                          {!account.isActive && account.authStatus === "connected" ? (
                            <button
                              type="button"
                              className="account-action-btn"
                              disabled={busy}
                              onClick={() =>
                                void runAccountAction(
                                  account.id,
                                  () => client.activateProviderAccount(account.id),
                                  `${activeGroup.label} active account updated.`,
                                )
                              }
                            >
                              {busy ? "Updating…" : "Set active"}
                            </button>
                          ) : null}
                          <button
                            type="button"
                            className="account-action-btn"
                            disabled={busy || account.authStatus !== "connected"}
                            onClick={() =>
                              void runAccountAction(
                                account.id,
                                () => client.openProviderAccountTerminal(account.id),
                                "Terminal opened for the selected account.",
                              )
                            }
                          >
                            {busy ? "Opening…" : "Open terminal"}
                          </button>
                        </div>
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      ) : null}
    </>
  );
}
