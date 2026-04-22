#!/usr/bin/env bash
# scripts/gen-bench-tree.sh — Phase 35 BASE-01 synthetic 10k mono-repo tree generator
#
# 生成一棵可重复（seed 控制）的合成 mono-repo 文件树，供 perf-benchmark.sh 使用。
# 文件分布：80% 小 / 15% 中 / 5% 大；附加 .git/objects/pack/ + node_modules/ 防御真实负载。
#
# 退出码：
#   0  成功
#   1  参数错误 / 非法 output 路径
#   2  磁盘空间不足（< 1GB）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

pass() { echo "[PASS]  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo "[FAIL]  $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
warn() { echo "[WARN]  $1"; WARN_COUNT=$((WARN_COUNT + 1)); }
info() { echo "[INFO]  $1"; }

usage() {
  cat <<'EOF'
gen-bench-tree.sh — synthetic 10k mono-repo tree for Phase 35 BASE-01 benchmarks

用法: scripts/gen-bench-tree.sh [--count=N] [--output=DIR] [--seed=N] [--help]

选项:
  --count=N     生成的文件总数（默认 10000，范围 1000..100000）
  --output=DIR  输出目录（默认 /tmp/bench-tree；存在则先 rm -rf）
  --seed=N      awk srand 伪随机种子，保证跨机器可复现（默认 42）
  --help, -h    显示本帮助

文件分布（mono-repo 80/15/5）：
  - 80% 小文件（< 4KB）   扩展名: .go .ts .tsx .py .rs .md .json .yaml
  - 15% 中等文件（100KB-1MB） 扩展名: .lock .sum .svg .html
  -  5% 大文件（1-9MB）    扩展名: .bin .pack .wasm

附加（Pitfall 4 防御，模拟真实 mono-repo 行为）：
  - .git/objects/pack/ 下 3 个 1-5MB 的 pack-<hex>.pack 文件
  - node_modules/ 下 200 个嵌套空目录 foo/node_modules/bar/...
  - 5% 文件内容尾部追加 NUL 字节，触发 rg binary skip

退出码:
  0  成功
  1  参数错误 / 非法 output 路径
  2  磁盘空间不足（< 1GB）
EOF
}

COUNT=10000
OUTPUT=/tmp/bench-tree
SEED=42

for arg in "$@"; do
  case "$arg" in
    --count=*) COUNT="${arg#--count=}" ;;
    --output=*) OUTPUT="${arg#--output=}" ;;
    --seed=*) SEED="${arg#--seed=}" ;;
    --help|-h) usage; exit 0 ;;
    *) fail "未知参数: $arg"; usage; exit 1 ;;
  esac
done

if ! [[ "$COUNT" =~ ^[0-9]+$ ]] || [ "$COUNT" -lt 1000 ] || [ "$COUNT" -gt 100000 ]; then
  fail "--count 必须是 1000..100000 之间的整数（当前: $COUNT）"
  exit 1
fi

if ! [[ "$SEED" =~ ^[0-9]+$ ]]; then
  fail "--seed 必须是非负整数（当前: $SEED）"
  exit 1
fi

# T-35-01-01 mitigation：output 路径黑名单（禁止空串、/、$HOME、$PROJECT_ROOT）
case "$OUTPUT" in
  ""|"/"|"$HOME"|"$PROJECT_ROOT")
    fail "非法 output 路径: $OUTPUT（禁止空串、/、HOME、PROJECT_ROOT）"
    exit 1
    ;;
