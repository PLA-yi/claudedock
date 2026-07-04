#!/usr/bin/env bash
set -euo pipefail

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

# check_apparmor_fusermount3 — Ubuntu 25.04+ AppArmor override advisory check.
# D-23: override path is /etc/apparmor.d/local/fusermount3 (not docker-default).
# D-24: detect + print manual fix instructions only; never auto-apply.
# Style: matches existing require_cmd / inline echo ... >&2 pattern in this file;
#        intentionally NOT introducing log_* helpers (would diverge from current script).
check_apparmor_fusermount3() {
  # --- OS gate: only Ubuntu 25.04+ is affected ---
  if [ ! -f /etc/os-release ]; then
    echo "host-preflight: /etc/os-release missing; skipping AppArmor fusermount3 check" >&2
    return 0
  fi
  # shellcheck source=/dev/null
  . /etc/os-release
  if [ "${ID:-}" != "ubuntu" ]; then
    echo "host-preflight: non-Ubuntu host (${ID:-unknown}); skipping AppArmor fusermount3 check" >&2
    return 0
  fi
  # VERSION_ID examples: 25.04 / 25.10 / 24.04
  ubuntu_major="${VERSION_ID%%.*}"
  ver_rest="${VERSION_ID#*.}"
  ubuntu_minor="${ver_rest%%.*}"
  if [ "${ubuntu_major}" -lt 25 ] || { [ "${ubuntu_major}" -eq 25 ] && [ "${ubuntu_minor:-0}" -lt 4 ]; }; then
    echo "host-preflight: Ubuntu ${VERSION_ID} < 25.04; skipping AppArmor fusermount3 check" >&2
    return 0
  fi

  # --- Tool gate: aa-status missing → advisory skip ---
  if ! command -v aa-status >/dev/null 2>&1; then
    echo "host-preflight: aa-status not installed; install with: apt-get install -y apparmor-utils" >&2
    echo "host-preflight: skipping AppArmor fusermount3 check (advisory)" >&2
    return 0
  fi

  # --- Profile gate: fusermount3 profile not loaded ---
  if ! aa-status 2>/dev/null | grep -q 'fusermount3'; then
    echo "host-preflight: AppArmor fusermount3 profile not loaded; nothing to override" >&2
    return 0
  fi

  # --- Override rule detection ---
  override=/etc/apparmor.d/local/fusermount3
  if [ ! -f "${override}" ] || ! grep -qE '^[[:space:]]*capability[[:space:]]+dac_override' "${override}" 2>/dev/null; then
    echo "host-preflight: FAIL AppArmor override missing — ${override} lacks 'capability dac_override,'" >&2
    cat >&2 <<'EOF'

To fix on Ubuntu 25.04+ (run as root on the host):

  1) Write /etc/apparmor.d/local/fusermount3 containing one rule line:

     capability dac_override,

  2) Reload the fusermount3 profile:

     sudo apparmor_parser -r /etc/apparmor.d/fusermount3

See deploy/README.md section "v3.0 AppArmor override 部署" for background.

EOF
    return 1
  fi

  echo "host-preflight: AppArmor fusermount3 override OK" >&2
  return 0
}

require_cmd docker
require_cmd ip
require_cmd systemctl

if ! command -v nft >/dev/null 2>&1 && ! command -v iptables >/dev/null 2>&1; then
  echo "missing required firewall tool: nft or iptables" >&2
  exit 1
fi

# FUSE kernel module must be loadable (required for sshfs directory mapping)
if ! modprobe fuse 2>/dev/null; then
  if [ ! -c /dev/fuse ]; then
    echo "missing fuse kernel module (required for sshfs directory mapping)" >&2
    echo "try: modprobe fuse" >&2
    exit 1
  fi
fi

if [ ! -c /dev/fuse ]; then
  echo "missing /dev/fuse device (required for container FUSE mount)" >&2
  exit 1
fi

# Phase 2: nsenter required for container namespace verification
require_cmd nsenter

# Phase 2: nft required for container firewall rules
require_cmd nft

# Phase 2: curl required for egress IP verification
require_cmd curl

# Phase 29: Ubuntu 25.04+ AppArmor advisory check (D-23 / D-24; advisory — non-blocking).
check_apparmor_fusermount3 || true

mkdir -p /var/lib/claudedock
mkdir -p /run/claudedock
