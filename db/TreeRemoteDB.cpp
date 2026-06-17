#include "db/TreeRemoteDB.h"

#include <yaml-cpp/yaml.h>

#include <boost/filesystem.hpp>
#include <boost/move/detail/meta_utils.hpp>

#include "TreeSeries/config.h"
#include "leveldb/db/dbformat.h"
#include "label/EqualMatcher.hpp"

namespace tsdb {
    namespace db {

        TreeRemoteDB::TreeRemoteDB(const std::string& dir, leveldb::DB* db, head::TreeHead* tree_head, slab::TreeSeries* tree_series)
                : dir_(dir),
                  db_(db),
                  tree_head_(tree_head),
                  tree_series_(tree_series),
                  pool_(32) {
            init_http_proto_server();
        }

        TreeRemoteDB::TreeRemoteDB(const std::string &dir, const std::string &log_path)
                : dir_(dir),
                  pool_(32) {
            setup(dir, log_path);
            init_http_proto_server();
        }

        TreeRemoteDB::~TreeRemoteDB() {
//            delete db_;
            delete tree_head_;
            delete tree_series_;
            server_.stop();
        }

        void TreeRemoteDB::init_http_proto_server() {
            server_.set_read_timeout(1000);
            server_.set_write_timeout(1000);
            server_.set_payload_max_length(1000000000000);
            server_.set_keep_alive_timeout(1000);
            server_.Post("/insert", [this](const httplib::Request& req, httplib::Response& resp) {
                this->HandleInsert(req, resp);
            });

            server_.Post("/query", [this](const httplib::Request& req, httplib::Response& resp) {
                this->HandleQuery(req, resp);
            });

            std::thread t([this]() { this->server_.listen(prom_host, prom_port); });
            t.detach();
        }

        leveldb::Status TreeRemoteDB::setup(const std::string& dbpath, const std::string& log_path) {
            //=================TreeSeries==========
            YAML::Node cfg = YAML::LoadFile("../config.yaml");

            config::tree_series_thread_pool_size = cfg["tree_series_thread_pool_size"].as<int>();
            config::max_slab_memory = cfg["max_slab_memory"].as<int>();
            config::read_buffer_size = cfg["read_buffer_size"].as<int>();
            config::max_series_num = cfg["max_series_num"].as<int>();
            config::slab_item_size = cfg["slab_item_size"].as<int>();
            config::chunk_size = cfg["chunk_size"].as<int>();
            config::slab_size = cfg["slab_size"].as<int>();

            std::string tree_series_path = cfg["tree_series_path"].as<std::string>();
            int fd = ::open(tree_series_path.c_str(), O_WRONLY | O_CREAT | O_TRUNC, 0644);
            if (fd >= 0) {
                ftruncate(fd, static_cast<off_t>(config::max_slab_memory) * 1024 * 1024 * 1024);
                close(fd);
            }
            slab::Setting *setting = new slab::Setting();
            setting->ssd_device_ = tree_series_path.data();
            std::string info_path = cfg["tree_series_info_path"].as<std::string>();
            int info_fd = ::open(info_path.c_str(), O_WRONLY | O_CREAT | O_TRUNC, 0644);
            if (info_fd >= 0) close(info_fd);
            setting->ssd_slab_info_ = info_path.data();
            tree_series_ = new slab::TreeSeries(*setting);

            //==========LevelDB============
//                std::string dbpath = "/tmp/tsdb_big";
//                std::string dbpath = "/mnt/HDD/tree_head_test";

            boost::filesystem::remove_all(dbpath);
            boost::filesystem::remove_all(log_path);

            leveldb::Options options;
            options.create_if_missing = true;
            options.max_imm_num = 3;
            options.write_buffer_size = 4 * 256 * 1024 * 1024;
            options.max_file_size = 4 * 256 * 1024 * 1024;
            options.use_log = false;
            options.block_cache = leveldb::NewLRUCache(256 * 1024 * 1024);  // 256MB, prevent OOM during query
            leveldb::Status st = leveldb::DB::Open(options, dbpath, &db_);
            if (!st.ok())return st;

            boost::filesystem::remove_all(dbpath);
            tree_head_ = new head::TreeHead(dbpath, log_path,"",db_,tree_series_);
            db_->SetTreeHead(tree_head_);
            tree_head_->bg_flush_data();
            return st;
        }

        void TreeRemoteDB::multi_add(db::AppenderInterface* appender, prometheus::WriteRequest* writeRequest, uint64_t left, uint64_t right, base::WaitGroup* _wg) {
            for (uint64_t i = left; i < right; i++) {
                auto ts = writeRequest->timeseries(i);
                label::Labels label_set;
                for (auto& lb : ts.labels()) {
                    label_set.emplace_back(lb.name(), lb.value());
                  // std::cout<<lb.name()<<" "<<lb.value()<<std::endl;
                }
                for (auto& sample : ts.samples()) {
                    appender->add(label_set, sample.timestamp(), sample.value());
                    // std::cout<< sample.timestamp() <<" "<<sample.value() <<std::endl;
                }
            }
            appender->commit();
            _wg->done();
        }


