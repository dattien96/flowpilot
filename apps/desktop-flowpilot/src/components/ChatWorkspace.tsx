import { Navigator } from "@/components/Navigator";
import { ChatInput } from "@/components/ChatInput";
import { Timeline } from "@/components/Timeline";
import { ScenarioSwitcher } from "@/components/ScenarioSwitcher";
import { SystemControls } from "@/components/SystemControls";

export function ChatWorkspace(): React.ReactElement {
  return (
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
  );
}
