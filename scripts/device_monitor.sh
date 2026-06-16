#!/bin/bash
# ============================================================
# device_monitor.sh — 独立于测试程序的系统资源监控
#
# 监控 CPU、内存、磁盘利用率。读取 /proc 文件系统，零外部依赖。
#
# 用法:
#   ./device_monitor.sh <SSD设备> <HDD设备> [输出文件] [间隔秒] [PID] [进程名]
#
#   SSD设备: 逗号分隔的设备名，如 nvme0n1,nvme1n1  无 SSD 填 none
#   HDD设备: 逗号分隔的设备名，如 sda,sdb          无 HDD 填 none
#   进程名:  纯进程名（非路径），如 PureLSM_test。指定后监控会在该进程
#            退出时自动结束。留空则不自动退出（需手动 kill 或 Ctrl+C）。
#            若未指定 PID，会尝试按此名称自动检测目标进程。
# ============================================================

set -uo pipefail

ssd_args="${1:-}" ; hdd_args="${2:-}"
OUTPUT_FILE="${3:-./device_monitor.csv}"
INTERVAL="${4:-1}"
TARGET_PID="${5:-}"
TARGET_NAME="${6:-}"

if [[ -z "$ssd_args" ]]; then
    echo "ERROR: usage: $0 <ssd_devs> <hdd_devs> [output] [interval] [pid] [proc_name]" >&2
    echo "  e.g. $0 nvme0n1 sda monitor.csv 1 12345 PureLSM_test" >&2
    exit 1
fi

# parse comma-separated device lists, filter out "none"
SSD_DEVS=(); IFS=',' read -ra SSD_DEVS <<< "$ssd_args"
HDD_DEVS=(); IFS=',' read -ra HDD_DEVS <<< "$hdd_args"
SSD_DEVS=($(printf '%s\n' "${SSD_DEVS[@]}" | grep -v '^none$' || true))
HDD_DEVS=($(printf '%s\n' "${HDD_DEVS[@]}" | grep -v '^none$' || true))

# validate: must be block devices, not partitions
validate_device() {
    local dev="$1"
    if ! grep -qw "$dev" /proc/diskstats; then
        echo "[device_monitor] WARNING: device '$dev' not found in /proc/diskstats" >&2
        return 1
    fi
    # /sys/class/block is the unified entry for both whole-disk and partition nodes
    if [[ -f "/sys/class/block/$dev/partition" ]]; then
        echo "[device_monitor] WARNING: '$dev' is a partition, not a block device — skipping" >&2
        return 1
    fi
    return 0
}

SSD_DEVS_F=(); for d in "${SSD_DEVS[@]}"; do validate_device "$d" && SSD_DEVS_F+=("$d"); done
HDD_DEVS_F=(); for d in "${HDD_DEVS[@]}"; do validate_device "$d" && HDD_DEVS_F+=("$d"); done
SSD_DEVS=("${SSD_DEVS_F[@]}")
HDD_DEVS=("${HDD_DEVS_F[@]}")

ALL_DEVS=("${SSD_DEVS[@]}" "${HDD_DEVS[@]}")

