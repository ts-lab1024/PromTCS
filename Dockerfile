# PromTCS Artifact Evaluation Docker Image
# Build:  docker build -t promtcs-ae .
# Export: docker save promtcs-ae | gzip > promtcs-ae-fast27.tar.gz
# Import: gunzip -c promtcs-ae-fast27.tar.gz | docker load
# Run:    docker run -it --init --rm -v /path/to/data:/data promtcs-ae

FROM ubuntu:20.04

ENV DEBIAN_FRONTEND=noninteractive \
    TZ=Etc/UTC

# ============================================================
# 1. Install system dependencies
# ============================================================
RUN apt-get update && apt-get install -y \
    build-essential \
    cmake \
    git \
    libboost-all-dev \
    libyaml-cpp-dev \
    libprotobuf-dev \
    protobuf-compiler \
    libsnappy-dev \
    libgoogle-perftools-dev \
    unzip \
    && rm -rf /var/lib/apt/lists/*

# ============================================================
# 2. Copy project source
# ============================================================
COPY . /opt/PromTCS
WORKDIR /opt/PromTCS

# ============================================================
# 3. Prepare test data
# ============================================================
RUN cd /opt/PromTCS/test \
    && if [ -f devops100000.txt.zip ]; then unzip -o devops100000.txt.zip; fi

# ============================================================
# 4. Create container-adapted config.yaml
# ============================================================
RUN cat > /opt/PromTCS/config.yaml <<'EOF'
# TreeHead
# db_path:  LevelDB (LSM-tree) directory; placed on HDD in the paper, can use SSD
# log_path: WAL directory for TreeHead; placed on SSD in the paper
db_path:  /data/promtcs_db/
log_path: /data/promtcs_log
wal_num: 5

# TreeSeries
tree_series_path: /data/tree_series
tree_series_info_path: /data/tree_series_info
tree_series_thread_pool_size: 32
max_slab_memory: 8  # GB
read_buffer_size: 256 # MB
max_series_num: 16
slab_item_size: 256   # Byte
chunk_size: 248 # Byte
slab_size: 4  # KB

# LevelDB Options
leveldb_max_imm_num: 3
leveldb_write_buffer_size: 256  # MB
leveldb_max_file_size: 256  # MB
EOF

# ============================================================
# 5. Build PromTCS_test and ForestRemoteStorage
# ============================================================
RUN mkdir -p build && cd build \
    && cmake .. -DCMAKE_BUILD_TYPE=Release \
    && make PromTCS_test ForestRemoteStorage -j$(nproc)

# ============================================================
# 6. Default command: interactive shell
# Usage examples:
#   docker run -it promtcs-ae                          # interactive bash
#   docker run --init --rm -v /data:/data promtcs-ae ./PromTCS_test 32 1000000 30 90
#   docker run --init --rm -p 9966:9966 -v /data:/data promtcs-ae ./ForestRemoteStorage
# ============================================================
WORKDIR /opt/PromTCS/build
CMD ["/bin/bash"]
