import { defineConfig, loadEnv } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";
import { fileURLToPath } from "node:url";

const dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, dirname, "");
  // VITE_PROXY_TARGET lets devs point the proxy at a remote backend
  // (Docker, staging, etc.) without editing this file.
  const proxyTarget = env.VITE_PROXY_TARGET || "http://localhost:8080";
  const proxyOpts = {
    target: proxyTarget,
    changeOrigin: true,
  } as const;

  return {
    plugins: [vue(), tailwindcss()],
    resolve: {
      alias: {
        "@": path.resolve(dirname, "./src"),
      },
    },
    build: {
      sourcemap: mode === "production" ? "hidden" : true,
      rolldownOptions: {
        output: {
          codeSplitting: {
            groups: [
              {
                name: "markdown-it",
                test: /node_modules[\\/]markdown-it/,
                priority: 30,
              },
              {
                name: "tiptap",
                test: /node_modules[\\/](@tiptap|tiptap-markdown)/,
                priority: 20,
              },
              {
                name: "vendor",
                test: /node_modules/,
                priority: 10,
              },
            ],
          },
        },
      },
    },
    server: {
      host: env.VITE_HOST || "localhost",
      allowedHosts: true,
      port: 5173,
      proxy: {
        "/api/v1/agent/events": { ...proxyOpts, timeout: 0 },
        "/api/v1/events": { ...proxyOpts, timeout: 0 },
        "/api": proxyOpts,
        "/webhooks": proxyOpts,
      },
    },
  };
});
