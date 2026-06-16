package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/01spirit/prom-client/prompb"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
)

//func TestSampleGenerateAndSend(t *testing.T) {
//	numSamples := 1
//	numSeries := 10000
//	sendNum := 1
//
//	queryString := `select * from node_cpu_seconds_total where time >= '2025-02-26T15:00:00Z' and time < '2025-02-26T16:00:00Z' group by cpu,mode,instance limit 10`
//	resp, err := QueryFromInflux(queryString)
//	if err != nil {
//		panic(err)
//	}
//
//	fmt.Println("Influx query complete")
//
//	client, err := InitWriteClient()
//	if err != nil {
//		panic(err)
//	}
//
//	queueManager := InitQueueManager(client)
//
//	samplesArr := make([][]record.RefSample, sendNum)
//	for i := 0; i < sendNum; i++ {
//		samples, series, err := createTimeseriesByInflux(numSamples, numSeries, resp, i)
//		if err != nil {
//			panic(err)
//		}
//
//		samplesArr[i] = samples
//		queueManager.StoreSeries(series, 0)
//	}
//
//	totalLag := int64(0)
//	var wg sync.WaitGroup
//	for i := 0; i < 32; i++ {
//		wg.Add(1)
//		go func() {
//			for j := 0; j < sendNum; j++ {
//				//samples, series, err := createTimeseriesByInflux(numSamples, numSeries, resp, i)
//				//if err != nil {
//				//	panic(err)
//				//}
//				//
//				//queueManager.StoreSeries(series, 0)
//
//				samples := samplesArr[j]
//				var (
//					pBuf = proto.NewBuffer(nil)
//					buf  []byte
//				)
//				batch := make([]timeSeries, len(samples))
//				for i, s := range samples {
//					batch[i] = timeSeries{
//						seriesLabels: queueManager.seriesLabels[s.Ref],
//						metadata:     nil,
//						timestamp:    s.T,
//						value:        s.V,
//					}
//				}
//				pendingData := make([]prompb.TimeSeries, len(samples))
//				for i := range pendingData {
//					pendingData[i].Samples = []prompb.Sample{{}}
//				}
//				nPendingSamples := populateTimeSeries(batch, pendingData)
//
//				now := time.Now()
//
//				err = queueManager.shards.sendSamples(context.Background(), pendingData, pBuf, &buf)
//				if err != nil {
//					panic(err)
//				}
//
//				lag := time.Since(now).Nanoseconds()
//				fmt.Printf("batch: %d , samples: %d , latency: %d ns\n", i, nPendingSamples, lag)
//				totalLag += lag
//			}
//			fmt.Printf("\ntotal batch: %d , samples: %d , latency: %d ns\n", sendNum, numSeries*numSamples*sendNum, totalLag)
//			fmt.Printf("average latency: %d ns\n", totalLag/int64(sendNum))
//			wg.Done()
//		}()
//		wg.Wait()
//
//	}
//
//	//totalLag := int64(0)
//	//for i := 0; i < sendNum; i++ {
//	//	samples, series, err := createTimeseriesByInflux(numSamples, numSeries, resp, i)
//	//	if err != nil {
//	//		panic(err)
//	//	}
//	//
//	//	queueManager.StoreSeries(series, 0)
//	//
//	//	var (
//	//		pBuf = proto.NewBuffer(nil)
//	//		buf  []byte
//	//	)
//	//	batch := make([]timeSeries, len(samples))
//	//	for i, s := range samples {
//	//		batch[i] = timeSeries{
//	//			seriesLabels: queueManager.seriesLabels[s.Ref],
//	//			metadata:     nil,
//	//			timestamp:    s.T,
//	//			value:        s.V,
//	//		}
//	//	}
//	//	pendingData := make([]prompb.TimeSeries, len(samples))
//	//	for i := range pendingData {
//	//		pendingData[i].Samples = []prompb.Sample{{}}
//	//	}
//	//	nPendingSamples := populateTimeSeries(batch, pendingData)
//	//
//	//	now := time.Now()
//	//
//	//	err = queueManager.shards.sendSamples(context.Background(), pendingData, pBuf, &buf)
//	//	if err != nil {
//	//		panic(err)
//	//	}
//	//
//	//	lag := time.Since(now).Nanoseconds()
//	//	fmt.Printf("batch: %d , samples: %d , latency: %d ns\n", i, nPendingSamples, lag)
//	//	totalLag += lag
//	//}
//	//fmt.Printf("\ntotal batch: %d , samples: %d , latency: %d ns\n", sendNum, numSeries*numSamples*sendNum, totalLag)
//	//fmt.Printf("average latency: %d ns\n", totalLag/int64(sendNum))
//	////wg.Wait()
//
//}

