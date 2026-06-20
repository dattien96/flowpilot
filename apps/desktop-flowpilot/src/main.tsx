import React from "react";
import ReactDOM from "react-dom/client";
import "@/styles.css";

(
  globalThis as typeof globalThis & {
    __FLOWPILOT_VITE_ENV__?: Record<string, string | undefined>;
  }
).__FLOWPILOT_VITE_ENV__ = (import.meta as ImportMeta & {
  env?: Record<string, string | undefined>;
}).env ?? {};

void import("@/App").then(({ App }) => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
});
