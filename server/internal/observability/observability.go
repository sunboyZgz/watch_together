package observability

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	DefaultMetricsPath   = "/metrics"
	DefaultReadinessPath = "/readyz"
)

type Config struct {
	MetricsEnabled bool
	MetricsAddr    string
	MetricsPath    string
	ReadinessPath  string
}

func (c Config) Normalized() Config {
	if strings.TrimSpace(c.MetricsPath) == "" {
		c.MetricsPath = DefaultMetricsPath
	}
	if strings.TrimSpace(c.ReadinessPath) == "" {
		c.ReadinessPath = DefaultReadinessPath
	}
	return c
}

func StartMetricsServer(ctx context.Context, config Config, metrics *Metrics) *http.Server {
	config = config.Normalized()
	if !config.MetricsEnabled || strings.TrimSpace(config.MetricsAddr) == "" {
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle(config.MetricsPath, metrics.Handler())
	server := &http.Server{
		Addr:    config.MetricsAddr,
		Handler: mux,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("metrics server stopped: %v", err)
		}
	}()
	return server
}

type DependencyStatus struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
}

type ReadinessSnapshot struct {
	Status          string             `json:"status"`
	AppEnv          string             `json:"appEnv"`
	InstanceID      string             `json:"instanceId,omitempty"`
	RoomRuntimeMode string             `json:"roomRuntimeMode"`
	Dependencies    []DependencyStatus `json:"dependencies"`
}

func NewReadinessSnapshot(appEnv string, instanceID string, runtimeMode string, dependencies []DependencyStatus) ReadinessSnapshot {
	status := "ready"
	for _, dependency := range dependencies {
		if dependency.Required && dependency.Status != "ok" {
			status = "not_ready"
			break
		}
	}
	return ReadinessSnapshot{
		Status:          status,
		AppEnv:          appEnv,
		InstanceID:      instanceID,
		RoomRuntimeMode: runtimeMode,
		Dependencies:    dependencies,
	}
}

type Metrics struct {
	registry *prometheus.Registry

	webSocketConnections prometheus.Gauge
	controlResults       *prometheus.CounterVec
	seekRateDecisions    *prometheus.CounterVec
	natsEvents           *prometheus.CounterVec
	authorityRecovery    *prometheus.CounterVec
	authorityRPCRequests *prometheus.CounterVec
	authorityRPCLatency  *prometheus.HistogramVec
	authorityControls    *prometheus.CounterVec
	presenceOnline       prometheus.Gauge
	workerEvents         *prometheus.CounterVec
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	metrics := &Metrics{
		registry: registry,
		webSocketConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "watch_together",
			Name:      "websocket_connections_current",
			Help:      "Current roomserver WebSocket connections.",
		}),
		controlResults: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "watch_together",
			Name:      "websocket_control_events_total",
			Help:      "WebSocket playback control events by type and result.",
		}, []string{"type", "result"}),
		seekRateDecisions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "watch_together",
			Name:      "seek_rate_limit_decisions_total",
			Help:      "Seek rate limit decisions.",
		}, []string{"result"}),
		natsEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "watch_together",
			Name:      "nats_room_events_total",
			Help:      "NATS room broadcast and control events.",
		}, []string{"kind", "result"}),
		authorityRecovery: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "watch_together",
			Name:      "authority_recovery_attempts_total",
			Help:      "Authority recovery attempts.",
		}, []string{"result"}),
		authorityRPCRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "watch_together",
			Name:      "authority_rpc_requests_total",
			Help:      "Internal authority RPC requests by method, result, and stable error.",
		}, []string{"method", "result", "error"}),
		authorityRPCLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "watch_together",
			Name:      "authority_rpc_request_duration_seconds",
			Help:      "Internal authority RPC request latency by method, result, and stable error.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "result", "error"}),
		authorityControls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "watch_together",
			Name:      "authority_control_results_total",
			Help:      "Authority control apply results by type, result, and stable error.",
		}, []string{"type", "result", "error"}),
		presenceOnline: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "watch_together",
			Name:      "presence_online_users_current",
			Help:      "Latest observed room presence online user count.",
		}),
		workerEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "watch_together",
			Name:      "worker_events_total",
			Help:      "Worker publish and dispatch events by worker and result.",
		}, []string{"worker", "result"}),
	}
	registry.MustRegister(
		metrics.webSocketConnections,
		metrics.controlResults,
		metrics.seekRateDecisions,
		metrics.natsEvents,
		metrics.authorityRecovery,
		metrics.authorityRPCRequests,
		metrics.authorityRPCLatency,
		metrics.authorityControls,
		metrics.presenceOnline,
		metrics.workerEvents,
	)
	return metrics
}

func (m *Metrics) Handler() http.Handler {
	if m == nil || m.registry == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) AddWebSocketConnection(delta float64) {
	if m == nil {
		return
	}
	m.webSocketConnections.Add(delta)
}

func (m *Metrics) RecordControlResult(controlType string, result string) {
	if m == nil {
		return
	}
	m.controlResults.WithLabelValues(labelValue(controlType), labelValue(result)).Inc()
}

func (m *Metrics) RecordSeekRateLimit(result string) {
	if m == nil {
		return
	}
	m.seekRateDecisions.WithLabelValues(labelValue(result)).Inc()
}

func (m *Metrics) RecordNATSEvent(kind string, result string) {
	if m == nil {
		return
	}
	m.natsEvents.WithLabelValues(labelValue(kind), labelValue(result)).Inc()
}

func (m *Metrics) RecordAuthorityRecovery(result string) {
	if m == nil {
		return
	}
	m.authorityRecovery.WithLabelValues(labelValue(result)).Inc()
}

func (m *Metrics) RecordAuthorityRPC(method string, result string, stableError string, duration time.Duration) {
	if m == nil {
		return
	}
	method = labelValue(method)
	result = labelValue(result)
	stableError = labelValue(stableError)
	m.authorityRPCRequests.WithLabelValues(method, result, stableError).Inc()
	if duration < 0 {
		duration = 0
	}
	m.authorityRPCLatency.WithLabelValues(method, result, stableError).Observe(duration.Seconds())
}

func (m *Metrics) RecordAuthorityControlResult(controlType string, result string, stableError string) {
	if m == nil {
		return
	}
	m.authorityControls.WithLabelValues(labelValue(controlType), labelValue(result), labelValue(stableError)).Inc()
}

func (m *Metrics) SetPresenceOnline(count int) {
	if m == nil {
		return
	}
	m.presenceOnline.Set(float64(count))
}

func (m *Metrics) RecordWorkerEvent(worker string, result string) {
	if m == nil {
		return
	}
	m.workerEvents.WithLabelValues(labelValue(worker), labelValue(result)).Inc()
}

func labelValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}
