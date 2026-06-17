#define GLOBAL_VALUE_DEFINE

#include "memory_usage.h"

int main() {
//    std::string proc_num = "self";
    std::string proc_num = "141681";
    std::string file = "./memory_usage.log";
    std::remove(file.data());
    double vm_usage, rss, swap;
    std::ofstream output_file(file, std::ios_base::app);  // Open file in append mode

    if (!output_file.is_open()) {
        std::cerr << "Failed to open output file." << std::endl;
        return 1;
    }

    uint64_t log_num = 0;
    while (true) {
        log_num++;
        if (!mem_usage(proc_num, vm_usage, rss, swap)) {
            return -1;
        }
        output_file << log_num << ": " << "Virtual Memory Usage: " << (vm_usage / 1024) << " MB, " << "Resident Set Size: " << (rss / 1024) << " MB, Swap: " << swap << " MB\n";
        std::cout << log_num << ": " << "Virtual Memory Usage: " << (vm_usage / 1024) << " MB, " << "Resident Set Size: " << (rss / 1024) << " MB, Swap: " << swap << " MB" << std::endl;
        output_file.flush();  // Ensure data is written to the file immediately
        sleep(10);  // Sleep for 10 second
    }

    output_file.close();
    return 0;
}