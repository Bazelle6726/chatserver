import { StrictMode, Suspense } from "react";
import { createRoot } from "react-dom/client";
import App from "./App.jsx";
import "./styles.css";

createRoot(document.getElementById("root")).render(
  <StrictMode>
    {/* App waits for "who am I?" before its first render. */}
    <Suspense fallback={null}>
      <App />
    </Suspense>
  </StrictMode>,
);
