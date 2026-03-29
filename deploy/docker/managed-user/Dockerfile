FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV HOME=/workspace
ENV WORKSPACE_USER=workspace
ENV WORKSPACE_UID=1000
ENV WORKSPACE_GID=1000

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        openssh-server \
        bash \
        zsh \
        curl \
        git \
        tmux \
        sudo \
        ca-certificates \
        jq \
        procps \
        iproute2 \
        nodejs \
        npm \
        locales \
        fluxbox \
        pcmanfm \
        dbus-x11 \
        fonts-liberation \
        fonts-noto-cjk \
        xdg-utils \
        xclip \
        xsel \
        gnupg \
        libegl1 \
        libgl1 \
        x11-utils \
        x11-xserver-utils \
        xterm \
    && rm -rf /var/lib/apt/lists/*

# KasmVNC：替代 x11vnc + noVNC + websockify + Xvfb，自带剪贴板同步和 Web 客户端。
ARG KASMVNC_VERSION=1.4.0
RUN ARCH=$(dpkg --print-architecture) \
    && curl -fsSL -o /tmp/kasmvnc.deb \
        "https://github.com/kasmtech/KasmVNC/releases/download/v${KASMVNC_VERSION}/kasmvncserver_noble_${KASMVNC_VERSION}_${ARCH}.deb" \
    && apt-get update \
    && apt-get install -y --no-install-recommends /tmp/kasmvnc.deb \
    && rm /tmp/kasmvnc.deb \
    && rm -rf /var/lib/apt/lists/*

# Chromium：Ubuntu 24.04 的 chromium-browser 是 snap 包装器，Docker 不支持 snap。
# 从 Debian bookworm 仓库安装真正的 .deb（支持 amd64 + arm64）。
RUN echo "deb [trusted=yes arch=$(dpkg --print-architecture)] http://deb.debian.org/debian bookworm main" \
        > /etc/apt/sources.list.d/debian-chromium.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends -t bookworm chromium \
    && rm /etc/apt/sources.list.d/debian-chromium.list \
    && apt-get update \
    && rm -rf /var/lib/apt/lists/*

# sing-box: 全隧道代理，由控制面通过 nsenter 启动
RUN ARCH=$(dpkg --print-architecture) \
    && curl -fsSL -o /tmp/sing-box.tar.gz \
        "https://github.com/SagerNet/sing-box/releases/download/v1.11.7/sing-box-1.11.7-linux-${ARCH}.tar.gz" \
    && tar -xzf /tmp/sing-box.tar.gz -C /tmp \
    && mv /tmp/sing-box-*/sing-box /usr/local/bin/sing-box \
    && chmod +x /usr/local/bin/sing-box \
    && rm -rf /tmp/sing-box*

RUN locale-gen en_US.UTF-8
ENV LANG=en_US.UTF-8
ENV LANGUAGE=en_US:en
ENV LC_ALL=en_US.UTF-8

RUN groupadd --force --gid "${WORKSPACE_GID}" "${WORKSPACE_USER}" \
    && useradd --non-unique --uid "${WORKSPACE_UID}" --gid "${WORKSPACE_GID}" --home-dir /workspace --create-home --shell /bin/bash "${WORKSPACE_USER}" \
    && echo "${WORKSPACE_USER} ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/workspace \
    && chmod 0440 /etc/sudoers.d/workspace

RUN adduser "${WORKSPACE_USER}" ssl-cert 2>/dev/null || true

RUN npm install -g @anthropic-ai/claude-code

RUN mkdir -p /workspace/.ssh /var/run/sshd \
    && chown -R "${WORKSPACE_UID}:${WORKSPACE_GID}" /workspace \
    && chmod 0700 /workspace/.ssh

COPY deploy/docker/managed-user/sshd_config /etc/ssh/sshd_config
COPY deploy/docker/managed-user/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY deploy/docker/managed-user/launch-chromium.sh /usr/local/bin/launch-chromium.sh

RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/launch-chromium.sh

WORKDIR /workspace

EXPOSE 22
EXPOSE 6080

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
