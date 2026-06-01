# Multi-arch build → tiny static image. Works for Raspberry Pi (arm64 / armv7)
# and amd64. No Node, no CGO: modernc.org/sqlite is pure Go, templates + static
# assets + migrations are embedded, so the result is a single self-contained binary.
#
# Build natively on the Pi:
#   docker build -t finance-tracker .
# Or cross-build from an amd64 machine with buildx:
#   docker buildx build --platform linux/arm64 -t finance-tracker --load .

# Cross-compile on the (fast) build platform, emit for the target platform.
FROM --platform=$BUILDPLATFORM golang:1.26 AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# TARGET* are provided by buildx; default to the build arch for plain `docker build`.
ARG TARGETOS=linux
ARG TARGETARCH
ARG TARGETVARIANT
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags="-s -w" -o /finance-tracker ./cmd/server

# Minimal runtime: distroless static is multi-arch (arm64, arm/v7, amd64).
FROM gcr.io/distroless/static-debian12
COPY --from=build /finance-tracker /finance-tracker

ENV PORT=8080 \
    DB_PATH=/data/finance.db \
    SEED_DEMO=false
VOLUME ["/data"]
EXPOSE 8080
# distroless has no shell/curl, so the binary probes itself via `-health`.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/finance-tracker", "-health"]
ENTRYPOINT ["/finance-tracker"]
