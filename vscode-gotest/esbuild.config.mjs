import * as esbuild from "esbuild";

const watch = process.argv.includes("--watch");

const config = {
  entryPoints: ["src/extension.ts"],
  bundle: true,
  outdir: "dist",
  external: ["vscode"],
  format: "cjs",
  platform: "node",
  target: "node22",
  sourcemap: true,
  minify: !watch,
};

if (watch) {
  const ctx = await esbuild.context(config);
  await ctx.watch();
  console.log("Watching...");
} else {
  await esbuild.build(config);
}
