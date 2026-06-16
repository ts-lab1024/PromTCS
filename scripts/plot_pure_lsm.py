#!/usr/bin/env python3
"""
plot_pure_lsm.py — 读取 PureLSM_test 输出的 pure_lsm_results.csv，生成折线图。

CSV 格式:
  batch_id,cumulative_samples,elapsed_ms,throughput_s,compaction_MB_written,total_compaction_MB

用法:
  python3 plot_pure_lsm.py <csv_file> [output.png]

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
out_path = sys.argv[2] if len(sys.argv) > 2 else 'pure_lsm_chart.png'

if not os.path.exists(csv_path):
    print(f"ERROR: file not found: {csv_path}")
    sys.exit(1)

# ============================================================
# 读取 CSV
# ============================================================
batches       = []
cum_samples   = []
elapsed_ms    = []
throughput    = []
comp_mb       = []
total_comp_mb = []

with open(csv_path, 'r') as f:
    reader = csv.DictReader(f)
    for row in reader:
        try:
            batches.append(int(row['batch_id']))
            cum_samples.append(float(row['cumulative_samples']))
            elapsed_ms.append(float(row['elapsed_ms']))
            throughput.append(float(row['throughput_s']))
            comp_mb.append(float(row['compaction_MB_written']))
            total_comp_mb.append(float(row['total_compaction_MB']))
        except (ValueError, KeyError):
            continue

if not batches:
    print("ERROR: no valid data rows in CSV")
    sys.exit(1)

n = len(batches)
x = batches

# 计算衍生数据
elapsed_s = [v / 1000.0 for v in elapsed_ms]
throughput_k = [v / 1000.0 for v in throughput]                # k samples/s
cum_samples_m = [v / 1_000_000.0 for v in cum_samples]         # M samples
comp_mb_per_batch = comp_mb

# ============================================================
# 绘图: 1x2 布局
# ============================================================
fig, axes = plt.subplots(1, 2, figsize=(16, 6))
fig.suptitle('PureLSM Write Benchmark', fontsize=16, fontweight='bold')

color_tp     = '#2ecc71'
color_comp_d = '#3498db'

# ---- 子图 1: Throughput (k samples/s) ----
ax = axes[0]
ax.fill_between(x, throughput_k, alpha=0.2, color=color_tp)
ax.plot(x, throughput_k, color=color_tp, linewidth=1.5, marker='o', markersize=4, label='Throughput')
ax.set_ylabel('k samples/s')
ax.set_xlabel('Batch')
ax.grid(True, alpha=0.3)
ax.legend(loc='upper right', fontsize=8)
ax.set_title('Throughput per Batch')

# ---- 子图 2: Compaction 写入量 ----
ax = axes[1]
ax.bar(x, comp_mb_per_batch, color=color_comp_d, alpha=0.5, width=0.6, label='Per batch')
ax.set_ylabel('MB')
ax.set_xlabel('Batch')
ax.grid(True, alpha=0.3, axis='y')
ax.legend(loc='upper left', fontsize=8)
ax.set_title('Compaction MB Written')

# ---- 统计摘要文本 ----
if n > 0:
    avg_tp = sum(throughput) / n
    avg_elap_ms = sum(elapsed_ms) / n
    total_comp = total_comp_mb[-1]
    total_samp = cum_samples_m[-1]
    summary = (
        f"Average throughput: {avg_tp:,.0f} samp/s\n"
        f"Average batch time: {avg_elap_ms:.1f} ms\n"
        f"Total compaction:   {total_comp:.1f} MB\n"
        f"Total samples:       {total_samp:.1f} M"
    )
    fig.text(0.02, 0.01, summary, fontsize=9, family='monospace',
             verticalalignment='bottom', bbox=dict(boxstyle='round', facecolor='wheat', alpha=0.5))

plt.tight_layout(rect=[0, 0.08, 1, 0.95])
plt.savefig(out_path, dpi=150, bbox_inches='tight')
print(f"Chart saved to: {out_path}")
