import { defineConfig } from "vitest/config";
import path from "node:path";

const targetRoot = path.resolve(import.meta.dirname, "mobile");

export default defineConfig({
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
});
