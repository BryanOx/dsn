package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RPCRequestsTotal counts total RPC requests by method
	RPCRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dsn_rpc_requests_total",
			Help: "Total RPC requests",
		},
		[]string{"method"},
	)

	// RPCDuration records RPC request duration in seconds
	RPCDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dsn_rpc_duration_seconds",
			Help:    "RPC request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)

	// IndexerHeight tracks the latest indexed block height
	IndexerHeight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dsn_indexer_height",
			Help: "Latest indexed block height",
		},
	)

	// MempoolTxCount tracks the current mempool transaction count
	MempoolTxCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dsn_mempool_tx_count",
			Help: "Current mempool transaction count",
		},
	)

	// PeerCount tracks the number of connected peers
	PeerCount = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dsn_peer_count",
			Help: "Number of connected peers",
		},
	)

	// WSConnections tracks active WebSocket connections
	WSConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dsn_ws_connections_active",
			Help: "Active WebSocket connections",
		},
	)

	// IndexerLag tracks the lag between node height and indexed height
	IndexerLag = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dsn_indexer_lag_blocks",
			Help: "Indexer lag behind node height",
		},
	)

	// ExplorerRequestsTotal counts total explorer requests by path
	ExplorerRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dsn_explorer_requests_total",
			Help: "Total explorer API requests",
		},
		[]string{"path", "method"},
	)

	// ExplorerDuration records explorer request duration in seconds
	ExplorerDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dsn_explorer_duration_seconds",
			Help:    "Explorer request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"path", "method"},
	)
)