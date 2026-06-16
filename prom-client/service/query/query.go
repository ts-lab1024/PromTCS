package main

import (
	"flag"

	"github.com/01spirit/prom-client/client"
)

func main() {
	addr := flag.String("addr", "", "remote storage address (e.g., http://localhost:9966). Also settable via PROMTCS_ADDR env var")
	qryType := flag.String("type", "1-8-1", "query type, set to 'all' to run all types")
	repeatCnt := flag.Int("repeat", 10, "number of times to repeat each query type")
	threads := flag.Int("threads", 1, "number of concurrent goroutines for queries")
	outputFile := flag.String("output", "query_result.txt", "output file path for -type=all results")

	flag.Parse()

	client.SetRemoteAddr(*addr)

	if *qryType == "all" {
		client.TSBSQueryAll(*repeatCnt, *outputFile, *threads)
	} else {
		client.TSBSQuery(*qryType, *repeatCnt, *threads)
	}
}
