#!/usr/bin/env bash
# Phase 52 OBS-01：e2e 失败一键采集脚本
#
# 用法：
#   bash tests/e2e/harness/collect-artifacts.sh <output-dir> [scenario-id]
#
# 行为：
#   - 在 <output-dir>/<scenario-id>/ 下建 5 个子目录（logs/network/docker/postgres/system）
#   - 每个子目录尝试运行对应采集命令，缺工具或失败时写空文件 / _skipped.txt 占位
#   - 脚本永远 exit 0（除非参数缺失）
#   - 设 COLLECT_DEBUG=1 打开 bash -x 调试
#
# 隐私守护：
#   - 不写死任何 /Users/<user>/ 或 /home/<user>/ 绝对路径
#   - postgres 采集只走 --schema-only + key 表 SELECT，永远不导出 password_hash / token
#
# 与 Phase 45 ArtifactDumper.Collect 关系：
#   - Phase 52 Plan 03 接入后，ArtifactDumper 内部调本脚本子进程，与 Go 侧目录树重合
#   - 本地开发也可直接 `bash collect-artifacts.sh ./out scenario_xyz` 复用同套逻辑

set -uo pipefail   # 注意：不带 -e，允许子命令失败

if [[ "${COLLECT_DEBUG:-0}" = "1" ]]; then set -x; fi

readonly SCRIPT_VERSION="v1"
readonly SCRIPT_DIR="$(cd "$(dirname "$0")" 2>/dev/null && pwd)"

OUT="${1:?需要 output-dir，用法：bash collect-artifacts.sh <output-dir> [scenario-id]}"
SCENARIO="${2:-default}"
ROOT="$OUT/$SCENARIO"

mkdir -p "$ROOT/logs" "$ROOT/network" "$ROOT/docker" "$ROOT/postgres" "$ROOT/system"

collect_metadata() {
    local sha
    sha="$(git -C "$SCRIPT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)"
    {
        echo "timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
        echo "scenario=$SCENARIO"
        echo "hostname=$(hostname 2>/dev/null || echo unknown)"
        echo "kernel=$(uname -srm 2>/dev/null || echo unknown)"
        echo "git_sha=$sha"
        echo "runner=${GITHUB_JOB:-local}"
        echo "script_version=$SCRIPT_VERSION"
    } > "$ROOT/metadata.txt"
}

collect_logs() {
    if ! command -v docker >/dev/null 2>&1; then
        echo "docker not available on this host (darwin local without Docker Desktop or CI minimal runner)" \
            > "$ROOT/logs/_skipped.txt"
        return
    fi
    # docker 在但 daemon 没起：docker ps -a 会报 "Cannot connect..."；这里 || true 兜住
    local names
    names="$(docker ps -a --format '{{.Names}}' 2>/dev/null || true)"
    if [[ -z "$names" ]]; then
        echo "docker ps -a 返回空（daemon 未起 / 无容器）" > "$ROOT/logs/_empty.txt"
        return
    fi
    while IFS= read -r name; do
        [[ -z "$name" ]] && continue
        docker logs --tail 500 --timestamps "$name" \
            > "$ROOT/logs/${name}.log" 2>&1 || true
    done <<< "$names"
}

collect_network() {
    # nft list ruleset
    if command -v nft >/dev/null 2>&1; then
        nft list ruleset > "$ROOT/network/nft-ruleset.txt" 2>&1 || true
    else
        echo "nft not available（darwin / 非 root linux）" > "$ROOT/network/nft-ruleset.txt"
    fi

    # ip 系列
    if command -v ip >/dev/null 2>&1; then
        ip -o link    > "$ROOT/network/ip-link.txt"  2>&1 || true
        ip -o addr    > "$ROOT/network/ip-addr.txt"  2>&1 || true
        ip route      > "$ROOT/network/ip-route.txt" 2>&1 || true
        ip netns list > "$ROOT/network/ip-netns.txt" 2>&1 || true
    else
        echo "ip not available（darwin）" > "$ROOT/network/ip-link.txt"
    fi

    # ss 优先，netstat 兜底
    if command -v ss >/dev/null 2>&1; then
        ss -tln > "$ROOT/network/listen-tcp.txt" 2>&1 || true
    elif command -v netstat >/dev/null 2>&1; then
        netstat -tln > "$ROOT/network/listen-tcp.txt" 2>&1 || true
    else
        echo "ss/netstat not available" > "$ROOT/network/listen-tcp.txt"
    fi
}

