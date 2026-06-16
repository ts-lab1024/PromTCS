package client

import (
	"context"
	"sync"
	"time"

	"github.com/01spirit/prom-client/prompb"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"go.uber.org/atomic"
)

const (
	ewmaWeight          = 0.2
	shardUpdateDuration = 10 * time.Second

	shardToleranceFraction = 0.3

	reasonTooOld                     = "too_old"
	reasonDroppedSeries              = "dropped_series"
	reasonUnintentionalDroppedSeries = "unintentionally_dropped_series"
)

type QueueManager struct {
	clientMtx   sync.RWMutex
	storeClient *Client
	protoMsg    config.RemoteWriteProtoMsg
	cfg         config.QueueConfig

	watcher *wlog.Watcher

	flushDeadline  time.Duration
	externalLabels []labels.Label
	relabelConfigs []*relabel.Config

	seriesMtx      sync.Mutex
	seriesLabels   map[chunks.HeadSeriesRef]labels.Labels
	seriesMetadata map[chunks.HeadSeriesRef]*metadata.Metadata
	builder        *labels.Builder

	seriesSegmentMtx     sync.Mutex
	seriesSegmentIndexes map[chunks.HeadSeriesRef]int

	shards      *shards
	numShards   int
	reshardChan chan int
	quit        chan struct{}
	wg          sync.WaitGroup

	interner *pool
}

func NewQueueManager(
	client *Client,
	protoMsg config.RemoteWriteProtoMsg,
	cfg config.QueueConfig,
	flushDeadline time.Duration,
	externalLabels labels.Labels,
	relabelConfigs []*relabel.Config,
	interner *pool,
) *QueueManager {
	extLabelsSlice := make([]labels.Label, 0, externalLabels.Len())
	externalLabels.Range(func(l labels.Label) {
		extLabelsSlice = append(extLabelsSlice, l)
	})

	t := &QueueManager{
		clientMtx:      sync.RWMutex{},
		storeClient:    client,
		protoMsg:       protoMsg,
		cfg:            cfg,
		flushDeadline:  flushDeadline,
		externalLabels: extLabelsSlice,
		relabelConfigs: relabelConfigs,

		seriesMtx:            sync.Mutex{},
		seriesLabels:         make(map[chunks.HeadSeriesRef]labels.Labels),
		seriesMetadata:       make(map[chunks.HeadSeriesRef]*metadata.Metadata),
		builder:              labels.NewBuilder(labels.EmptyLabels()),
		seriesSegmentIndexes: make(map[chunks.HeadSeriesRef]int),
		seriesSegmentMtx:     sync.Mutex{},

		numShards:   cfg.MinShards,
		reshardChan: make(chan int),
		quit:        make(chan struct{}),

		interner: interner,
	}

	t.shards = t.newShards()

	return t
}

func (t *QueueManager) client() *Client {
	t.clientMtx.RLock()
	defer t.clientMtx.RUnlock()
	return t.storeClient
}

func (t *QueueManager) SetClient(c *Client) {
	t.clientMtx.Lock()
	t.storeClient = c
	t.clientMtx.Unlock()
}

func (t *QueueManager) Append(samples []record.RefSample) bool {
outer:
	for _, s := range samples {
		t.seriesMtx.Lock()
		lbls, ok := t.seriesLabels[s.Ref]
		if !ok {
			t.seriesMtx.Unlock()
			continue
		}
		meta := t.seriesMetadata[s.Ref]
		t.seriesMtx.Unlock()
		backoff := model.Duration(5 * time.Millisecond)
		for {
			select {
			case <-t.quit:
				return false
			default:
			}
			if t.shards.enqueue(s.Ref, timeSeries{
				seriesLabels: lbls,
				metadata:     meta,
				timestamp:    s.T,
				value:        s.V,
			}) {
				continue outer
			}

			time.Sleep(time.Duration(backoff))
			backoff *= 2
			if backoff > t.cfg.MaxBackoff {
				backoff = t.cfg.MaxBackoff
			}
		}
	}
	return true
}

func (t *QueueManager) Start() {
	t.shards.start(t.numShards)
	t.wg.Add(2)
}

