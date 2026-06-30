# syntax=docker/dockerfile:1
# twiceshy server image (ADR-0001 §9: one Go service in Docker).
# Pure-Go / CGO-free (ADR-0009) → a static binary on a distroless nonroot base.

FROM golang:1.25.11-bookworm@sha256:b96f24a8d7d010ea0acb9c3ba99064740f02b6b984612b28bd3c9c5ab9453e38 AS build
WORKDIR /src
# Cache modules first.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO-free static build (matches the release; keeps the image distroless-safe).
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/twiceshy ./cmd/twiceshy

# Distroless nonroot: no shell, no package manager, runs as uid 65532 (#0013
# container hardening — non-root, minimal surface).
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
WORKDIR /app
COPY --from=build /out/twiceshy /usr/local/bin/twiceshy
# The corpus (experience/) and the derived index live on a mounted volume;
# /data is writable by the nonroot user via the volume mount.
ENV TWICESHY_DB=/data/twiceshy.db
EXPOSE 8722
# Liveness probe: the binary probes its own /healthz (distroless has no curl), so a
# crash-loop / failed-to-come-up serve goes unhealthy instead of looking "running".
# start-period covers the index rebuild on a large corpus.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
	CMD ["/usr/local/bin/twiceshy", "healthcheck"]
# TWICESHY_TOKEN must be supplied at runtime (no unauthenticated mode).
# serve rebuilds the index from the mounted corpus on start, then serves MCP.
ENTRYPOINT ["twiceshy"]
CMD ["serve", "-corpus", "/data/corpus", "-db", "/data/twiceshy.db", "-addr", ":8722"]
