# cloud-claude local — Local Dev Containers

`cloud-claude local` allows users to launch a locally-managed container that is isomorphic to the cloud-hosted ones, for local development, debugging, and offline scenarios.

## Quick Start

### 1. Initialize Config

```bash
cloud-claude local init
```

Generates `~/.cloud-claude/local.yaml` with default local container parameters.

### 2. Start Local Container

```bash
cloud-claude local up
```

This command:
- Pulls the managed image (if not present locally)
- Creates a `--network=none` container (same as cloud)
- Injects sing-box outbound config (supports tun/proxy dual mode)
- Skips KasmVNC and heartbeat, keeps only sshd + sing-box
- Auto-mounts local SSH keys

### 3. Check Status

```bash
cloud-claude local status
```

Shows local container status, SSH port mapping, and sing-box tunnel state.

### 4. Stop Container

```bash
cloud-claude local down
```

Stops and optionally removes the local container.

## Egress IP Configuration

Use `--egress-config` to inject sing-box outbound JSON:

```bash
cloud-claude local up --egress-config '{
  "type": "shadowsocks",
  "server": "198.51.100.5",
  "server_port": 8388,
  "method": "aes-256-gcm",
  "password": "your-password"
}'
```

Supported protocols: Shadowsocks, VMess, SOCKS5, Trojan, HTTP.

## VS Code Dev Containers Integration

The project root provides a `.devcontainer/devcontainer.json` template for VS Code Remote-Containers:

```json
{
  "name": "cloud-claude-local",
  "image": "ghcr.io/your-org/managed-user:v3.4.0",
  "runArgs": ["--network=none"],
  "postCreateCommand": "sing-box run -c /etc/sing-box/outbound.json"
}
```

## Differences from Cloud Containers

| Feature | Cloud Container | Local Container |
|---------|-----------------|-----------------|
| Network isolation | `--network=none` + sing-box tun | `--network=none` + sing-box tun/proxy |
| Desktop env | KasmVNC + Fluxbox + Chromium | None (terminal-only) |
| Heartbeat | Yes | No |
| Expiry governance | Admin-controlled | User-managed |
| Persistent volume | Docker named volume | Docker named volume |

## Typical Use Cases

- **Offline development**: Work locally when network is unavailable
- **Local debugging**: Reproduce cloud environment issues locally
- **Quick experiments**: Test configurations without waiting for cloud container startup
- **CI/CD baseline**: Validate container image behavior locally before deployment

## Command Reference

```
cloud-claude local up [flags]
  --egress-config string   sing-box outbound JSON
  --image string           Custom image (defaults to managed image)
  --name string            Container name
  --rm                     Auto-remove container after stop

cloud-claude local down [flags]
  --name string            Target container name
  --volumes                Also remove associated volumes

cloud-claude local status
  Show all local containers and their status
```
