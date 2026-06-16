package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsHandler(t *testing.T) {
	metrics := NewMetrics()
	metrics.AddWebSocketConnection(1)
	metrics.SetRoomserverDraining(true)
	metrics.RecordWebSocketDrainClose()
	metrics.RecordWebSocketReconnectJoin("success")
	metrics.RecordControlResult("seek", "accepted")
	metrics.RecordSeekRateLimit("limited")
	metrics.RecordNATSEvent("broadcast_publish", "ok")
	metrics.RecordAuthorityRecovery("success")
	metrics.RecordAuthorityRPC("ApplyRoomControl", "error", "room authority unavailable", 10*time.Millisecond)
	metrics.RecordAuthorityControlResult("play", "rejected", "room authority unavailable")
	metrics.SetPresenceOnline(2)
	metrics.RecordWorkerEvent("outboxworker", "published")

	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected metrics status %d, got %d", http.StatusOK, recorder.Code)
	}
	body := recorder.Body.String()
	for _, expected := range []string{
		"watch_together_websocket_connections_current",
		"watch_together_roomserver_draining",
		"watch_together_websocket_drain_closes_total",
		"watch_together_websocket_reconnect_joins_total",
		"watch_together_websocket_control_events_total",
		"watch_together_seek_rate_limit_decisions_total",
		"watch_together_nats_room_events_total",
		"watch_together_authority_recovery_attempts_total",
		"watch_together_authority_rpc_requests_total",
		"watch_together_authority_rpc_request_duration_seconds",
		"watch_together_authority_control_results_total",
		"watch_together_presence_online_users_current",
		"watch_together_worker_events_total",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected metric %q in body:\n%s", expected, body)
		}
	}
}