if [[ ${#ALL_DEVS[@]} -eq 0 ]]; then
    echo "[device_monitor] ERROR: no valid devices specified" >&2
    exit 1
fi

CLK_TCK=$(getconf CLK_TCK 2>/dev/null || echo 100)

# 杀掉旧的同名监控进程，等待它们退出后再操作文件，防止竞态写入
for old_pid in $(pgrep -f "device_monitor\.sh" 2>/dev/null); do
    [[ "$old_pid" != "$$" && "$old_pid" != "$PPID" ]] && kill "$old_pid" 2>/dev/null
done
# 短暂等待，给旧进程的 trap（write_summary）时间执行完
sleep 0.5

echo "[device_monitor] SSD devices: ${SSD_DEVS[*]:-(none)}"
echo "[device_monitor] HDD devices: ${HDD_DEVS[*]:-(none)}"
echo "[device_monitor] CLK_TCK=${CLK_TCK}"
if [[ -n "$TARGET_PID" ]]; then
    echo "[device_monitor] monitoring PID=$TARGET_PID"
fi

# 清理上次的监控文件（旧进程的 write_summary 已在 sleep 期间完成）
rm -f "$OUTPUT_FILE"
echo "[device_monitor] output=$OUTPUT_FILE interval=${INTERVAL}s monitor_pid=$$"

# ============================================================
# 工具函数 — 读取（纯读取，不做计算）
# ============================================================

# CPU: /proc/stat → "idle total"
read_cpustat() {
    awk '/^cpu / {
        idle = $5; sum = 0
        for (i=2; i<=NF; i++) sum += $i
        printf "%.0f %.0f", idle, sum
    }' /proc/stat
}

# CPU: 纯计算，不读文件 — prev_idle prev_sum cur_idle cur_sum → util_pct
compute_cpu_pct() {
    awk -v pi="$1" -v ps="$2" -v ci="$3" -v cs="$4" \
    'BEGIN {
        di = ci - pi; ds = cs - ps
        if (ds > 0) printf "%.2f", (ds - di) / ds * 100
        else printf "0.00"
    }'
}

# Disk: 聚合读取一组设备的 8 个原始字段 → "ri rs rt wi ws wt io wio"
read_diskstats_group() {
    local -n _devs=$1
    local devlist="${_devs[*]}"
    if [[ -z "$devlist" ]]; then
        echo "0 0 0 0 0 0 0 0"
        return
    fi
    awk -v devlist="$devlist" '
    BEGIN { split(devlist, devs, " ") }
    {
        for (i in devs) {
            if ($3 == devs[i]) {
                ri += $4; rs += $6; rt += $7
                wi += $8; ws += $10; wt += $11
                io += $13; wio += $14
            }
        }
    }
    END { printf "%.0f %.0f %.0f %.0f %.0f %.0f %.0f %.0f", ri, rs, rt, wi, ws, wt, io, wio }
    ' /proc/diskstats
}

# Disk: 纯计算，不读文件 — 8 prev + 8 cur + elapsed → "rd_iops rd_kbps wr_iops wr_kbps"
compute_disk_metrics() {
    awk -v pri="$1" -v prs="$2" -v prt="$3" -v pwi="$4" \
        -v pws="$5" -v pwt="$6" -v pio="$7" -v pwio="$8" \
        -v cri="$9" -v crs="${10}" -v crt="${11}" -v cwi="${12}" \
        -v cws="${13}" -v cwt="${14}" -v cio="${15}" -v cwio="${16}" \
        -v e="${17}" \
    'BEGIN {
        if (e > 0.001)
            printf "%.1f %.1f %.1f %.1f",
                (cri - pri) / e,
                (crs - prs) * 512.0 / 1024.0 / e,
                (cwi - pwi) / e,
                (cws - pws) * 512.0 / 1024.0 / e
        else
            printf "0.0 0.0 0.0 0.0"
    }'
}

# 进程 CPU: /proc/<pid>/stat $14=utime $15=stime
read_proc_cpu() {
    local pid="$1"
    [[ ! -f "/proc/$pid/stat" ]] && { echo "0 0"; return; }
    awk '{ printf "%.0f %.0f", $14, $15 }' "/proc/$pid/stat" 2>/dev/null || echo "0 0"
}

# 内存: /proc/meminfo
read_meminfo() {
    awk '
    /^MemTotal:/     { total=$2 }
    /^MemAvailable:/ { avail=$2 }
    /^MemFree:/      { free=$2 }
    END {
        if (avail == "") avail = free
        printf "%.0f %.0f", total, avail
    }' /proc/meminfo
}

# 进程 RSS: /proc/<pid>/status
read_process_rss() {
    local pid="$1"
    [[ ! -f "/proc/$pid/status" ]] && { echo "0"; return; }
    awk '/^VmRSS:/ { found=1; printf "%.0f", $2 }
         END { if (!found) printf "0" }' "/proc/$pid/status" 2>/dev/null || echo "0"
}

