import { useEffect, useState } from "react";
import { RUNNER_URL } from "@/config";
import type { RunnerHealth } from "@flowpilot/client-core";
import { loadRunnerHealthUseCase } from "@/clientCore";

export function RunnerHealthPanel(): React.ReactElement {
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [health, setHealth] = useState<RunnerHealth | null>(null);

  useEffect(() => {
    let active = true;
    void (async () => {
      setBusy(true);
      setError(null);
      try {
        const payload = await loadRunnerHealthUseCase.execute();
        if (active) {
          setHealth(payload);
        }
      } catch (nextError) {
        if (active) {
          setError(
            nextError instanceof Error
              ? nextError.message
              : "Unable to reach local runner.",
          );
        }
      } finally {
        if (active) {
          setBusy(false);
        }
      }
    })();
    return () => {
      active = false;
    };
  }, []);

  return (
    <section className="settings-panel">
      <div className="settings-panel-head">
        <div>
          <div className="settings-eyebrow">Local Control Plane</div>
          <h2>Runner</h2>
          <p>Inspect the local runner used by desktop settings and chat execution.</p>
        </div>
        <div className={`runner-pill ${error ? "offline" : "online"}`}>
          {error ? "Offline" : busy ? "Checking" : "Online"}
        </div>
      </div>

      {busy ? <div className="settings-feedback">Checking runner health...</div> : null}
      {error ? <div className="settings-feedback error">{error}</div> : null}

      {health ? (
        <div className="settings-validation">
          <div className="validation-row passed">
            <span>Base URL</span>
            <span>{RUNNER_URL}</span>
          </div>
          <div className="validation-row passed">
            <span>Workspace</span>
            <span>{health.cwd ?? "Unavailable"}</span>
          </div>
          <div className="validation-row passed">
            <span>Platform</span>
            <span>{health.os ?? "Unavailable"}</span>
          </div>
          <div className="validation-row passed">
            <span>Version</span>
            <span>{health.version ?? "Unavailable"}</span>
          </div>
        </div>
      ) : null}
    </section>
  );
}
