# claudedock local — Local Dev Containers

`claudedock local` launches a container in local Docker that is isomorphic to the cloud-hosted ones, for development, debugging, and offline use.

## Usage

### Initialize

```bash
claudedock local init
```

Generates `~/.claudedock/local.yaml`.

### Start

```bash
claudedock local up
```

Pulls the image, creates a `--network=none` container, and injects sing-box config. Skips KasmVNC and heartbeat, keeps only sshd + sing-box.

### Check Status

```bash
claudedock local status
```

### Stop

```bash
claudedock local down
```

## Egress IP Configuration

```bash
claudedock local up --egress-config '{
  "type": "shadowsocks",
  "server": "198.51.100.5",
  "server_port": 8388,
  "method": "aes-256-gcm",
  "password": "your-password"
}'
```

Supports Shadowsocks, VMess, SOCKS5, Trojan, HTTP.

## VS Code Dev Containers Integration

A `.devcontainer/devcontainer.json` template is provided in the project root:

```json
{
  "name": "claudedock-local",
  "image": "ghcr.io/your-org/managed-user:v3.4.0",
  "runArgs": ["--network=none"],
  "postCreateCommand": "sing-box run -c /etc/sing-box/outbound.json"
}
```

## Differences from Cloud Containers

| Feature | Cloud | Local |
|---------|-------|-------|
| Network | `--network=none` + sing-box tun | Same |
| Desktop | KasmVNC + Chromium | None |
| Heartbeat | Yes | No |
| Expiry governance | Admin-controlled | User-managed |
| Persistent volume | Docker named volume | Same |

## Typical Use Cases

- Offline development: work locally without network
- Local debugging: reproduce cloud environment issues
- Quick experiments: test configs without waiting for cloud container startup
- CI/CD baseline: validate container image behavior before deployment

## Command Reference

```
claudedock local up [flags]
  --egress-config string   sing-box outbound JSON
  --image string           Custom image
  --name string            Container name
  --rm                     Auto-remove after stop

claudedock local down [flags]
  --name string            Target container
  --volumes                Also remove associated volumes

claudedock local status
  Show all local containers and status
```
