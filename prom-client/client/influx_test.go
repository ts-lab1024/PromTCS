package client

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	influx "github.com/influxdata/influxdb1-client/v2"
)

var measurements []string = []string{
	"node_cpu_seconds_total",    //每个 CPU 核心的总使用时间，按状态（如 user、system、idle 等）分类
	"process_cpu_seconds_total", //当前进程的 CPU 使用时间

	"node_memory_MemTotal_bytes",     //系统的总内存大小
	"node_memory_MemFree_bytes",      //系统的空闲内存大小
	"node_memory_MemAvailable_bytes", //系统可用的内存量
	"node_memory_Active_bytes",       //当前活动的内存量
	"process_virtual_memory_bytes",   //当前进程的虚拟内存大小

	"node_network_transmit_bytes_total", //网络接口发送的字节数总量

}

func TestQueryFromInflux(t *testing.T) {
	//queryString := `SHOW MEASUREMENTS`

	//queryString := `select * from node_cpu_seconds_total where time >= '2025-02-26T15:00:00Z' and time < '2025-02-26T16:00:00Z' group by cpu,mode,instance limit 10`

	//queryString := `select * from process_cpu_seconds_total where time >= '2025-02-26T15:00:00Z' and time < '2025-02-26T16:00:00Z' limit 10`
	queryString := `select * from node_network_transmit_bytes_total where time >= '2025-02-26T15:00:00Z' and time < '2025-02-26T16:00:00Z' limit 10`
	resp, err := QueryFromInflux(queryString)
	if err != nil {
		panic(err)
	}
	if len(resp.Results[0].Series) == 0 {
		fmt.Println("query result empty")
	} else {
		fmt.Println(len(resp.Results[0].Series[0].Values))
		for _, series := range resp.Results[0].Series {
			fmt.Println(series.Columns)
			for _, val := range series.Values {
				fmt.Println(val)
			}
		}
	}

	//fmt.Println(resp)
}

func genHostString(num int) string {
	ret := ""

	hosts := make([]string, 0)
	for i := 0; i < num; i++ {
		hosts = append(hosts, fmt.Sprintf("hostname = 'host_%d'", i))
	}

	ret = strings.Join(hosts, " or ")
	return ret
}

func TestBigQueryFromInflux(t *testing.T) {
	hosts := genHostString(1000)

	//queryString := `select * from node_network_transmit_bytes_total where time >= '2025-02-26T15:00:00Z' and time < '2025-02-26T16:00:00Z' limit 10`
	queryString := fmt.Sprintf("select usage_user,usage_system,usage_idle,usage_nice,usage_iowait,usage_irq,usage_softirq,usage_steal,usage_guest,usage_guest_nice from \"cpu\" where (%s) AND time >= '2022-12-15T03:00:00Z' and time < '2022-12-15T03:15:00Z'", hosts)
	fmt.Println(queryString)

	client, err := influx.NewHTTPClient(influx.HTTPConfig{
		Addr: InfluxAddr,
	})
	if err != nil {
		panic(err)
	}

	start := time.Now()
	qry := influx.NewQuery(queryString, "devops_large", "s")
	//resp, err := client.Query(qry)
	//if err != nil {
	//	panic(err)
	//	return
	//}
	//fmt.Println(len(resp.Results[0].Series[0].Values))

	var wg sync.WaitGroup
	wg.Add(32)
	for i := 0; i < 32; i++ {
		go func(id int) {
			defer wg.Done()
			_, err := client.Query(qry)
			if err != nil {
				fmt.Printf("goroutine %d: query error: %v\n", id, err)
				return
			}
		}(i)
	}
	wg.Wait()

	//fmt.Println(len(resp.Results[0].Series))

	fmt.Println(time.Since(start).Milliseconds())
}
