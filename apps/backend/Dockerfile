# syntax=docker/dockerfile:1
#
# Alga backend image. Build from the repository root so the context can reach
# both apps/backend and the optional Mattermost plugin bundle:
#
#   docker build -t alga-backend -f apps/backend/Dockerfile .
#
# The Mattermost plugin is NOT bundled by default — the default target omits
# the COPY entirely, so the build succeeds even though the gitignored tarball
# is absent from the context. Deployments that need /internal/mm-plugin to
# serve the tarball build the opt-in stage instead (after running `make bundle`
# in integrations/alga-mattermost-plugin):
#
#   docker build -t alga-backend -f apps/backend/Dockerfile \
#     --target with-mm-plugin .

FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY apps/backend/go.mod apps/backend/go.sum ./
RUN go mod download

COPY apps/backend/ .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o alga .

# runtime-base: the shared final image. CI and local `docker build` (no
# --target) stop here by default — the plugin COPY is in a later stage only.
FROM alpine:3.24 AS runtime-base
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /app/alga .

RUN adduser -D -u 1000 appuser
USER appuser

EXPOSE 8080
ENTRYPOINT ["./alga"]

# Optional Mattermost plugin stage. Build with --target=with-mm-plugin to bake
# the tarball into /app/plugins/ so /internal/mm-plugin can serve it. The COPY
# requires the tarball to exist in the context (run `make bundle` in
# integrations/alga-mattermost-plugin first); the default build never reaches
# this stage, so the gitignored tarball's absence does not break CI.
FROM runtime-base AS with-mm-plugin
USER root
RUN mkdir -p /app/plugins
COPY integrations/alga-mattermost-plugin/dist/com.alga.mattermost-plugin-0.0.1.tar.gz /app/plugins/
USER appuser

# Default target. Docker builds the LAST stage when --target is omitted, so
# this trailing alias must follow with-mm-plugin — otherwise a bare `docker
# build` would try to build with-mm-plugin and fail on the missing tarball.
FROM runtime-base
