#!/usr/bin/env python3
"""
plot_monitor.py — 读取 device_monitor.sh 输出的 CSV，生成折线图。

用法:
  python3 plot_monitor.py <csv_file> [output.png]

依赖:
  pip3 install matplotlib
"""

import sys, csv, os

try:
    import matplotlib
    matplotlib.use('Agg')
    import matplotlib.pyplot as plt
    import matplotlib.ticker as mticker
except ImportError:
    print("ERROR: matplotlib 未安装。请执行: pip3 install matplotlib")
    sys.exit(1)

if len(sys.argv) < 2:
    print(__doc__)
    sys.exit(1)

csv_path = sys.argv[1]
out_path = sys.argv[2] if len(sys.argv) > 2 else 'monitor_chart.png'

if not os.path.exists(csv_path):
    print(f"ERROR: file not found: {csv_path}")
    sys.exit(1)

# ============================================================
# 读取 CSV
# ============================================================
cols = {
    'elapsed':       [],
    'cpu':           [],
    'proc_cpu':      [],
    'mem_gb':        [],
    'proc_rss':      [],
    'ssd_rd_kbps':   [],
    'ssd_wr_kbps':   [],
    'ssd_util':      [],
    'ssd_util_avg':  [],
    'hdd_rd_kbps':   [],
    'hdd_wr_kbps':   [],
    'hdd_util':      [],
    'hdd_util_avg':  [],
}

with open(csv_path, 'r') as f:
    reader = csv.DictReader(f)
    for row in reader:
        if not row.get('elapsed_s'):
            continue
        try:
            cols['elapsed'].append(float(row['elapsed_s']))
            cols['cpu'].append(float(row['cpu_pct']))
            cols['proc_cpu'].append(float(row.get('proc_cpu_pct_total', 0) or 0))
            cols['mem_gb'].append(float(row['mem_used_gb']))
            cols['proc_rss'].append(float(row.get('proc_rss_mb', 0) or 0))
            cols['ssd_rd_kbps'].append(float(row['ssd_rd_kbps']))
            cols['ssd_wr_kbps'].append(float(row['ssd_wr_kbps']))
            cols['ssd_util'].append(float(row.get('ssd_util_pct', 0) or 0))
            cols['ssd_util_avg'].append(float(row.get('ssd_avg_util_pct', 0) or 0))
            cols['hdd_rd_kbps'].append(float(row['hdd_rd_kbps']))
            cols['hdd_wr_kbps'].append(float(row['hdd_wr_kbps']))
            cols['hdd_util'].append(float(row.get('hdd_util_pct', 0) or 0))
            cols['hdd_util_avg'].append(float(row.get('hdd_avg_util_pct', 0) or 0))
        except (ValueError, KeyError):
            continue

if not cols['elapsed']:
    print("ERROR: no valid data rows in CSV")
    sys.exit(1)

t = cols['elapsed']
has_proc_cpu   = any(v > 0 for v in cols['proc_cpu'])
has_proc_rss   = any(v > 0 for v in cols['proc_rss'])
has_ssd_avg    = any(v > 0 for v in cols['ssd_util_avg'])
has_hdd_avg    = any(v > 0 for v in cols['hdd_util_avg'])

# ============================================================
# 绘图: 2x2 布局
# ============================================================
fig, axes = plt.subplots(2, 2, figsize=(16, 10))
fig.suptitle('System Resource Monitor', fontsize=16, fontweight='bold')

color_cpu    = '#e74c3c'
color_pcpu   = '#c0392b'
color_mem    = '#8e44ad'
color_read   = '#3498db'
color_write  = '#2ecc71'
color_util   = '#e67e22'
color_avg    = '#f39c12'

# ---- 子图 1: CPU 利用率 (双 Y 轴) ----
ax = axes[0, 0]
ax.fill_between(t, cols['cpu'], alpha=0.2, color=color_cpu)
ax.plot(t, cols['cpu'], color=color_cpu, linewidth=1.0, label='System CPU %')
ax.set_ylabel('System CPU %', color=color_cpu)
ax.set_ylim(0, 105)
ax.yaxis.set_major_formatter(mticker.FormatStrFormatter('%.0f%%'))
ax.tick_params(axis='y', colors=color_cpu)
ax.grid(True, alpha=0.3)

