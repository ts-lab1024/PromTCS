package main

import (
	"flag"
	"fmt"

	"github.com/01spirit/prom-client/client"
)

func main() {
	addr := flag.String("addr", "", "remote storage address (e.g., http://localhost:9966). Also settable via PROMTCS_ADDR env var")
	numSeries := flag.Int("series", 10000, "number of series")
	numSamples := flag.Int("samples", 50, "number of samples per series")
	sendNum := flag.Int("send", 64, "total send requests")

	flag.Parse()

	client.SetRemoteAddr(*addr)

	fmt.Printf("addr=%s series=%d samples=%d send=%d\n", client.RemoteWriteServer, *numSeries, *numSamples, *sendNum)

	client.SendSyntheticData(*numSeries, *numSamples, *sendNum)
}
