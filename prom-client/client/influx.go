package client

import (
	influx "github.com/influxdata/influxdb1-client/v2"
)

const (
	InfluxAddr = "http://192.168.1.101:8086"
	DBName     = "prometheus"
)

// QueryFromInflux
// SELECT * FROM node_cpu_seconds_total WHERE TIME >= '2025-02-26T15:00:00Z' AND TIME < '2025-02-26T16:00:00Z' GROUP BY instance LIMIT 10
func QueryFromInflux(queryString string) (*influx.Response, error) {
	client, err := influx.NewHTTPClient(influx.HTTPConfig{
		Addr: InfluxAddr,
	})
	if err != nil {
		return nil, err
	}

	qry := influx.NewQuery(queryString, DBName, "s")
	resp, err := client.Query(qry)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
