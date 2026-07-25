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
    plugins: [
      vue(),
      tailwindcss(),
      // index.html deliberately ships without a CSP <meta> tag: in production
      // the policy comes exclusively from the nginx response header so it can
      // be adjusted per deployment (see security-headers.conf.template and
      // the Helm chart). The dev server has no such header, so inject a
      // dev-only meta CSP here. It covers meta-supported CSP directives only
      // (script-src gains 'unsafe-eval' for Vite HMR/source maps); directives
      // not supported via <meta> tags (e.g. frame-ancestors) remain enforced
      // via the dev server's response header instead. Gating on apply: "serve"
      // guarantees the tag never appears in ANY build output, including staging.
      {
        name: "alga-csp-dev",
        apply: "serve",
        transformIndexHtml: {
          order: "pre",
          handler() {
            return [
              {
                tag: "meta",
                injectTo: "head-prepend",
                attrs: {
                  "http-equiv": "Content-Security-Policy",
                  content:
                    "default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com data:; img-src 'self' data: blob:; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self';",
                },
              },
            ];
          },
        },
      },
    ],
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
