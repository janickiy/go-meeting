import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { buildMetadataPlugin, readBuildMetadata } from "./build-metadata.ts";

const buildMetadata = readBuildMetadata(process.env);
const telemetryFlag = process.env.VITE_CLIENT_TELEMETRY_ENABLED || "false";
if (!["true", "false"].includes(telemetryFlag))
  throw new Error("VITE_CLIENT_TELEMETRY_ENABLED must be true or false");

export default defineConfig({
  plugins: [react(), buildMetadataPlugin(buildMetadata)],
  define: {
    __APP_BUILD__: JSON.stringify(buildMetadata),
    __CLIENT_TELEMETRY_ENABLED__: telemetryFlag === "true",
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": {
        target: process.env.API_PROXY_TARGET || "http://127.0.0.1:8085",
        changeOrigin: false,
        ws: true,
      },
    },
  },
  build: { target: ["chrome120", "firefox120", "safari17"], sourcemap: false },
});
