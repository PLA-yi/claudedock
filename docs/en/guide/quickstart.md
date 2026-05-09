# Quick Start

## Docker Compose Deployment (Recommended)

### Prerequisites

- Linux host (Ubuntu 22.04+ / Debian 12+)
- Docker Engine 28+, Docker Compose v2
- At least one egress IP (proxy server)

### UI Preview

> The screenshots below are from repository `imgs/`, so you can see the admin and user experience before deploying.

#### Dashboard Overview

![Dashboard Overview](/imgs/1.png)

#### Host Management List

![Host Management List](/imgs/2.png)

#### Host Detail and Access Entry

![Host Detail and Access Entry](/imgs/4.png)

#### Lifecycle and Network Operations

![Lifecycle and Network Operations](/imgs/5.png)

#### Browser Remote Desktop (KasmVNC)

![Browser Remote Desktop](/imgs/3.png)

### Step 1: Clone

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy
```

### Step 2: Generate Environment Config

Run the setup script to auto-generate all passwords and secrets:

```bash
bash deploy/scripts/setup-env.sh
```

Choose a database mode:

- **Built-in Docker PostgreSQL (recommended)**: auto-generates DB password, managed by Docker Compose, zero config.
- **External PostgreSQL**: interactively enter your DB host, port, credentials, with SSL support.

Both options auto-generate an admin password (20 chars) and JWT secret (48 chars).

::: warning Important
The script displays the admin password once. Save it immediately!
:::

### Step 3: Start Services

Default recommendation: **prefer prebuilt images** (`latest`) for faster and consistent CI-aligned deployment.

```bash
# Built-in Docker PostgreSQL
docker compose pull
docker compose up -d

# External PostgreSQL (skip built-in DB)
docker compose pull control-plane admin
docker compose up -d control-plane admin
```

Optional local source build:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yaml --profile build-only build --no-cache
docker compose -f docker-compose.yml -f docker-compose.build.yaml up -d --force-recreate
```

### Step 4: Verify

```bash
curl http://127.0.0.1:8080/healthz
# {"status":"ok","checks":{"database":"ok","agent":"ok"}}
```

Service endpoints:
- **API**: `http://YOUR_HOST:8080`
- **Admin dashboard**: `http://YOUR_HOST:3000`
- **SSH proxy**: `YOUR_HOST:2222`

## Provisioning Users

Five steps: **login → add egress IP → create user → create host & bind → send connection command**.

### 1. Get Admin Token

Log in via the admin dashboard, or use the API:

```bash
TOKEN=$(curl -s -X POST http://YOUR_HOST:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-admin-password"}' | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
```

### 2. Add Egress IP

Egress IPs use sing-box tun full-tunnel with `tunnel_type` set to `proxy`; configure the upstream in `proxy_config` (sing-box outbound).

Supports 6 protocols — SOCKS5, VMess, VLESS, Shadowsocks, Trojan, HTTP.

```bash
# Shadowsocks example
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "jp-ss-01",
    "ip_address": "198.51.100.5",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "shadowsocks",
      "server": "198.51.100.5",
      "server_port": 8388,
      "method": "aes-256-gcm",
      "password": "your-ss-password"
    }
  }'
```

```bash
# SOCKS5 example
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "label": "us-socks-01",
    "ip_address": "192.0.2.50",
    "tunnel_type": "proxy",
    "provider": "manual",
    "proxy_config": {
      "type": "socks",
      "server": "192.0.2.50",
      "server_port": 1080,
      "username": "user",
      "password": "pass"
    }
  }'
```

**Test egress IP connectivity:**

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/egress-ips/{ipID}/test \
  -H "Authorization: Bearer $TOKEN"
```

Tests connectivity, exit IP match, and DNS leak detection.

### 3. Create User

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "zhangsan",
    "password": "initial-password-for-user",
    "expires_at": "2026-12-31T23:59:59Z"
  }'
```

