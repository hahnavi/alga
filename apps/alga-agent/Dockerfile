# syntax=docker/dockerfile:1
#
# Multi-stage build for Alga Agent. Build from the repository root so the
# Docker build context can access both apps/alga-agent and the local SDK:
#
#   docker build -t alga-agent -f apps/alga-agent/Dockerfile .
#
# Final image is ~25MB.

FROM golang:1.26-alpine AS builder

WORKDIR /src

# Copy the Go workspace manifests and the local SDK first for layer caching.
COPY go.work go.work.sum* ./
COPY apps/alga-agent/go.mod apps/alga-agent/go.sum apps/alga-agent/
COPY integrations/alga-agent-sdk-go/ integrations/alga-agent-sdk-go/

WORKDIR /src/apps/alga-agent
RUN go mod download

# Copy the rest of the agent source.
WORKDIR /src
COPY apps/alga-agent/ apps/alga-agent/

# Build a static binary. The version is injected via -ldflags.
WORKDIR /src/apps/alga-agent
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/alga-agent .

# --- Runtime stage ---
FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1000 appuser

WORKDIR /app
COPY --from=builder /out/alga-agent /app/alga-agent
COPY apps/alga-agent/config.yaml.example /app/config.yaml.example

USER appuser
EXPOSE 8443 9101
ENTRYPOINT ["/app/alga-agent"]
