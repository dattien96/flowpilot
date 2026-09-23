import React, { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { useStore } from "@/state/store";
import { resolveTerminalCwd } from "@/terminal/cwd";
import { TerminalTabs } from "@/terminal/termTabs";
import { CloseIcon, PlusIcon } from "./icons";

interface TermInstance {
  term: Terminal;
  fit: FitAddon;
}

function readTermTheme(): Record<string, string> {
  const cs = getComputedStyle(document.documentElement);
  const v = (name: string, fallback: string) => cs.getPropertyValue(name).trim() || fallback;
  return {
    background: v("--bg", "#141414"),
    foreground: v("--text", "rgba(255,255,255,0.9)"),
    cursor: v("--accent", "#49b0ff"),
    selectionBackground: v("--accent", "#49b0ff") + "44",
  };
}

/**
 * Task-428: VS Code-style bottom terminal dock. Tab strip + xterm instances
 * wired through `flowpilot.term` (main-process ptys). Cwd is resolved at
 * spawn time and pinned per tab (T-1) — the panel never touches run/timeline
 * state (zero-coupling invariant).
 */
export function TerminalPanel(): React.ReactElement | null {
  const open = useStore((s) => s.terminalOpen);
  const toggleTerminal = useStore((s) => s.toggleTerminal);
  const [, bump] = useReducer((x: number) => x + 1, 0);
  const [spawnError, setSpawnError] = useState<string | null>(null);

  const tabsRef = useRef<TerminalTabs | null>(null);
  const instancesRef = useRef(new Map<string, TermInstance>());
  const bodyRef = useRef<HTMLDivElement>(null);

  const bridge = window.flowpilot?.term;

  const getTabs = useCallback((): TerminalTabs => {
    if (!tabsRef.current) {
      if (!bridge) throw new Error("terminal bridge unavailable");
      tabsRef.current = new TerminalTabs(
        bridge,
        () => resolveTerminalCwd(useStore.getState()),
        bump,
      );
    }
    return tabsRef.current;
  }, [bridge]);

  // Bridge subscriptions — route output by pty id to the owning instance.
  useEffect(() => {
    if (!bridge) return;
    const offData = bridge.onData(({ id, data }) => {
      instancesRef.current.get(id)?.term.write(data);
    });
    const offExit = bridge.onExit(({ id, exitCode }) => {
      tabsRef.current?.markExited(id);
      instancesRef.current
        .get(id)
        ?.term.write(`\r\n\x1b[90m[process exited with code ${exitCode}]\x1b[0m`);
    });
    return () => {
      offData();
      offExit();
    };
  }, [bridge]);

  // First-open: spawn an initial shell.
  useEffect(() => {
    if (!open || !bridge) return;
    if (getTabs().tabs.length === 0) {
      void openTab();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, bridge]);

  const openTab = async (): Promise<void> => {
    setSpawnError(null);
    try {
      if (!bridge) throw new Error("terminal requires the desktop app");
      await getTabs().open(80, 24);
    } catch (err) {
      // T-4: surface the IPC error in the panel — never silently fall back to
      // a different cwd.
      setSpawnError(err instanceof Error ? err.message : String(err));
    }
  };

  const closeTab = async (id: string): Promise<void> => {
    await getTabs().close(id);
    const inst = instancesRef.current.get(id);
    inst?.term.dispose();
    instancesRef.current.delete(id);
  };

  // Mount (or re-mount) a Terminal into its host div; called by ref callback.
  const mountHost = useCallback(
    (id: string, el: HTMLDivElement | null) => {
      if (!el) return;
      let inst = instancesRef.current.get(id);
      if (!inst) {
        const term = new Terminal({
          fontSize: 12,
          fontFamily: "Consolas, 'Courier New', monospace",
          cursorBlink: true,
          theme: readTermTheme(),
        });
        const fit = new FitAddon();
        term.loadAddon(fit);
        term.onData((data) => tabsRef.current?.write(id, data));
        inst = { term, fit };
        instancesRef.current.set(id, inst);
      }
      if (inst.term.element !== el) {
        inst.term.open(el);
      }
      inst.fit.fit();
      tabsRef.current?.resize(id, inst.term.cols, inst.term.rows);
    },
    [],
  );

  // Panel resize → refit active terminal → sync pty size.
  useEffect(() => {
    if (!open) return;
    const body = bodyRef.current;
    if (!body) return;
    const ro = new ResizeObserver(() => {
      const active = tabsRef.current?.active;
      if (!active) return;
      const inst = instancesRef.current.get(active.id);
      if (!inst) return;
      inst.fit.fit();
      tabsRef.current?.resize(active.id, inst.term.cols, inst.term.rows);
    });
    ro.observe(body);
    return () => ro.disconnect();
  }, [open]);

  if (!open) return null;

  const tabs = tabsRef.current?.tabs ?? [];
  const activeId = tabsRef.current?.activeId;

  return (
    <section className="term-panel" aria-label="Terminal">
      <div className="term-tabs" role="tablist" aria-label="Terminal tabs">
        {tabs.map((tab) => (
          <div
            key={tab.id}
            role="tab"
            aria-selected={tab.id === activeId}
            className={`term-tab${tab.id === activeId ? " active" : ""}${tab.exited ? " exited" : ""}`}
            title={tab.cwd}
            onClick={() => getTabs().activate(tab.id)}
          >
            <span className="term-tab-title">{tab.title}</span>
            {tab.exited && <span className="term-tab-exited">exited</span>}
            <button
              type="button"
              className="term-tab-close"
              aria-label={`Close terminal ${tab.title}`}
              onClick={(e) => {
                e.stopPropagation();
                void closeTab(tab.id);
              }}
            >
              <CloseIcon size={11} />
            </button>
          </div>
        ))}
        <button
          type="button"
          className="term-tab-new"
          aria-label="New terminal"
          title="New terminal"
          onClick={() => void openTab()}
        >
          <PlusIcon size={13} />
        </button>
        <span className="term-tabs-spacer" />
        <button
          type="button"
          className="term-tab-new"
          aria-label="Hide terminal panel"
          title="Hide terminal"
          onClick={() => toggleTerminal()}
        >
          <CloseIcon size={13} />
        </button>
      </div>
      <div className="term-body" ref={bodyRef}>
        {spawnError && (
          <div className="term-error" role="alert">
            Terminal failed to start: {spawnError}
          </div>
        )}
        {tabs.map((tab) => (
          <div
            key={tab.id}
            className={`term-host${tab.id === activeId ? "" : " hidden"}`}
            ref={(el) => mountHost(tab.id, el)}
          />
        ))}
      </div>
    </section>
  );
}