# ============================================================
# 聚合 / 差分（util% 部分）
# ============================================================

# 读取每个设备的 io_ticks ($13)，存入关联数组
snap_device_wio() {
    local -n _devs=$1
    local -n _map=$2
    for dev in "${_devs[@]}"; do
        _map[$dev]=$(awk -v dev="$dev" '$3 == dev { printf "%.0f", $13; exit }' /proc/diskstats)
        _map[$dev]=${_map[$dev]:-0}
    done
}

# 单设备 util%: io_ticks 差 / (elapsed_ms) * 100
per_device_util() {
    local prev_io_ticks="$1" cur_io_ticks="$2" elapsed="$3"
    local d_io=$(( cur_io_ticks - prev_io_ticks ))
    local util=0
    if (( $(awk "BEGIN { print ($elapsed > 0.001) }") )); then
        util=$(awk "BEGIN { printf \"%.2f\", $d_io / ($elapsed * 1000) * 100 }")
        if (( $(awk "BEGIN { print ($util > 100) }") )); then util=100.00; fi
        if (( $(awk "BEGIN { print ($util < 0)   }") )); then util=0.00;  fi
    fi
    echo "$util"
}

# 批量计算多设备 util%
# 通过 nameref 返回结果，不再使用 stdout（避免 $(…) 子 shell 丢失副作用）
compute_utils() {
    local -n _devs=$1
    local -n _prev_map=$2
    local -n _cur_map=$3
    local elapsed="$4"
    local -n _out_max=$5
    local -n _out_avg=$6
    local max_u=0 sum_u=0 count=0

    for dev in "${_devs[@]}"; do
        local cur_io="${_cur_map[$dev]:-0}"
        local prev_io="${_prev_map[$dev]:-0}"
        local util
        util=$(per_device_util "$prev_io" "$cur_io" "$elapsed")
        _prev_map[$dev]=$cur_io
        count=$((count + 1))
        sum_u=$(awk "BEGIN { printf \"%.2f\", $sum_u + $util }")
        if (( $(awk "BEGIN { print ($util > $max_u) }") )); then max_u=$util; fi
        # track per-device max util
        local pmax="${PER_DEV_MAX_UTIL[$dev]:-0}"
        if (( $(awk "BEGIN { print ($util > $pmax) }") )); then PER_DEV_MAX_UTIL[$dev]=$util; fi
    done

    local avg_u=0
    if [[ $count -gt 0 ]]; then
        avg_u=$(awk "BEGIN { printf \"%.2f\", $sum_u / $count }")
    fi
    _out_max=$max_u
    _out_avg=$avg_u
}

proc_cpu_delta() {
    local d_ticks=$(( ($3 + $4) - ($1 + $2) ))
    local util=0
    if (( $(awk "BEGIN { print ($5 > 0.001) }") )); then
        util=$(awk "BEGIN { printf \"%.2f\", $d_ticks / $CLK_TCK / $5 * 100 }")
    fi
    echo "$util"
}

mem_used_gb() {
    awk -v total="$1" -v avail="$2" 'BEGIN { printf "%.2f", (total - avail) / 1024 / 1024 }'
}

# ============================================================
# 初始化
# ============================================================

echo "timestamp,elapsed_s,cpu_pct,proc_cpu_pct_total,mem_used_gb,proc_rss_mb,ssd_rd_iops,ssd_rd_kbps,ssd_wr_iops,ssd_wr_kbps,ssd_util_pct,ssd_avg_util_pct,hdd_rd_iops,hdd_rd_kbps,hdd_wr_iops,hdd_wr_kbps,hdd_util_pct,hdd_avg_util_pct" > "$OUTPUT_FILE"

SSD_INIT=($(read_diskstats_group SSD_DEVS))
HDD_INIT=($(read_diskstats_group HDD_DEVS))
PREV_SSD=("${SSD_INIT[@]}")
PREV_HDD=("${HDD_INIT[@]}")
PREV_CPU=($(read_cpustat))
START_SEC=$(date +%s.%N)
PREV_WALL_SEC=$START_SEC

