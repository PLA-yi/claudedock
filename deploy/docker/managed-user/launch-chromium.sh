#!/usr/bin/env bash
set -euo pipefail

export DISPLAY="${DISPLAY:-:99}"
export HOME="${HOME:-/workspace}"

browser_cmd=""
for candidate in chromium chromium-browser google-chrome; do
  if command -v "${candidate}" >/dev/null 2>&1; then
    browser_cmd="${candidate}"
    break
  fi
done

if [[ -z "${browser_cmd}" ]]; then
  exec xterm -fa Monospace -fs 12 -geometry 120x30+60+60 -title "claudedock desktop" \
    -e bash -lc "echo Chromium is not installed.; exec bash"
fi

if [[ $# -gt 0 ]]; then
  exec "${browser_cmd}" "$@"
fi

exec "${browser_cmd}" \
  --no-sandbox \
  --disable-dev-shm-usage \
  --user-data-dir=/workspace/.chrome-data \
  --start-maximized \
  --no-first-run \
  --disable-gpu \
  --disable-features=WebRtcHideLocalIpsWithMdns \
  --enforce-webrtc-ip-permission-check \
  --force-webrtc-ip-handling-policy=disable_non_proxied_udp \
  --window-position=0,0 \
  --window-size=1920,1080 \
  "https://www.google.com"
