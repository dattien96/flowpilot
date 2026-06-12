import { useEffect } from "react";
import { Navigator } from "@/components/Navigator";
import { ChatInput } from "@/components/ChatInput";
import { Timeline } from "@/components/Timeline";
import { RunStatus } from "@/components/RunStatus";
import { ScenarioSwitcher } from "@/components/ScenarioSwitcher";
import { SystemControls } from "@/components/SystemControls";
import { runnerModeLabel } from "@/client/createRunnerClient";

export function App(): React.ReactElement {
  const modeLabel = runnerModeLabel();

  useEffect(() => {
    document.title = `FlowPilot Desktop (${modeLabel})`;
  }, [modeLabel]);

  return (
    <div className="app">
      <header className="app-header">
        <div className="brand">
          FlowPilot <span className="brand-sub">desktop · {modeLabel}</span>
        </div>
        <RunStatus />
      </header>

      <div className="app-body">
        <aside className="sidebar">
          <Navigator />
          <div className="sidebar-bottom">
            <SystemControls />
            <ScenarioSwitcher />
          </div>
        </aside>

        <main className="main">
          <Timeline />
          <ChatInput />
        </main>
      </div>
    </div>
  );
}
