# Phase 40: VS Code Remote-SSH 手动测试 Checklist

**测试日期:** ____________
**测试环境:** ____________ (macOS / Linux)
**容器名:** ____________
**egress 配置:** ____________ (有 / 无)

---

## 前置条件

- [ ] 宿主机已安装 VS Code + Remote-SSH 扩展
- [ ] 已运行 `cloud-claude local up` 或 `cloud-claude local up --egress-config <file>` 启动容器
- [ ] 容器已启动且 sshd 运行中（`docker ps | grep cloud-claude-local`）
- [ ] 记录 SSH 连接信息（host, port, user, password）

**连接信息：**
- Host: ____________
- Port: ____________
- User: ____________
- Password: ____________

---

## Happy Path 测试

### 步骤 1: VS Code 连接

**操作：**
1. 打开 VS Code
2. 按 `Ctrl+Shift+P`（macOS: `Cmd+Shift+P`）
3. 输入 "Remote-SSH: Connect to Host..."
4. 输入 `user@127.0.0.1:PORT`（使用上方记录的信息）
5. 输入密码
6. 等待 VS Code 连接成功

**预期：** VS Code 成功连接，显示远程文件系统。

**实际结果：** ____________
**状态：** [ ] PASS  [ ] FAIL

---

### 步骤 2: 文件浏览

**操作：**
1. 在 VS Code 资源管理器中浏览 `/workspace` 目录
2. 打开一个文件查看内容

**预期：** 文件列表正常显示，文件内容正确加载。

**实际结果：** ____________
**状态：** [ ] PASS  [ ] FAIL

---

### 步骤 3: 终端操作

**操作：**
1. VS Code → 终端（`` Ctrl+` ``）
2. 运行 `whoami` → 预期返回容器用户
3. 运行 `claude --version` → 预期输出版本号
4. 运行 `echo hello` → 预期正常输出

**预期：** 终端交互正常，claude 命令可用。

**实际结果：**
- whoami: ____________
- claude --version: ____________
- echo hello: ____________

**状态：** [ ] PASS  [ ] FAIL

---

### 步骤 4: 端口转发

**操作：**
1. 在 VS Code 终端中运行：`python3 -m http.server 8888 &`
2. 观察 VS Code 是否自动检测端口转发（右下角通知或端口面板）
3. 在宿主机浏览器访问 `http://localhost:8888`

**预期：** VS Code 提示端口已转发，浏览器可访问。

**实际结果：** ____________
**状态：** [ ] PASS  [ ] FAIL

---

### 步骤 5: 扩展安装

**操作：**
1. VS Code → 扩展面板
2. 搜索一个轻量扩展（如 "Go" 或 "Python"）
3. 在远程侧安装扩展

**预期：** 扩展下载并安装成功。

**实际结果：** ____________
**状态：** [ ] PASS  [ ] FAIL

---

## Egress 强约束测试

### 步骤 6: 出口 IP 验证

**操作：**
1. 在 VS Code 终端运行：`curl -s ifconfig.me`
2. 对比返回的 IP 与 egress 配置的 ExpectedIP

**预期：** 返回的 IP 等于 egress 配置的 ExpectedIP，不是宿主机出口 IP。

**实际结果：**
- 容器内出口 IP: ____________
- 期望 egress IP: ____________

**状态：** [ ] PASS  [ ] FAIL  [ ] SKIP（未配置 egress）

---

### 步骤 7: DNS 泄漏验证

**操作：**
1. 在 VS Code 终端运行：`nslookup ifconfig.me`
2. 运行：`nslookup google.com`

**预期：** DNS 解析成功，走 sing-box DNS（不返回宿主机 DNS 服务器地址）。

**实际结果：** ____________
**状态：** [ ] PASS  [ ] FAIL  [ ] SKIP（未配置 egress）

---

### 步骤 8: VS Code 更新流量验证

**操作：**
1. 触发 VS Code Server 更新（或检查已有 sing-box 日志）
2. 检查容器内 sing-box 日志：`docker logs $CONTAINER 2>&1 | grep -i "visualstudio"`
3. 或检查：`docker exec $CONTAINER cat /var/log/sing-box.log 2>/dev/null | grep visualstudio`

**预期：** sing-box 日志中出现 `update.code.visualstudio.com` 走 proxy-out。

**实际结果：** ____________
**状态：** [ ] PASS  [ ] FAIL  [ ] SKIP（日志中未发现）

---

### 步骤 9: 端口转发出口 IP 验证

**操作：**
1. 在容器内运行：`python3 -c "from http.server import HTTPServer, BaseHTTPRequestHandler; class H(BaseHTTPRequestHandler): do_GET=lambda s:s.wfile.write(s.client_address[0].encode()); HTTPServer(('0.0.0.0',9999),H).serve_forever()" &`
2. 通过 VS Code 端口转发从宿主机访问：`curl http://localhost:9999`
3. 检查返回的 IP

**预期：** 返回的 IP 是容器内部 IP（如 172.x.x.x），非宿主机 IP。

**实际结果：** ____________
**状态：** [ ] PASS  [ ] FAIL  [ ] SKIP

---

## 汇总

| 步骤 | 场景 | 状态 |
|------|------|------|
| 1 | VS Code 连接 | |
| 2 | 文件浏览 | |
| 3 | 终端操作 | |
| 4 | 端口转发 | |
| 5 | 扩展安装 | |
| 6 | 出口 IP 验证 | |
| 7 | DNS 泄漏验证 | |
| 8 | VS Code 更新流量 | |
| 9 | 端口转发出口 IP | |

**总评：** [ ] 全部通过  [ ] 部分通过  [ ] 存在阻塞问题

**备注：** ____________
