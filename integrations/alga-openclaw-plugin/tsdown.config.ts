export default {
  entry: ["src/index.ts", "setup-entry.ts"],
  format: "esm",
  outDir: "dist",
  clean: true,
  external: ["openclaw", "openclaw/*"],
  shims: true,
};
