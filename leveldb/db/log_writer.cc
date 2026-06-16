// Copyright (c) 2011 The LevelDB Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file. See the AUTHORS file for names of contributors.

#include "db/log_writer.h"

#include <cstdint>

#include "leveldb/env.h"

#include "util/coding.h"
#include "util/crc32c.h"

#include "base/Endian.hpp"
#include "tsdbutil/tsdbutils.hpp"

namespace leveldb {
namespace log {

std::string series(const std::vector<RefSeries>& s) {
  std::string result;
  series(s, &result);
  return result;
}
void series(const std::vector<RefSeries>& series, std::string* result) {
  result->push_back(kSeries);

  for (const RefSeries& r : series) {
    PutFixed64BE(result, r.ref);
    PutFixed64BE(result, r.flushed_txn);
    PutFixed64BE(result, r.log_clean_txn);
    const tsdb::label::Labels* lptr = &r.lset;
    if (r.lset_ptr) lptr = r.lset_ptr;
    PutVarint32(result, lptr->size());

    for (const ::tsdb::label::Label& l : *lptr) {
      PutLengthPrefixedSlice(result, l.label);
      PutLengthPrefixedSlice(result, l.value);
    }
  }
}

void series(const ::tsdb::tsdbutil::RefSeries& r, std::string* result) {
  result->push_back(kSeries);

  PutFixed64BE(result, r.ref);
  PutFixed64BE(result, r.flushed_txn);
  PutFixed64BE(result, r.log_clean_txn);
  const tsdb::label::Labels* lptr = &r.lset;
  if (r.lset_ptr) lptr = r.lset_ptr;
  PutVarint32(result, lptr->size());

  for (const ::tsdb::label::Label& l : *lptr) {
    PutLengthPrefixedSlice(result, l.label);
    PutLengthPrefixedSlice(result, l.value);
  }
}

void series_without_labels(uint64_t ref, int64_t flushed_txn, int64_t log_clean_txn, std::string* result) {
  result->push_back(kSeries);

  PutFixed64BE(result, ref);
  PutFixed64BE(result, flushed_txn);
  PutFixed64BE(result, log_clean_txn);
}

void series_without_labels(const ::tsdb::tsdbutil::RefSeries& r, std::string* result) {
  result->push_back(kSeries);

  PutFixed64BE(result, r.ref);
  PutFixed64BE(result, r.flushed_txn);
  PutFixed64BE(result, r.log_clean_txn);
}

void series(const std::vector<::tsdb::tsdbutil::RefSeries>& series,
            std::string* result) {
  result->push_back(kSeries);

  for (const ::tsdb::tsdbutil::RefSeries& r : series) {
    PutFixed64BE(result, r.ref);
    PutFixed64BE(result, r.flushed_txn);
    PutFixed64BE(result, r.log_clean_txn);
    const tsdb::label::Labels* lptr = &r.lset;
    if (r.lset_ptr) lptr = r.lset_ptr;
    PutVarint32(result, lptr->size());

    for (const ::tsdb::label::Label& l : *lptr) {
      PutLengthPrefixedSlice(result, l.label);
      PutLengthPrefixedSlice(result, l.value);
    }
  }
}

std::string group(const RefGroup& g) {
  std::string result;
  group(g, &result);
  return result;
}
void group(const RefGroup& g, std::string* result) {
  result->push_back(kGroup);

  PutFixed64BE(result, g.ref);
  PutVarint32(result, g.group_lset.size());
  for (const ::tsdb::label::Label& l : g.group_lset) {
    PutLengthPrefixedSlice(result, l.label);
    PutLengthPrefixedSlice(result, l.value);
  }

  PutVarint32(result, g.individual_lsets.size());
  for (const ::tsdb::label::Labels& lset : g.individual_lsets) {
    PutVarint32(result, lset.size());
    for (const ::tsdb::label::Label& l : lset) {
      PutLengthPrefixedSlice(result, l.label);
      PutLengthPrefixedSlice(result, l.value);
    }
  }
}

std::string samples(const std::vector<RefSample>& s) {
  std::string result;
  samples(s, &result);
  return result;
}
void samples(const std::vector<RefSample>& samples, std::string* result) {
  result->push_back(kSample);

  PutFixed64BE(result, samples[0].ref);
  PutFixed64BE(result, samples[0].logical_id);
  PutFixed64BE(result, samples[0].t);
  PutFixed64BE(result, ::tsdb::base::encode_double(samples[0].v));
  PutFixed64BE(result, samples[0].txn);
  for (int i = 1; i < samples.size(); i++) {
    PutVarint64(result,
                ::tsdb::base::encode_signed_varint(
                    (int64_t)(samples[i].ref) - (int64_t)(samples[i - 1].ref)));
    PutVarint64(result, ::tsdb::base::encode_signed_varint(
                            (int64_t)(samples[i].logical_id) -
                            (int64_t)(samples[i - 1].logical_id)));
    PutVarint64(result, ::tsdb::base::encode_signed_varint(samples[i].t -
                                                           samples[i - 1].t));
    PutFixed64BE(result, ::tsdb::base::encode_double(samples[i].v));
    PutVarint64(result, ::tsdb::base::encode_signed_varint(samples[i].txn -
                                                           samples[i - 1].txn));
  }
}

std::string samples(const std::vector<::tsdb::tsdbutil::RefSample>& s) {
  std::string result;
  samples(s, &result);
  return result;
}
void samples(const std::vector<::tsdb::tsdbutil::RefSample>& samples,
             std::string* result) {
  result->push_back(kSample);

  PutFixed64BE(result, samples[0].ref);
  PutFixed64BE(result, samples[0].logical_id);
  PutFixed64BE(result, samples[0].t);
  PutFixed64BE(result, ::tsdb::base::encode_double(samples[0].v));
  PutFixed64BE(result, samples[0].txn);
  for (int i = 1; i < samples.size(); i++) {
    PutVarint64(result,
                ::tsdb::base::encode_signed_varint(
                    (int64_t)(samples[i].ref) - (int64_t)(samples[i - 1].ref)));
    PutVarint64(result, ::tsdb::base::encode_signed_varint(
                            (int64_t)(samples[i].logical_id) -
                            (int64_t)(samples[i - 1].logical_id)));
    PutVarint64(result, ::tsdb::base::encode_signed_varint(samples[i].t -
                                                           samples[i - 1].t));
    PutFixed64BE(result, ::tsdb::base::encode_double(samples[i].v));
    PutVarint64(result, ::tsdb::base::encode_signed_varint(samples[i].txn -
                                                           samples[i - 1].txn));
  }
}

std::string group_sample(const RefGroupSample& g) {
  std::string result;
  group_sample(g, &result);
  return result;
}
void group_sample(const RefGroupSample& g, std::string* result) {
  result->push_back(kGroupSample);

  PutFixed64BE(result, g.ref);
  if (g.individual_lsets.empty()) {
    result->push_back(1);
    if (g.slots.empty())
      PutVarint32(result, ((uint32_t)(1) << 31) | (uint32_t)(g.v.size()));
    else
      PutVarint32(result, g.slots.size());
    for (size_t i = 0; i < g.slots.size(); i++) PutVarint32(result, g.slots[i]);
  } else {
    result->push_back(2);
    PutVarint32(result, g.individual_lsets.size());
    for (const ::tsdb::label::Labels& lset : g.individual_lsets) {
      PutVarint32(result, lset.size());
      for (const ::tsdb::label::Label& l : lset) {
        PutLengthPrefixedSlice(result, l.label);
        PutLengthPrefixedSlice(result, l.value);
      }
    }
  }
  PutFixed64BE(result, g.t);
  for (size_t i = 0; i < g.v.size(); i++)
    PutFixed64BE(result, ::tsdb::base::encode_double(g.v[i]));
  PutFixed64BE(result, g.txn);
}

std::string flush(const RefFlush& f) {
  std::string result;
  flush(f, &result);

  return result;
}
void flush(const RefFlush& f, std::string* result) {
  result->push_back(kFlush);

  PutFixed64BE(result, f.ref);
  PutFixed64BE(result, f.logical_id);
  PutFixed64BE(result, f.txn);
}

std::string treeSamples(const std::vector<tsdb::tsdbutil::TreeRefSample*>& samples) {
    std::string result;
    treeSamples(samples, &result);
    return result;
}

std::string treeSamples(tsdb::tsdbutil::TreeRefSample* samples, uint32_t item_num) {
    std::string result;
    treeSamples(samples, item_num, &result);
    return result;
}

void treeSamples(tsdb::tsdbutil::TreeRefSample* samples, uint32_t item_num, std::string* result) {
    result->push_back(kSample);

    PutFixed16(result, samples->mid);
    PutFixed64BE(result, samples->sgid);
    PutFixed64BE(result, samples->t);
    PutFixed64BE(result, tsdb::base::encode_double(samples->v));
    PutFixed64BE(result, samples->txn);
    for (int i = 1; i < item_num; i++) {
        tsdb::tsdbutil::TreeRefSample* s1 = samples + i;
        tsdb::tsdbutil::TreeRefSample* s2 = samples + i - 1;
        PutVarint64(result, tsdb::base::encode_signed_varint((int64_t)(s1->mid) - (int64_t)(s2->mid)));
        PutVarint64(result, tsdb::base::encode_signed_varint((int64_t)(s1->sgid) - (int64_t)(s2->sgid)));
        PutVarint64(result, tsdb::base::encode_signed_varint(s1->t - s2->t));
        PutFixed64BE(result, tsdb::base::encode_double(s1->v));
        PutVarint64(result, tsdb::base::encode_signed_varint(s1->txn - s2->txn));
    }
}

void treeSamples(const std::vector<tsdb::tsdbutil::TreeRefSample*>& samples, std::string* result) {
    result->push_back(kSample);

    PutFixed16(result, samples[0]->mid);
    PutFixed64BE(result, samples[0]->sgid);
    PutFixed64BE(result, samples[0]->t);
    PutFixed64BE(result, tsdb::base::encode_double(samples[0]->v));
    PutFixed64BE(result, samples[0]->txn);
    for (int i = 1; i < samples.size(); i++) {
        PutVarint64(result, tsdb::base::encode_signed_varint((int64_t)(samples[i]->mid) - (int64_t)(samples[i-1]->mid)));
        PutVarint64(result, tsdb::base::encode_signed_varint((int64_t)(samples[i]->sgid) - (int64_t)(samples[i-1]->sgid)));
        PutVarint64(result, tsdb::base::encode_signed_varint(samples[i]->t - samples[i-1]->t));
        PutFixed64BE(result, tsdb::base::encode_double(samples[i]->v));
        PutVarint64(result, tsdb::base::encode_signed_varint(samples[i]->txn - samples[i-1]->txn));
    }
}

std::string treeSamples(const std::vector<tsdb::tsdbutil::TreeRefSample>& samples) {
    std::string result;
    treeSamples(samples, &result);
    return result;
}

void treeSamples(const std::vector<tsdb::tsdbutil::TreeRefSample>& samples, std::string* result) {
    result->push_back(kSample);

    PutFixed16(result, samples[0].mid);
    PutFixed64BE(result, samples[0].sgid);
    PutFixed64BE(result, samples[0].t);
    PutFixed64BE(result, tsdb::base::encode_double(samples[0].v));
    PutFixed64BE(result, samples[0].txn);
    for (int i = 1; i < samples.size(); i++) {
        PutVarint64(result, tsdb::base::encode_signed_varint((int64_t)(samples[i].mid) - (int64_t)(samples[i-1].mid)));
        PutVarint64(result, tsdb::base::encode_signed_varint((int64_t)(samples[i].sgid) - (int64_t)(samples[i-1].sgid)));
        PutVarint64(result, tsdb::base::encode_signed_varint(samples[i].t - samples[i-1].t));
        PutFixed64BE(result, tsdb::base::encode_double(samples[i].v));
        PutVarint64(result, tsdb::base::encode_signed_varint(samples[i].txn - samples[i-1].txn));
    }
}

// ============================================================================
// Gorilla-style bit-stream compression for WAL tree samples.
// Based on Facebook's Gorilla paper (VLDB 2015):
//   - Timestamp (t): delta-of-delta encoding
//   - Value (v): XOR compression with adaptive window
//   - mid/sgid: same-value check (1-bit when unchanged)
//   - txn: same-delta check (1-bit when delta unchanged)
// ============================================================================

class GorillaBitWriter {
private:
    std::string* buf_;
    uint8_t cur_byte_;
    int bit_pos_;  // 0-7, next bit position (MSB first)

public:
    explicit GorillaBitWriter(std::string* buf)
        : buf_(buf), cur_byte_(0), bit_pos_(0) {}

