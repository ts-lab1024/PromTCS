package client

import (
	"encoding/json"
	"fmt"

	influx "github.com/influxdata/influxdb1-client/v2"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

const LabelNum = 8
const NumMetrics = 5

// Extra labels to make a more realistic workload - taken from Kubernetes' embedded cAdvisor metrics.
var extraLabels []labels.Label = []labels.Label{
	{Name: "kubernetes_io_arch", Value: "amd64"},
	{Name: "kubernetes_io_instance_type", Value: "c3.somesize"},
	{Name: "kubernetes_io_os", Value: "linux"},
	{Name: "container_name", Value: "some-name"},
	{Name: "failure_domain_kubernetes_io_region", Value: "somewhere-1"},
	{Name: "failure_domain_kubernetes_io_zone", Value: "somewhere-1b"},
	{Name: "id", Value: "/kubepods/burstable/pod6e91c467-e4c5-11e7-ace3-0a97ed59c75e/a3c8498918bd6866349fed5a6f8c643b77c91836427fb6327913276ebc6bde28"},
	{Name: "image", Value: "registry/organisation/name@sha256:dca3d877a80008b45d71d7edc4fd2e44c0c8c8e7102ba5cbabec63a374d1d506"},
	{Name: "instance", Value: "ip-111-11-1-11.ec2.internal"},
	{Name: "job", Value: "kubernetes-cadvisor"},
	{Name: "kubernetes_io_hostname", Value: "ip-111-11-1-11"},
	{Name: "monitor", Value: "prod"},
	{Name: "name", Value: "k8s_some-name_some-other-name-5j8s8_kube-system_6e91c467-e4c5-11e7-ace3-0a97ed59c75e_0"},
	{Name: "namespace", Value: "kube-system"},
	{Name: "pod_name", Value: "some-other-name-5j8s8"},
}

func createTimeseries(numSamples, numSeries int, extraLabels ...labels.Label) ([]record.RefSample, []record.RefSeries) {
	samples := make([]record.RefSample, 0, numSamples)
	series := make([]record.RefSeries, 0, numSeries)
	lb := labels.NewScratchBuilder(1 + len(extraLabels))
	for i := 0; i < numSeries; i++ {
		//name := fmt.Sprintf("test_metric_%d", i)
		name := "test_metric"
		for j := 0; j < numSamples; j++ {
			samples = append(samples, record.RefSample{
				Ref: chunks.HeadSeriesRef(i),
				T:   int64(j),
				V:   float64(i),
			})
		}
		// Create Labels that is name of series plus any extra labels supplied.
		lb.Reset()
		lb.Add(labels.MetricName, name)
		lb.Add("label_all", "value_all")
		lb.Add(fmt.Sprintf("label_%d", i), fmt.Sprintf("value_%d", i))
		//rand.Shuffle(len(extraLabels), func(i, j int) {
		//	extraLabels[i], extraLabels[j] = extraLabels[j], extraLabels[i]
		//})
		//for _, l := range extraLabels {
		//	lb.Add(l.Name, l.Value)
		//}
		lb.Sort()
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: lb.Labels(),
		})
	}
	return samples, series
}

func createSingleTimeseries(numSeries int, startTime int64) ([]record.RefSample, []record.RefSeries) {
	samples := make([]record.RefSample, 0, 1)
	series := make([]record.RefSeries, 0, numSeries)
	lb := labels.NewScratchBuilder(1 + LabelNum)

	var metricNum, sourceNum int
	if numSeries < 1000*500 {
		sourceNum = 1000
		metricNum = numSeries / sourceNum
	} else {
		metricNum = 500
		sourceNum = numSeries / metricNum
	}

	for i := 0; i < metricNum; i++ {
		name := fmt.Sprintf("metric_%d", i)
		for j := 0; j < sourceNum; j++ {
			samples = append(samples, record.RefSample{
				Ref: chunks.HeadSeriesRef(i*sourceNum + j),
				T:   startTime,
				V:   float64(i*sourceNum + j),
			})
			lb.Reset()
			lb.Add(labels.MetricName, name)
			for k := 0; k < LabelNum-1; k++ {
				lb.Add(fmt.Sprintf("label_%d", j*10+k), fmt.Sprintf("value_%d", j*10+k))
			}
			lb.Sort()
			series = append(series, record.RefSeries{
				Ref:    chunks.HeadSeriesRef(i*sourceNum + j),
				Labels: lb.Labels(),
			})
		}
	}

	return samples, series
}

func createBatchTimeseries(numSeries int, numSamples int, startTime int64) ([][]record.RefSample, []record.RefSeries) {
	samples := make([][]record.RefSample, 0, 1)
	series := make([]record.RefSeries, 0, numSeries)
	lb := labels.NewScratchBuilder(2)

	for i := 0; i < numSeries; i++ {
		sps := make([]record.RefSample, 0, 1)
		metricIdx := i % NumMetrics
		tagIdx := i / NumMetrics
		name := fmt.Sprintf("metric_%d", metricIdx)
		lb.Reset()
		lb.Add(labels.MetricName, name)
		lb.Add("tag", fmt.Sprintf("value_%d", tagIdx))
		lb.Sort()
		for j := range numSamples {
			sps = append(sps, record.RefSample{
				T: startTime + int64(j)*StepMs,
				V: float64(i * 10),
			})
		}
		samples = append(samples, sps)
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: lb.Labels(),
		})
	}

	return samples, series
}

func createTimeseriesByInflux(numSamples, numSeries int, resp *influx.Response, rowIdx int) ([][]record.RefSample, []record.RefSeries, error) {
	seriesSamples := make([][]record.RefSample, 0, numSeries)
	series := make([]record.RefSeries, 0, numSeries)

	if len(resp.Results) == 0 || len(resp.Results[0].Series) == 0 {
		return nil, nil, fmt.Errorf("InfluxDB query returned empty results, check if data exists in the target database")
	}
	respSeriesNUm := len(resp.Results[0].Series)

	lb := labels.NewScratchBuilder(LabelNum)
	metricCnt := 0
	sourceCnt := 0
	if numSeries <= 1000*500 {
		sourceCnt = 1000
		metricCnt = numSeries / sourceCnt
	} else {
		metricCnt = 500
		sourceCnt = numSeries / metricCnt
	}

	for i := 0; i < metricCnt; i++ {
		name := fmt.Sprintf("metric_%d", i)
		for j := 0; j < sourceCnt; j++ {
			s := resp.Results[0].Series[(i*sourceCnt+j)%respSeriesNUm]
			samples := make([]record.RefSample, 0, numSamples)
			for k := 0; k < numSamples; k++ {
				jnVal := s.Values[(rowIdx+k)%len(s.Values)][len(s.Columns)-1].(json.Number)
				val, err := jnVal.Float64()
				if err != nil {
					panic(err)
				}
				samples = append(samples, record.RefSample{
					Ref: chunks.HeadSeriesRef(i*sourceCnt + j),
					T:   int64((i*sourceCnt + j) * 100),
					V:   val,
				})
			}
			lb.Reset()
			lb.Add(labels.MetricName, name)
			for k := 0; k < LabelNum-1; k++ {
				lb.Add(fmt.Sprintf("label_%d", j*10+k), fmt.Sprintf("value_%d", j*10+k))
			}
			lb.Sort()
			series = append(series, record.RefSeries{
				Ref:    chunks.HeadSeriesRef(i*sourceCnt + j),
				Labels: lb.Labels(),
			})
			seriesSamples = append(seriesSamples, samples)
		}
	}

	return seriesSamples, series, nil
}
