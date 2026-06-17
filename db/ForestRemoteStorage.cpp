#define GLOBAL_VALUE_DEFINE

#include <iostream>
#include <yaml-cpp/yaml.h>
#include <boost/filesystem.hpp>

#include "protobuf/types.pb.h"
#include "protobuf/remote.pb.h"
#include <snappy.h>

#include "TreeRemoteDB.h"
#include "db/DB.hpp"

int main(int argc, char** argv) {
  YAML::Node cfg = YAML::LoadFile("../config.yaml");
  std::string dbpath = cfg["db_path"].as<std::string>();
  std::string log_path = cfg["log_path"].as<std::string>();
  boost::filesystem::remove_all(dbpath);
  boost::filesystem::remove_all(log_path);

  tsdb::db::TreeRemoteDB db(dbpath, log_path);
  sleep(1);

  std::cout<< "Successfully start remote storage service." <<std::endl;
  for(;;) {
    //        sleep(1);
  }
}