    void writeBit(int bit) {
        if (bit) cur_byte_ |= (1 << (7 - bit_pos_));
        if (++bit_pos_ == 8) {
            buf_->push_back(static_cast<char>(cur_byte_));
            cur_byte_ = 0;
            bit_pos_ = 0;
        }
    }

    void writeBits(uint64_t value, int num_bits) {
        for (int i = num_bits - 1; i >= 0; i--) {
            writeBit((value >> i) & 1);
        }
    }

    void flushToByte() {
        if (bit_pos_ > 0) {
            buf_->push_back(static_cast<char>(cur_byte_));
            cur_byte_ = 0;
            bit_pos_ = 0;
        }
    }

    void writeUnsignedVarint(uint64_t value) {
        flushToByte();
        do {
            uint8_t byte = value & 0x7F;
            value >>= 7;
            if (value) byte |= 0x80;
            buf_->push_back(static_cast<char>(byte));
        } while (value);
    }

    void writeSignedVarint(int64_t value) {
        uint64_t uval = static_cast<uint64_t>(
            value < 0 ? ~(value << 1) : (value << 1));
        writeUnsignedVarint(uval);
    }

    void writeFixed64BE(uint64_t value) {
        flushToByte();
        char b[8];
        EncodeFixed64BE(b, value);
        buf_->append(b, 8);
    }

