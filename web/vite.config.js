import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// During development (npm run dev) the page is served by Vite on :5173, and
// these API paths are passed through to the Go server on :8080. In production
// (npm run build) the Go server serves the built files itself.
const api = "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: { "/users": api, "/rooms": api, "/admin": api },
  },
});
