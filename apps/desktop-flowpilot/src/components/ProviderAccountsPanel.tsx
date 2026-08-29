import { useEffect, useMemo, useState } from "react";
import { useStore } from "@/state/store";
import type { ProviderAccountSummary } from "@/types/contract";

const PROVIDER_VISIBILITY_STORAGE_KEY = "flowpilot.desktop.account-provider-visibility";

const PROVIDERS = [
  { key: "claude", label: "Claude" },
  { key: "codex", label: "Codex" },
  { key: "gemini", label: "Gemini" },
  { key: "grok", label: "Grok" },
  { key: "opencode", label: "OpenCode" },
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

function loadProviderVisibility(): Record<string, boolean> {
  if (typeof window === "undefined") {
    return {};
  }

  try {
    const raw = window.localStorage.getItem(PROVIDER_VISIBILITY_STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as Record<string, boolean>;
    return typeof parsed === "object" && parsed !== null ? parsed : {};
  } catch {
    return {};
  }
}

function saveProviderVisibility(next: Record<string, boolean>): void {
  if (typeof window === "undefined") {
    return;
  }

  try {
    window.localStorage.setItem(PROVIDER_VISIBILITY_STORAGE_KEY, JSON.stringify(next));
  } catch {
    // ignore localStorage write failures
  }
}

function compactUsageLines(account: ProviderAccountSummary): ProviderAccountSummary["usageDetailLines"] {
  if (account.providerKey !== "gemini") {
    // CA-683: opencode lines are real `opencode stats` text rows (runner-side,
    // 60s cached) — safe to surface on the pin card next to the N/A chip. The
    // fabricated 0% meters from Task-302 T-6 no longer exist.
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
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [providerVisibility, setProviderVisibility] = useState<Record<string, boolean>>(() => loadProviderVisibility());
  const [connectingProviderKey, setConnectingProviderKey] = useState<string | null>(null);
  const [pendingProviderKey, setPendingProviderKey] = useState<string | null>(null);
  const [pendingKnownAccountIds, setPendingKnownAccountIds] = useState<string[]>([]);

  const providerGroups = useMemo(() => {
    return PROVIDERS.map((provider) => {
      const accounts = providerAccounts.filter((account) => account.providerKey === provider.key);
      const connectedAccounts = accounts.filter((account) => account.authStatus === "connected");
      const pinnedAccount = connectedAccounts.find((account) => account.isActive) ?? connectedAccounts[0] ?? null;
      const enabled = providerVisibility[provider.key] ?? true;
      return {
        ...provider,
        accounts,
        connectedAccounts,
        pinnedAccount,
        enabled,
        visible: pinnedAccount !== null && enabled,
      };
    });
  }, [providerAccounts, providerVisibility]);

  const visibleProviderGroups = providerGroups.filter((group) => group.visible);

  useEffect(() => {
    if (!openProviderKey && !settingsOpen) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpenProviderKey(null);
        setSettingsOpen(false);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [openProviderKey, settingsOpen]);

  useEffect(() => {
    if (!pendingProviderKey) return;
    const intervalId = window.setInterval(() => {
      void loadProviderAccounts();
    }, 2000);
    return () => window.clearInterval(intervalId);
  }, [loadProviderAccounts, pendingProviderKey]);

  useEffect(() => {
    if (!pendingProviderKey) return;
    const nextAccounts = providerAccounts.filter((account) => account.providerKey === pendingProviderKey);
    const hasNewAccount = nextAccounts.some((account) => !pendingKnownAccountIds.includes(account.id));
    if (!hasNewAccount) return;
    setPendingProviderKey(null);
    setPendingKnownAccountIds([]);
    setMessage("New account detected. The modal refreshed automatically.");
    setError(null);
  }, [pendingKnownAccountIds, pendingProviderKey, providerAccounts]);

  if (visibleProviderGroups.length === 0 && providerGroups.every((group) => group.pinnedAccount === null)) {
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

  const connectAccount = async (providerKey: string) => {
    setConnectingProviderKey(providerKey);
    setMessage(null);
    setError(null);
    try {
      const knownAccountIds = providerAccounts
        .filter((account) => account.providerKey === providerKey)
        .map((account) => account.id);
      await client.connectProviderAccount(providerKey as "claude" | "codex" | "gemini" | "grok" | "opencode");
      setPendingKnownAccountIds(knownAccountIds);
      setPendingProviderKey(providerKey);
      setMessage(
        "Login terminal requested. Complete authentication in the opened terminal/browser. FlowPilot will refresh this modal automatically when the new account is detected.",
      );
      await loadProviderAccounts();
    } catch (err) {
      setPendingProviderKey(null);
      setPendingKnownAccountIds([]);
      setError(err instanceof Error ? err.message : "Failed to connect account.");
    } finally {
      setConnectingProviderKey(null);
    }
  };

  const updateProviderVisibility = (providerKey: string, checked: boolean) => {
    setProviderVisibility((current) => {
      const next = { ...current, [providerKey]: checked };
      saveProviderVisibility(next);
      return next;
    });
  };

  return (
    <>
      <section className="accounts-panel">
        <div className="accounts-title-row">
          <div className="accounts-title">Accounts</div>
          <div className="accounts-title-actions">
            <button
              type="button"
              className="account-mini-btn icon"
              onClick={() => setSettingsOpen(true)}
              title="Account display settings"
              aria-label="Account display settings"
            >
              ⚙
            </button>
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
        </div>
        <div className="accounts-list">
          {visibleProviderGroups.map((group) => {
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
                  {group.key === "opencode" ? (
                    <>
                      <div className="account-pin-sub" title="Opencode is a zen proxy — limit depends on upstream provider, see opencode stats">Limit: N/A (zen proxy)</div>
                      {lines.slice(0, 2).map((line) => (
                        <div key={`${pinned.id}-pin-${line.label}`} className="account-pin-sub" title="Opencode is a zen proxy — limit depends on upstream provider, see opencode stats">{line.label}</div>
                      ))}
                    </>
                  ) : lines.length > 0 ? (
                    <div className="account-bars">
                      {lines.map((line) => (
                        <div key={`${pinned.id}-${line.label}`} className="account-bar-row">
                          <div className="account-bar-meta">
                            <span>{line.label}</span>
                            <span>
                              {line.remainingPercent}%
                              {formatDateTime(line.resetAt) ? ` · resets ${formatDateTime(line.resetAt)}` : ""}
                            </span>
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

      {settingsOpen ? (
        <div className="modal-backdrop" onClick={() => setSettingsOpen(false)}>
          <div className="modal account-settings-modal" onClick={(event) => event.stopPropagation()}>
            <div className="modal-head">
              <div>
                <div className="modal-title">Account Menu Settings</div>
                <div className="modal-subtitle">Choose which valid providers are shown in the sidebar.</div>
              </div>
              <button type="button" className="modal-close" onClick={() => setSettingsOpen(false)}>
                Close
              </button>
            </div>

            <div className="provider-settings-list">
              {providerGroups.map((group) => {
                const disabled = group.pinnedAccount === null;
                const checked = group.enabled;
                return (
                  <label
                    key={group.key}
                    className={`provider-setting-row ${disabled ? "disabled" : ""}`}
                  >
                    <input
                      type="checkbox"
                      checked={checked}
                      disabled={disabled}
                      onChange={(event) => updateProviderVisibility(group.key, event.target.checked)}
                    />
                    <div className="provider-setting-copy">
                      <div className="provider-setting-title">{group.label}</div>
                      <div className="provider-setting-sub">
                        {disabled ? "No valid active/connected account available." : "Show this provider in the accounts menu."}
                      </div>
                    </div>
                  </label>
                );
              })}
            </div>
          </div>
        </div>
      ) : null}

      {activeGroup ? (
        <div className="modal-backdrop" onClick={() => setOpenProviderKey(null)}>
          <div className="modal account-modal" onClick={(event) => event.stopPropagation()}>
            <div className="modal-head">
              <div>
                <div className="modal-title">{activeGroup.label} Accounts</div>
                <div className="modal-subtitle">{activeGroup.accounts.length} account(s)</div>
              </div>
              <div className="account-modal-head-actions">
                <button
                  type="button"
                  className="account-action-btn"
                  disabled={connectingProviderKey === activeGroup.key}
                  onClick={() => void connectAccount(activeGroup.key)}
                >
                  {connectingProviderKey === activeGroup.key || pendingProviderKey === activeGroup.key
                    ? "Connecting..."
                    : "Connect New Account"}
                </button>
                <button type="button" className="modal-close" onClick={() => setOpenProviderKey(null)}>
                  Close
                </button>
              </div>
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
                          account.providerKey === "opencode" ? (
                            <div className="account-bars details">
                              {account.usageDetailLines.map((line) => (
                                <div key={`${account.id}-detail-${line.label}`} className="account-bar-row">
                                  <div className="account-bar-meta">
                                    <span title="Opencode is a zen proxy — limit depends on upstream provider, see opencode stats">{line.label}</span>
                                    <span>{formatDateTime(line.resetAt) ? `resets ${formatDateTime(line.resetAt)}` : ""}</span>
                                  </div>
                                </div>
                              ))}
                              <div className="account-pin-sub" title="Opencode is a zen proxy — limit depends on upstream provider, see opencode stats">Limit: N/A (zen proxy)</div>
                            </div>
                          ) : (
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
                          )
                        ) : account.providerKey === "opencode" ? (
                          <div className="account-pin-sub" title="Opencode is a zen proxy — limit depends on upstream provider, see opencode stats">Limit: N/A (zen proxy)</div>
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
