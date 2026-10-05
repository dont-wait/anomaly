import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";
import process from "node:process";

const targetConfig = {
  mobile: { port: 1420, hmrPort: 1421 },
  desktop: { port: 1422, hmrPort: 1423 },
} as const;

export default defineConfig(({ mode }) => {
  const target = targetConfig[mode as keyof typeof targetConfig];

  if (!target) {
    throw new Error(`Unsupported client target: ${mode}`);
  }

  const targetRoot = path.resolve(import.meta.dirname, mode);
  const host = process.env.TAURI_DEV_HOST;

  return {
    root: targetRoot,
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: {
        "@": path.resolve(targetRoot, "src"),
      },
    },
    clearScreen: false,
    server: {
      port: target.port,
      strictPort: true,
      host: host || false,
      hmr: host
        ? {
            protocol: "ws",
            host,
            port: target.hmrPort,
          }
        : undefined,
      watch: {
        ignored: ["**/src-tauri/**"],
      },
    },
  };
});
