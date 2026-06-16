#define GLOBAL_VALUE_DEFINE

#include "protobuf/types.pb.h"
#include "protobuf/remote.pb.h"
#include <snappy.h>

#include "TreeRemoteDB.h"
#include "db/DB.hpp"

int main(int argc, char** argv) {
  std::string dbpath = "/tmp/tsdb_big";
  std::string log_path = "/tmp/tsdb_log";
  boost::filesystem::remove_all(dbpath);
  boost::filesystem::remove_all(log_path);

  tsdb::db::TreeRemoteDB db(dbpath, log_path);
  sleep(1);

  std::cout<< "Successfully start remote storage service." <<std::endl;
  for(;;) {
    //        sleep(1);
  }
}