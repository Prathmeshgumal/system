// Package metrics defines every Prometheus metric the API exposes on its
// private /metrics endpoint (Part 2.4, Note 9). Other packages only call the
// small helpers here, so all names, labels and buckets live in one file.
//
// Two rules keep Prometheus healthy:
//   - labels have few, known values (route PATTERN, never the raw path with ids)
//   - everything is cheap to update: a counter increment is ~10 ns
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds our metrics plus Go runtime and process metrics
// (goroutines, heap, GC, CPU seconds, open file descriptors).
var Registry = prometheus.NewRegistry()

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests by method, route pattern and status code.",
	}, []string{"method", "route", "code"})

	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by method and route pattern (WebSocket sessions excluded).",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method", "route"})

	wsConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ws_connections",
		Help: "Open WebSocket connections (one per browser tab).",
	})

	wsEvents = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_events_sent_total",
		Help: "Events queued to WebSocket connections, by event type.",
	}, []string{"type"})

	wsDropped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ws_clients_dropped_total",
		Help: "WebSocket connections closed by the server, by reason.",
	}, []string{"reason"})

	messagesSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "chat_messages_sent_total",
		Help: "Chat messages stored (retries of the same client_id not counted).",
	})

	hashSlotsInUse = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "auth_hash_slots_in_use",
		Help: "bcrypt hashes running right now (capped by the bulkhead, Part 1.4).",
	})

	hashQueueWait = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "auth_hash_queue_wait_seconds",
		Help:    "How long a login or signup waited for a free bcrypt slot.",
		Buckets: []float64{.001, .01, .05, .1, .25, .5, 1, 2.5, 5, 10},
	})
)

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequests, httpDuration, wsConnections, wsEvents, wsDropped, messagesSent, hashSlotsInUse, hashQueueWait,
	)
}

// Handler serves the metrics in Prometheus's text format.
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{Registry: Registry})
}

// ObserveHTTP records one finished request. route is the mux pattern
// ("/api/chats/{id}"), or "unmatched" — never the raw path, which would create
// a new time series for every chat id (unbounded cardinality).
func ObserveHTTP(method, route string, status int, d time.Duration) {
	httpRequests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	if route != "/ws" { // a WebSocket "request" lasts as long as the connection
		httpDuration.WithLabelValues(method, route).Observe(d.Seconds())
	}
}

func WSConnected()                 { wsConnections.Inc() }
func WSDisconnected()              { wsConnections.Dec() }
func WSEventSent(eventType string) { wsEvents.WithLabelValues(eventType).Inc() }
func WSDropped(reason string)      { wsDropped.WithLabelValues(reason).Inc() }
func MessageSent()                 { messagesSent.Inc() }
func HashStarted(waited time.Duration) {
	hashSlotsInUse.Inc()
	hashQueueWait.Observe(waited.Seconds())
}
func HashFinished() { hashSlotsInUse.Dec() }

// WatchPool exports the database pool's numbers, read fresh at every scrape.
func WatchPool(pool *pgxpool.Pool) {
	Registry.MustRegister(poolCollector{pool})
}

type poolCollector struct{ pool *pgxpool.Pool }

var (
	poolConns      = prometheus.NewDesc("db_pool_connections", "Pool connections by state.", []string{"state"}, nil)
	poolEmptyWaits = prometheus.NewDesc("db_pool_empty_acquire_total", "Acquires that had to wait because every connection was busy.", nil, nil)
	poolWaitSecs   = prometheus.NewDesc("db_pool_acquire_wait_seconds_total", "Total time spent waiting for a connection.", nil, nil)
	poolAcquires   = prometheus.NewDesc("db_pool_acquire_total", "Connections handed out.", nil, nil)
)

func (c poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- poolConns
	ch <- poolEmptyWaits
	ch <- poolWaitSecs
	ch <- poolAcquires
}

func (c poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(poolConns, prometheus.GaugeValue, float64(s.AcquiredConns()), "in_use")
	ch <- prometheus.MustNewConstMetric(poolConns, prometheus.GaugeValue, float64(s.IdleConns()), "idle")
	ch <- prometheus.MustNewConstMetric(poolConns, prometheus.GaugeValue, float64(s.MaxConns()), "max")
	ch <- prometheus.MustNewConstMetric(poolEmptyWaits, prometheus.CounterValue, float64(s.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(poolWaitSecs, prometheus.CounterValue, s.AcquireDuration().Seconds())
	ch <- prometheus.MustNewConstMetric(poolAcquires, prometheus.CounterValue, float64(s.AcquireCount()))
}
