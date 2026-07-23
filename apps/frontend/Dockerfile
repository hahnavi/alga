FROM node:lts-alpine3.23 AS builder
RUN corepack enable && corepack prepare pnpm@12.0.0-alpha.13 --activate

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

FROM nginx:1.31.2-alpine3.23-slim
# Upgrade base packages to pick up security fixes that the pinned nginx image
# lags behind (e.g. CVE-2026-45447: heap UAF in OpenSSL PKCS7_verify(), shipped
# as libcrypto3/libssl3 3.5.6-r0 in this image, fixed in 3.5.7-r0). The nginx
# base image is republished far less often than Alpine's package repo, so the
# Trivy HIGH/CRITICAL gate would otherwise flap on every new openssl advisory.
RUN apk --no-cache upgrade
RUN rm /etc/nginx/conf.d/default.conf
COPY apps/frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY apps/frontend/security-headers.conf /etc/nginx/conf.d/security-headers.conf
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
