import { useCallback, useEffect, useState } from "react";
import type { DispatchAttentionItem, DispatchInspectResult, DispatchResolveAction } from "@/types/contract";
import { useStore } from "@/state/store";

/**
 * SS-17 / CP-51 Task-256 operator surface: shows uncertain dispatch + repair_required
 * items for the active run and offers resolve / retry-as-new / retry-load / abandon
 * actions, backed by the per-run REST surface (never a root /dispatch namespace).
 * Mirrors FlowAwaitingUserCard placement (above composer).
 */
export function DispatchAttentionCard(): React.ReactElement | null {
  const client = useStore((s) => s.client);
  const runId = useStore((s) => s.runId);
  const [items, setItems] = useState<DispatchAttentionItem[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [inspected, setInspected] = useState<Record<string, DispatchInspectResult>>({});
  const [confirmRetry, setConfirmRetry] = useState<string | null>(null);
  // Nothing in this card is dismissible (settle_pending in particular clears itself once
  // the durable settle finishes, per refresh() above) — but it can still crowd the
  // composer while an operator waits, so let them collapse it out of the way manually.
  const [collapsed, setCollapsed] = useState(false);

  const refresh = useCallback(async () => {
    if (!client?.listDispatchAttention || !runId) {
      setItems([]);
      return;
    }
    try {
      const list = await client.listDispatchAttention(runId);
      setItems(list);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [client, runId]);

  useEffect(() => {
    void refresh();
    const t = window.setInterval(() => void refresh(), 8000);
    return () => window.clearInterval(t);
  }, [refresh]);

  if (!client?.listDispatchAttention || items.length === 0) {
    return null;
  }

  const onlyPendingSettles = items.every((item) => item.kind === "settle_pending");

  const itemKey = (item: DispatchAttentionItem) => `${item.runId}:${item.turnId ?? ""}`;

  const inspect = async (item: DispatchAttentionItem) => {
    if (!client.inspectDispatch || !item.turnId) return;
    const key = itemKey(item);
    setBusy(`${key}/inspect`);
    setError(null);
    try {
      const result = await client.inspectDispatch(item.runId, item.turnId);
      setInspected((prev) => ({ ...prev, [key]: result }));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };

  const resolve = async (item: DispatchAttentionItem, action: DispatchResolveAction) => {
    if (!client.resolveDispatchUncertain || !item.turnId) return;
    const key = `${itemKey(item)}/${action}`;
    setBusy(key);
    setError(null);
    try {
      const info = inspected[itemKey(item)];
      await client.resolveDispatchUncertain(item.runId, item.turnId, {
        expectedRev: info?.revision ?? 0, // store rejects stale; operator can Inspect first to refresh
        resolutionId: `ui-${Date.now()}-${action}`,
        action,
        detail: "desktop operator",
      });
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };

  const retryAsNew = async (item: DispatchAttentionItem) => {
    if (!client.retryDispatchAsNew || !client.inspectDispatch || !item.turnId) return;
    const key = itemKey(item);
    // T-5 cancel-bias (SS-17 BR-3): if the record is cancel-requested, retry-as-new
    // requires an explicit extra confirm — the card's default action is confirm_cancelled.
    const info = inspected[key] ?? (await client.inspectDispatch(item.runId, item.turnId).catch(() => undefined));
    if (info?.cancelRequested && confirmRetry !== key) {
      setConfirmRetry(key);
      return;
    }
    setConfirmRetry(null);
    setBusy(`${key}/retry-as-new`);
    setError(null);
    try {
      await client.retryDispatchAsNew(item.runId, item.turnId, {
        expectedRev: info?.revision ?? 0,
        resolutionId: `ui-${Date.now()}-retry`,
        expectedIntentGen: info?.outerIntentGen ?? 0,
        expectedEnvelopeHash: info?.envelopeHash ?? "",
      });
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };

  const resolveRepair = async (item: DispatchAttentionItem, action: "retry_load" | "abandon") => {
    if (!client.resolveDispatchRepair) return;
    const key = `${itemKey(item)}/repair-${action}`;
    setBusy(key);
    setError(null);
    try {
      const info = inspected[itemKey(item)];
      await client.resolveDispatchRepair(item.runId, {
        expectedRepairRev: info?.openRepair?.repairRevision ?? 1,
        resolutionId: `ui-repair-${Date.now()}`,
        action,
      });
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="card question dispatch-attention-card" role="status" aria-live="polite">
      <div className="card-head">
        <button
          type="button"
          className="dispatch-attention-collapse-toggle"
          aria-expanded={!collapsed}
          aria-label={collapsed ? "Expand dispatch attention" : "Collapse dispatch attention"}
          title={collapsed ? "Expand" : "Collapse"}
          onClick={() => setCollapsed((value) => !value)}
        >
          <span className="dispatch-attention-caret" aria-hidden="true">{collapsed ? "▸" : "▾"}</span>
          <span className="badge badge-warn">Dispatch attention</span>
          <span className="meta">{items.length} item(s)</span>
        </button>
      </div>
      {error && <p className="error-text">{error}</p>}
      {collapsed ? null : (
      <>
      <p className="card-reason">
        {onlyPendingSettles
          ? "A terminal turn is finishing durable bookkeeping automatically. The task result is already recorded; details show settlement progress."
          : "Uncertain turns or repair-required runs need an operator decision (SS-17). Automated dispatch stays blocked until resolved."}
      </p>
      <ul className="dispatch-attention-list">
        {items.map((item) => {
          const key = itemKey(item);
          const info = inspected[key];
          const cancelBiased = info?.cancelRequested === true;
          return (
            <li key={key} className="dispatch-attention-item">
              <div className="meta">
                <strong>{item.kind}</strong> · run {item.runId}
                {item.turnId ? ` · turn ${item.turnId}` : ""}
              </div>
              {item.reason && <div className="card-reason">{item.reason}</div>}
              {info && (
                <pre className="dispatch-inspect-detail">
                  {JSON.stringify(
                    { state: info.state, settlePhase: info.settlePhase, cancelRequested: info.cancelRequested },
                    null,
                    2,
                  )}
                </pre>
              )}
              <div className="card-actions">
                <button type="button" disabled={busy !== null} onClick={() => void inspect(item)}>
                  {item.kind === "settle_pending" ? "View details" : "Inspect"}
                </button>
                {item.kind === "uncertain" && item.turnId && (
                  <>
                    <button
                      type="button"
                      // T-5 cancel-bias: the record is cancel-requested, so this is
                      // the recommended default action.
                      className={cancelBiased ? "primary" : undefined}
                      disabled={busy !== null}
                      onClick={() => void resolve(item, "confirm_cancelled")}
                    >
                      Confirm cancelled
                    </button>
                    <button
                      type="button"
                      disabled={busy !== null}
                      onClick={() => void resolve(item, "mark_completed")}
                    >
                      Mark completed
                    </button>
                    <button
                      type="button"
                      disabled={busy !== null}
                      onClick={() => void resolve(item, "mark_failed")}
                    >
                      Mark failed
                    </button>
                    <button
                      type="button"
                      disabled={busy !== null}
                      onClick={() => void retryAsNew(item)}
                    >
                      {confirmRetry === key ? "Confirm retry as new?" : "Retry as new"}
                    </button>
                    <button
                      type="button"
                      className="danger"
                      disabled={busy !== null}
                      onClick={() => void resolve(item, "abandon")}
                    >
                      Abandon
                    </button>
                  </>
                )}
                {item.kind === "repair_required" && (
                  <>
                    <button
                      type="button"
                      disabled={busy !== null}
                      onClick={() => void resolveRepair(item, "retry_load")}
                    >
                      Retry load
                    </button>
                    <button
                      type="button"
                      className="danger"
                      disabled={busy !== null}
                      onClick={() => void resolveRepair(item, "abandon")}
                    >
                      Abandon repair
                    </button>
                  </>
                )}
              </div>
            </li>
          );
        })}
      </ul>
      </>
      )}
    </div>
  );
}
