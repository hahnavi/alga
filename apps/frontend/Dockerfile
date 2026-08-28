FROM node:24-alpine3.23 AS builder
RUN corepack enable && corepack prepare pnpm@11.17.0 --activate

WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/frontend/package.json apps/frontend/package.json
# --ignore-scripts: the root package.json `prepare` script runs
# `git config core.hooksPath .githooks`, but git is absent from this builder
# image and git-hook setup is irrelevant in a container build. Skipping scripts
# also avoids running third-party postinstall scripts during the reproducible
# build; the frontend build (`pnpm build`) runs explicitly below, so nothing
# it depends on is skipped.
RUN pnpm install --frozen-lockfile --filter frontend... --ignore-scripts

COPY apps/frontend/ apps/frontend/
WORKDIR /app/apps/frontend
RUN pnpm build

FROM nginx:1.31.4-alpine3.24-slim
# Upgrade base packages to pick up security fixes that the pinned nginx image
# lags behind (e.g. CVE-2026-14456: DoS via unbounded memory growth in the
# OpenSSL QUIC server, shipped as libcrypto3/libssl3 3.5.7-r0 in the older
# alpine3.23 base, fixed in 3.5.8-r0). The nginx base image is republished far
# less often than Alpine's package repo, so the Trivy HIGH/CRITICAL gate would
# otherwise flap on every new openssl advisory.
RUN apk --no-cache upgrade
RUN rm /etc/nginx/conf.d/default.conf
COPY apps/frontend/nginx.conf /etc/nginx/conf.d/default.conf
# hsts-map.conf defines the http-scope `map` ($strict_transport_security) and
# must sit in conf.d/ so the stock nginx.conf auto-includes it at http scope.
COPY apps/frontend/hsts-map.conf /etc/nginx/conf.d/hsts-map.conf
# security-headers.conf is a location-scoped snippet, NOT a top-level config:
# keep it OUT of conf.d/ (which is auto-globbed at http scope) so its
# add_header directives apply only where explicitly included and do not leak
# SPA CSP/HSTS into the /api/, /api/v1/events and /webhooks/ proxy locations.
#
# It ships as an envsubst template: the stock nginx entrypoint renders
# /etc/nginx/templates/*.template into NGINX_ENVSUBST_OUTPUT_DIR at startup,
# substituting only defined environment variables. This lets deployments
# extend the CSP (e.g. allow a CDN-injected analytics script) via env vars
# without rebuilding the image, while the defaults below keep the policy
# strict. Space-separated CSP source expressions, empty means "no additions":
ENV ALGA_CSP_SCRIPT_SRC_EXTRA=""
ENV ALGA_CSP_CONNECT_SRC_EXTRA=""
ENV NGINX_ENVSUBST_FILTER='^(ALGA_CSP_SCRIPT_SRC_EXTRA|ALGA_CSP_CONNECT_SRC_EXTRA)$'
ENV NGINX_ENVSUBST_OUTPUT_DIR=/etc/nginx/snippets
COPY apps/frontend/security-headers.conf.template /etc/nginx/templates/security-headers.conf.template
RUN mkdir -p /etc/nginx/snippets && chown nginx:nginx /etc/nginx/snippets
COPY --from=builder /app/apps/frontend/dist /usr/share/nginx/html
RUN chown -R nginx:nginx /usr/share/nginx/html \
    && mkdir -p /var/cache/nginx \
    && chown -R nginx:nginx /var/cache/nginx \
    && mkdir -p /var/run \
    && chown -R nginx:nginx /var/run \
    && touch /var/run/nginx.pid \
    && chown nginx:nginx /var/run/nginx.pid
EXPOSE 80
USER nginx
CMD ["nginx", "-g", "daemon off;"]
