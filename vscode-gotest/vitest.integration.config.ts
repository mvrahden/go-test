import { defineConfig } from "vitest/config";

// The integration project deliberately does NOT mock node:child_process. It
// spawns the real CLI, so it needs a Go toolchain and a much longer budget than
// the hermetic unit project.
export default defineConfig({
  test: {
    include: ["test/integration/**/*.test.ts"],
    testTimeout: 180_000,
    hookTimeout: 180_000,
    // Real `go run` invocations compete for the build cache; serialising keeps
    // the first cold compile from timing out its siblings.
    fileParallelism: false,
  },
  resolve: {
    extensions: [".ts", ".js"],
  },
});