esac
case "$OUTPUT" in
  /tmp/*|/var/tmp/*) ;;
  *) warn "output 路径不在 /tmp 内: $OUTPUT（请确认非误操作）" ;;
esac

PARENT_DIR="$(dirname "$OUTPUT")"
mkdir -p "$PARENT_DIR"

# T-35-01-02 mitigation：磁盘空间 ≥ 1GB
AVAIL_KB=$(df -k "$PARENT_DIR" | awk 'NR==2 { print $4 }')
if [ "${AVAIL_KB:-0}" -lt 1048576 ]; then
  fail "磁盘空间不足: $PARENT_DIR 可用 ${AVAIL_KB}KB，需要 ≥ 1048576KB (1GB)"
  exit 2
fi

if [ -d "$OUTPUT" ]; then
  info "清理已存在的输出目录: $OUTPUT"
  rm -rf "$OUTPUT"
fi

mkdir -p "$OUTPUT"/{src,pkg,internal,test,docs}
mkdir -p "$OUTPUT/.git/objects/pack"
mkdir -p "$OUTPUT/node_modules"

info "生成 $COUNT 个文件到 $OUTPUT (seed=$SEED)"

TOP_DIRS=(src pkg internal test docs)
SMALL_EXTS=(.go .ts .tsx .py .rs .md .json .yaml)
MEDIUM_EXTS=(.lock .sum .svg .html)
LARGE_EXTS=(.bin .pack .wasm)

PLAN_FILE="$(mktemp)"
trap 'rm -f "$PLAN_FILE"' EXIT

# 用 awk 预生成每个文件的属性，srand(SEED) 保证跨机器可复现
awk -v n="$COUNT" -v seed="$SEED" 'BEGIN {
  srand(seed)
  for (i = 1; i <= n; i++) {
    r = rand()
    if (r < 0.80)      { type = "S"; size = int(rand() * 3800) + 200 }
    else if (r < 0.95) { type = "M"; size = int(rand() * 900000) + 100000 }
    else               { type = "L"; size = int(rand() * 9) + 1 }
    ext_idx   = int(rand() * 8)
    dir_idx   = int(rand() * 5)
    sub_depth = int(rand() * 3) + 2
    sub_a     = int(rand() * 8)
    sub_b     = int(rand() * 8)
    sub_c     = int(rand() * 8)
    nul_flag  = (rand() < 0.05) ? 1 : 0
    printf "%d %s %d %d %d %d %d %d %d %d\n", \
      i, type, size, ext_idx, dir_idx, sub_depth, sub_a, sub_b, sub_c, nul_flag
  }
}' > "$PLAN_FILE"

created=0
while read -r idx type size ext_idx dir_idx sub_depth sub_a sub_b sub_c nul_flag; do
  top="${TOP_DIRS[$dir_idx]}"
  case "$sub_depth" in
    2) sub="${sub_a}/${sub_b}" ;;
    3) sub="${sub_a}/${sub_b}/${sub_c}" ;;
    4) sub="${sub_a}/${sub_b}/${sub_c}/leaf" ;;
    *) sub="${sub_a}" ;;
  esac
  case "$type" in
    S) ext="${SMALL_EXTS[$((ext_idx % 8))]}" ;;
    M) ext="${MEDIUM_EXTS[$((ext_idx % 4))]}" ;;
    L) ext="${LARGE_EXTS[$((ext_idx % 3))]}" ;;
  esac
  dir="$OUTPUT/$top/$sub"
  mkdir -p "$dir"
  fpath="$dir/file_${idx}${ext}"
  case "$type" in
    S|M)
      head -c "$size" /dev/urandom | base64 > "$fpath"
      ;;
    L)
      dd if=/dev/urandom of="$fpath" bs=1M count="$size" status=none
      ;;
  esac
  if [ "$nul_flag" -eq 1 ]; then
    printf '\x00\x00binary\x00\x00' >> "$fpath"
  fi
  created=$((created + 1))
done < "$PLAN_FILE"

info "主循环完成: 创建 $created 个文件"

# Pitfall 4: .git/objects/pack/ 三个 pack-<hex>.pack（1-5MB 各，模拟大型 git pack）
for i in 1 2 3; do
  hex=$(printf '%040x' $((SEED * 1000 + i * 7919)))
  pack_size=$(( (SEED + i) % 5 + 1 ))
  dd if=/dev/urandom of="$OUTPUT/.git/objects/pack/pack-${hex}.pack" \
    bs=1M count="$pack_size" status=none
done
info ".git/objects/pack/ 已创建 3 个 pack 文件"

# Pitfall 4: node_modules/ 下 200 个嵌套空目录 foo/node_modules/bar/node_modules/...
for i in $(seq 1 200); do
  mkdir -p "$OUTPUT/node_modules/pkg-${i}/node_modules/sub/node_modules/leaf"
done
info "node_modules/ 已创建 200 个嵌套空目录"

total=$(find "$OUTPUT" -type f | wc -l | tr -d ' ')
size=$(du -sh "$OUTPUT" 2>/dev/null | awk '{ print $1 }')

echo ""
echo "========================================"
info "输出目录: ${OUTPUT}"
info "文件总数: ${total} (目标 ${COUNT} +/- 50)"
info "占用空间: ${size}"

if [ "${total}" -lt $((COUNT - 50)) ] || [ "${total}" -gt $((COUNT + 50)) ]; then
  fail "文件总数 ${total} 超出 ${COUNT} +/- 50 容差"
  exit 1
fi

pass "synthetic mono-repo 树生成成功"
echo "验证结果: ${PASS_COUNT} PASS, ${FAIL_COUNT} FAIL, ${WARN_COUNT} WARN"
exit 0