# 每设备的 util 历史值（cur 值在采样循环中通过 snap_device_wio 获取）
declare -A PREV_SSD_WIO PREV_HDD_WIO
declare -A PER_DEV_MAX_UTIL
for dev in "${ALL_DEVS[@]}"; do PER_DEV_MAX_UTIL[$dev]=0; done
snap_device_wio SSD_DEVS PREV_SSD_WIO
snap_device_wio HDD_DEVS PREV_HDD_WIO

# 若未指定目标 PID 但指定了进程名，尝试自动检测
if [[ -z "$TARGET_PID" && -n "$TARGET_NAME" ]]; then
    TARGET_PID=$(pgrep -nx "$TARGET_NAME" 2>/dev/null || true)
fi
if [[ -n "$TARGET_PID" ]]; then
    PREV_PCPU=($(read_proc_cpu "$TARGET_PID"))
    echo "[device_monitor] monitoring process '${TARGET_NAME:-<pid>}' PID=$TARGET_PID"
elif [[ -n "$TARGET_NAME" ]]; then
    PREV_PCPU=(0 0)
    echo "[device_monitor] waiting for process '$TARGET_NAME' to start..."
else
    PREV_PCPU=(0 0)
    echo "[device_monitor] no target PID or process name specified, tracking system-level metrics only"
fi

# 峰值追踪 + 累计 IO（高精度）
PEAK_CPU=0;       PEAK_PCPU=0;     PEAK_MEM=0;      PEAK_PROC_RSS=0
PEAK_SSD_RD_IOPS=0; PEAK_SSD_WR_IOPS=0; PEAK_SSD_RD_KBPS=0; PEAK_SSD_WR_KBPS=0; PEAK_SSD_UTIL=0
PEAK_HDD_RD_IOPS=0; PEAK_HDD_WR_IOPS=0; PEAK_HDD_RD_KBPS=0; PEAK_HDD_WR_KBPS=0; PEAK_HDD_UTIL=0
TOTAL_SSD_RD_KB="0"; TOTAL_SSD_WR_KB="0"
TOTAL_HDD_RD_KB="0"; TOTAL_HDD_WR_KB="0"