func (t *QueueManager) newShards() *shards {
	s := &shards{
		qm:   t,
		done: make(chan struct{}),
	}
	return s
}

func (t *QueueManager) Stop() {
	close(t.quit)
	t.wg.Wait()
	t.shards.stop()
	t.seriesMtx.Lock()
	for _, lbs := range t.seriesLabels {
		t.releaseLabels(lbs)
	}
	t.seriesMtx.Unlock()
}

func (t *QueueManager) internLabels(lbls labels.Labels) {
	lbls.InternStrings(t.interner.intern)
}

func (t *QueueManager) releaseLabels(ls labels.Labels) {
	ls.ReleaseStrings(t.interner.release)
}

func processExternalLabels(b *labels.Builder, externalLabels []labels.Label) {
	for _, el := range externalLabels {
		if b.Get(el.Name) == "" {
			b.Set(el.Name, el.Value)
		}
	}
}

func (t *QueueManager) StoreSeries(series []record.RefSeries, index int) {
	t.seriesMtx.Lock()
	defer t.seriesMtx.Unlock()
	t.seriesSegmentMtx.Lock()
	defer t.seriesSegmentMtx.Unlock()
	for _, s := range series {
		// Just make sure all the Refs of Series will insert into seriesSegmentIndexes map for tracking.
		t.seriesSegmentIndexes[s.Ref] = index

		t.builder.Reset(s.Labels)
		processExternalLabels(t.builder, t.externalLabels)
		keep := relabel.ProcessBuilder(t.builder, t.relabelConfigs...)
		if !keep {
			continue
		}
		lbls := t.builder.Labels()
		t.internLabels(lbls)

		// We should not ever be replacing a series labels in the map, but just
		// in case we do we need to ensure we do not leak the replaced interned
		// strings.
		if orig, ok := t.seriesLabels[s.Ref]; ok {
			t.releaseLabels(orig)
		}
		t.seriesLabels[s.Ref] = lbls
	}
}

func (t *QueueManager) UpdateSeriesSegment(series []record.RefSeries, index int) {
	t.seriesSegmentMtx.Lock()
	defer t.seriesSegmentMtx.Unlock()
	for _, s := range series {
		t.seriesSegmentIndexes[s.Ref] = index
	}
}

type shards struct {
	mtx          sync.RWMutex
	qm           *QueueManager
	queues       []*queue
	done         chan struct{}
	running      atomic.Int32
	softShutdown chan struct{}
	hardShutdown context.CancelFunc
}

func (s *shards) start(n int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	newQueues := make([]*queue, n)
	for i := 0; i < n; i++ {
		newQueues[i] = newQueue(s.qm.cfg.MaxSamplesPerSend, s.qm.cfg.Capacity)
	}

	s.queues = newQueues

	var hardShutdownCtx context.Context
	hardShutdownCtx, s.hardShutdown = context.WithCancel(context.Background())
	s.softShutdown = make(chan struct{})
	s.running.Store(int32(n))
	s.done = make(chan struct{})
	for i := 0; i < n; i++ {
		go s.runShard(hardShutdownCtx, i, newQueues[i])
	}
}

func (s *shards) stop() {
	s.mtx.RLock()
	close(s.softShutdown)
	s.mtx.RUnlock()

	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, queue := range s.queues {
		go queue.FlushAndShutdown(s.done)
	}
	select {
	case <-s.done:
		return
	case <-time.After(s.qm.flushDeadline):
	}

	s.hardShutdown()
	<-s.done
}

func (s *shards) enqueue(ref chunks.HeadSeriesRef, data timeSeries) bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	shard := uint64(ref) % uint64(len(s.queues))
	select {
	case <-s.softShutdown:
		return false
	default:
		appended := s.queues[shard].Append(data)
		if !appended {
			return false
		}
		return true
	}
}

func (s *shards) sendSamples(ctx context.Context, samples []prompb.TimeSeries, pBuf *proto.Buffer, buf *[]byte) error {
	req, err := buildWriteRequest(samples, nil, pBuf, buf)
	if err != nil {
		return err
	}

	try := 0
	*buf = req
	_, err = s.qm.client().Store(ctx, *buf, try)

	return err
}

