import { useStore } from "@/state/store";
import { SCENARIO_NAMES } from "@/client/mockData";
import { isMockMode } from "@/client/createRunnerClient";

// Dev-only scenario switcher (04-01 Part A). Selecting a scenario sets what the
// MockRunnerClient streams on the next prompt. Only shown in mock mode — against a
// real runner the scenario is moot (it streams real provider events).
export function ScenarioSwitcher(): React.ReactElement | null {
  const scenario = useStore((s) => s.scenario);
  const setScenario = useStore((s) => s.setScenario);
  const resetRun = useStore((s) => s.resetRun);

  if (!isMockMode()) return null;

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