### 4. Create Host & Bind Egress IP

**Create host:**

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/hosts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "user-uuid"}'
```

**Bind egress IP:**

```bash
curl -s -X POST http://YOUR_HOST:8080/v1/admin/bindings \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"host_id": "host-uuid", "egress_ip_id": "egress-ip-uuid"}'
```

::: tip
A host requires at least one bound egress IP to start.
:::

### 5. Send to User

After the host is created, an egress IP is bound, and the task shows the container is **ready**, copy access info from the host detail page in the admin UI.

**Option A: One-liner SSH (classic)**

Send the user (replace `YOUR_HOST` and `SHORT_ID` with the public gateway and the **host** short ID):

```bash
curl -sSf http://YOUR_HOST/entry/SHORT_ID | bash
```

Or the bootstrap flow (asks for username):

```bash
curl -sSf http://YOUR_HOST:8080/v1/bootstrap/script | bash
```

**Option B: cloud-claude (recommended)**

In addition to the `curl` command, share these three values (as shown in the admin UI):

| Field | Meaning |
|-------|---------|
| **Gateway URL** | Public HTTPS base URL of the control plane, e.g. `https://gw.example.com` (same origin as the admin UI in the browser; usually **not** the `:3000` dev frontend port) |
| **Short ID** | **Host** short ID from the host detail page. If the user configures a **user** short ID instead, they connect to that user’s primary host |
| **Password** | The user’s password from the admin dashboard |

After installing `cloud-claude` and running `init` once with those values, the user runs `cloud-claude` from their **project directory**; the cwd is sshfs-mounted at the **same path** in the container. By default `git` runs on the laptop (tune with `proxy_commands`).

## User Access

> Share this section directly with your users.

### Option 1: cloud-claude Local CLI (Recommended)

#### Install

**Homebrew (macOS / Linux, recommended):**

```bash
brew tap ZaneL1u/tap
brew install cloud-claude
```

**One-liner (any platform):**

```bash
curl -fsSL https://raw.githubusercontent.com/ZaneL1u/cloud-cli-proxy/main/scripts/install.sh | bash
```

