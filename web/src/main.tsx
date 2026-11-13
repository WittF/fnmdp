import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import { App } from "./App";
import { $theme, applyTheme } from "./lib/theme";

// Mirror the persisted theme onto <html> (the inline script in index.html already
// set it before first paint; this keeps things consistent if storage changed).
applyTheme($theme.get());

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