        void TreeRemoteDB::multi_add_fast(db::AppenderInterface* appender, prometheus::WriteRequest* writeRequest, uint64_t left, uint64_t right, base::WaitGroup* _wg) {
            for (uint64_t i = left; i < right; i++) {
                auto ts = writeRequest->timeseries(i);
                label::Labels label_set;
                for (auto& lb : ts.labels()) {
                    label_set.emplace_back(lb.name(), lb.value());
                }
                // for (auto& sample : ts.samples()) {
                //     appender->add(label_set, sample.timestamp(), sample.value());
                // }
                uint64_t sgid = 0;
                uint16_t mid = 0;
                appender->add(label_set, ts.samples(0).timestamp(), ts.samples(0).value(), sgid, mid, 0);
                for (uint32_t i = 1; i < ts.samples_size(); ++i) {
                    appender->add_fast(sgid, mid, ts.samples(i).timestamp(), ts.samples(i).value());
                }
            }
            appender->commit();
            _wg->done();
        }

        void TreeRemoteDB::HandleInsert(const httplib::Request &req,httplib::Response &resp) {
            MasstreeWrapper<slab::SlabInfo>::ti = threadinfo::make(threadinfo::TI_PROCESS, 16);
            std::string data;
            snappy::Uncompress(req.body.data(), req.body.size(), &data);
            prometheus::WriteRequest write_requset;
            write_requset.ParseFromString(data);

            uint64_t batch_size = write_requset.timeseries_size() / 32;
            std::vector<std::unique_ptr<db::AppenderInterface>> apps;
            base::WaitGroup wg;

            for (uint64_t i = 0; i < write_requset.timeseries_size(); i+=batch_size) {
                wg.add(1);
                apps.push_back(std::move(tree_head_->appender()));
                auto right = std::min(i+batch_size, uint64_t(write_requset.timeseries_size()));
                AppenderInterface* app = apps.back().get();
                pool_.enqueue([this, app, &write_requset, i, right, &wg]{
                    return multi_add_fast(app, &write_requset, i, right, &wg);
                });
            }
            wg.wait();


//            int ts_cnt = 0, smp_cnt = 0;
//            for (auto& ts : write_requset.timeseries()) {
//                ts_cnt++;
//                label::Labels label_set;
//                for (auto& lb : ts.labels()) {
////                    label::lbs_add(label_set, label::Label(lb.name(), lb.value()));
//                    label_set.emplace_back(lb.name(), lb.value());
//                }
//                for (auto& sample : ts.samples()) {
//                    smp_cnt++;
//                    appender->add(label_set, sample.timestamp(), sample.value());
////                    std::cout<<sample.timestamp()<<" "<<sample.value()<<std::endl;
//                }
//            }
//            appender->commit();

            // std::cout<<"Insert "<<write_requset.timeseries_size()<<" timeseries, "
            //     <<write_requset.timeseries().size()*write_requset.timeseries(0).samples_size()<<" samples"
            // <<std::endl;

            data.clear();
            resp.set_content(data, "text/plain");
        }

        void TreeRemoteDB::HandleQuery(const httplib::Request &req, httplib::Response &resp) {
            MasstreeWrapper<slab::SlabInfo>::ti = threadinfo::make(threadinfo::TI_PROCESS, 16);
            std::string data;
            snappy::Uncompress(req.body.data(), req.body.size(), &data);
            prometheus::ReadRequest read_request;
            read_request.ParseFromString(data);

            prometheus::ReadResponse read_resp;
            for (auto& qry : read_request.queries()) {
                std::unique_ptr<querier::TreeQuerier> q(this->querier(qry.start_timestamp_ms(), qry.end_timestamp_ms()));

                std::vector<std::shared_ptr<tsdb::label::MatcherInterface>> matchers;
                for (auto& matcher : qry.matchers()) {
                    if (matcher.type() == prometheus::LabelMatcher_Type_EQ) {
                        matchers.emplace_back(std::shared_ptr<tsdb::label::MatcherInterface>(
                                new tsdb::label::EqualMatcher(matcher.name(), matcher.value())));
                    }
                }

                prometheus::QueryResult* query_result = read_resp.add_results();
                std::unique_ptr<tsdb::querier::SeriesSetInterface> series_set = q->select(matchers);
                while (series_set->next()) {
                    prometheus::TimeSeries* timeseries = query_result->add_timeseries();
                    std::unique_ptr<tsdb::querier::SeriesInterface> series = series_set->at();
                    if (series == nullptr) continue;

                    tsdb::label::Labels series_labels = series->labels();
                    for (auto& lb : series_labels) {
                        prometheus::Label* label = timeseries->add_labels();
                        label->set_name(lb.label);
                        label->set_value(lb.value);
                    }

                    std::unique_ptr<tsdb::querier::SeriesIteratorInterface> series_iter = series->iterator();
                    while (series_iter->next()) {
                        prometheus::Sample* sample = timeseries->add_samples();
                        sample->set_timestamp(series_iter->at().first);
                        sample->set_value(series_iter->at().second);
                    }
                }
            }
            std::string resp_data, compressed_data;
            read_resp.SerializeToString(&resp_data);
            snappy::Compress(resp_data.data(), resp_data.size(), &compressed_data);
            resp.set_content(compressed_data, "text/plain");
        }

    }
}