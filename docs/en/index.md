---
layout: home

hero:
  name: ClaudeDock
  text: A smarter Claude Code Wrapper
  tagline: Containerize Claude Code with full-stack spoofing — from IP to system fingerprint — so you look like a regular American developer on a local machine
  image:
    src: /logo.svg
    alt: ClaudeDock Logo
  actions:
    - theme: brand
      text: Quick Start
      link: /en/guide/quickstart
    - theme: alt
      text: GitHub
      link: https://github.com/claudedock/claudedock

features:
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
    title: Full-Stack Identity Spoofing
    details: CPU shows as AMD EPYC, MAC address and machine-id rewritten, system probe output intercepted. Windows-style hostnames, uTLS Chrome fingerprint, DNS-level telemetry blocking. To Claude Code, you're just a regular American user.
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
    title: Strict IP Isolation
    details: Every container gets its own exit IP. All traffic forced through sing-box tun tunnel. nftables default-deny — DNS and WebRTC can't leak. 6 proxy protocols. AT&T residential IPs recommended — clean and reliable.
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>
    title: Code Mapping — Feels Native
    details: Your local project mounts at the exact same path inside the container. Work in ~/my-project locally, it's ~/my-project in the container too. Claude Code sees paths identical to your machine. Auto/Full/SSHFS-Only modes. Auto-reconnect within 30s.
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>
    title: One Command, Then Code
    details: Admin creates the container, sends you a curl command. Paste it, enter your password, wait for boot, and you're SSHed in. Claude Code pre-installed. Just type claude.
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="9" y1="21" x2="9" y2="9"/></svg>
    title: Admin Dashboard
    details: Full user and container lifecycle management. Egress IP CRUD with connectivity testing. Bypass firewall. Event auditing, SSE real-time push. Built-in KasmVNC remote desktop.
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
    title: Auto-expiry Governance
    details: Background scanner checks user expiration. Expired users get their containers stopped and logins blocked. Every operation written to auditable event log.
---
