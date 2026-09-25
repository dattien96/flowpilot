import React from "react";

// Task-444 (CP-86 P-5) — the est-vs-usage honesty surface. `prompt ~Nk est`
// (the runner's heuristic input-length estimate) and `usage Nk` (provider-
// reported) are two different quantities and must never collapse into one
// label; missing data renders "—", never a fake zero. context_pressure
// (aware) and provider_compacted are inline notices — no buttons, no modal,
// no focus steal. Decision-tier items arrive through the existing
// user_question_required card path and need nothing here.

function formatTokenCount(value: number): string {
  if (value >= 1000) {
    return `${(value / 1000).toFixed(1)}k`;
  }
  return `${value}`;
}

/** T-1 — two labeled figures; absent data renders "—". */
export function UsageFigures(props: {
  estPromptTokens: number | null;
  usageTokens: number | null;
}): React.ReactElement {
  return (
    <span className="usage-figures">
      <span className="usage-figure usage-figure-est">
        {props.estPromptTokens != null && props.estPromptTokens > 0
          ? `prompt ~${formatTokenCount(props.estPromptTokens)} est`
          : "prompt — est"}
      </span>
      <span className="usage-figure usage-figure-actual">
        {props.usageTokens != null && props.usageTokens > 0
          ? `usage ${formatTokenCount(props.usageTokens)}`
          : "usage —"}
      </span>
    </span>
  );
}

/** T-2/T-3 — inline notices for aware-pressure and provider compaction. */
export function ContextNotice(props: {
  kind: "pressure_aware" | "provider_compacted";
  ratio?: number;
  prev?: number;
  cur?: number;
}): React.ReactElement | null {
  if (props.kind === "provider_compacted") {
    const figures =
      props.prev != null && props.cur != null
        ? ` (${formatTokenCount(props.prev)}→${formatTokenCount(props.cur)})`
        : "";
    return (
      <div className="context-notice context-notice-compacted" role="status">
        provider compressed context{figures} — leg output may degrade
      </div>
    );
  }
  const pct = props.ratio != null ? Math.round(props.ratio * 100) : null;
  if (pct == null) return null;
  return (
    <div className="context-notice context-notice-pressure" role="status">
      context at ~{pct}% of window
    </div>
  );
}