if has_proc_cpu:
    # 将 process CPU 转换为物理核数（除以 100），避免 Y 轴比例失衡
    proc_cores = [v / 100.0 for v in cols['proc_cpu']]
    ax2_cpu = ax.twinx()
    ax2_cpu.plot(t, proc_cores, color=color_pcpu, linewidth=1.0, linestyle='--', label='Process CPU (cores)')
    ax2_cpu.set_ylabel('Process CPU (cores)', color=color_pcpu)
    ax2_cpu.tick_params(axis='y', colors=color_pcpu)
    # 合并图例
    lines1, labels1 = ax.get_legend_handles_labels()
    lines2, labels2 = ax2_cpu.get_legend_handles_labels()
    ax.legend(lines1 + lines2, labels1 + labels2, loc='upper left', fontsize=8)

ax.set_title('CPU Utilization')

# ---- 子图 2: 内存 ----
ax = axes[0, 1]
ax.fill_between(t, cols['mem_gb'], alpha=0.2, color=color_mem)
ax.plot(t, cols['mem_gb'], color=color_mem, linewidth=1.0, label='System Used (GB)')
if has_proc_rss:
    rss_gb = [v / 1024.0 for v in cols['proc_rss']]
    ax.plot(t, rss_gb, color=color_cpu, linewidth=1.0, linestyle='--', label='Process RSS (GB)')
    ax.legend(loc='upper left', fontsize=8)
ax.set_ylabel('GB')
ax.grid(True, alpha=0.3)
ax.set_title('Memory Usage')

# ---- 子图 3: SSD ----
ax = axes[1, 0]
ssd_rd_mb = [v / 1024.0 for v in cols['ssd_rd_kbps']]
ssd_wr_mb = [v / 1024.0 for v in cols['ssd_wr_kbps']]
ax.fill_between(t, ssd_rd_mb, alpha=0.15, color=color_read)
ax.plot(t, ssd_rd_mb, color=color_read, linewidth=1.0, label='Read MB/s')
ax.fill_between(t, ssd_wr_mb, alpha=0.15, color=color_write)
ax.plot(t, ssd_wr_mb, color=color_write, linewidth=1.0, label='Write MB/s')
ax.set_ylabel('MB/s')
ax.legend(loc='upper left', fontsize=8)
ax.grid(True, alpha=0.3)

ax2 = ax.twinx()
ax2.plot(t, cols['ssd_util'], color=color_util, linewidth=0.8, alpha=0.9, label='Util max %')
if has_ssd_avg:
    ax2.plot(t, cols['ssd_util_avg'], color=color_avg, linewidth=0.6, alpha=0.5, linestyle=':', label='Util avg %')
ax2.set_ylabel('Util %', color=color_util)
ax2.set_ylim(0, 105)
ax2.yaxis.set_major_formatter(mticker.FormatStrFormatter('%.0f%%'))
ax2.legend(loc='upper right', fontsize=7)
ax.set_title('SSD IO')

# ---- 子图 4: HDD ----
ax = axes[1, 1]
hdd_rd_mb = [v / 1024.0 for v in cols['hdd_rd_kbps']]
hdd_wr_mb = [v / 1024.0 for v in cols['hdd_wr_kbps']]
ax.fill_between(t, hdd_rd_mb, alpha=0.15, color=color_read)
ax.plot(t, hdd_rd_mb, color=color_read, linewidth=1.0, label='Read MB/s')
ax.fill_between(t, hdd_wr_mb, alpha=0.15, color=color_write)
ax.plot(t, hdd_wr_mb, color=color_write, linewidth=1.0, label='Write MB/s')
ax.set_ylabel('MB/s')
ax.set_xlabel('Time (s)')
ax.legend(loc='upper left', fontsize=8)
ax.grid(True, alpha=0.3)

ax2 = ax.twinx()
ax2.plot(t, cols['hdd_util'], color=color_util, linewidth=0.8, alpha=0.9, label='Util max %')
if has_hdd_avg:
    ax2.plot(t, cols['hdd_util_avg'], color=color_avg, linewidth=0.6, alpha=0.5, linestyle=':', label='Util avg %')
ax2.set_ylabel('Util %', color=color_util)
ax2.set_ylim(0, 105)
ax2.yaxis.set_major_formatter(mticker.FormatStrFormatter('%.0f%%'))
ax2.legend(loc='upper right', fontsize=7)
ax.set_title('HDD IO')

plt.tight_layout()
plt.savefig(out_path, dpi=150, bbox_inches='tight')
print(f"Chart saved to: {out_path}")