    void writeFixed16(uint16_t value) {
        flushToByte();
        char b[2];
        EncodeFixed16(b, value);
        buf_->append(b, 2);
    }
};

void treeSamplesGorilla(tsdb::tsdbutil::TreeRefSample* samples, uint32_t item_num,
                        std::string* result) {
    if (item_num == 0) return;

    result->push_back(static_cast<char>(kSampleGorilla));
    PutVarint32(result, item_num);  // sample count, so decoder knows when to stop
    GorillaBitWriter bw(result);

    // --- Sample 0: full encoding ---
    tsdb::tsdbutil::TreeRefSample* s0 = samples;
    bw.writeFixed16(s0->mid);
    bw.writeFixed64BE(s0->sgid);
    bw.writeFixed64BE(static_cast<uint64_t>(s0->t));
    bw.writeFixed64BE(tsdb::base::encode_double(s0->v));
    bw.writeFixed64BE(static_cast<uint64_t>(s0->txn));

    if (item_num == 1) {
        bw.flushToByte();
        return;
    }

    // Tracking state for delta/delta-of-delta
    uint16_t prev_mid = s0->mid;
    uint64_t prev_sgid = s0->sgid;
    int64_t  prev_t = s0->t;
    uint64_t prev_v = tsdb::base::encode_double(s0->v);
    int64_t  prev_txn = s0->txn;

    int64_t prev_t_delta = 0;
    int64_t prev_txn_delta = 0;

    int prev_leading_zeros = 64;
    int prev_trailing_zeros = 0;

    for (uint32_t i = 1; i < item_num; i++) {
        tsdb::tsdbutil::TreeRefSample* s = samples + i;

        // mid: same value check
        if (s->mid == prev_mid) {
            bw.writeBit(0);
        } else {
            bw.writeBit(1);
            bw.writeSignedVarint(static_cast<int64_t>(s->mid) -
                                 static_cast<int64_t>(prev_mid));
            prev_mid = s->mid;
        }

        // sgid: same value check
        if (s->sgid == prev_sgid) {
            bw.writeBit(0);
        } else {
            bw.writeBit(1);
            bw.writeSignedVarint(static_cast<int64_t>(s->sgid) -
                                 static_cast<int64_t>(prev_sgid));
            prev_sgid = s->sgid;
        }

        // t: Gorilla delta-of-delta
        int64_t t_delta = s->t - prev_t;
        int64_t dod = t_delta - prev_t_delta;
        if (dod == 0) {
            bw.writeBit(0);
        } else {
            bw.writeBit(1);
            bw.writeSignedVarint(dod);
        }
        prev_t_delta = t_delta;
        prev_t = s->t;

        // v: Gorilla XOR compression
        uint64_t cur_v = tsdb::base::encode_double(s->v);
        uint64_t xor_val = cur_v ^ prev_v;
        if (xor_val == 0) {
            bw.writeBit(0);
        } else {
            bw.writeBit(1);
            int leading_zeros = __builtin_clzll(xor_val);
            int trailing_zeros = __builtin_ctzll(xor_val);

            if (leading_zeros >= prev_leading_zeros &&
                trailing_zeros >= prev_trailing_zeros) {
                bw.writeBit(0);
                int meaningful_bits = 64 - prev_leading_zeros - prev_trailing_zeros;
                bw.writeBits(xor_val >> prev_trailing_zeros, meaningful_bits);
            } else {
                bw.writeBit(1);
                bw.writeBits(static_cast<uint64_t>(leading_zeros), 5);
                int meaningful_bits = 64 - leading_zeros - trailing_zeros;
                bw.writeBits(static_cast<uint64_t>(meaningful_bits - 1), 6);
                bw.writeBits(xor_val >> trailing_zeros, meaningful_bits);
                prev_leading_zeros = leading_zeros;
                prev_trailing_zeros = trailing_zeros;
            }
        }
        prev_v = cur_v;

        // txn: same delta check
        int64_t txn_delta = s->txn - prev_txn;
        if (txn_delta == prev_txn_delta) {
            bw.writeBit(0);
        } else {
            bw.writeBit(1);
            bw.writeSignedVarint(txn_delta);
        }
        prev_txn_delta = txn_delta;
        prev_txn = s->txn;
    }

    bw.flushToByte();
}

std::string treeSamplesGorilla(tsdb::tsdbutil::TreeRefSample* samples,
                               uint32_t item_num) {
    std::string result;
    treeSamplesGorilla(samples, item_num, &result);
    return result;
}

std::string treeSamplesGorilla(
    const std::vector<tsdb::tsdbutil::TreeRefSample*>& samples) {
    std::string result;
    // Pack vector elements into a single encode call via pointer+count overload
    uint32_t n = static_cast<uint32_t>(samples.size());
    if (n == 0) return result;

    result.push_back(static_cast<char>(kSampleGorilla));
    PutVarint32(&result, n);
    GorillaBitWriter bw(&result);

    tsdb::tsdbutil::TreeRefSample* s0 = samples[0];
    bw.writeFixed16(s0->mid);
    bw.writeFixed64BE(s0->sgid);
    bw.writeFixed64BE(static_cast<uint64_t>(s0->t));
    bw.writeFixed64BE(tsdb::base::encode_double(s0->v));
    bw.writeFixed64BE(static_cast<uint64_t>(s0->txn));

    if (n == 1) { bw.flushToByte(); return result; }

    uint16_t prev_mid = s0->mid;
    uint64_t prev_sgid = s0->sgid;
    int64_t  prev_t = s0->t;
    uint64_t prev_v = tsdb::base::encode_double(s0->v);
    int64_t  prev_txn = s0->txn;
    int64_t prev_t_delta = 0, prev_txn_delta = 0;
    int prev_lz = 64, prev_tz = 0;

    for (uint32_t i = 1; i < n; i++) {
        tsdb::tsdbutil::TreeRefSample* s = samples[i];

        if (s->mid == prev_mid) { bw.writeBit(0); }
        else { bw.writeBit(1); bw.writeSignedVarint(
            (int64_t)(s->mid) - (int64_t)(prev_mid)); prev_mid = s->mid; }

        if (s->sgid == prev_sgid) { bw.writeBit(0); }
        else { bw.writeBit(1); bw.writeSignedVarint(
            (int64_t)(s->sgid) - (int64_t)(prev_sgid)); prev_sgid = s->sgid; }

        int64_t td = s->t - prev_t;
        int64_t dod = td - prev_t_delta;
        if (dod == 0) { bw.writeBit(0); }
        else { bw.writeBit(1); bw.writeSignedVarint(dod); }
        prev_t_delta = td; prev_t = s->t;

        uint64_t cv = tsdb::base::encode_double(s->v);
        uint64_t xv = cv ^ prev_v;
        if (xv == 0) { bw.writeBit(0); }
        else {
            bw.writeBit(1);
            int lz = __builtin_clzll(xv), tz = __builtin_ctzll(xv);
            if (lz >= prev_lz && tz >= prev_tz) {
                bw.writeBit(0);
                int mb = 64 - prev_lz - prev_tz;
                bw.writeBits(xv >> prev_tz, mb);
            } else {
                bw.writeBit(1);
                bw.writeBits(static_cast<uint64_t>(lz), 5);
                int mb = 64 - lz - tz;
                bw.writeBits(static_cast<uint64_t>(mb - 1), 6);
                bw.writeBits(xv >> tz, mb);
                prev_lz = lz; prev_tz = tz;
            }
        }
        prev_v = cv;

        int64_t txn_d = s->txn - prev_txn;
        if (txn_d == prev_txn_delta) { bw.writeBit(0); }
        else { bw.writeBit(1); bw.writeSignedVarint(txn_d); }
        prev_txn_delta = txn_d; prev_txn = s->txn;
    }
    bw.flushToByte();
    return result;
}

std::string treeSeries(const std::vector<tsdb::tsdbutil::TreeRefSeries>& series) {
    std::string result;
    treeSeries(series, &result);
    return result;
}

std::string treeSeries(tsdb::tsdbutil::TreeRefSeries* series, uint32_t item_size) {
    std::string result;
    treeSeries(series,item_size, &result);
    return result;
}

void treeSeries(tsdb::tsdbutil::TreeRefSeries* series, uint32_t item_size, std::string* result){
    result->push_back(kSeries);
    for(uint32_t i = 0;i < item_size; i++){
        tsdb::tsdbutil::TreeRefSeries* r = series + i;
        PutFixed16(result, r->mid);
        PutFixed64BE(result, r->sgid);
        PutFixed64BE(result, r->flushed_txn);
        PutFixed64BE(result, r->log_clean_txn);
        const tsdb::label::Labels* lptr = &r->lset;
//        std::cout<<tsdb::label::lbs_string(r->lset)<<std::endl;
        if (r->lset_ptr) {
            lptr = r->lset_ptr;
        }
        PutVarint32(result, lptr->size());

        for (const tsdb::label::Label& l : *lptr) {
            PutLengthPrefixedSlice(result, l.label);
            PutLengthPrefixedSlice(result, l.value);
        }
    }
}

void treeSeries(const std::vector<tsdb::tsdbutil::TreeRefSeries>& series, std::string* result) {
    result->push_back(kSeries);

    for (tsdb::tsdbutil::TreeRefSeries r : series) {
        PutFixed16(result, r.mid);
        PutFixed64BE(result, r.sgid);
        PutFixed64BE(result, r.flushed_txn);
        PutFixed64BE(result, r.log_clean_txn);
        const tsdb::label::Labels* lptr = &r.lset;
        std::cout<<tsdb::label::lbs_string(r.lset)<<std::endl;
        if (r.lset_ptr) {
            lptr = r.lset_ptr;
        }
        PutVarint32(result, lptr->size());

        for (const tsdb::label::Label& l : *lptr) {
            PutLengthPrefixedSlice(result, l.label);
            PutLengthPrefixedSlice(result, l.value);
        }
    }
}

void treeSeries(const tsdb::tsdbutil::TreeRefSeries& series, std::string* result) {
    result->push_back(kSeries);

    PutFixed16(result, series.mid);
    PutFixed64BE(result, series.sgid);
    PutFixed64BE(result, series.flushed_txn);
    PutFixed64BE(result, series.log_clean_txn);
    const tsdb::label::Labels* lptr = &series.lset;
    if (series.lset_ptr) {
        lptr = series.lset_ptr;
    }
    PutVarint32(result, lptr->size());

    for (const tsdb::label::Label& l : *lptr) {
        PutLengthPrefixedSlice(result, l.label);
        PutLengthPrefixedSlice(result, l.value);
    }
}

void tree_series_without_label(uint16_t mid, uint64_t sgid, int64_t flushed_txn, int64_t log_clean_txn, std::string* result) {
    result->push_back(kSeries);

    PutFixed16(result, mid);
    PutFixed64BE(result, sgid);
    PutFixed64BE(result, flushed_txn);
    PutFixed64BE(result, log_clean_txn);
}

void tree_series_without_label(const tsdb::tsdbutil::TreeRefSeries& series, std::string* result) {
    result->push_back(kSeries);

    PutFixed16(result, series.mid);
    PutFixed64BE(result, series.sgid);
    PutFixed64BE(result, series.flushed_txn);
    PutFixed64BE(result, series.log_clean_txn);
}

std::string treeFlush(const tsdb::tsdbutil::TreeRefFlush& flush) {
    std::string result;
    treeFlush(flush, &result);
    return result;
}

void treeFlush(const tsdb::tsdbutil::TreeRefFlush& flush, std::string* result) {
    result->push_back(kFlush);

    PutFixed16(result, flush.mid);
    PutFixed64BE(result, flush.sgid);
    PutFixed64BE(result, flush.txn);
}

static void InitTypeCrc(uint32_t* type_crc) {
  for (int i = 0; i <= kMaxRecordType; i++) {
    char t = static_cast<char>(i);
    type_crc[i] = crc32c::Value(&t, 1);
  }
}

Writer::Writer(WritableFile* dest)
    : dest_(dest), block_offset_(0), num_block_(0) {
  InitTypeCrc(type_crc_);
}

Writer::Writer(WritableFile* dest, uint64_t dest_length)
    : dest_(dest),
      block_offset_(dest_length % kBlockSize),
      num_block_(dest_length / kBlockSize) {
  InitTypeCrc(type_crc_);
}

Writer::~Writer() = default;

Status Writer::AddRecord(const Slice& slice) {
  const char* ptr = slice.data();
  size_t left = slice.size();

  // Fragment the record if necessary and emit it.  Note that if slice
  // is empty, we still want to iterate once to emit a single
  // zero-length record
  Status s;
  bool begin = true;
  do {
    const int leftover = kBlockSize - block_offset_;
    assert(leftover >= 0);
    if (leftover < kHeaderSize) {
      // Switch to a new block
      if (leftover > 0) {
        // Fill the trailer (literal below relies on kHeaderSize being 7)
        static_assert(kHeaderSize == 7, "");
        dest_->Append(Slice("\x00\x00\x00\x00\x00\x00", leftover));
      }
      block_offset_ = 0;
      num_block_++;
    }

    // Invariant: we never leave < kHeaderSize bytes in a block.
    assert(kBlockSize - block_offset_ - kHeaderSize >= 0);

    const size_t avail = kBlockSize - block_offset_ - kHeaderSize;
    const size_t fragment_length = (left < avail) ? left : avail;

    RecordType type;
    const bool end = (left == fragment_length);
    if (begin && end) {
      type = kFullType;
    } else if (begin) {
      type = kFirstType;
    } else if (end) {
      type = kLastType;
    } else {
      type = kMiddleType;
    }

    s = EmitPhysicalRecord(type, ptr, fragment_length);
    ptr += fragment_length;
    left -= fragment_length;
    begin = false;

  } while (s.ok() && left > 0);
  return s;
}

Status Writer::EmitPhysicalRecord(RecordType t, const char* ptr,
                                  size_t length) {
  assert(length <= 0xffff);  // Must fit in two bytes
  assert(block_offset_ + kHeaderSize + length <= kBlockSize);

  // Format the header
  char buf[kHeaderSize];
  buf[4] = static_cast<char>(length & 0xff);
  buf[5] = static_cast<char>(length >> 8);
  buf[6] = static_cast<char>(t);

  // Compute the crc of the record type and the payload.
  uint32_t crc = crc32c::Extend(type_crc_[t], ptr, length);
  crc = crc32c::Mask(crc);  // Adjust for storage
  EncodeFixed32(buf, crc);

  // Write the header and the payload
  Status s = dest_->Append(Slice(buf, kHeaderSize));
  if (s.ok()) {
    s = dest_->Append(Slice(ptr, length));
    // if (s.ok()) {
    //   s = dest_->Flush();
    // }
  }
  block_offset_ += kHeaderSize + length;
  return s;
}

void Writer::flush() { dest_->Flush(); }

RandomWriter::RandomWriter(RandomRWFile* dest)
    : dest_(dest), block_offset_(0), num_block_(0) {
  InitTypeCrc(type_crc_);
}

RandomWriter::~RandomWriter() = default;

Status RandomWriter::AddRecord(uint64_t pos, const Slice& slice, bool write_header) {
  const char* ptr = slice.data();
  size_t left = slice.size();

  block_offset_ = pos % kBlockSize;
  num_block_ = pos / kBlockSize;

  // Fragment the record if necessary and emit it.  Note that if slice
  // is empty, we still want to iterate once to emit a single
  // zero-length record
  Status s;
  bool begin = true;
  do {
    const int leftover = kBlockSize - block_offset_;
    assert(leftover >= 0);
    if (leftover < kHeaderSize) {
      // Switch to a new block
      block_offset_ = 0;
      num_block_++;
    }

    // Invariant: we never leave < kHeaderSize bytes in a block.
    assert(kBlockSize - block_offset_ - kHeaderSize >= 0);

    const size_t avail = kBlockSize - block_offset_ - kHeaderSize;
    const size_t fragment_length = (left < avail) ? left : avail;

    RecordType type;
    const bool end = (left == fragment_length);
    if (begin && end) {
      type = kFullType;
    } else if (begin) {
      type = kFirstType;
    } else if (end) {
      type = kLastType;
    } else {
      type = kMiddleType;
    }

    s = EmitPhysicalRecord(type, ptr, fragment_length, write_header);
    ptr += fragment_length;
    left -= fragment_length;
    begin = false;

  } while (s.ok() && left > 0);
  return s;
}

Status RandomWriter::EmitPhysicalRecord(RecordType t, const char* ptr,
                                        size_t length, bool write_header) {
  assert(length <= 0xffff);  // Must fit in two bytes
  assert(block_offset_ + kHeaderSize + length <= kBlockSize);

  // Format the header
  char buf[kHeaderSize];
  buf[4] = static_cast<char>(length & 0xff);
  buf[5] = static_cast<char>(length >> 8);
  buf[6] = static_cast<char>(t);

  // Compute the crc of the record type and the payload.
  uint32_t crc = crc32c::Extend(type_crc_[t], ptr, length);
  crc = crc32c::Mask(crc);  // Adjust for storage
  EncodeFixed32(buf, crc);

  // Write the header and the payload
  Status s;
  if (write_header)
    s = dest_->Write(num_block_ * kBlockSize + block_offset_,
                     Slice(buf, kHeaderSize));
  if (s.ok()) {
    s = dest_->Write(num_block_ * kBlockSize + block_offset_ + kHeaderSize,
                     Slice(ptr, length));
    // if (s.ok()) {
    //   s = dest_->Flush();
    // }
  }
  block_offset_ += kHeaderSize + length;
  return s;
}

void RandomWriter::flush() { dest_->Flush(); }

}  // namespace log
}  // namespace leveldb