func (s *shards) runShard(ctx context.Context, shardID int, queue *queue) {
	defer func() {
		if s.running.Dec() == 0 {
			close(s.done)
		}
	}()

	var (
		maxCount = s.qm.cfg.MaxSamplesPerSend

		pBuf = proto.NewBuffer(nil)
		buf  []byte
	)

	batchQueue := queue.Chan()
	pendingData := make([]prompb.TimeSeries, maxCount)
	for i := range pendingData {
		pendingData[i].Samples = []prompb.Sample{{}}
	}

	timer := time.NewTimer(time.Duration(s.qm.cfg.BatchSendDeadline))
	stop := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	defer stop()

	sendBatch := func(batch []timeSeries, protoMsg config.RemoteWriteProtoMsg) {
		switch protoMsg {
		case config.RemoteWriteProtoMsgV1:
			nPendingSamples := populateTimeSeries(batch, pendingData)
			n := nPendingSamples
			_ = s.sendSamples(ctx, pendingData[:n], pBuf, &buf)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case batch, ok := <-batchQueue:
			if !ok {
				return
			}
			sendBatch(batch, s.qm.protoMsg)
			queue.ReturnForReuse(batch)
			stop()
			timer.Reset(time.Duration(s.qm.cfg.BatchSendDeadline))

		case <-timer.C:
			batch := queue.Batch()
			if len(batch) > 0 {
				sendBatch(batch, s.qm.protoMsg)
			}
			queue.ReturnForReuse(batch)
			timer.Reset(time.Duration(s.qm.cfg.BatchSendDeadline))
		}
	}
}

type queue struct {
	batchMtx   sync.Mutex
	batch      []timeSeries
	batchQueue chan []timeSeries

	poolMtx   sync.Mutex
	batchPool [][]timeSeries
}

type timeSeries struct {
	seriesLabels labels.Labels
	value        float64
	metadata     *metadata.Metadata
	timestamp    int64
}

func newQueue(batchSize, capacity int) *queue {
	batches := capacity / batchSize
	if batches == 0 {
		batches = 1
	}
	return &queue{
		batchMtx:   sync.Mutex{},
		batch:      make([]timeSeries, 0, batchSize),
		batchQueue: make(chan []timeSeries, batches),
		poolMtx:    sync.Mutex{},
		batchPool:  make([][]timeSeries, 0, batches+1),
	}
}

func (q *queue) Append(datum timeSeries) bool {
	q.batchMtx.Lock()
	defer q.batchMtx.Unlock()
	q.batch = append(q.batch, datum)
	if len(q.batch) == cap(q.batch) {
		select {
		case q.batchQueue <- q.batch:
			q.batch = q.newBatch(cap(q.batch))
			return true
		default:
			q.batch = q.batch[:len(q.batch)-1]
			return false
		}
	}
	return true
}

func (q *queue) Chan() <-chan []timeSeries {
	return q.batchQueue
}

func (q *queue) Batch() []timeSeries {
	q.batchMtx.Lock()
	defer q.batchMtx.Unlock()
	select {
	case batch := <-q.batchQueue:
		return batch
	default:
		batch := q.batch
		q.batch = q.newBatch(cap(batch))
		return batch
	}
}

func (q *queue) ReturnForReuse(batch []timeSeries) {
	q.poolMtx.Lock()
	defer q.poolMtx.Unlock()
	if len(q.batchPool) < cap(q.batchPool) {
		q.batchPool = append(q.batchPool, batch[:0])
	}
}

func (q *queue) FlushAndShutdown(done <-chan struct{}) {
	for q.tryEnqueueingBatch(done) {
		time.Sleep(time.Second)
	}
	q.batchMtx.Lock()
	defer q.batchMtx.Unlock()
	q.batch = nil
	close(q.batchQueue)
}

func (q *queue) tryEnqueueingBatch(done <-chan struct{}) bool {
	q.batchMtx.Lock()
	defer q.batchMtx.Unlock()
	if len(q.batch) == 0 {
		return false
	}

	select {
	case q.batchQueue <- q.batch:
		return false
	case <-done:
		// The shard has been hard shut down, so no more samples can be sent.
		// No need to try again as we will drop everything left in the queue.
		return false
	default:
		// The batchQueue is full, so we need to try again later.
		return true
	}
}

