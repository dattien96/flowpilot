import React from "react";
import ReactDOM from "react-dom/client";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import "@/styles.css";

(
  globalThis as typeof globalThis & {
    __FLOWPILOT_VITE_ENV__?: Record<string, string | undefined>;
  }
).__FLOWPILOT_VITE_ENV__ = (import.meta as ImportMeta & {
  env?: Record<string, string | undefined>;
}).env ?? {};

// Platform hook for CSS (e.g. extra left padding for macOS traffic lights when
// running frameless). Absent in a plain browser tab — no-op there.
document.body.dataset.platform = window.flowpilot?.platform ?? "web";

void import("@/App").then(({ App }) => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
    </React.StrictMode>,
  );
});
