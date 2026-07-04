---
phase: quick
plan: 260505-fjq
type: execute
wave: 1
depends_on: []
files_modified:
  - claude-shell/docker/Dockerfile
  - web/admin/src/components/hosts/create-host-dialog.tsx
autonomous: true
requirements:
  - QUICK-TZ-01
  - QUICK-TZ-02
must_haves:
  truths:
    - "用户容器镜像包含 tzdata，TZ 环境变量可正确解析"
    - "前端时区选择列表显示固定标准偏移，不随季节变化"
  artifacts:
    - path: "claude-shell/docker/Dockerfile"
      provides: "tzdata 包安装"
      contains: "tzdata"
    - path: "web/admin/src/components/hosts/create-host-dialog.tsx"
      provides: "固定标准偏移时区列表"
      contains: "UTC-8|UTC-5|UTC-6|UTC-7|UTC+0|UTC+1|UTC+9|UTC+8|UTC+10"
  key_links:
    - from: "Dockerfile"
      to: "容器运行时 TZ 解析"
      via: "apt-get install tzdata"
    - from: "create-host-dialog.tsx"
      to: "用户时区选择体验"
      via: "硬编码标准偏移映射表"
---

<objective>
修复两个时区相关问题：
1. 用户容器 Dockerfile 缺少 tzdata 包，导致 TZ 环境变量无法被 glibc 正确解析；
2. 前端创建主机对话框的时区偏移使用动态计算，夏令时导致显示值随季节跳动，改为固定标准偏移。

Purpose: 保证容器内时区设置可靠生效，前端时区展示稳定一致。
Output: Dockerfile 安装 tzdata；前端移除 getUTCOffset 动态计算，改用硬编码标准偏移映射表。
</objective>

<execution_context>
@/workspace/Desktop/claudedock/.claude/get-shit-done/workflows/execute-plan.md
@/workspace/Desktop/claudedock/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@claude-shell/docker/Dockerfile
@web/admin/src/components/hosts/create-host-dialog.tsx
</context>

<tasks>

<task type="auto">
  <name>Task 1: Dockerfile 添加 tzdata 包</name>
  <files>claude-shell/docker/Dockerfile</files>
  <action>
在 Dockerfile 第 10 行的 apt-get install 列表中加入 `tzdata`。当前列表为：
```
curl git jq procps iproute2 ca-certificates bash sudo nftables
```
修改为：
```
curl git jq procps iproute2 ca-certificates bash sudo nftables tzdata
```
确保 `tzdata` 放在同一行，保持 `--no-install-recommends` 和 `rm -rf /var/lib/apt/lists/*` 不变。
  </action>
  <verify>
    <automated>grep -n "tzdata" claude-shell/docker/Dockerfile</automated>
  </verify>
  <done>Dockerfile 的 apt-get install 列表中包含 tzdata，且位于同一 RUN 指令内</done>
</task>

<task type="auto">
  <name>Task 2: 前端时区选择改为固定标准偏移</name>
  <files>web/admin/src/components/hosts/create-host-dialog.tsx</files>
  <action>
1. 删除现有的 `getUTCOffset` 函数（第 33-46 行）。
2. 将 `TIMEZONE_OPTIONS` 数组从仅含 `value` 和 `label` 改为同时包含固定的 `offset` 字段，使用标准偏移（非夏令时）：
   - America/Los_Angeles → UTC-8
   - America/New_York → UTC-5
   - America/Chicago → UTC-6
   - America/Denver → UTC-7
   - Europe/London → UTC+0
   - Europe/Paris → UTC+1
   - Europe/Berlin → UTC+1
   - Asia/Tokyo → UTC+9
   - Asia/Shanghai → UTC+8
   - Asia/Singapore → UTC+8
   - Asia/Seoul → UTC+9
   - Australia/Sydney → UTC+10
   - Pacific/Honolulu → UTC-10
3. 在 SelectItem 渲染处，将 `{getUTCOffset(tz.value)}` 替换为 `{tz.offset}`，保持括号格式 `({tz.offset})` 不变。

修改后 TIMEZONE_OPTIONS 示例结构：
```typescript
const TIMEZONE_OPTIONS = [
  { value: "America/Los_Angeles", label: "美西 / 洛杉矶", offset: "UTC-8" },
  ...
];
```
  </action>
  <verify>
    <automated>grep -c "getUTCOffset" web/admin/src/components/hosts/create-host-dialog.tsx || echo "0"</automated>
  </verify>
  <done>getUTCOffset 函数已删除；TIMEZONE_OPTIONS 每项包含固定 offset 字段；SelectItem 渲染使用 tz.offset；无 getUTCOffset 残留引用</done>
</task>

</tasks>

<verification>
- `grep tzdata claude-shell/docker/Dockerfile` 命中且位于 apt-get install 行
- `grep getUTCOffset web/admin/src/components/hosts/create-host-dialog.tsx` 返回空
- `grep -E 'UTC[-+][0-9]+' web/admin/src/components/hosts/create-host-dialog.tsx` 命中 13 条固定偏移
</verification>

<success_criteria>
- Dockerfile 成功安装 tzdata，容器内 `TZ=America/Los_Angeles date` 可正确输出太平洋时间
- 前端时区下拉列表显示固定标准偏移，冬季和夏季访问同一时区显示相同偏移值
</success_criteria>

<output>
After completion, create `.planning/quick/260505-fjq-1-dockerfile-tzdata-tz-2/260505-fjq-SUMMARY.md`
</output>
