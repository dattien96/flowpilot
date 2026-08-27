import { useEffect, useState } from "react";
import type { CompatCheckResult, CompatConfig, CompatItem, CompatVersionInfo } from "@flowpilot/client-core";
import { loadCompatConfigUseCase, loadCompatInfoUseCase, runCompatCheckUseCase, runCompatDeepCheckUseCase, saveCompatConfigUseCase } from "@/clientCore";

type CheckMode = "fast" | "deep";

function VersionRow({
  label,
  tested,
  installed,
}: {
  label: string;
  tested: string;
  installed: string;
}) {
  const match = installed.includes(tested);
  const missing = installed === "";
  return (
    <div className="validation-row">
      <span>{label}</span>
      <span className="check-version-versions">
        <span className="check-version-badge baseline">tested: {tested}</span>
        <span className={`check-version-badge installed ${missing ? "fail" : match ? "pass" : "warn"}`}>
          {missing ? "not found" : `installed: ${installed}`}
        </span>
      </span>
    </div>
  );
}

function ItemRow({ item }: { item: CompatItem }) {
  const label =
    item.status === "fail" ? "FAILED" : item.status === "warn" ? "WARNING" : "PASSED";
  return (
    <div className={`validation-row ${item.status}`}>
      <span className="check-version-item-title">
        <span className={`check-version-status ${item.status}`}>{label}</span>
        <span className="check-version-item-name">{item.name}</span>
      </span>
      <span className="check-version-item-detail">{item.detail}</span>
    </div>
  );
}

function buildNextSteps(result: CompatCheckResult, mode: CheckMode | null): string[] {
  const findItem = (name: string) => result.items.find((item) => item.name === name);
  const steps: string[] = [];
  const claudeProbe = findItem("claude stream-json live probe");
  const codexProbe = findItem("codex app-server initialize");
  const claudeVersion = findItem("Claude version");
  const codexVersion = findItem("Codex version");

  if (claudeProbe?.status === "fail") {
    steps.push(
      "Fix the Claude live probe first. Check Claude login and run a simple `claude -p --output-format stream-json` prompt in terminal to see the full error.",
    );
  }
  if (codexProbe?.status === "fail") {
    steps.push("Fix Codex app-server first. FlowPilot cannot trust Codex app-server until initialize succeeds.");
  } else if (codexProbe?.status === "warn") {
    steps.push("Codex app-server responded. The missing capabilities field is a warning; FlowPilot can continue with optimistic feature support.");
  }
  if (codexVersion?.status === "fail") {
    steps.push(
      "Keep the Codex baseline unchanged until every live protocol probe is pass or accepted warning. Then update CompatTestedCodexVersion to the installed version.",
    );
  }
  if (claudeVersion?.status === "warn") {
    steps.push("Claude is only patch drift, so it usually needs no adapter fix unless the Claude live probe fails.");
  }
  if (steps.length === 0 && result.failed === 0) {
    steps.push(
      mode === "deep"
        ? "Deep checks found no blocking protocol failure. You can keep using the current adapter."
        : "Fast checks found no blocking failure. Deep checks are only needed after provider upgrades or suspicious behavior.",
    );
  }
  return steps;
}

