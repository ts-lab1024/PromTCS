package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

type Compression string

const (
	// SnappyBlockCompression represents https://github.com/google/snappy/blob/2c94e11145f0b7b184b831577c93e5a41c4c0346/format_description.txt
	SnappyBlockCompression         Compression = "snappy"
	appProtoContentType                        = "application/x-protobuf"
	RemoteWriteVersionHeader                   = "X-Prometheus-Remote-Write-Version"
	RemoteWriteVersion1HeaderValue             = "0.1.0"
	defaultBackoff                             = 0
	maxErrMsgLen                               = 1024
)

var (
	RemoteWriteServer string
	RemoteQueryServer string
)

func init() {
	addr := os.Getenv("PROMTCS_ADDR")
	if addr == "" {
		addr = "http://localhost:9966"
	}
	RemoteWriteServer = addr + "/insert"
	RemoteQueryServer = addr + "/query"
}

// SetRemoteAddr overrides the remote storage address at runtime.
// addr should be the base URL, e.g. "http://localhost:9966".
func SetRemoteAddr(addr string) {
	if addr != "" {
		RemoteWriteServer = addr + "/insert"
		RemoteQueryServer = addr + "/query"
	}
}

const defaultFlushDeadline = 10 * time.Minute

type Client struct {
	remoteName       string
	urlString        string
	Client           *http.Client
	timeout          time.Duration
	retryOnRateLimit bool
	writeProtoMsg    config.RemoteWriteProtoMsg
	writeCompression Compression
}

type ClientConfig struct {
	URL              *config_util.URL
	Timeout          model.Duration
	HTTPClientConfig config_util.HTTPClientConfig
	Headers          map[string]string
	RetryOnRateLimit bool
	WriteProtoMsg    config.RemoteWriteProtoMsg
}

// Name uniquely identifies the client.
func (c *Client) Name() string {
	return c.remoteName
}

// Endpoint is the remote read or write endpoint.
func (c *Client) Endpoint() string {
	return c.urlString
}

func NewWriteClient(name string, conf *ClientConfig) (*Client, error) {
	httpClient, err := config_util.NewClientFromConfig(conf.HTTPClientConfig, "remote_storage_write_client")
	if err != nil {
		return nil, err
	}
	t := httpClient.Transport

	if len(conf.Headers) > 0 {
		t = newInjectHeadersRoundTripper(conf.Headers, t)
	}

	writeProtoMsg := config.RemoteWriteProtoMsgV1
	if conf.WriteProtoMsg != "" {
		writeProtoMsg = conf.WriteProtoMsg
	}

	httpClient.Transport = otelhttp.NewTransport(t)
	return &Client{
		remoteName:       name,
		urlString:        conf.URL.String(),
		Client:           httpClient,
		retryOnRateLimit: conf.RetryOnRateLimit,
		timeout:          time.Duration(conf.Timeout),
		writeProtoMsg:    writeProtoMsg,
		writeCompression: SnappyBlockCompression,
	}, nil
}

func newInjectHeadersRoundTripper(h map[string]string, underlyingRT http.RoundTripper) *injectHeadersRoundTripper {
	return &injectHeadersRoundTripper{headers: h, RoundTripper: underlyingRT}
}

type injectHeadersRoundTripper struct {
	headers map[string]string
	http.RoundTripper
}

func (t *injectHeadersRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}
	return t.RoundTripper.RoundTrip(req)
}

// retryAfterDuration returns the duration for the Retry-After header. In case of any errors, it
// returns the defaultBackoff as if the header was never supplied.
func retryAfterDuration(t string) model.Duration {
	parsedDuration, err := time.Parse(http.TimeFormat, t)
	if err == nil {
		s := time.Until(parsedDuration).Seconds()
		return model.Duration(s) * model.Duration(time.Second)
	}
	// The duration can be in seconds.
	d, err := strconv.Atoi(t)
	if err != nil {
		return defaultBackoff
	}
	return model.Duration(d) * model.Duration(time.Second)
}

// Store sends a batch of samples to the HTTP endpoint, the request is the proto marshalled
// and encoded bytes from codec.go.
func (c *Client) Store(ctx context.Context, req []byte, attempt int) (int, error) {
	httpReq, err := http.NewRequest(http.MethodPost, c.urlString, bytes.NewReader(req))
	sampleNum := 0
	if err != nil {
		// Errors from NewRequest are from unparsable URLs, so are not
		// recoverable.
		return sampleNum, err
	}

	httpReq.Header.Add("Content-Encoding", string(c.writeCompression))
	httpReq.Header.Set("Content-Type", appProtoContentType)
	httpReq.Header.Set("User-Agent", "Prometheus"+RemoteWriteVersion1HeaderValue)
	httpReq.Header.Set(RemoteWriteVersionHeader, RemoteWriteVersion1HeaderValue)

	if attempt > 0 {
		httpReq.Header.Set("Retry-Attempt", strconv.Itoa(attempt))
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ctx, span := otel.Tracer("").Start(ctx, "Remote Store", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	httpResp, err := c.Client.Do(httpReq.WithContext(ctx))
	if err != nil {
		// Errors from Client.Do are from (for example) network errors, so are
		// recoverable.
		return sampleNum, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, httpResp.Body)
		_ = httpResp.Body.Close()
	}()

	if httpResp.StatusCode/100 == 2 {
		return sampleNum, nil
	}

	body, _ := io.ReadAll(io.LimitReader(httpResp.Body, maxErrMsgLen))
	err = fmt.Errorf("server returned HTTP status %s: %s", httpResp.Status, body)

	if httpResp.StatusCode/100 == 5 ||
		(c.retryOnRateLimit && httpResp.StatusCode == http.StatusTooManyRequests) {
		return sampleNum, err
	}
	return sampleNum, err
}

func InitWriteClient() (*Client, error) {
	serverURL, err := url.Parse(RemoteWriteServer)
	if err != nil {
		panic(err)
	}
	rmtWriteConf := config.DefaultRemoteWriteConfig
	cliConf := &ClientConfig{
		URL: &config_util.URL{URL: serverURL},
		//Timeout:          model.Duration(time.Second),
		Timeout:          model.Duration(10 * time.Minute),
		HTTPClientConfig: rmtWriteConf.HTTPClientConfig,
		Headers:          rmtWriteConf.Headers,
		RetryOnRateLimit: rmtWriteConf.QueueConfig.RetryOnRateLimit,
		WriteProtoMsg:    rmtWriteConf.ProtobufMessage,
	}
	client, err := NewWriteClient("Remote-write", cliConf)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func InitQueryClient() (*Client, error) {
	serverURL, err := url.Parse(RemoteQueryServer)
	if err != nil {
		panic(err)
	}
	rmtWriteConf := config.DefaultRemoteWriteConfig
	cliConf := &ClientConfig{
		URL: &config_util.URL{URL: serverURL},
		//Timeout:          model.Duration(time.Second),
		Timeout:          model.Duration(10 * time.Minute),
		HTTPClientConfig: rmtWriteConf.HTTPClientConfig,
		Headers:          rmtWriteConf.Headers,
		RetryOnRateLimit: rmtWriteConf.QueueConfig.RetryOnRateLimit,
		WriteProtoMsg:    rmtWriteConf.ProtobufMessage,
	}
	client, err := NewWriteClient("Remote-query", cliConf)
	if err != nil {
		return nil, err
	}
	return client, nil
}