collect_docker() {
    if ! command -v docker >/dev/null 2>&1; then
        echo "docker not available" > "$ROOT/docker/_skipped.txt"
        return
    fi
    docker ps -a > "$ROOT/docker/ps.txt" 2>&1 || true
    docker network ls > "$ROOT/docker/network-ls.txt" 2>&1 || true

    local names
    names="$(docker ps -a --format '{{.Names}}' 2>/dev/null || true)"
    if [[ -z "$names" ]]; then
        return
    fi
    while IFS= read -r name; do
        [[ -z "$name" ]] && continue
        docker inspect "$name" \
            > "$ROOT/docker/inspect-${name}.json" 2>&1 || true
    done <<< "$names"
}

collect_postgres() {
    # 优先 PG_DUMP_URL，回退 DATABASE_URL；任一未设 → 跳过
    local url="${PG_DUMP_URL:-${DATABASE_URL:-}}"
    if [[ -z "$url" ]]; then
        echo "PG_DUMP_URL / DATABASE_URL 未设置，跳过 postgres 采集（这是预期：e2e 控制面外的本地复现）" \
            > "$ROOT/postgres/_skipped.txt"
        return
    fi
    if ! command -v pg_dump >/dev/null 2>&1; then
        echo "pg_dump not available（darwin 通常无 PG 客户端）" > "$ROOT/postgres/_skipped.txt"
        return
    fi

    # 仅 schema，不含敏感行数据；超时兜底避免 connection refused 拖慢
    pg_dump --schema-only --no-owner --no-privileges "$url" \
        > "$ROOT/postgres/schema.sql" 2> "$ROOT/postgres/schema.err" || true

    # 几个 key 表的安全 SELECT（不含密码 / token）；缺 psql 时跳过
    if command -v psql >/dev/null 2>&1; then
        psql "$url" -At -F$'\t' \
            -c "select id, username, status from users limit 50" \
            > "$ROOT/postgres/users.tsv" 2> "$ROOT/postgres/users.err" || true
        psql "$url" -At -F$'\t' \
            -c "select id, host_id, egress_ip_id, created_at from host_egress_bindings limit 50" \
            > "$ROOT/postgres/host-egress-bindings.tsv" \
            2> "$ROOT/postgres/host-egress-bindings.err" || true
        psql "$url" -At -F$'\t' \
            -c "select id, type, created_at from events order by id desc limit 100" \
            > "$ROOT/postgres/events.tsv" 2> "$ROOT/postgres/events.err" || true
    fi
}

collect_system() {
    uname -a > "$ROOT/system/uname.txt" 2>&1 || true

    if command -v free >/dev/null 2>&1; then
        free -m > "$ROOT/system/free.txt" 2>&1 || true
    elif command -v vm_stat >/dev/null 2>&1; then
        vm_stat > "$ROOT/system/free.txt" 2>&1 || true   # darwin 兜底
    else
        echo "free/vm_stat not available" > "$ROOT/system/free.txt"
    fi

    df -h > "$ROOT/system/df.txt" 2>&1 || true

    # dmesg 最近 100 行：先尝试带 --time-format=iso，失败回退 plain
    if command -v dmesg >/dev/null 2>&1; then
        if ! dmesg --time-format=iso 2>/dev/null | tail -n 100 > "$ROOT/system/dmesg-tail.txt" 2>&1; then
            dmesg 2>/dev/null | tail -n 100 > "$ROOT/system/dmesg-tail.txt" 2>&1 || \
                echo "dmesg failed (permission denied?)" > "$ROOT/system/dmesg-tail.txt"
        fi
    else
        echo "dmesg not available" > "$ROOT/system/dmesg-tail.txt"
    fi
}

# README 模板复制（Plan 02 落地后激活；模板缺失走 || true 兜底）
copy_readmes() {
    local template_dir="$SCRIPT_DIR/artifacts"
    local sub
    for sub in logs network docker postgres system; do
        if [[ -f "$template_dir/$sub/README.md" ]]; then
            cp "$template_dir/$sub/README.md" "$ROOT/$sub/README.md" 2>/dev/null || true
        fi
    done
}

collect_metadata
collect_logs
collect_network
collect_docker
collect_postgres
collect_system
copy_readmes

echo "[collect-artifacts] done: $ROOT"
exit 0
