import { defineConfig } from "vitest/config";
import path from "node:path";

const targetRoots = ["mobile", "desktop"] as const;

export default defineConfig(({ mode }) => {
  if (!targetRoots.includes(mode as (typeof targetRoots)[number])) {
    throw new Error(`Unsupported client target: ${mode}`);
  }

  const targetRoot = path.resolve(import.meta.dirname, mode);

  return {
    root: targetRoot,
    resolve: {
      alias: {
        "@": path.resolve(targetRoot, "src"),
      },
    },
    test: {
      globals: true,
      environment: "jsdom",
    },
  };
});
