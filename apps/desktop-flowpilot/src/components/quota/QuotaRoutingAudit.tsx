import type { QuotaRoutingAuditRecord } from "../../types/contract";

interface Props {
  record: QuotaRoutingAuditRecord;
}

function fmtTokens(v: number | undefined | null): string {
  if (v === undefined || v === null) return "—";
  return `${v}`;
}

/** QuotaRoutingAudit — the forensic route record (Task-450 T-4): requested →
 *  resolved binding, reason, policy version, headroom evidence, and the
 *  CP-86 usage figures (estimated prompt, node usage cap, actual usage). */
export function QuotaRoutingAudit({ record }: Props): React.ReactElement {
  const rows: [string, string][] = [
    ["Outcome", record.outcome || "none"],
    ["Committed at", record.committedAt ?? "—"],
    ["Requested", `${record.fromProvider ?? "?"} / ${record.fromAccount ?? "?"}`.trim()],
    ["Resolved", `${record.toProvider ?? "—"} / ${record.toAccount ?? "—"}${record.toModel ? ` / ${record.toModel}` : ""}`],
    ["Scope", record.scope ?? "—"],
    ["Reason", record.reason ?? "—"],
    ["Policy version", `${record.policyVersion}`],
    [
      "Headroom evidence",
      record.headroom
        ? `${record.headroom.state}${record.headroom.remainingPercent !== undefined ? ` ${record.headroom.remainingPercent}%` : ""} (${record.headroom.confidence})`
        : "—",
    ],
    ["Est. prompt tokens", fmtTokens(record.estPromptTokens)],
    ["Max usage tokens", fmtTokens(record.maxUsageTokens)],
    ["Actual usage", record.actualUsage ? fmtTokens(record.actualUsage.totalTokens) : "—"],
  ];
  return (
    <dl className="quota-audit" aria-label="Quota routing audit">
      {rows.map(([k, v]) => (
        <div className="quota-audit-row" key={k}>
          <dt>{k}</dt>
          <dd>{v}</dd>
        </div>
      ))}
    </dl>
  );
}
