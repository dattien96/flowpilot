import { useEffect, useState } from "react";
import { useStore } from "../../state/store";
import type { ModelClassBinding, QuotaRoutingSettings as QuotaRoutingSettingsType, WorkloadClass } from "../../types/contract";

const PROVIDER_KEYS = ["claude", "codex", "grok", "opencode", "devin", "gemini"];
const WORKLOAD_CLASSES: WorkloadClass[] = ["scan", "high_reasoning", "coding"];

const COOLDOWN_COPY =
  "Safety cooldown — rapid switching between multiple accounts of the same provider on one IP may trigger provider risk controls.";

const BINDING_MODELS: Record<string, string[]> = {};

/** QuotaRoutingSettings (Task-450 T-1): Manual/Auto rotation mode, provider
 *  priority order, provider+workload model bindings, thresholds, and the
 *  same-provider cooldown. Auto mode rotates only high-confidence
 *  candidates — it never cycles indefinitely (bounded switch cap). */
export function QuotaRoutingSettings(): React.ReactElement {
  const settings = useStore((s) => s.quotaRoutingSettings);
  const save = useStore((s) => s.saveQuotaRoutingSettings);
  const load = useStore((s) => s.loadQuotaRoutingSettings);
  const [message, setMessage] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [draft, setDraft] = useState<QuotaRoutingSettingsType>(settings);

  // Refresh the runner-owned document on mount; adopt the result into the
  // draft so a reopened panel never edits stale policy.
  useEffect(() => {
    void (async () => {
      await load();
      setDraft(useStore.getState().quotaRoutingSettings);
    })();
  }, [load]);

  const persist = async (next: QuotaRoutingSettingsType) => {
    setSaving(true);
    setMessage(null);
    try {
      await save(next);
      setDraft(useStore.getState().quotaRoutingSettings);
      setMessage("Quota routing settings saved.");
    } catch (e) {
      setMessage(`Unable to save quota routing settings: ${e instanceof Error ? e.message : e}`);
    } finally {
      setSaving(false);
    }
  };

  const movePriority = (key: string, dir: -1 | 1) => {
    const order = [...(draft.providerPriority ?? [])];
    const i = order.indexOf(key);
    if (i < 0) return;
    const j = i + dir;
    if (j < 0 || j >= order.length) return;
    [order[i], order[j]] = [order[j], order[i]];
    setDraft({ ...draft, providerPriority: order });
  };

  const togglePriority = (key: string) => {
    const order = new Set(draft.providerPriority ?? []);
    if (order.has(key)) order.delete(key);
    else order.add(key);
    setDraft({ ...draft, providerPriority: [...order] });
  };

  const setBinding = (providerKey: string, workloadClass: WorkloadClass, model: string) => {
    const bindings = (draft.modelBindings ?? []).filter(
      (b) => !(b.providerKey === providerKey && b.workloadClass === workloadClass),
    );
    if (model.trim() !== "") bindings.push({ providerKey, workloadClass, model: model.trim() });
    setDraft({ ...draft, modelBindings: bindings });
  };

  const bindingFor = (providerKey: string, workloadClass: WorkloadClass): ModelClassBinding | undefined =>
    (draft.modelBindings ?? []).find((b) => b.providerKey === providerKey && b.workloadClass === workloadClass);

  return (
    <section className="settings-panel" aria-label="Quota routing">
      <div className="settings-panel-head">
        <div>
          <div className="settings-eyebrow">Quota routing</div>
          <h3>Provider rotation policy</h3>
        </div>
      </div>
      {message ? <div className={`settings-feedback ${message.toLowerCase().includes("unable") ? "error" : ""}`}>{message}</div> : null}

      <div className="settings-subpanel">
        <fieldset>
          <legend>Rotation mode</legend>
          <label>
            <input
              type="radio"
              name="quota-mode"
              checked={draft.mode === "manual"}
              onChange={() => setDraft({ ...draft, mode: "manual" })}
            />
            Manual — always ask before routing to a different account or provider (default)
          </label>
          <label>
            <input
              type="radio"
              name="quota-mode"
              checked={draft.mode === "auto"}
              onChange={() => setDraft({ ...draft, mode: "auto" })}
            />
            Automatic — rotate only onto exact, healthy, fresh-quota candidates; anything else still asks
          </label>
          <p className="settings-hint">
            Automatic rotation is bounded: at most 2 provider/account switches per run, never mid-turn, and never
            to accounts with unknown or stale quota.
          </p>
        </fieldset>
      </div>

      <div className="settings-subpanel">
        <fieldset>
          <legend>Provider priority (cross-provider order)</legend>
          <ul className="quota-priority-list">
            {(draft.providerPriority ?? []).map((key, i) => (
              <li key={key}>
                <span>{i + 1}. {key}</span>{" "}
                <button className="btn" onClick={() => movePriority(key, -1)} aria-label={`Move ${key} up`}>↑</button>{" "}
                <button className="btn" onClick={() => movePriority(key, 1)} aria-label={`Move ${key} down`}>↓</button>{" "}
                <button className="btn" onClick={() => togglePriority(key)} aria-label={`Remove ${key}`}>✕</button>
              </li>
            ))}
          </ul>
          <div className="quota-priority-add">
            {PROVIDER_KEYS.filter((k) => !(draft.providerPriority ?? []).includes(k)).map((k) => (
              <button key={k} className="btn" onClick={() => togglePriority(k)} aria-label={`Add ${k} to priority`}>
                + {k}
              </button>
            ))}
          </div>
        </fieldset>
      </div>

      <div className="settings-subpanel">
        <fieldset>
          <legend>Model per workload class</legend>
          <table className="quota-bindings-table" aria-label="Model bindings">
            <thead>
              <tr><th>Provider</th><th>Workload</th><th>Model</th></tr>
            </thead>
            <tbody>
              {PROVIDER_KEYS.flatMap((pk) =>
                WORKLOAD_CLASSES.map((wc) => (
                  <tr key={`${pk}|${wc}`}>
                    <td>{pk}</td>
                    <td>{wc}</td>
                    <td>
                      <input
                        className="text-input"
                        aria-label={`${pk} ${wc} model`}
                        value={bindingFor(pk, wc)?.model ?? ""}
                        list={`models-${pk}-${wc}`}
                        onChange={(e) => setBinding(pk, wc, e.target.value)}
                        placeholder="(provider default)"
                      />
                      <datalist id={`models-${pk}-${wc}`}>
                        {(BINDING_MODELS[pk] ?? []).map((m) => <option key={m} value={m} />)}
                      </datalist>
                    </td>
                  </tr>
                )),
              )}
            </tbody>
          </table>
        </fieldset>
      </div>

      <div className="settings-subpanel">
        <fieldset>
          <legend>Thresholds</legend>
          <label>
            Low-headroom warning below (%):{" "}
            <input
              className="text-input" type="number" min={0} max={100}
              aria-label="Low headroom percent"
              value={draft.headroomLowPercent}
              onChange={(e) => setDraft({ ...draft, headroomLowPercent: Number(e.target.value) })}
            />
          </label>
          <label>
            Telemetry freshness (seconds):{" "}
            <input
              className="text-input" type="number" min={1}
              aria-label="Telemetry TTL seconds"
              value={draft.telemetryTtlSeconds}
              onChange={(e) => setDraft({ ...draft, telemetryTtlSeconds: Number(e.target.value) })}
            />
          </label>
          <label>
            Same-provider cooldown (seconds):{" "}
            <input
              className="text-input" type="number" min={1}
              aria-label="Same provider cooldown seconds"
              value={draft.sameProviderCooldownSeconds}
              onChange={(e) => setDraft({ ...draft, sameProviderCooldownSeconds: Number(e.target.value) })}
            />
          </label>
          <p className="settings-hint">{COOLDOWN_COPY}</p>
        </fieldset>
      </div>

      <div className="settings-actions">
        <button className="btn btn-primary" disabled={saving} onClick={() => void persist(draft)}>
          {saving ? "Saving…" : "Save quota routing"}
        </button>
      </div>
    </section>
  );
}