func TestQuery(t *testing.T) {
	numSamples := 30
	numSeries := 50 * 1000

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

		var queryLatency int64 = 0
		for i := 0; i < qryCnt; i++ {
			matcher, err := labels.NewMatcher(labels.MatchEqual, "__name__", fmt.Sprintf("metric_%d", k))
			matcher2, err := labels.NewMatcher(labels.MatchEqual, fmt.Sprintf("label_%d", i*10+k), fmt.Sprintf("value_%d", i*10+k))
			require.NoError(t, err)
			var st, et int64
			st = 0
			et = int64(numSamples + 1)

			var query *prompb.Query
			if k == 1 {
				matcher1, _ := labels.NewMatcher(labels.MatchEqual, "__name__", "metric_11")
				query, err = ToQuery(st, et, []*labels.Matcher{matcher1, matcher2}, nil)
				require.NoError(t, err)
				req := &prompb.ReadRequest{
					Queries:               []*prompb.Query{query},
					AcceptedResponseTypes: nil,
				}
				data, err := proto.Marshal(req)
				require.NoError(t, err)
				compressed := snappy.Encode(nil, data)
				request, err := http.NewRequest(http.MethodPost, RemoteQueryServer, bytes.NewBuffer(compressed))
				require.NoError(t, err)

				now := time.Now()

				response, err := cli.Client.Do(request)
				require.NoError(t, err)
				defer response.Body.Close()

				body, err := io.ReadAll(response.Body)
				uncompressed, err := snappy.Decode(nil, body)
				require.NoError(t, err)

				resp := &prompb.ReadResponse{}
				err = proto.Unmarshal(uncompressed, resp)
				require.NoError(t, err)

				assert.Equal(t, len(resp.Results[0].Timeseries), 1)
				assert.Equal(t, len(resp.Results[0].Timeseries[0].Samples), numSamples)

				lag := time.Since(now).Nanoseconds()
				queryLatency += lag
			}

			query, err = ToQuery(st, et, []*labels.Matcher{matcher, matcher2}, nil)
			require.NoError(t, err)

			req := &prompb.ReadRequest{
				Queries:               []*prompb.Query{query},
				AcceptedResponseTypes: nil,
			}
			data, err := proto.Marshal(req)
			require.NoError(t, err)
			compressed := snappy.Encode(nil, data)
			request, err := http.NewRequest(http.MethodPost, RemoteQueryServer, bytes.NewBuffer(compressed))
			require.NoError(t, err)

			now := time.Now()

			response, err := cli.Client.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()

			body, err := io.ReadAll(response.Body)
			uncompressed, err := snappy.Decode(nil, body)
			require.NoError(t, err)

			resp := &prompb.ReadResponse{}
			err = proto.Unmarshal(uncompressed, resp)
			require.NoError(t, err)

			assert.Equal(t, len(resp.Results[0].Timeseries), 1)
			assert.Equal(t, len(resp.Results[0].Timeseries[0].Samples), numSamples)

			lag := time.Since(now).Nanoseconds()
			queryLatency += lag
		}

		fmt.Printf("Query: series: %d , samples: %d , avg latency: %f ms\n", qryCnt, numSamples, float64(queryLatency)/float64(qryCnt)/1e6)
	}

}
