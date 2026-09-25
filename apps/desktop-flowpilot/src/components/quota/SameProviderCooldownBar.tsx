import { useEffect, useState } from "react";

interface Props {
  /** Server timestamps (RFC3339) — the countdown derives from these, so a
   *  remount resumes at the true remaining time instead of restarting. */
  startedAt: string;
  until: string;
  reason: "same_provider_ip_safety";
  /** Test seam — ms epoch overriding Date.now(). */
  now?: number;
}

const SAFETY_COPY =
  "Safety cooldown — rapid switching between multiple accounts of the same provider on one IP may trigger provider risk controls.";

function remainingSeconds(until: string, nowMs: number): number {
  const untilMs = Date.parse(until);
  if (Number.isNaN(untilMs)) return 0;
  return Math.max(0, Math.ceil((untilMs - nowMs) / 1000));
}

function windowSeconds(startedAt: string, until: string): number {
  const start = Date.parse(startedAt);
  const end = Date.parse(until);
  if (Number.isNaN(start) || Number.isNaN(end) || end <= start) return 0;
  return Math.round((end - start) / 1000);
}

/** SameProviderCooldownBar — determinate countdown for the same-provider
 *  IP-safety window (Task-450 T-6). Ticks at most once per second, reaches
 *  zero without client-side extension, and the bar is never color-only:
 *  the seconds-remaining text carries the state for screen readers. */
export function SameProviderCooldownBar({ startedAt, until, reason, now }: Props): React.ReactElement {
  const [nowMs, setNowMs] = useState(() => now ?? Date.now());

  useEffect(() => {
    if (now !== undefined) return; // fixed-clock tests don't tick
    const t = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(t);
  }, [now]);

  const remaining = remainingSeconds(until, nowMs);
  const total = windowSeconds(startedAt, until);
  const pct = total > 0 ? Math.min(100, Math.max(0, (remaining / total) * 100)) : 0;
  const label = `${remaining}s remaining`;

  return (
    <div className="quota-cooldown" role="group" aria-label="Same-provider cooldown">
      <div
        className="quota-cooldown-bar"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={total}
        aria-valuenow={remaining}
        aria-valuetext={label}
      >
        <div className="quota-cooldown-fill" style={{ width: `${pct}%` }} />
      </div>
      <div className="quota-cooldown-text">
        <span className="quota-cooldown-seconds">{label}</span>
        <span className="quota-cooldown-reason">{reason === "same_provider_ip_safety" ? SAFETY_COPY : reason}</span>
      </div>
    </div>
  );
}
