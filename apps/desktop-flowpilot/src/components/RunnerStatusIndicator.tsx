import { useEffect, useState } from "react";
import type { RunnerHealth } from "@flowpilot/client-core";
import { loadRunnerHealthUseCase } from "@/clientCore";
import { RUNNER_URL } from "@/config";
import { useStore } from "@/state/store";

// CP-81 T-5: compact lifecycle fragment for the runner chip — reconnecting
// overrides everything; then idle countdown, shared clients, update pending.
function lifecycleChip(status: ReturnType<typeof useStore.getState>["lifecycleStatus"], nowMs: number): string | null {
  if (!status) return null;
  if (status.reconnecting || status.phase === "draining_restart") return "Runner restarting…";
  const parts: string[] = [];
  if (status.sharedClients > 1) parts.push(`shared:${status.sharedClients}`);
  if (status.activeWork > 0) parts.push(`work:${status.activeWork}`);
  if (status.idleDeadlineMs !== undefined && status.phase === "idle_grace") {
    const secs = Math.max(0, Math.round((status.idleDeadlineMs - nowMs) / 1000));
    parts.push(`idle shutdown in ${secs}s`);
  }
  if (status.updatePending) parts.push("update pending");
  return parts.length > 0 ? parts.join(" · ") : null;
}

function toErrorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Unable to reach local runner.";
}

export function RunnerStatusIndicator(): React.ReactElement {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(true);
  const [health, setHealth] = useState<RunnerHealth | null>(null);
  const [error, setError] = useState<string | null>(null);
  const lifecycleStatus = useStore((s) => s.lifecycleStatus);

  const refresh = async () => {
    setBusy(true);
    setError(null);
    try {
      setHealth(await loadRunnerHealthUseCase.execute());
    } catch (nextError) {
      setHealth(null);
      setError(toErrorMessage(nextError));
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  const label = error ? "Offline" : busy ? "Checking" : "Online";
  const tone = error ? "offline" : busy ? "checking" : "online";
  const chip = lifecycleChip(lifecycleStatus, Date.now());

  return (
    <>
      <button
        className={`runner-status-trigger ${tone}`}
        onClick={() => {
          setOpen(true);
          void refresh();
        }}
        type="button"
      >
        <span className={`status-dot status-${tone === "checking" ? "starting" : tone}`} />
        <span>Runner {label}</span>
        {chip ? <span className="runner-lifecycle-chip">{chip}</span> : null}
      </button>

      {open ? (
        <div
          className="settings-modal-backdrop"
          onClick={() => setOpen(false)}
          role="presentation"
        >
          <div
            aria-label="Runner health"
            className="settings-modal"
            onClick={(event) => event.stopPropagation()}
            role="dialog"
          >
            <div className="settings-panel-head">
              <div>
                <div className="settings-eyebrow">Local Control Plane</div>
                <h2>Runner Health</h2>
                <p>Desktop uses this runner for configuration, validation, and chat execution.</p>
              </div>
              <div className={`runner-pill ${error ? "offline" : "online"}`}>{label}</div>
            </div>

            {busy ? <div className="settings-feedback">Checking runner health...</div> : null}
            {error ? <div className="settings-feedback error">{error}</div> : null}

            <div className="settings-validation">
              <div className="validation-row passed">
                <span>Base URL</span>
                <span>{RUNNER_URL}</span>
              </div>
              <div className={`validation-row ${error ? "failed" : "passed"}`}>
                <span>Workspace</span>
                <span>{health?.cwd ?? "Unavailable"}</span>
              </div>
              <div className={`validation-row ${error ? "failed" : "passed"}`}>
                <span>Platform</span>
                <span>{health?.os ?? "Unavailable"}</span>
              </div>
              <div className={`validation-row ${error ? "failed" : "passed"}`}>
                <span>Version</span>
                <span>{health?.version ?? "Unavailable"}</span>
              </div>
              <div className={`validation-row ${error ? "failed" : "passed"}`}>
                <span>Started</span>
                <span>{health?.startedAt ?? "Unavailable"}</span>
              </div>
              <div className={`validation-row ${error ? "failed" : "passed"}`}>
                <span>Error</span>
                <span>{error ?? "None"}</span>
              </div>
            </div>

            <div className="settings-actions">
              <button className="secondary-btn" onClick={() => void refresh()} type="button">
                Refresh
              </button>
              <button className="ghost-btn" onClick={() => setOpen(false)} type="button">
                Close
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </>
  );
}
