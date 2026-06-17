# PromTCS Artifact Evaluation README

## What's Provided

- **Docker image** (Dockerfile) — self-contained environment with all dependencies pre-built.
- **Source code** — C++ storage engine, Go-based client benchmark tools.

## Hardware Requirements

| Resource | Minimum | Recommended |
|---|---|---|
| CPU cores | 8 | 32+ |
| RAM | 32 GB | 64+ GB |
| SSD | 50 GB free | 200+ GB free |
| HDD | 1 TB free | 10 TB free |
| OS | Ubuntu 20.04 | Ubuntu 20.04 |

## Quick Test — Storage Engine


### Configuration

For builds, edit `config.yaml` to set storage paths:

```yaml
# LSM-tree storage (HDD)
db_path:  /path/to/data/promtcs_db/

# log (SSD)
log_path: /path/to/data/promtcs_log

# TreeSeries (SSD)
tree_series_path: /path/to/ssd/tree_series
tree_series_info_path: /path/to/ssd/tree_series_info
```

### Build & Run

**Docker (Recommended):**

```bash
# Via browser
# Visit https://zenodo.org/records/20728578 and download promtcs-ae-fast27.tar.gz

# Via command line
wget https://zenodo.org/records/20728578/files/promtcs-ae-fast27.tar.gz

gunzip -c promtcs-ae-fast27.tar.gz | docker load

mkdir -p /tmp/promtcs-data
docker run -it --init --rm -p 9966:9966 -v /tmp/promtcs-data:/data promtcs-ae
# Now inside the container at /opt/PromTCS/build/.
# Run: ./PromTCS_test 4 100000 30 30
```

**Native:**

```bash
sudo apt-get install -y build-essential cmake git \
    libboost-all-dev libyaml-cpp-dev libprotobuf-dev protobuf-compiler \
    libsnappy-dev libgoogle-perftools-dev unzip

cd test && unzip -o devops100000.txt.zip && cd ..
mkdir -p build && cd build && cmake .. && make PromTCS_test -j$(nproc)
./PromTCS_test 4 100000 30 30
```

Parameters: 4 threads, 100,000 time series, 30-second interval, 30 data tuples per series. Each tuple contains 32 samples (hardcoded), so each series has 30 × 32 = 960 samples.

### Expected Output

You should see insertion throughput statistics, memory usage (VM/RSS), and query latency for 8 query patterns.


## End-to-End Test — Remote Storage Service

This test verifies the full pipeline: `prom-client` (Prometheus-compatible workload generator) sends write/query requests to `ForestRemoteStorage` (C++ remote database server) over HTTP.

### Architecture

```
prom-client (Go)                    ForestRemoteStorage (C++)
===========================================================
bin/storage ───> POST /insert ──>   TreeRemoteDB (port 9966)
bin/query   ───> POST /query  ──>
Protocol: Prometheus Remote Write/Read (Protobuf + Snappy)
```

### Step 1 — Start the Server

```bash
cd build && make ForestRemoteStorage -j$(nproc)
./ForestRemoteStorage
# Output: "Successfully start remote storage service."
```

The server listens on port 9966.

### Step 2 — Build prom-client (requires Go 1.23+)

```bash
cd /path/to/prom-client
make all -j$(nproc)
# Produces bin/storage and bin/query
```

### Step 3 — Write Benchmark

```bash
./bin/storage -addr http://localhost:9966 -series 10000 -samples 50 -send 64
```

| Flag | Default | Description |
|---|---|---|
| `-series` | 10000 | Unique time series |
| `-samples` | 50 | Samples per series per request |
| `-send` | 64 | Number of requests |

### Step 4 — Query Benchmark

```bash
./bin/query -addr http://localhost:9966 -repeat=1000 -type=5-1-1 -threads=4
./bin/query -addr http://localhost:9966 -repeat=1000 -type=all -threads=4 -output=query_result.txt
```

| Flag | Default | Description |
|---|---|---|
| `-type` | `1-8-1` | `1-1-1`, `1-8-1`, `5-1-1`, `5-8-1`, `all`, etc. |
| `-repeat` | 10 | Repetitions |
| `-threads` | 1 | Concurrent goroutines |

Successful output shows per-request latency for writes and avg/P50/P90/P99 for queries.

## Troubleshooting

| Symptom | Solution |
|---|---|
| `No such file` for test data | `cd test && unzip devops100000.txt.zip` |
| Build fails with `yaml-cpp` | `sudo apt install libyaml-cpp-dev` |
| Path errors | Pre-create directories in `config.yaml` |

## Baselines
ForestTI: https://github.com/naivewong/forestti

TimeUnion: https://github.com/naivewong/timeunion

Prometheus tsdb: https://github.com/prometheus-junkyard/tsdb

Apache IoTDB: https://github.com/apache/iotdb

InfluxDB https://github.com/influxdata/influxdb

TimescaleDB https://github.com/timescale/timescaledb
