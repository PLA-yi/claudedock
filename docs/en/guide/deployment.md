# Deployment Guide

Docker Compose is the recommended way to deploy. No need to install PostgreSQL, Go, or compile from source.

## 1. Install Docker

### Linux

```bash
curl -fsSL https://get.docker.com | sh
```

Add your user to the `docker` group:

```bash
sudo usermod -aG docker $USER
# Log out and back in for it to take effect
```

### macOS

Install [Docker Desktop](https://www.docker.com/products/docker-desktop/), or via Homebrew:

```bash
brew install --cask docker
```

### Windows

Install [Docker Desktop](https://www.docker.com/products/docker-desktop/) with WSL 2 backend enabled.

## 2. Start

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy

bash deploy/scripts/setup-env.sh
docker compose pull
docker compose up -d
```

`setup-env.sh` interactively generates all passwords and secrets. It supports:

- **Built-in PostgreSQL (recommended)**: auto-created, managed by Docker Compose, zero config
- **External PostgreSQL**: provide your existing database connection details

After startup:

- Admin dashboard: `http://YOUR_HOST:3000`
- API: `http://YOUR_HOST:8080`

Verify:

```bash
curl http://127.0.0.1:8080/healthz
# {"status":"ok"}
```

## Users in Mainland China

### Option 1: Use docker-compose.cn.yml (recommended)

The repo includes `docker-compose.cn.yml`, which pre-configures `ghcr.1ms.run` mirror for compose images and sets the `CONTAINER_REGISTRY` environment variable so that runtime image pulls (`managed-user`, `sing-box` probe) also go through the mirror. Start with the override file:

```bash
docker compose -f docker-compose.yml -f docker-compose.cn.yml pull
docker compose -f docker-compose.yml -f docker-compose.cn.yml up -d
```

For bare-metal systemd deployments, add to `/etc/cloud-cli-proxy/env`:

```bash
CONTAINER_REGISTRY=ghcr.1ms.run
```

Restart the control plane. All `docker pull` operations will use the mirror.

Building from source (fallback):

```bash
docker compose -f docker-compose.yml -f docker-compose.cn.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.cn.yml -f docker-compose.build.yaml up -d --force-recreate
```

### Option 2: Pull through a local proxy

If your machine already has a TUN-mode proxy or global proxy, the default compose file works:

```bash
docker compose pull
docker compose up -d
```

## Environment Variables

After running `setup-env.sh`, manual changes are usually unnecessary. See [Configuration](./configuration) for the full reference.

## Building from Source

Only needed when prebuilt images are unavailable:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

## Bare-metal Deployment

For scenarios that require native systemd deployment:

```bash
sudo bash deploy/scripts/deploy.sh
```

This automates: creating the system user → building binaries and images → generating config → installing systemd units → starting services. See `deploy/scripts/deploy.sh` in the repo for details.