update_peaks() {
    local cpu=$1 pcpu=$2 mem=$3 rss=$4
    local ssd_rd_iops=$5 ssd_rd_kbps=$6 ssd_wr_iops=$7 ssd_wr_kbps=$8 ssd_util=$9
    local hdd_rd_iops=${10} hdd_rd_kbps=${11} hdd_wr_iops=${12} hdd_wr_kbps=${13} hdd_util=${14}
    local elapsed=${15}

    (( $(awk "BEGIN { print ($cpu          > $PEAK_CPU)          }") )) && PEAK_CPU=$cpu
    (( $(awk "BEGIN { print ($pcpu         > $PEAK_PCPU)         }") )) && PEAK_PCPU=$pcpu
    (( $(awk "BEGIN { print ($mem          > $PEAK_MEM)          }") )) && PEAK_MEM=$mem
    (( $(awk "BEGIN { print ($rss          > $PEAK_PROC_RSS)     }") )) && PEAK_PROC_RSS=$rss
    (( $(awk "BEGIN { print ($ssd_rd_iops  > $PEAK_SSD_RD_IOPS)  }") )) && PEAK_SSD_RD_IOPS=$ssd_rd_iops
    (( $(awk "BEGIN { print ($ssd_wr_iops  > $PEAK_SSD_WR_IOPS)  }") )) && PEAK_SSD_WR_IOPS=$ssd_wr_iops
    (( $(awk "BEGIN { print ($ssd_rd_kbps  > $PEAK_SSD_RD_KBPS)  }") )) && PEAK_SSD_RD_KBPS=$ssd_rd_kbps
    (( $(awk "BEGIN { print ($ssd_wr_kbps  > $PEAK_SSD_WR_KBPS)  }") )) && PEAK_SSD_WR_KBPS=$ssd_wr_kbps
    (( $(awk "BEGIN { print ($ssd_util     > $PEAK_SSD_UTIL)     }") )) && PEAK_SSD_UTIL=$ssd_util
    (( $(awk "BEGIN { print ($hdd_rd_iops  > $PEAK_HDD_RD_IOPS)  }") )) && PEAK_HDD_RD_IOPS=$hdd_rd_iops
    (( $(awk "BEGIN { print ($hdd_wr_iops  > $PEAK_HDD_WR_IOPS)  }") )) && PEAK_HDD_WR_IOPS=$hdd_wr_iops
    (( $(awk "BEGIN { print ($hdd_rd_kbps  > $PEAK_HDD_RD_KBPS)  }") )) && PEAK_HDD_RD_KBPS=$hdd_rd_kbps
    (( $(awk "BEGIN { print ($hdd_wr_kbps  > $PEAK_HDD_WR_KBPS)  }") )) && PEAK_HDD_WR_KBPS=$hdd_wr_kbps
    (( $(awk "BEGIN { print ($hdd_util     > $PEAK_HDD_UTIL)     }") )) && PEAK_HDD_UTIL=$hdd_util

    TOTAL_SSD_RD_KB=$(awk "BEGIN { printf \"%.3f\", $TOTAL_SSD_RD_KB + $ssd_rd_kbps * $elapsed }")
    TOTAL_SSD_WR_KB=$(awk "BEGIN { printf \"%.3f\", $TOTAL_SSD_WR_KB + $ssd_wr_kbps * $elapsed }")
    TOTAL_HDD_RD_KB=$(awk "BEGIN { printf \"%.3f\", $TOTAL_HDD_RD_KB + $hdd_rd_kbps * $elapsed }")
    TOTAL_HDD_WR_KB=$(awk "BEGIN { printf \"%.3f\", $TOTAL_HDD_WR_KB + $hdd_wr_kbps * $elapsed }")
}

# ============================================================
# 退出时写入汇总报告
# ============================================================
write_summary() {
    local end_sec elapsed
    end_sec=$(date +%s.%N)
    elapsed=$(awk "BEGIN { printf \"%.2f\", $end_sec - $START_SEC }")

    ssd_rd_gb=$(awk "BEGIN { printf \"%.2f\", $TOTAL_SSD_RD_KB / 1024 / 1024 }")
    ssd_wr_gb=$(awk "BEGIN { printf \"%.2f\", $TOTAL_SSD_WR_KB / 1024 / 1024 }")
    hdd_rd_gb=$(awk "BEGIN { printf \"%.2f\", $TOTAL_HDD_RD_KB / 1024 / 1024 }")
    hdd_wr_gb=$(awk "BEGIN { printf \"%.2f\", $TOTAL_HDD_WR_KB / 1024 / 1024 }")

    cat >> "$OUTPUT_FILE" <<EOF

============================================================
  DEVICE MONITOR SUMMARY
============================================================
  Duration: ${elapsed} s

--- Peak Values ---
  System CPU:   ${PEAK_CPU} %
  Process CPU:  ${PEAK_PCPU} %
  Mem Used:     ${PEAK_MEM} GB
EOF
    if [[ -n "$TARGET_PID" ]]; then
        echo "  Proc RSS:     ${PEAK_PROC_RSS} MB" >> "$OUTPUT_FILE"
    fi
    cat >> "$OUTPUT_FILE" <<EOF
  SSD Read:     ${PEAK_SSD_RD_IOPS} IOPS / ${PEAK_SSD_RD_KBPS} KB/s
  SSD Write:    ${PEAK_SSD_WR_IOPS} IOPS / ${PEAK_SSD_WR_KBPS} KB/s
  SSD Util:     ${PEAK_SSD_UTIL} %
  HDD Read:     ${PEAK_HDD_RD_IOPS} IOPS / ${PEAK_HDD_RD_KBPS} KB/s
  HDD Write:    ${PEAK_HDD_WR_IOPS} IOPS / ${PEAK_HDD_WR_KBPS} KB/s
  HDD Util:     ${PEAK_HDD_UTIL} %

--- Total IO Volume ---
  SSD Read:     ${ssd_rd_gb} GB
  SSD Write:    ${ssd_wr_gb} GB
  HDD Read:     ${hdd_rd_gb} GB
  HDD Write:    ${hdd_wr_gb} GB

--- Per-Device Detail ---
EOF
    for dev in "${SSD_DEVS[@]}"; do
        echo "  [SSD] $dev   peak_util=${PER_DEV_MAX_UTIL[$dev]:-0.00}%" >> "$OUTPUT_FILE"
    done
    for dev in "${HDD_DEVS[@]}"; do
        echo "  [HDD] $dev   peak_util=${PER_DEV_MAX_UTIL[$dev]:-0.00}%" >> "$OUTPUT_FILE"
    done

    echo "============================================================" >> "$OUTPUT_FILE"
}