export function CheckVersionSettings(): React.ReactElement {
  const [config, setConfig] = useState<CompatConfig>({
    testedClaudeVersion: "",
    testedCodexVersion: "",
    testedGrokVersion: "",
    testedOpencodeVersion: "",
  });
  const [deepRun, setDeepRun] = useState(false);
  const [info, setInfo] = useState<CompatVersionInfo | null>(null);
  const [result, setResult] = useState<CompatCheckResult | null>(null);
  const [resultMode, setResultMode] = useState<CheckMode | null>(null);
  const [running, setRunning] = useState(false);
  const [savingConfig, setSavingConfig] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    void (async () => {
      try {
        const [nextConfig, nextInfo] = await Promise.all([
          loadCompatConfigUseCase.execute(),
          loadCompatInfoUseCase.execute(),
        ]);
        setConfig(nextConfig);
        setInfo(nextInfo);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Could not reach runner.");
      }
    })();
  }, []);

  const runChecks = async () => {
    setRunning(true);
    setError(null);
    setMessage(null);
    setResult(null);
    setResultMode(null);
    try {
      if (deepRun) {
        setResult(await runCompatDeepCheckUseCase.execute());
        setResultMode("deep");
      } else {
        setResult(await runCompatCheckUseCase.execute());
        setResultMode("fast");
      }
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : deepRun
            ? "Deep check failed."
            : "Check failed.",
      );
    } finally {
      setRunning(false);
    }
  };

  const saveConfig = async () => {
    setSavingConfig(true);
    setError(null);
    setMessage(null);
    try {
      const saved = await saveCompatConfigUseCase.execute(config);
      setConfig(saved);
      setInfo(await loadCompatInfoUseCase.execute());
      setMessage("Latest tested versions saved to .flowpilot/settings/compat-config.json.");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not save tested versions.");
    } finally {
      setSavingConfig(false);
    }
  };

  const summary = result
    ? result.failed > 0
      ? `${result.failed} failed, ${result.warned} warning${result.warned === 1 ? "" : "s"}, ${result.passed} passed`
      : result.warned > 0
        ? `${result.warned} warning${result.warned === 1 ? "" : "s"}, ${result.passed} passed`
        : `All ${result.passed} checks passed`
    : null;

  const summaryTone = result
    ? result.failed > 0
      ? "fail"
      : result.warned > 0
        ? "warn"
        : "pass"
    : null;

  const guidance = result
    ? result.failed > 0
      ? "At least one major/minor version changed. Run deep checks; if only the baseline drift fails, update the tested version baseline, and if protocol checks fail, fix the runner adapter."
      : result.warned > 0
        ? "Only patch drift was detected. Deep checks are optional, but this usually does not require a tool fix."
        : "The installed tools still match the validated baseline."
    : null;
  const deepProbeItems = result?.items.filter((item) => item.name.includes("live probe") || item.name === "codex app-server initialize") ?? [];
  const regularItems = result?.items.filter((item) => !deepProbeItems.includes(item)) ?? [];
  const resultLabel = resultMode === "deep" ? "Deep check result" : "Fast check result";
  const nextSteps = result ? buildNextSteps(result, resultMode) : [];

  return (
    <section className="settings-panel">
      <div className="settings-panel-head">
        <div>
          <div className="settings-eyebrow">Compatibility</div>
          <h2>Check Version</h2>
          <p>
            Verify that the installed Claude Code CLI and Codex match the versions this
            app was built and tested against.
          </p>
        </div>
        <div className="check-version-run-controls">
          <label className="check-version-checkbox">
            <input
              checked={deepRun}
              disabled={running}
              onChange={(event) => setDeepRun(event.target.checked)}
              type="checkbox"
            />
            <span>deep run</span>
          </label>
          <button
            className="btn btn-primary"
            disabled={running}
            onClick={() => void runChecks()}
            type="button"
          >
            {running ? (deepRun ? "Running Deep Checks…" : "Running…") : "Run Checks"}
          </button>
        </div>
      </div>

      {error ? <div className="settings-feedback error">{error}</div> : null}
      {message ? <div className="settings-feedback">{message}</div> : null}

      {info ? (
        <div className="settings-validation">
          <VersionRow
            installed={info.installedClaudeVersion}
            label="Claude Code CLI"
            tested={info.testedClaudeVersion}
          />
          <VersionRow
            installed={info.installedCodexVersion}
            label="Codex"
            tested={info.testedCodexVersion}
          />
          <VersionRow
            installed={info.installedGrokVersion}
            label="Grok"
            tested={info.testedGrokVersion}
          />
          <VersionRow
            installed={info.installedOpencodeVersion}
            label="Opencode"
            tested={info.testedOpencodeVersion}
          />
        </div>
      ) : null}

      <div className="settings-subpanel">
        <h3>Latest Tested Versions</h3>
        <div className="settings-grid">
          <label className="settings-field">
            <span>Claude Code CLI</span>
            <input
              disabled={savingConfig}
              onChange={(event) =>
                setConfig((current) => ({ ...current, testedClaudeVersion: event.target.value }))
              }
              value={config.testedClaudeVersion}
            />
          </label>
          <label className="settings-field">
            <span>Codex</span>
            <input
              disabled={savingConfig}
              onChange={(event) =>
                setConfig((current) => ({ ...current, testedCodexVersion: event.target.value }))
              }
              value={config.testedCodexVersion}
            />
          </label>
          <label className="settings-field">
            <span>Grok</span>
            <input
              disabled={savingConfig}
              onChange={(event) =>
                setConfig((current) => ({ ...current, testedGrokVersion: event.target.value }))
              }
              value={config.testedGrokVersion}
            />
          </label>
          <label className="settings-field">
            <span>Opencode</span>
            <input
              disabled={savingConfig}
              onChange={(event) =>
                setConfig((current) => ({ ...current, testedOpencodeVersion: event.target.value }))
              }
              value={config.testedOpencodeVersion}
            />
          </label>
        </div>
        <div className="settings-actions">
          <button
            className="primary-btn"
            disabled={savingConfig || running}
            onClick={() => void saveConfig()}
            type="button"
          >
            {savingConfig ? "Saving..." : "Save Tested Versions"}
          </button>
        </div>
      </div>

      {result ? (
        <>
          <div className={`check-version-summary ${summaryTone ?? ""}`} aria-live="polite">
            {resultLabel}: {summary}
          </div>
          {resultMode === "deep" ? (
            <div className="settings-feedback">
              Deep checks completed. The live Claude stream-json and Codex app-server probes are shown first.
            </div>
          ) : null}
          <div className={`settings-feedback ${summaryTone === "fail" ? "error" : ""}`}>
            {guidance}
          </div>
          {nextSteps.length > 0 ? (
            <div className="check-version-next-steps">
              <div className="check-version-next-title">Next steps</div>
              {nextSteps.map((step) => (
                <div className="check-version-next-step" key={step}>
                  {step}
                </div>
              ))}
            </div>
          ) : null}
          <div className="settings-validation check-version-items">
            {[...deepProbeItems, ...regularItems].map((item) => (
              <ItemRow item={item} key={item.name} />
            ))}
          </div>
        </>
      ) : null}
    </section>
  );
}
