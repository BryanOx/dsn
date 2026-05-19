package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// BlockProcessingTime measures block processing duration in seconds
	BlockProcessingTime = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "dsn_block_processing_seconds",
			Help:    "Time spent processing blocks",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
	)

	// MempoolSize tracks the current mempool size
	MempoolSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dsn_mempool_current_size",
			Help: "Current number of transactions in mempool",
		},
	)

	// MempoolMaxSize is the maximum mempool capacity
	MempoolMaxSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dsn_mempool_max_size",
			Help: "Maximum mempool capacity",
		},
	)

	// PeerCountGauge tracks the number of connected peers
	PeerCountGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "dsn_peer_count_gauge",
			Help: "Number of connected peers (defensive metric)",
		},
	)

	// ConsensusRoundCounter counts consensus rounds
	ConsensusRoundCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "dsn_consensus_rounds_total",
			Help: "Total number of consensus rounds completed",
		},
	)

	// StateRootMismatchCounter counts state root mismatches detected
	StateRootMismatchCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "dsn_state_root_mismatch_total",
			Help: "Number of state root mismatches detected",
		},
	)

	// PanicRecoveryCounter counts panic recoveries
	PanicRecoveryCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "dsn_panic_recovery_total",
			Help: "Number of panic recoveries",
		},
	)

	// GasLimitRejections counts transactions rejected due to gas limits
	GasLimitRejections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dsn_gas_limit_rejections_total",
			Help: "Number of transactions rejected due to gas limits",
		},
		[]string{"reason"},
	)

	// MempoolEvictions counts transactions evicted from mempool
	MempoolEvictions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dsn_mempool_evictions_total",
			Help: "Number of transactions evicted from mempool",
		},
		[]string{"reason"},
	)

	// GossipDroppedMessages counts gossip messages dropped due to rate limiting
	GossipDroppedMessages = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dsn_gossip_dropped_total",
			Help: "Number of gossip messages dropped",
		},
		[]string{"reason", "message_type"},
	)

	// BlockValidationFailures counts block validation failures
	BlockValidationFailures = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dsn_block_validation_failures_total",
			Help: "Number of block validation failures",
		},
		[]string{"reason"},
	)

	// SignatureVerificationFailures counts signature verification failures
	SignatureVerificationFailures = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "dsn_signature_verification_failures_total",
			Help: "Number of signature verification failures",
		},
	)

	// ConnectionErrors counts P2P connection errors
	ConnectionErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dsn_connection_errors_total",
			Help: "Number of P2P connection errors",
		},
		[]string{"direction"},
	)
)

// RecordBlockProcessingTime records the time spent processing a block
func RecordBlockProcessingTime(seconds float64) {
	BlockProcessingTime.Observe(seconds)
}

// SetMempoolSize sets the current mempool size
func SetMempoolSize(size int) {
	MempoolSize.Set(float64(size))
}

// SetMempoolMaxSize sets the maximum mempool capacity
func SetMempoolMaxSize(size int) {
	MempoolMaxSize.Set(float64(size))
}

// SetPeerCount sets the current peer count
func SetPeerCount(count int) {
	PeerCountGauge.Set(float64(count))
}

// IncrementConsensusRound increments the consensus round counter
func IncrementConsensusRound() {
	ConsensusRoundCounter.Inc()
}

// RecordStateRootMismatch records a state root mismatch
func RecordStateRootMismatch() {
	StateRootMismatchCounter.Inc()
}

// RecordPanicRecovery records a panic recovery
func RecordPanicRecovery() {
	PanicRecoveryCounter.Inc()
}

// RecordGasLimitRejection records a gas limit rejection
func RecordGasLimitRejection(reason string) {
	GasLimitRejections.WithLabelValues(reason).Inc()
}

// RecordMempoolEviction records a mempool eviction
func RecordMempoolEviction(reason string) {
	MempoolEvictions.WithLabelValues(reason).Inc()
}

// RecordGossipDropped records a dropped gossip message
func RecordGossipDropped(reason, msgType string) {
	GossipDroppedMessages.WithLabelValues(reason, msgType).Inc()
}

// RecordBlockValidationFailure records a block validation failure
func RecordBlockValidationFailure(reason string) {
	BlockValidationFailures.WithLabelValues(reason).Inc()
}

// RecordSignatureVerificationFailure records a signature verification failure
func RecordSignatureVerificationFailure() {
	SignatureVerificationFailures.Inc()
}

// RecordConnectionError records a P2P connection error
func RecordConnectionError(direction string) {
	ConnectionErrors.WithLabelValues(direction).Inc()
}