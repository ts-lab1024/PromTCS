package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/01spirit/prom-client/prompb"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/model/labels"
)

const (
	StepMs             = 30 * 1000            // 30 seconds in milliseconds
	HourMs             = 60 * 60 * 1000       // 1 hour in milliseconds = 3,600,000
	DayMs              = 24 * HourMs          // 24 hours in milliseconds = 86,400,000
	SamplesPerDay      = DayMs / StepMs       // 2880 samples per series for 24h
	SamplesPerThreeDay = 3*DayMs/StepMs + 2   // 8642 samples per series for 72h+
)

func calcLatencyStats(latencies []float64) (avg, p50, p90, p99 float64) {
	if len(latencies) == 0 {
		return 0, 0, 0, 0
	}
	sort.Float64s(latencies)
	var sum float64
	for _, v := range latencies {
		sum += v
	}
	avg = sum / float64(len(latencies))
	p50 = latencies[len(latencies)*50/100]
	p90 = latencies[len(latencies)*90/100]
	p99 = latencies[len(latencies)*99/100]
	return
}

// runQueryWorkers executes qryCnt identical queries using threads concurrent goroutines.
// The request body is pre-built once and reused. Returns all latencies (ms) and total samples.
func runQueryWorkers(cli *Client, requestBody []byte, qryCnt, threads int) ([]float64, int) {
	if threads <= 0 {
		threads = 1
	}
	if threads > qryCnt {
		threads = qryCnt
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	latencies := make([]float64, 0, qryCnt)
	totalSamples := 0

	queriesPerWorker := qryCnt / threads
	remainder := qryCnt % threads

	for t := 0; t < threads; t++ {
		count := queriesPerWorker
		if t < remainder {
			count++
		}
		if count == 0 {
			continue
		}
		wg.Add(1)
		go func(count int) {
			defer wg.Done()
			for i := 0; i < count; i++ {
				request, _ := http.NewRequest(http.MethodPost, RemoteQueryServer, bytes.NewBuffer(requestBody))

				now := time.Now()
				response, _ := cli.Client.Do(request)
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				lag := time.Since(now).Nanoseconds()

				uncompressed, _ := snappy.Decode(nil, body)
				resp := &prompb.ReadResponse{}
				_ = proto.Unmarshal(uncompressed, resp)

				nSamples := 0
				for _, result := range resp.Results {
					for _, ts := range result.Timeseries {
						nSamples += len(ts.Samples)
					}
				}

				mu.Lock()
				latencies = append(latencies, float64(lag)/1e6)
				totalSamples += nSamples
				mu.Unlock()
			}
		}(count)
	}
	wg.Wait()
	return latencies, totalSamples
}

func SendSamples(numSamples, numSeries int) int64 {
	//numSamples := 1
	//numSeries := 1000000

	samples, series := createTimeseries(numSamples, numSeries, extraLabels...)

	client, err := InitWriteClient()
	if err != nil {
		panic(err)
	}

	queueManager := InitQueueManager(client)

	queueManager.StoreSeries(series, 0)

	var (
		pBuf = proto.NewBuffer(nil)
		buf  []byte
	)
	batch := make([]timeSeries, len(samples))
	for i, s := range samples {
		batch[i] = timeSeries{
			seriesLabels: queueManager.seriesLabels[s.Ref],
			metadata:     nil,
			timestamp:    s.T,
			value:        s.V,
		}
	}
	pendingData := make([]prompb.TimeSeries, len(samples))
	for i := range pendingData {
		pendingData[i].Samples = []prompb.Sample{{}}
	}
	nPendingSamples := populateTimeSeries(batch, pendingData)
	fmt.Println("pending samples: ", nPendingSamples)

	now := time.Now()

	err = queueManager.shards.sendSamples(context.Background(), pendingData, pBuf, &buf)
	if err != nil {
		panic(err)
	}

	lag := time.Since(now).Nanoseconds()
	fmt.Println("write latency: ", lag)

	return lag
}

func SendSingleSample(numSeries int, idx int) int64 {
	//numSamples := 1
	//numSeries := 1000000

	samples, series := createSingleTimeseries(numSeries, int64(idx))

	client, err := InitWriteClient()
	if err != nil {
		panic(err)
	}

	queueManager := InitQueueManager(client)

	queueManager.StoreSeries(series, 0)

	var (
		pBuf = proto.NewBuffer(nil)
		buf  []byte
	)
	batch := make([]timeSeries, len(samples))
	for i, s := range samples {
		batch[i] = timeSeries{
			seriesLabels: queueManager.seriesLabels[s.Ref],
			metadata:     nil,
			timestamp:    s.T,
			value:        s.V,
		}
	}
	pendingData := make([]prompb.TimeSeries, len(samples))
	for i := range pendingData {
		pendingData[i].Samples = []prompb.Sample{{}}
	}
	nPendingSamples := populateTimeSeries(batch, pendingData)
	fmt.Println("pending samples: ", nPendingSamples)

	now := time.Now()

	err = queueManager.shards.sendSamples(context.Background(), pendingData, pBuf, &buf)
	if err != nil {
		panic(err)
	}

	lag := time.Since(now).Nanoseconds()
	fmt.Println("write latency: ", lag)

	return lag
}

func SendBatchSample(numSeries, numSamples int, idx int) int64 {

	//samples, series := createSingleTimeseries(numSeries, int64(idx))
	samplesArr, series := createBatchTimeseries(numSeries, numSamples, int64(idx))
	//for _, s := range series {
	//	fmt.Println(s.Labels)
	//}

	client, err := InitWriteClient()
	if err != nil {
		panic(err)
	}

	queueManager := InitQueueManager(client)

	queueManager.StoreSeries(series, 0)

	var (
		pBuf = proto.NewBuffer(nil)
		buf  []byte
	)
	batch := make([]timeSeries, len(series))
	for k, s := range series {
		batch[k] = timeSeries{
			seriesLabels: queueManager.seriesLabels[s.Ref],
			metadata:     nil,
			timestamp:    samplesArr[k][0].T,
			value:        samplesArr[k][0].V,
		}
	}
	pendingData := make([]prompb.TimeSeries, len(series))
	for i := range pendingData {
		pendingData[i].Samples = []prompb.Sample{}
	}
	_ = populateTimeSeriesWithMultiSamples(batch, samplesArr, numSamples, pendingData)

	now := time.Now()

	err = queueManager.shards.sendSamples(context.Background(), pendingData, pBuf, &buf)
	if err != nil {
		//panic(err)
		fmt.Println(err)
	}

	lag := time.Since(now).Nanoseconds()

	return lag
}

func SampleGenerateAndSend(numSeries, numSamples, sendNum int) {
	//numSamples := 50
	//numSeries := 100000
	//sendNum := 50

	queryString := `select * from node_cpu_seconds_total where time >= '2025-02-25T00:00:00Z' and time < '2025-02-26T00:00:00Z' group by cpu,mode,instance limit 10`
	resp, err := QueryFromInflux(queryString)
	if err != nil {
		panic(err)
	}

	fmt.Println("Influx query complete")

	client, err := InitWriteClient()
	if err != nil {
		panic(err)
	}

	queueManager := InitQueueManager(client)

	totalLag := int64(0)
	start := time.Now()
	for i := 0; i < sendNum; i++ {
		samplesArr, series, err := createTimeseriesByInflux(numSamples, numSeries, resp, i)
		if err != nil {
			panic(err)
		}

		queueManager.StoreSeries(series, 0)

		var (
			pBuf = proto.NewBuffer(nil)
			buf  []byte
		)
		batch := make([]timeSeries, len(series))
		for k, s := range series {
			batch[k] = timeSeries{
				seriesLabels: queueManager.seriesLabels[s.Ref],
				metadata:     nil,
				timestamp:    samplesArr[k][i%numSamples].T,
				value:        samplesArr[k][i%numSamples].V,
			}
		}
		pendingData := make([]prompb.TimeSeries, len(series))
		for i := range pendingData {
			pendingData[i].Samples = []prompb.Sample{}
		}
		nPendingSamples := populateTimeSeriesWithMultiSamples(batch, samplesArr, numSamples, pendingData)

		now := time.Now()

		err = queueManager.shards.sendSamples(context.Background(), pendingData, pBuf, &buf)
		if err != nil {
			panic(err)
		}

		lag := time.Since(now).Nanoseconds()
		fmt.Printf("batch: %d , samples: %d , latency: %.3f ms\n", i, nPendingSamples, float64(lag)/1e6)
		totalLag += lag
	}

	elapsed := time.Since(start).Milliseconds()
	fmt.Printf("\ntotal batch: %d , samples: %d , total request latency: %.3f ms\n", sendNum, numSeries*numSamples*sendNum, float64(totalLag)/1e6)
	fmt.Printf("average request latency: %.3f ms\n", float64(totalLag)/1e6/float64(sendNum))

	fmt.Println()
	fmt.Printf("wallclock time: %d ms, average wallclock latency: %.3f ms\n", elapsed, float64(elapsed)/float64(sendNum))
}

// SendSyntheticData generates synthetic time series and sends them, no InfluxDB dependency.
func SendSyntheticData(numSeries, numSamples, sendNum int) {
	client, err := InitWriteClient()
	if err != nil {
		panic(err)
	}

	queueManager := InitQueueManager(client)

	totalLag := int64(0)
	start := time.Now()
	for i := 0; i < sendNum; i++ {
		samplesArr, series := createBatchTimeseries(numSeries, numSamples, int64(i)*int64(numSamples)*StepMs)

		queueManager.StoreSeries(series, 0)

		var (
			pBuf = proto.NewBuffer(nil)
			buf  []byte
		)
		batch := make([]timeSeries, len(series))
		for k, s := range series {
			batch[k] = timeSeries{
				seriesLabels: queueManager.seriesLabels[s.Ref],
				metadata:     nil,
				timestamp:    samplesArr[k][0].T,
				value:        samplesArr[k][0].V,
			}
		}
		pendingData := make([]prompb.TimeSeries, len(series))
		for j := range pendingData {
			pendingData[j].Samples = []prompb.Sample{}
		}
		nPendingSamples := populateTimeSeriesWithMultiSamples(batch, samplesArr, numSamples, pendingData)

		now := time.Now()

		err = queueManager.shards.sendSamples(context.Background(), pendingData, pBuf, &buf)
		if err != nil {
			panic(err)
		}

		lag := time.Since(now).Nanoseconds()
		fmt.Printf("batch: %d , samples: %d , latency: %.3f ms\n", i, nPendingSamples, float64(lag)/1e6)
		totalLag += lag
	}

	elapsed := time.Since(start).Milliseconds()
	fmt.Printf("\ntotal batch: %d , samples: %d , total request latency: %.3f ms\n", sendNum, numSeries*numSamples*sendNum, float64(totalLag)/1e6)
	fmt.Printf("average request latency: %.3f ms\n", float64(totalLag)/1e6/float64(sendNum))
	fmt.Printf("wallclock time: %d ms, average wallclock latency: %.3f ms\n", elapsed, float64(elapsed)/float64(sendNum))
}

func SampleGenerateAdSendMultiThreaded(numSeries, numSamples, sendNum, numThread int) {
	queryString := `select * from node_cpu_seconds_total where time >= '2025-02-25T00:00:00Z' and time < '2025-02-26T00:00:00Z' group by cpu,mode,instance limit 10`
	resp, err := QueryFromInflux(queryString)
	if err != nil {
		panic(err)
	}

	fmt.Println("Influx query complete")

	client, err := InitWriteClient()
	if err != nil {
		panic(err)
	}

	queueManager := InitQueueManager(client)

	samplesArr, series, err := createTimeseriesByInflux(numSamples, numSeries, resp, 0)
	if err != nil {
		panic(err)
	}
	queueManager.StoreSeries(series, 0)

	totalLag := int64(0)
	var wg sync.WaitGroup
	var mtx = sync.Mutex{}
	perThread := sendNum / numThread
	remainDer := sendNum % numThread
	start := time.Now()

	for i := 0; i < numThread; i++ {
		wg.Add(1)
		go func(threadID int) {
			defer wg.Done()
			start := threadID * perThread
			end := start + perThread
			if threadID == numThread-1 {
				end += remainDer
			}

			var localLag int64 = 0
			for j := start; j < end; j++ {
				var (
					pBuf = proto.NewBuffer(nil)
					buf  []byte
				)
				batch := make([]timeSeries, len(series))
				for k, s := range series {
					batch[k] = timeSeries{
						seriesLabels: queueManager.seriesLabels[s.Ref],
						metadata:     nil,
						timestamp:    samplesArr[k][j%numSamples].T + int64(j*30),
						value:        samplesArr[k][j%numSamples].V,
					}
				}
				pendingData := make([]prompb.TimeSeries, len(series))
				for i := range pendingData {
					pendingData[i].Samples = []prompb.Sample{}
				}
				nPendingSamples := populateTimeSeriesWithMultiSamples(batch, samplesArr, numSamples, pendingData)

				now := time.Now()

				err = queueManager.shards.sendSamples(context.Background(), pendingData, pBuf, &buf)
				if err != nil {
					//panic(err)
					fmt.Println(err)
				}

				lag := time.Since(now).Nanoseconds()
				localLag += lag
				fmt.Printf("thread %d , batch %d , samples %d , latency %.3f ms\n", threadID, j, nPendingSamples, float64(lag)/1e6)
			}

			mtx.Lock()
			totalLag += localLag
			mtx.Unlock()
		}(i)
	}

	wg.Wait()
	avg := totalLag / int64(sendNum)
	totalSamples := numSeries * numSamples * sendNum
	fmt.Printf("\n=== total batch %d , samples %d , total request latency %.3f ms ===\n", sendNum, totalSamples, float64(totalLag)/1e6)
	fmt.Printf("=== average request latency %.3f ms ===\n", float64(avg)/1e6)

	fmt.Println()
	elapsed := time.Since(start).Milliseconds()
	fmt.Printf("wallclock time: %d ms, average wallclock latency: %.3f ms\n", elapsed, float64(elapsed)/float64(sendNum))
	fmt.Printf("=== throughput %.2E samples per second ===\n", float64(totalSamples)/float64(elapsed/1e3))
}

func SampleGenerateAdSendMultiThreadedDividedBySeries(numSeries, numSamples, sendNum, numThread int) {
	queryString := `select * from node_cpu_seconds_total where time >= '2025-02-25T00:00:00Z' and time < '2025-02-26T00:00:00Z' group by cpu,mode,instance limit 10`
	resp, err := QueryFromInflux(queryString)
	if err != nil {
		panic(err)
	}

	fmt.Println("Influx query complete")

	client, err := InitWriteClient()
	if err != nil {
		panic(err)
	}

	queueManager := InitQueueManager(client)

	samplesArr, series, err := createTimeseriesByInflux(numSamples, numSeries, resp, 0)
	if err != nil {
		panic(err)
	}
	queueManager.StoreSeries(series, 0)

	totalLag := int64(0)
	var wg sync.WaitGroup
	var mtx = sync.Mutex{}
	perThread := numSeries / numThread
	remainDer := numSeries % numThread
	start := time.Now()

	for i := 0; i < numThread; i++ {
		wg.Add(1)
		go func(threadID int) {
			defer wg.Done()
			start := threadID * perThread
			end := start + perThread
			if threadID == numThread-1 {
				end += remainDer
			}

			var localLag int64 = 0
			seriesLen := end - start
			for j := 0; j < sendNum; j++ {
				var (
					pBuf = proto.NewBuffer(nil)
					buf  []byte
				)
				batch := make([]timeSeries, seriesLen)
				for k := start; k < end; k++ {
					batch[k-start] = timeSeries{
						seriesLabels: queueManager.seriesLabels[series[k].Ref],
						metadata:     nil,
						timestamp:    int64(j * 30),
						value:        samplesArr[k][j%numSamples].V,
					}
				}
				pendingData := make([]prompb.TimeSeries, seriesLen)
				for i := range pendingData {
					pendingData[i].Samples = []prompb.Sample{}
				}
				//nPendingSamples := populateTimeSeriesWithMultiSamples(batch, samplesArr, numSamples, pendingData)
				nPendingSamples := populateTimeSeriesWithMultiSamplesWithStartTime(batch, samplesArr, numSamples, int64(j)*int64(numSamples)*StepMs, pendingData)

				now := time.Now()

				err = queueManager.shards.sendSamples(context.Background(), pendingData, pBuf, &buf)
				if err != nil {
					//panic(err)
					fmt.Println(err)
				}

				lag := time.Since(now).Nanoseconds()
				localLag += lag
				fmt.Printf("thread %d , batch %d , samples %d , latency %.3f ms\n", threadID, j, nPendingSamples, float64(lag)/1e6)
			}

			mtx.Lock()
			totalLag += localLag
			mtx.Unlock()
		}(i)
	}

	wg.Wait()
	avg := totalLag / int64(sendNum)
	totalSamples := numSeries * numSamples * sendNum
	fmt.Printf("\n=== total batch %d , samples %d , total request latency %.3f ms ===\n", sendNum, totalSamples, float64(totalLag)/1e6)
	fmt.Printf("=== average request latency %.3f ms ===\n", float64(avg)/1e6)

	fmt.Println()
	elapsed := time.Since(start).Milliseconds()
	fmt.Printf("wallclock time: %d ms, average wallclock latency: %.3f ms\n", elapsed, float64(elapsed)/float64(sendNum))
	fmt.Printf("=== throughput %.2E samples per second ===\n", float64(totalSamples)/float64(elapsed/1e3))
}

func Query(numSamples, numSeries int) {
	//numSamples := 30
	//numSeries := 50 * 1000

	var writeLatency int64 = 0
	for i := 0; i < numSamples; i++ {
		writeLatency += SendSingleSample(numSeries, i)
		//writeLatency += SendSamples(1, numSeries)
	}

	avgLatency := float64(writeLatency) / float64(numSamples) / 1e9
	fmt.Printf("\nInsert: series: %d , samples: %d , avg latency: %f ms, throughput: %f\n", numSeries, numSamples, avgLatency, float64(numSeries)/(avgLatency))
	//SendSamples(numSamples, numSeries)

	cli, err := InitQueryClient()
	if err != nil {
		panic(err)
	}

	for k := 0; k < 3; k++ {
		qryCnt := numSeries / 500

		queryLatencies := make([]float64, 0, qryCnt)
		for i := 0; i < qryCnt; i++ {
			matcher, _ := labels.NewMatcher(labels.MatchEqual, "__name__", fmt.Sprintf("metric_%d", k))
			matcher2, _ := labels.NewMatcher(labels.MatchEqual, fmt.Sprintf("label_%d", i*10+k), fmt.Sprintf("value_%d", i*10+k))

			var st, et int64
			st = 0
			et = int64(numSamples + 1)

			var query *prompb.Query
			if k == 1 {
				matcher1, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric_11")
				query, _ = ToQuery(st, et, []*labels.Matcher{matcher1, matcher2}, nil)
				req := &prompb.ReadRequest{
					Queries:               []*prompb.Query{query},
					AcceptedResponseTypes: nil,
				}
				data, _ := proto.Marshal(req)
				compressed := snappy.Encode(nil, data)
				request, _ := http.NewRequest(http.MethodPost, RemoteQueryServer, bytes.NewBuffer(compressed))

				now := time.Now()

				response, _ := cli.Client.Do(request)
				defer response.Body.Close()

				body, _ := io.ReadAll(response.Body)
				uncompressed, _ := snappy.Decode(nil, body)

				resp := &prompb.ReadResponse{}
				_ = proto.Unmarshal(uncompressed, resp)

				lag := time.Since(now).Nanoseconds()
				queryLatencies = append(queryLatencies, float64(lag)/1e6)
			}

			query, _ = ToQuery(st, et, []*labels.Matcher{matcher, matcher2}, nil)

			req := &prompb.ReadRequest{
				Queries:               []*prompb.Query{query},
				AcceptedResponseTypes: nil,
			}
			data, _ := proto.Marshal(req)
			compressed := snappy.Encode(nil, data)
			request, _ := http.NewRequest(http.MethodPost, RemoteQueryServer, bytes.NewBuffer(compressed))

			now := time.Now()

			response, _ := cli.Client.Do(request)
			defer response.Body.Close()

			body, _ := io.ReadAll(response.Body)
			uncompressed, _ := snappy.Decode(nil, body)

			resp := &prompb.ReadResponse{}
			_ = proto.Unmarshal(uncompressed, resp)

			lag := time.Since(now).Nanoseconds()
			queryLatencies = append(queryLatencies, float64(lag)/1e6)
		}

		avg, p50, p90, p99 := calcLatencyStats(queryLatencies)
		fmt.Printf("Query: series: %d , samples: %d , avg: %.3f ms, P50: %.3f ms, P90: %.3f ms, P99: %.3f ms\n", qryCnt, numSamples, avg, p50, p90, p99)
	}

}

func TSBSQuery(qryType string, qryCnt, threads int) {

	// Generate SamplesPerThreeDay samples so data spans > 72h (last timestamp at 259,230,000 ms)
	numSamples := int(SamplesPerThreeDay)
	numSeries := 1000

	SendBatchSample(numSeries, numSamples, 0)
	fmt.Printf("\nInsert: series: %d , samples: %d\n", numSeries, numSamples)

	cli, err := InitQueryClient()
	if err != nil {
		panic(err)
	}

	metricCnt := 0
	tagCnt := 0
	startTime := 0
	endTime := 0
	switch qryType {
	case "1-1-1":
		metricCnt = 1
		tagCnt = 1
		startTime = 0
		endTime = HourMs

		break
	case "1-1-24":
		metricCnt = 1
		tagCnt = 1
		startTime = 0
		endTime = DayMs

		break
	case "1-8-1":
		metricCnt = 1
		tagCnt = 8
		startTime = 0
		endTime = HourMs

		break
	case "5-1-1":
		metricCnt = 5
		tagCnt = 1
		startTime = 0
		endTime = HourMs

		break
	case "5-8-1":
		metricCnt = 5
		tagCnt = 8
		startTime = 0
		endTime = HourMs

		break
	case "5-1-24":
		metricCnt = 5
		tagCnt = 1
		startTime = 0
		endTime = DayMs

		break
	case "5-8-24":
		metricCnt = 5
		tagCnt = 8
		startTime = 0
		endTime = DayMs

		break
	case "double-groupby-all":
		metricCnt = NumMetrics
		tagCnt = numSeries / NumMetrics
		startTime = 0
		endTime = DayMs

		break
	default:
		panic("Invalid query type")
	}

	matchers := make([]*labels.Matcher, 0)
	matchers2 := make([][]*labels.Matcher, 0)
	for i := range metricCnt {
		m, _ := labels.NewMatcher(labels.MatchEqual, "__name__", fmt.Sprintf("metric_%d", i))
		matchers = append(matchers, m)

		tags := make([]*labels.Matcher, 0)
		for j := range tagCnt {
			t, _ := labels.NewMatcher(labels.MatchEqual, "tag", fmt.Sprintf("value_%d", j))
			tags = append(tags, t)
		}
		matchers2 = append(matchers2, tags)
	}

	// Pre-build the query request body once (all qryCnt requests are identical).
	queries := make([]*prompb.Query, 0)
	for j := range tagCnt {
		for k := range metricCnt {
			query, _ := ToQuery(int64(startTime), int64(endTime), []*labels.Matcher{matchers[k], matchers2[k][j]}, nil)
			queries = append(queries, query)
		}
	}
	req := &prompb.ReadRequest{
		Queries:               queries,
		AcceptedResponseTypes: nil,
	}
	data, _ := proto.Marshal(req)
	requestBody := snappy.Encode(nil, data)

	// Run concurrent query workers.
	queryLatencies, totalSamples := runQueryWorkers(cli, requestBody, qryCnt, threads)

	avg, p50, p90, p99 := calcLatencyStats(queryLatencies)
	fmt.Printf("Query: %d , type: %s , avg: %.3f ms, P50: %.3f ms, P90: %.3f ms, P99: %.3f ms, samples: %d\n", qryCnt, qryType, avg, p50, p90, p99, totalSamples)

}

// TSBSQueryAll runs all query types (except double-groupby-all) and saves results to a file.
func TSBSQueryAll(qryCnt int, outputFile string, threads int) {
	// Generate SamplesPerThreeDay samples so data spans > 72h (last timestamp at 259,230,000 ms)
	numSamples := int(SamplesPerThreeDay)
	numSeries := 1000

	SendBatchSample(numSeries, numSamples, 0)
	fmt.Printf("\nInsert: series: %d , samples: %d\n", numSeries, numSamples)

	cli, err := InitQueryClient()
	if err != nil {
		panic(err)
	}

	type queryType struct {
		name      string
		metricCnt int
		tagCnt    int
		startTime int
		endTime   int
	}

	types := []queryType{
		{"1-1-1", 1, 1, 0, HourMs},
		{"1-1-24", 1, 1, 0, DayMs},
		{"1-8-1", 1, 8, 0, HourMs},
		{"5-1-1", 5, 1, 0, HourMs},
		{"5-8-1", 5, 8, 0, HourMs},
		{"5-1-24", 5, 1, 0, DayMs},
		{"5-8-24", 5, 8, 0, DayMs},
	}

	f, err := os.Create(outputFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	fmt.Fprintf(f, "Query Results (qryCnt=%d)\n", qryCnt)
	fmt.Fprintf(f, "%-20s %10s %10s %10s %10s %10s\n", "Type", "Avg(ms)", "P50(ms)", "P90(ms)", "P99(ms)", "Samples")
	fmt.Fprintf(f, "-------------------------------------------------------------------\n")

	for _, qt := range types {
		matchers := make([]*labels.Matcher, 0, qt.metricCnt)
		matchers2 := make([][]*labels.Matcher, 0, qt.metricCnt)
		for i := range qt.metricCnt {
			m, _ := labels.NewMatcher(labels.MatchEqual, "__name__", fmt.Sprintf("metric_%d", i))
			matchers = append(matchers, m)

			tags := make([]*labels.Matcher, 0, qt.tagCnt)
			for j := range qt.tagCnt {
				t, _ := labels.NewMatcher(labels.MatchEqual, "tag", fmt.Sprintf("value_%d", j))
				tags = append(tags, t)
			}
			matchers2 = append(matchers2, tags)
		}

		// Pre-build the query request body once for this query type.
		queries := make([]*prompb.Query, 0)
		for j := range qt.tagCnt {
			for k := range qt.metricCnt {
				query, _ := ToQuery(int64(qt.startTime), int64(qt.endTime), []*labels.Matcher{matchers[k], matchers2[k][j]}, nil)
				queries = append(queries, query)
			}
		}
		req := &prompb.ReadRequest{
			Queries:               queries,
			AcceptedResponseTypes: nil,
		}
		data, _ := proto.Marshal(req)
		requestBody := snappy.Encode(nil, data)

		// Run concurrent query workers.
		queryLatencies, totalSamples := runQueryWorkers(cli, requestBody, qryCnt, threads)

		avg, p50, p90, p99 := calcLatencyStats(queryLatencies)
		fmt.Printf("Query: %d , type: %s , avg: %.3f ms, P50: %.3f ms, P90: %.3f ms, P99: %.3f ms, samples: %d\n", qryCnt, qt.name, avg, p50, p90, p99, totalSamples)
		fmt.Fprintf(f, "%-20s %10.3f %10.3f %10.3f %10.3f %10d\n", qt.name, avg, p50, p90, p99, totalSamples)
	}

	fmt.Printf("\nResults saved to %s\n", outputFile)
}
