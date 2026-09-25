import type { QuotaRouteCandidateDTO, QuotaRouteDecisionDTO } from "../../types/contract";
import { SameProviderCooldownBar } from "./SameProviderCooldownBar";

interface Props {
  decision: QuotaRouteDecisionDTO;
  onSelect(optionId: string): void;
}

/** Headroom renders honestly: percent only for exact telemetry, the state
 *  word otherwise — "unknown" is never rendered as a count or zero. */
function headroomCell(c: QuotaRouteCandidateDTO): string {
  const h = c.headroom;
  if (!h) return "unknown";
  if (h.remainingPercent !== undefined && h.state && h.state !== "unknown") {
    return `${h.remainingPercent}%`;
  }
  return h.state || "unknown";
}

function resetCell(c: QuotaRouteCandidateDTO): string {
  const r = c.headroom?.resetAt;
  if (!r) return "—";
  const t = Date.parse(r);
  if (Number.isNaN(t)) return r;
  return new Date(t).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function reasonCell(c: QuotaRouteCandidateDTO): string {
  const parts: string[] = [];
  if (c.autoEligible) parts.push("eligible");
  if (c.rejectionReasons?.length) parts.push(...c.rejectionReasons);
  return parts.length ? parts.join(", ") : "—";
}

/** The option id the runner parses back — must match emitQuotaRouteCard's
 *  label encoding: use_once|provider|account|model / use_for_run|…. */
function optionId(scope: "use_once" | "use_for_run", c: QuotaRouteCandidateDTO): string {
  return `${scope}|${c.providerKey}|${c.accountId}|${c.model ?? ""}`;
}

/** QuotaCandidateTable — the manual quota gate's route picker (Task-450 T-2).
 *  Columns are contractual; runner order is preserved verbatim; rejected and
 *  cooling rows render disabled with their reasons rather than vanishing.
 *  Keyboard: rows are real <button>s inside a real <table>. */
export function QuotaCandidateTable({ decision, onSelect }: Props): React.ReactElement {
  const candidates = decision.candidates ?? [];
  return (
    <div className="quota-table-wrap">
      <p className="card-prompt">
        Binding {decision.providerKey}/{decision.accountId ?? "?"} unusable ({decision.reason ?? decision.trigger})
      </p>
      <table className="quota-candidate-table" aria-label="Quota route candidates">
        <thead>
          <tr>
            <th scope="col">Provider</th>
            <th scope="col">Model</th>
            <th scope="col">Workload</th>
            <th scope="col">Account</th>
            <th scope="col">Headroom</th>
            <th scope="col">Reset</th>
            <th scope="col">Confidence</th>
            <th scope="col">Reason</th>
            <th scope="col">Actions</th>
          </tr>
        </thead>
        <tbody>
          {candidates.map((c) => {
            const rejected = (c.rejectionReasons ?? []).some((r) =>
              ["billing_required", "exhausted_quota", "account_disconnected", "provider_unavailable", "no_connected_account"].includes(r),
            );
            const cooling = !!c.cooldownUntil;
            const disabled = rejected || cooling;
            return (
              <tr key={`${c.providerKey}|${c.accountId}|${c.model ?? ""}`} className={disabled ? "quota-row-disabled" : ""}>
                <td>{c.providerKey}</td>
                <td>{c.model ?? "—"}</td>
                <td>{c.workloadClass ?? "—"}</td>
                <td>{c.displayName ? `${c.displayName} (${c.accountId})` : c.accountId}</td>
                <td>{headroomCell(c)}</td>
                <td>{resetCell(c)}</td>
                <td>{c.headroom?.confidence || "—"}</td>
                <td>{reasonCell(c)}</td>
                <td>
                  {cooling ? (
                    <SameProviderCooldownBar
                      startedAt={c.cooldownStartedAt ?? ""}
                      until={c.cooldownUntil ?? ""}
                      reason="same_provider_ip_safety"
                    />
                  ) : (
                    <>
                      <button
                        className="btn"
                        disabled={disabled}
                        onClick={() => onSelect(optionId("use_once", c))}
                        aria-label={`Use ${c.providerKey} ${c.accountId} once`}
                      >
                        Once
                      </button>{" "}
                      <button
                        className="btn"
                        disabled={disabled}
                        onClick={() => onSelect(optionId("use_for_run", c))}
                        aria-label={`Use ${c.providerKey} ${c.accountId} for this run`}
                      >
                        For run
                      </button>
                    </>
                  )}
                </td>
              </tr>
            );
          })}
          {candidates.length === 0 && (
            <tr>
              <td colSpan={9}>(no candidates)</td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="quota-table-actions">
        <button className="btn btn-danger" onClick={() => onSelect("stop")} aria-label="Stop this run">
          Stop
        </button>
      </div>
    </div>
  );
}
