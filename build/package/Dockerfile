# syntax=docker/dockerfile:1.7

# ─── Build stage ─────────────────────────────────────────────────────────────
# --platform=$BUILDPLATFORM: builder 永远跑在构建机的原生架构上,靠 GOARCH 交叉
# 编译出目标架构的二进制。多架构构建时如果让 builder 跟着目标平台走,arm64 那份
# 会在 QEMU 里模拟编译,Go 编译是 CPU 密集型,慢一个数量级。
ARG GO_VERSION=1.27.2
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

# Cache deps separately for faster rebuilds
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# 仅复制二进制入口、内部实现和嵌入资源，不包含本机配置、Cookie 或数据库。
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/ ./web/

# TARGETARCH 由 buildx 注入(amd64 / arm64 ...)。经典 builder 不注入,兜底 amd64。
ARG TARGETARCH

# CGO disabled — modernc/sqlite is pure Go.
# -ldflags strip symbols to shave ~5MB off binary.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build \
    -ldflags="-s -w" \
    -trimpath \
    -o /out/gemrelay ./cmd/gemrelay

# ─── Runtime stage ───────────────────────────────────────────────────────────
# distroless/static: ~2MB base, no shell, no package manager — minimal attack surface.
# We pre-create /data with nonroot ownership in the builder so distroless
# (which has no `mkdir`) doesn't choke at startup.
# 也钉在构建机架构:这一层只是 mkdir + chown(数字 uid,跟架构无关),
# 没必要为它拉一份目标架构的 alpine 再进 QEMU。
FROM --platform=$BUILDPLATFORM alpine:3.20 AS prepare
RUN mkdir -p /out/data /out/app && chown -R 65532:65532 /out/data /out/app

FROM gcr.io/distroless/static-debian12:nonroot

# /data is owned by uid 65532 (nonroot) so sqlite can create gemrelay.db.
COPY --from=prepare --chown=65532:65532 /out/data /data
COPY --from=prepare --chown=65532:65532 /out/app /app

WORKDIR /app
COPY --from=builder /out/gemrelay /app/gemrelay

USER nonroot:nonroot

EXPOSE 8083

# 数据库路径由下面的 --db 设置；程序不读取 DB_PATH 环境变量。
ENTRYPOINT ["/app/gemrelay"]
CMD ["--db", "/data/gemrelay.db", "--port", "8083"]