Or download the matching archive from [Releases](https://github.com/ZaneL1u/cloud-cli-proxy/releases), or `go build ./cmd/cloud-claude`.

#### First-time setup (once)

```bash
cloud-claude init
```

Prompts: **Gateway URL**, **Short ID** (host or user), **Password** → `~/.cloud-claude/config.yaml`.

Flags or environment variables:

```bash
cloud-claude init --gateway https://gw.example.com --short-id abc123 --password your-password

export CLOUD_CLAUDE_GATEWAY=https://gw.example.com
export CLOUD_CLAUDE_SHORT_ID=abc123
export CLOUD_CLAUDE_PASSWORD=your-password
cloud-claude init
```

#### Connect and run Claude Code

```bash
cd ~/your/project   # directory to mount into the container

alias claude=cloud-claude   # optional

cloud-claude
cloud-claude -p "refactor this function"
```

**Session management:** By default attaches the existing tmux session for the same account; disconnects do not lose the workspace:

```bash
cloud-claude                  # default: attach existing session (multi-client)
cloud-claude --new-session    # force a new isolated session
cloud-claude --take-over      # take over the primary session and detach others

cloud-claude sessions                  # list current tmux sessions
cloud-claude sessions --attach 0       # attach a specific session
```

**Mount modes:** Auto mode picks the best strategy; you can also specify manually:

```bash
cloud-claude --mount-mode=auto         # default: HotSync preferred, falls back to SSHFS
cloud-claude --mount-mode=full         # HotSync + SSHFS dual-track (full features)
cloud-claude --mount-mode=sshfs-only   # SSHFS only (compatibility first)
```

**Self-checks and troubleshooting:**

```bash
cloud-claude doctor                    # full five-domain check (network / auth / ssh / mount / disk)
cloud-claude doctor mount --fix        # mount-only check with auto-repair
cloud-claude explain MOUNT_SSHFS_DISCONNECTED   # query error code details and remediation
cloud-claude env check                 # verify remote timezone, locale, egress IP, FUSE, etc.
```

**Environment variables:**

- `CLOUD_CLAUDE_NO_PROMOTION=1` — disable cold-file read promotion (enabled by default on Linux; skipped on macOS)
- Set `proxy_commands` in `~/.cloud-claude/config.yaml` (list of command names to run on the host). Default is `git` only; use an empty list to disable.
- `hot_sync_max_file_mb` — per-file throttling threshold (default 50MB); files larger than this fall back to the cold path.

When you run `cloud-claude`, it: (1) authenticates; (2) waits for the container; (3) sshfs-mounts the cwd at the **same path** in the container; (4) starts Claude Code remotely. Terminal size, signals, and exit codes are forwarded; network jitters auto-recover within 30s with buffered input surviving reconnections.

**Error codes:**

| Exit Code | Meaning | Action |
|-----------|---------|--------|
| 1 | Auth failed | Check Short ID and password |
| 2 | Network error | Check gateway URL is reachable |
| 3 | Timeout | Container startup timeout, contact admin |
| 4 | Config error | Run `cloud-claude init` to reconfigure |

### Option 2: curl + SSH Access

Run the command your admin provided:

```bash
curl -sSf http://YOUR_HOST/entry/abc123 | bash
```

Enter your password and you'll be in your cloud host within seconds.

### Pre-installed Tools

| Tool | Description |
|------|-------------|
| **Claude Code** | AI coding assistant — just run `claude` in terminal |
| **KasmVNC + Chromium** | Browser remote desktop, accessible via the admin dashboard |
| **Git** | Version control |
| **tmux** | Terminal multiplexer, sessions survive disconnects |
| **zsh** | Enhanced shell experience |
| **Node.js** | JavaScript runtime |

### Using Claude Code (via SSH)

Once inside your cloud host, just run:

```bash
claude
```

All Claude API requests are automatically routed through the admin-designated exit IP. No proxy configuration needed.

### Reconnecting

If your SSH connection drops, re-run the same `curl` command to reconnect. Your container keeps running.

### Rebuilding

If you need to reset your environment, click "Rebuild" in the admin dashboard. This recreates the container but preserves your home directory data.

## Local Source Development (From Clone)

If you want to contribute or customize behavior, use this local development flow.

### 1. Install Dependencies

- Git
- Go `1.25.7+`
- Node.js `20+` (recommended with `corepack` enabled)
- pnpm `10+`
- Docker Engine + Docker Compose v2
- GNU Make

### 2. Clone Repository

```bash
git clone https://github.com/ZaneL1u/cloud-cli-proxy.git
cd cloud-cli-proxy
```

### 3. Initialize Dependencies and Environment

```bash
make setup
```

This installs frontend dependencies and auto-creates `.env` from `.env.example` when missing.

### 4. Start Local Database

```bash
make db
```

The default local PostgreSQL endpoint is `127.0.0.1:5433`.

### 5. Start Dev Mode

```bash
make dev
```

After startup:

- Admin frontend: `http://localhost:2568`
- Control Plane API: `http://127.0.0.1:8090`

### 6. Verify and Run Tests

```bash
curl http://127.0.0.1:8090/healthz
make test
```

### Common Commands

```bash
make dev-api   # backend only
make dev-web   # frontend only
make db-stop   # stop local database
make db-reset  # recreate local database
make help      # list all commands
```

## Next Steps

- [Deployment Guide](./deployment) — systemd native deployment
- [Configuration](./configuration) — Environment variables and networking
- [API Reference](../reference/api) — Full Admin API docs
- [FAQ & Recovery](../reference/faq) — Troubleshooting and disaster recovery
