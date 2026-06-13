package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	wtconfig "watch_together/server/internal/config"
	"watch_together/server/internal/observability"
)

func TestRoomAuthorityReadinessReportsDynamicDependencies(t *testing.T) {
	readyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	defer readyServer.Close()

	snapshot := roomauthorityReadiness(
		context.Background(),
		wtconfig.ServerRuntimeConfig{
			ServiceClients: wtconfig.ServiceClientsConfig{
				RoomAddr:     readyServer.URL,
				TimelineAddr: readyServer.URL,
			},
			Observability: wtconfig.ObservabilityConfig{ReadinessPath: "/readyz"},
		},
		fakeRedisPinger{},
		fakeNATSStatus{connected: true},
	)

	if dependencyStatus(t, snapshot, "redis") != "ok" {
		t.Fatalf("expected redis ping success to be ok")
	}
	if dependencyStatus(t, snapshot, "nats_broadcast") != "ok" {
		t.Fatalf("expected connected NATS bus to be ok")
	}
	if dependencyStatus(t, snapshot, "room_rpc") != "ok" {
		t.Fatalf("expected ready room service to be ok")
	}
	if dependencyStatus(t, snapshot, "timeline_rpc") != "ok" {
		t.Fatalf("expected ready timeline service to be ok")
	}
}

func TestRoomAuthorityReadinessMarksUnavailableDependencies(t *testing.T) {
	unavailableServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not_ready"}`))
	}))
	defer unavailableServer.Close()

	snapshot := roomauthorityReadiness(
		context.Background(),
		wtconfig.ServerRuntimeConfig{
			ServiceClients: wtconfig.ServiceClientsConfig{
				RoomAddr:     unavailableServer.URL,
				TimelineAddr: unavailableServer.URL,
			},
			Observability: wtconfig.ObservabilityConfig{ReadinessPath: "/readyz"},
		},
		fakeRedisPinger{err: errors.New("redis down")},
		fakeNATSStatus{},
	)

	if snapshot.Status != "not_ready" {
		t.Fatalf("expected unavailable dependencies to make service not ready, got %q", snapshot.Status)
	}
	for _, name := range []string{"redis", "nats_broadcast", "room_rpc", "timeline_rpc"} {
		if dependencyStatus(t, snapshot, name) != "unavailable" {
			t.Fatalf("expected %s to be unavailable", name)
		}
	}
}

type fakeRedisPinger struct {
	err error
}

func (p fakeRedisPinger) Ping(context.Context) error {
	return p.err
}

type fakeNATSStatus struct {
	connected bool
}

func (s fakeNATSStatus) IsConnected() bool {
	return s.connected
}

func dependencyStatus(t *testing.T, snapshot observability.ReadinessSnapshot, name string) string {
	t.Helper()
	for _, dependency := range snapshot.Dependencies {
		if dependency.Name == name {
			return dependency.Status
		}
	}
	t.Fatalf("missing dependency %q in %+v", name, snapshot.Dependencies)
	return ""
}
