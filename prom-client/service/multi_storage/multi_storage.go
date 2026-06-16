package main

import (
	"flag"
	"fmt"

	"github.com/01spirit/prom-client/client"
)

func main() {

	numSeries := flag.Int("series", 10000, "number of series")
	numSamples := flag.Int("samples", 50, "number of samples per series in each request")
	sendNum := flag.Int("send", 64, "total send requests")
	numThread := flag.Int("thread", 8, "number of worker threads")

	flag.Parse()
	fmt.Printf("series=%d samples=%d send=%d thread=%d\n", *numSeries, *numSamples, *sendNum, *numThread)

	//client.SampleGenerateAdSendMultiThreaded(*numSeries, *numSamples, *sendNum, *numThread)
	client.SampleGenerateAdSendMultiThreadedDividedBySeries(*numSeries, *numSamples, *sendNum, *numThread)
}
