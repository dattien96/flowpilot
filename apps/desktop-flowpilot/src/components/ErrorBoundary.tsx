import { Component, type ErrorInfo, type ReactNode } from "react";

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

// BUG-172: the app had no error boundary anywhere, so any uncaught render
// exception (e.g. mid Flow-Mode-run state churn right after an approval
// resumes the turn) unmounted the entire React tree. Only the native
// Electron/OS menu bar survived; the body's dark theme background
// (--bg: #0e1117) then reads as a fully black, unrecoverable screen with no
// way to tell what happened. This boundary turns that into a visible,
// recoverable error card instead.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // eslint-disable-next-line no-console
    console.error("[FlowPilot] Unhandled render error:", error, info.componentStack);
  }

  private handleReload = (): void => {
    window.location.reload();
  };

  render(): ReactNode {
    const { error } = this.state;
    if (!error) return this.props.children;

    return (
      <div className="status-shell">
        <div className="status-card">
          <div className="status-title">Something went wrong</div>
          <div className="status-copy">
            The app hit an unexpected error and could not continue rendering. Reload
            to recover; if this keeps happening, please report it with the details
            below.
          </div>
          <pre className="code-block" style={{ marginTop: "16px", whiteSpace: "pre-wrap" }}>
            <code>{error.message}</code>
          </pre>
          <button className="btn btn-primary" style={{ marginTop: "16px" }} onClick={this.handleReload} type="button">
            Reload
          </button>
        </div>
      </div>
    );
  }
}
