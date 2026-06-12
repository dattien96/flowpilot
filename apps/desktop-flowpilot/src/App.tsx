import { Navigator } from "@/components/Navigator";
import { ChatInput } from "@/components/ChatInput";
import { Timeline } from "@/components/Timeline";
import { RunStatus } from "@/components/RunStatus";
import { ScenarioSwitcher } from "@/components/ScenarioSwitcher";
import { SystemControls } from "@/components/SystemControls";

export function App(): React.ReactElement {
  return (
    <div className="app">
      <header className="app-header">
        <div className="brand">
          FlowPilot <span className="brand-sub">desktop · mock</span>
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
