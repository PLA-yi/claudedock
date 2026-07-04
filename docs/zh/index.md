---
layout: home

hero:
  name: ClaudeDock
  text: 一个更聪明的 Claude Code Wrapper
  tagline: 把 Claude Code 装进容器，从 IP 到系统指纹全层伪装，像地道的美国开发者在本地写代码
  image:
    src: /logo.svg
    alt: ClaudeDock Logo
  actions:
    - theme: brand
      text: 快速开始
      link: /zh/guide/quickstart
    - theme: alt
      text: GitHub
      link: https://github.com/claudedock/claudedock

features:
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
    title: 全层身份伪装
    details: CPU 型号伪装成 AMD EPYC，MAC 地址和 machine-id 重写，系统探测命令输出拦截篡改。Windows 风格主机名，uTLS Chrome 指纹，遥测 DNS 级别屏蔽。Claude Code 从里到外看到的都是一个普通美国用户。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
    title: IP 严格隔离
    details: 每个容器绑定独立出口 IP，全流量 sing-box tun 隧道强制出网，nftables 默认拒绝直连。DNS、WebRTC 不会漏。支持 6 种代理协议。推荐 AT&T 家宽 IP，干净，风控通过率高。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>
    title: 代码映射——跟本地一样
    details: 本地项目目录通过 sshfs 挂载到容器内同名路径。你在 ~/my-project 里干活，容器里也是 ~/my-project。Claude Code 看到的路径跟你本地完全一致。Auto/Full/SSHFS-Only 三种模式，断线 30s 内自动重连。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>
    title: 一条命令就开写
    details: 管理员后台建好容器，发你一条 curl 命令。终端里跑一下，输密码，等容器启动，自动 SSH 进去。Claude Code 已预装，直接敲 claude 就能用。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="9" y1="21" x2="9" y2="9"/></svg>
    title: 管理后台
    details: 用户和容器全生命周期管理，出口 IP 增删改查与连通性测试，bypass 防火墙，事件审计，SSE 实时推送。容器内置 KasmVNC 远程桌面。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
    title: 到期自动回收
    details: 后台定时扫描用户到期状态，过期自动停机并禁止登录。所有操作写入审计事件，完整可追溯。
---
