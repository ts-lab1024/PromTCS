#ifndef MEMORY_USAGE_H
#define MEMORY_USAGE_H
#include <iostream>
#include <fstream>
#include <sstream>
#include <unistd.h>
#include <sys/syscall.h>

bool mem_usage(std::string proc_num, double& vm_usage, double& rss, double& swap) {
    rss = 0.0;
    swap = 0.0;

    // Read RSS from /proc/self/stat
    std::ifstream stat_stream("/proc/"+proc_num+"/stat", std::ios_base::in);
    if (!stat_stream.is_open()) {
        std::cerr << "Failed to open /proc/self/stat" << std::endl;
        return false;
    }

    std::string pid, comm, state, ppid, pgrp, session, tty_nr;
    std::string tpgid, flags, minflt, cminflt, majflt, cmajflt;
    std::string utime, stime, cutime, cstime, priority, nice;
    std::string O, itrealvalue, starttime;
    unsigned long vsize;
    long rss_pages;

    stat_stream >> pid >> comm >> state >> ppid >> pgrp >> session >> tty_nr >>
                tpgid >> flags >> minflt >> cminflt >> majflt >> cmajflt >> utime >>
                stime >> cutime >> cstime >> priority >> nice >> O >> itrealvalue >>
                starttime >> vsize >> rss_pages;

    stat_stream.close();

    long page_size_kb = sysconf(_SC_PAGE_SIZE) / 1024;  // for x86-64 is configured to use 2MB pages   KB
    vm_usage = vsize / 1024.0; // KB
    rss = rss_pages * page_size_kb; // KB

    // Read Swap from /proc/self/status
    std::ifstream status_stream("/proc/"+proc_num+"/stat", std::ios_base::in);
    if (!status_stream.is_open()) {
        std::cerr << "Failed to open /proc/self/status" << std::endl;
        return false;
    }

    std::string line;
    while (std::getline(status_stream, line)) {
        if (line.find("VmSwap:") != std::string::npos) {
            std::istringstream iss(line);
            std::string key, value, unit;
            iss >> key >> value >> unit;
            swap = std::stod(value);
            break;
        }
    }

    status_stream.close();
    return true;
}

#endif //MEMORY_USAGE_H
