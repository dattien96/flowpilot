import { useCallback, useEffect, useState } from "react";
import type { DispatchAttentionItem, DispatchResolveAction } from "@/types/contract";
import { useStore } from "@/state/store";

/**
 * SS-17 / CP-51 Task-256 operator surface: shows uncertain dispatch + repair_required
 * items from GET /client/dispatch/attention and offers resolve / abandon actions.
 * Mirrors FlowAwaitingUserCard placement (above composer).
 */
export function DispatchAttentionCard(): React.ReactElement | null {
  const client = useStore((s) => s.client);
  const runId = useStore((s) => s.runId);
  const [items, setItems] = useState<DispatchAttentionItem[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!client?.listDispatchAttention) {
      setItems([]);
      return;
    }
    try {
      const list = await client.listDispatchAttention();
      // Prefer items for the active run; still show global attention if none.
      const filtered = runId
        ? list.filter((i) => i.runId === runId || !i.runId)
        : list;
      setItems(filtered.length > 0 ? filtered : list.slice(0, 5));
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

  const resolve = async (item: DispatchAttentionItem, action: DispatchResolveAction) => {
    if (!client.resolveDispatchUncertain || !item.turnId) return;
    const key = `${item.runId}/${item.turnId}/${action}`;
    setBusy(key);
    setError(null);
    try {
      await client.resolveDispatchUncertain({
        runId: item.runId,
        turnId: item.turnId,
        expectedRev: 0, // store may reject stale; operator can refresh
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

  const abandonRepair = async (item: DispatchAttentionItem) => {
    if (!client.beginDispatchRepair || !client.commitDispatchRepair) return;
    const key = `${item.runId}/repair`;
    setBusy(key);
    setError(null);
    try {
      const begin = await client.beginDispatchRepair({
        runId: item.runId,
        expectedRepairRev: 1,
        resolutionId: `ui-repair-${Date.now()}`,
        action: "abandon",
      });
      await client.commitDispatchRepair({
        runId: item.runId,
        attemptRev: begin.attemptRev,
        resolutionId: `ui-repair-${Date.now()}`,
        outcome: "resolved_abandon",
        detail: "desktop operator abandon",
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
        <span className="badge badge-warn">Dispatch attention</span>
        <span className="meta">{items.length} item(s)</span>
      </div>
      <p className="card-reason">
        Uncertain turns or repair-required runs need an operator decision (SS-17). Automated
        dispatch stays blocked until resolved.
      </p>
      {error && <p className="error-text">{error}</p>}
      <ul className="dispatch-attention-list">
        {items.map((item) => {
          const id = `${item.kind}:${item.runId}:${item.turnId ?? ""}`;
          return (
            <li key={id} className="dispatch-attention-item">
              <div className="meta">
                <strong>{item.kind}</strong> · run {item.runId}
                {item.turnId ? ` · turn ${item.turnId}` : ""}
              </div>
              {item.reason && <div className="card-reason">{item.reason}</div>}
              <div className="card-actions">
                {item.kind === "uncertain" && item.turnId && (
                  <>
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
                      onClick={() => void resolve(item, "confirm_cancelled")}
                    >
                      Confirm cancelled
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
                  <button
                    type="button"
                    className="danger"
                    disabled={busy !== null}
                    onClick={() => void abandonRepair(item)}
                  >
                    Abandon repair
                  </button>
                )}
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