func (q *queue) newBatch(capacity int) []timeSeries {
	q.poolMtx.Lock()
	defer q.poolMtx.Unlock()
	batches := len(q.batchPool)
	if batches > 0 {
		batch := q.batchPool[batches-1]
		q.batchPool = q.batchPool[:batches-1]
		return batch
	}
	return make([]timeSeries, 0, capacity)
}

func compressPayload(tmpbuf *[]byte, inp []byte) (compressed []byte) {
	compressed = snappy.Encode(*tmpbuf, inp)
	if n := snappy.MaxEncodedLen(len(inp)); n > len(*tmpbuf) {
		*tmpbuf = make([]byte, n)
	}
	return compressed
}

func buildWriteRequest(timeSeries []prompb.TimeSeries, metadata []prompb.MetricMetadata, pBuf *proto.Buffer, buf *[]byte) (compressed []byte, _ error) {
	req := &prompb.WriteRequest{
		Timeseries: timeSeries,
		Metadata:   metadata,
	}

	if pBuf == nil {
		pBuf = proto.NewBuffer(nil)
	} else {
		pBuf.Reset()
	}
	err := pBuf.Marshal(req)
	if err != nil {
		return nil, err
	}

	if buf != nil {
		*buf = (*buf)[0:cap(*buf)]
	} else {
		buf = &[]byte{}
	}

	compressed = compressPayload(buf, pBuf.Bytes())
	return compressed, nil
}

func populateTimeSeriesWithMultiSamples(batch []timeSeries, samplesArr [][]record.RefSample, numSamples int, pendingData []prompb.TimeSeries) int {
	var nPendingSamples int

	for nPending, d := range batch {
		pendingData[nPending].Samples = pendingData[nPending].Samples[:0]
		pendingData[nPending].Labels = prompb.FromLabels(d.seriesLabels, pendingData[nPending].Labels)
		for i := 0; i < numSamples; i++ {
			pendingData[nPending].Samples = append(pendingData[nPending].Samples, prompb.Sample{
				Value:     samplesArr[nPending][i%numSamples].V,
				Timestamp: samplesArr[nPending][i%numSamples].T,
			})
			nPendingSamples++
		}

	}

	return nPendingSamples
}

func populateTimeSeriesWithMultiSamplesWithStartTime(batch []timeSeries, samplesArr [][]record.RefSample, numSamples int, startTime int64, pendingData []prompb.TimeSeries) int {
	var nPendingSamples int

	for nPending, d := range batch {
		pendingData[nPending].Samples = pendingData[nPending].Samples[:0]
		pendingData[nPending].Labels = prompb.FromLabels(d.seriesLabels, pendingData[nPending].Labels)
		for i := 0; i < numSamples; i++ {
			pendingData[nPending].Samples = append(pendingData[nPending].Samples, prompb.Sample{
				Value:     samplesArr[nPending][i%numSamples].V,
				Timestamp: startTime + int64(i)*StepMs,
			})
			nPendingSamples++
		}

	}

	return nPendingSamples
}

func populateTimeSeries(batch []timeSeries, pendingData []prompb.TimeSeries) int {
	var nPendingSamples int

	for nPending, d := range batch {
		pendingData[nPending].Samples = pendingData[nPending].Samples[:0]
		pendingData[nPending].Labels = prompb.FromLabels(d.seriesLabels, pendingData[nPending].Labels)
		pendingData[nPending].Samples = append(pendingData[nPending].Samples, prompb.Sample{
			Value:     d.value,
			Timestamp: d.timestamp,
		})
		nPendingSamples++
	}

	return nPendingSamples
}

func InitQueueManager(c *Client) *QueueManager {
	queConf := config.DefaultQueueConfig
	queConf.BatchSendDeadline = model.Duration(1 * time.Minute)
	queConf.MinShards = 20
	queConf.MaxShards = 20
	queueManager := NewQueueManager(c, config.RemoteWriteProtoMsgV1, queConf, defaultFlushDeadline, labels.EmptyLabels(), nil, newPool())
	return queueManager
}