trap 'write_summary; echo "[device_monitor] stopped."; exit 0' INT TERM

# ============================================================
# 采样循环
# ============================================================

while true; do
    sleep "$INTERVAL"

    # 如果指定了进程名但还没找到，每轮重试（测试可能晚于监控启动）
    if [[ -z "$TARGET_PID" && -n "$TARGET_NAME" ]]; then
        TARGET_PID=$(pgrep -nx "$TARGET_NAME" 2>/dev/null || true)
        if [[ -n "$TARGET_PID" ]]; then
            echo "[device_monitor] detected process '$TARGET_NAME' PID=$TARGET_PID"
            PREV_PCPU=($(read_proc_cpu "$TARGET_PID"))
        fi
    fi

    if [[ -n "$TARGET_PID" ]] && ! kill -0 "$TARGET_PID" 2>/dev/null; then
        echo "[device_monitor] target PID $TARGET_PID exited"
        write_summary
        echo "[device_monitor] stopped."
        exit 0
    fi

    NOW_SEC=$(date +%s.%N)
    ELAPSED_SINCE_START=$(awk "BEGIN { printf \"%.3f\", $NOW_SEC - $START_SEC }")
    ACTUAL_ELAPSED=$(awk "BEGIN { printf \"%.3f\", $NOW_SEC - $PREV_WALL_SEC }")
    PREV_WALL_SEC=$NOW_SEC
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')

    # === Disk: 读取原始数据 → 独立计算 IOPS/KBPS ===
    CUR_SSD=($(read_diskstats_group SSD_DEVS))
    CUR_HDD=($(read_diskstats_group HDD_DEVS))

    SSD_D=($(compute_disk_metrics \
        ${PREV_SSD[0]} ${PREV_SSD[1]} ${PREV_SSD[2]} ${PREV_SSD[3]} \
        ${PREV_SSD[4]} ${PREV_SSD[5]} ${PREV_SSD[6]} ${PREV_SSD[7]} \
        ${CUR_SSD[0]}  ${CUR_SSD[1]}  ${CUR_SSD[2]}  ${CUR_SSD[3]} \
        ${CUR_SSD[4]}  ${CUR_SSD[5]}  ${CUR_SSD[6]}  ${CUR_SSD[7]} \
        "$ACTUAL_ELAPSED"))
    HDD_D=($(compute_disk_metrics \
        ${PREV_HDD[0]} ${PREV_HDD[1]} ${PREV_HDD[2]} ${PREV_HDD[3]} \
        ${PREV_HDD[4]} ${PREV_HDD[5]} ${PREV_HDD[6]} ${PREV_HDD[7]} \
        ${CUR_HDD[0]}  ${CUR_HDD[1]}  ${CUR_HDD[2]}  ${CUR_HDD[3]} \
        ${CUR_HDD[4]}  ${CUR_HDD[5]}  ${CUR_HDD[6]}  ${CUR_HDD[7]} \
        "$ACTUAL_ELAPSED"))

    PREV_SSD=("${CUR_SSD[@]}")
    PREV_HDD=("${CUR_HDD[@]}")

    # === CPU: 读取原始数据 → 独立计算 ===
    CUR_CPU=($(read_cpustat))
    CPU_UTIL=$(compute_cpu_pct ${PREV_CPU[0]} ${PREV_CPU[1]} ${CUR_CPU[0]} ${CUR_CPU[1]})
    PREV_CPU=("${CUR_CPU[@]}")

    # === Memory ===
    CUR_MEM=($(read_meminfo))
    MEM_USED=$(mem_used_gb ${CUR_MEM[0]} ${CUR_MEM[1]})

    # === Per-device io_ticks ($13) for util ===
    declare -A CUR_SSD_WIO CUR_HDD_WIO
    snap_device_wio SSD_DEVS CUR_SSD_WIO
    snap_device_wio HDD_DEVS CUR_HDD_WIO

    PROC_CPU=0
    if [[ -n "$TARGET_PID" ]]; then
        CUR_PCPU=($(read_proc_cpu "$TARGET_PID"))
        PROC_CPU=$(proc_cpu_delta ${PREV_PCPU[0]} ${PREV_PCPU[1]} \
                                  ${CUR_PCPU[0]}  ${CUR_PCPU[1]} \
                                  "$ACTUAL_ELAPSED")
        PREV_PCPU=("${CUR_PCPU[@]}")
    fi

    PROC_RSS_MB=0
    if [[ -n "$TARGET_PID" ]]; then
        RSS_KB=$(read_process_rss "$TARGET_PID")
        RSS_KB=${RSS_KB:-0}
        PROC_RSS_MB=$(awk "BEGIN { printf \"%.2f\", $RSS_KB / 1024 }")
    fi

    # util 使用同一次 snap 的数据，不再重复读 /proc/diskstats
    # 直接调用（不使用 $(…)），让 PER_DEV_MAX_UTIL / PREV_*_WIO 的更新生效
    SSD_UTIL_MAX=0; SSD_UTIL_AVG=0
    HDD_UTIL_MAX=0; HDD_UTIL_AVG=0
    if [[ ${#SSD_DEVS[@]} -gt 0 ]]; then
        compute_utils SSD_DEVS PREV_SSD_WIO CUR_SSD_WIO "$ACTUAL_ELAPSED" SSD_UTIL_MAX SSD_UTIL_AVG
    fi
    if [[ ${#HDD_DEVS[@]} -gt 0 ]]; then
        compute_utils HDD_DEVS PREV_HDD_WIO CUR_HDD_WIO "$ACTUAL_ELAPSED" HDD_UTIL_MAX HDD_UTIL_AVG
    fi

    update_peaks "$CPU_UTIL" "$PROC_CPU" "$MEM_USED" "$PROC_RSS_MB" \
        "${SSD_D[0]}" "${SSD_D[1]}" "${SSD_D[2]}" "${SSD_D[3]}" "$SSD_UTIL_MAX" \
        "${HDD_D[0]}" "${HDD_D[1]}" "${HDD_D[2]}" "${HDD_D[3]}" "$HDD_UTIL_MAX" \
        "$ACTUAL_ELAPSED"

    printf "%s,%.3f,%.2f,%.2f,%.2f,%s,%s,%s,%s,%s,%.2f,%.2f,%s,%s,%s,%s,%.2f,%.2f\n" \
        "$TIMESTAMP" "$ELAPSED_SINCE_START" "$CPU_UTIL" "$PROC_CPU" "$MEM_USED" "$PROC_RSS_MB" \
        "${SSD_D[0]}" "${SSD_D[1]}" "${SSD_D[2]}" "${SSD_D[3]}" "$SSD_UTIL_MAX" "$SSD_UTIL_AVG" \
        "${HDD_D[0]}" "${HDD_D[1]}" "${HDD_D[2]}" "${HDD_D[3]}" "$HDD_UTIL_MAX" "$HDD_UTIL_AVG" \
        >> "$OUTPUT_FILE"
done
