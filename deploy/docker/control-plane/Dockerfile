FROM node:24-alpine AS frontend-builder
WORKDIR /app
COPY web/admin/package.json web/admin/pnpm-lock.yaml ./
RUN npm install -g pnpm && pnpm install --frozen-lockfile
COPY web/admin ./
RUN pnpm build

FROM golang:1.26.1 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY --from=frontend-builder /app/dist ./cmd/control-plane/dist

RUN CGO_ENABLED=0 go build -o /usr/local/bin/control-plane ./cmd/control-plane

FROM debian:bookworm-slim

# 安装 Docker 官方新版 docker-ce-cli（旧 docker.io 的 API 1.41 太老，
# 新版 Docker Desktop 的 daemon 最低要求 API 1.44；新 CLI 会自动协商 API 版本，新旧 daemon 都兼容）
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates gnupg \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc \
    && chmod a+r /etc/apt/keyrings/docker.asc \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian bookworm stable" > /etc/apt/sources.list.d/docker.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends docker-ce-cli \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/bin/control-plane /usr/local/bin/control-plane

ARG SINGBOX_VERSION=1.13.3
RUN arch=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/') && \
    curl -fsSL -o /tmp/sb.tgz "https://github.com/SagerNet/sing-box/releases/download/v${SINGBOX_VERSION}/sing-box-${SINGBOX_VERSION}-linux-${arch}.tar.gz" && \
    tar -xzf /tmp/sb.tgz -C /tmp && \
    install -m 0755 "/tmp/sing-box-${SINGBOX_VERSION}-linux-${arch}/sing-box" /usr/local/bin/sing-box && \
    rm -rf /tmp/sb.tgz "/tmp/sing-box-${SINGBOX_VERSION}-linux-${arch}"

WORKDIR /app
COPY internal/store/migrations internal/store/migrations
COPY deploy/docker/managed-user/image.lock deploy/docker/managed-user/image.lock

EXPOSE 8080

CMD ["control-plane"]
