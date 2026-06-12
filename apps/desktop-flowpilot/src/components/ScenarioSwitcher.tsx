import { useStore } from "@/state/store";
import { SCENARIO_NAMES } from "@/client/mockData";

// Dev-only scenario switcher (04-01 Part A). Selecting a scenario sets what the
// MockRunnerClient streams on the next prompt. Not shipped in the real client.
export function ScenarioSwitcher(): React.ReactElement {
  const scenario = useStore((s) => s.scenario);
  const setScenario = useStore((s) => s.setScenario);
  const resetRun = useStore((s) => s.resetRun);

  return (
    <div className="scenario-switcher">
      <span className="scenario-title">DEV · mock scenario</span>
      <div className="scenario-btns">
        {SCENARIO_NAMES.map((name) => (
          <button
            key={name}
            className={`scenario-btn ${scenario === name ? "active" : ""}`}
            onClick={() => {
              setScenario(name);
              resetRun();
            }}
          >
            {name}
          </button>
        ))}
      </div>
    </div>
  );
}
