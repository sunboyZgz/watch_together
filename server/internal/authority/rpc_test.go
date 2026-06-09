package authority

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"watch_together/server/internal/cache"
	"watch_together/server/internal/internalrpc"
)

func TestRPCClientApplyRoomControlRoundTrip(t *testing.T) {
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "secret", fakeLifecycleRegistry{}, fakeControlApplier{})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRPCClient(server.URL, internalrpc.ClientConfig{
		Timeout:   time.Second,
		AuthToken: "secret",
	})

	response, err := client.ApplyRoomControl(context.Background(), ApplyControlRequest{
		SourceInstanceID:       "roomserver-a",
		RoomID:                 "ROOM01",
		UserID:                 "user-a",
		DeviceID:               "device-a",
		ConnectionID:           "conn-a",
		Type:                   "play",
		Payload:                json.RawMessage(`{"roomId":"ROOM01"}`),
		RequestID:              "request-a",
		Seq:                    1,
		ExpectedAuthorityEpoch: 2,
		RequestedAtMs:          123,
	})
	if err != nil {
		t.Fatalf("apply room control rpc: %v", err)
	}
	if !response.Accepted || response.Type != "play" || response.Seq != 2 || response.AuthorityEpoch != 2 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if string(response.Payload) != `{"accepted":true}` {
		t.Fatalf("unexpected payload: %s", response.Payload)
	}
}

func TestRPCObserverRecordsApplyControl(t *testing.T) {
	observer := &recordingObserver{}
	mux := http.NewServeMux()
	RegisterInternalRPCWithObserver(mux, "", "secret", fakeLifecycleRegistry{}, fakeControlApplier{}, observer)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRPCClient(server.URL, internalrpc.ClientConfig{
		Timeout:   time.Second,
		AuthToken: "secret",
	})

	if _, err := client.ApplyRoomControl(context.Background(), ApplyControlRequest{
		RoomID: "ROOM01",
		Type:   "play",
	}); err != nil {
		t.Fatalf("apply room control rpc: %v", err)
	}

	if observer.rpcResult("ApplyRoomControl", "accepted", "none") != 1 {
		t.Fatalf("expected accepted authority RPC metric, got %+v", observer.rpcRecords)
	}
	if observer.controlResult("play", "accepted", "none") != 1 {
		t.Fatalf("expected accepted authority control metric, got %+v", observer.controlRecords)
	}

	badClient := NewRPCClient(server.URL, internalrpc.ClientConfig{
		Timeout:   time.Second,
		AuthToken: "wrong",
	})
	if _, err := badClient.ApplyRoomControl(context.Background(), ApplyControlRequest{
		RoomID: "ROOM01",
		Type:   "play",
	}); err == nil {
		t.Fatalf("expected invalid authority RPC auth to fail")
	}
	if observer.rpcResult("ApplyRoomControl", "error", "unauthenticated") != 1 {
		t.Fatalf("expected unauthenticated authority RPC metric, got %+v", observer.rpcRecords)
	}
	if observer.controlResult("play", "error", "unauthenticated") != 1 {
		t.Fatalf("expected unauthenticated authority control metric, got %+v", observer.controlRecords)
	}
}

type fakeControlApplier struct{}

func (fakeControlApplier) ApplyRoomControl(context.Context, ApplyControlRequest) (ApplyControlResponse, error) {
	return ApplyControlResponse{
		Accepted:       true,
		Type:           "play",
		Payload:        json.RawMessage(`{"accepted":true}`),
		Seq:            2,
		AuthorityEpoch: 2,
	}, nil
}

type fakeLifecycleRegistry struct{}

func (fakeLifecycleRegistry) GetAuthority(context.Context, string) (cache.RoomAuthorityLease, bool, error) {
	return cache.RoomAuthorityLease{}, false, nil
}

func (fakeLifecycleRegistry) ClaimAuthority(context.Context, string, string) (cache.RoomAuthorityLease, bool, error) {
	return cache.RoomAuthorityLease{}, false, nil
}

func (fakeLifecycleRegistry) RenewAuthorityEpoch(context.Context, string, string, int64) (cache.RoomAuthorityLease, bool, error) {
	return cache.RoomAuthorityLease{}, false, nil
}

func (fakeLifecycleRegistry) BeginRecovery(context.Context, string, string) (cache.RoomAuthorityLease, bool, error) {
	return cache.RoomAuthorityLease{}, false, nil
}

func (fakeLifecycleRegistry) CompleteRecovery(context.Context, string, string, int64) (cache.RoomAuthorityLease, bool, error) {
	return cache.RoomAuthorityLease{}, false, nil
}

type recordingObserver struct {
	rpcRecords     []observerRPCRecord
	controlRecords []observerControlRecord
}

type observerRPCRecord struct {
	method string
	result string
	err    string
}

type observerControlRecord struct {
	controlType string
	result      string
	err         string
}

func (o *recordingObserver) RecordAuthorityRPC(method string, result string, stableError string, _ time.Duration) {
	o.rpcRecords = append(o.rpcRecords, observerRPCRecord{
		method: method,
		result: result,
		err:    stableError,
	})
}

func (o *recordingObserver) RecordAuthorityControlResult(controlType string, result string, stableError string) {
	o.controlRecords = append(o.controlRecords, observerControlRecord{
		controlType: controlType,
		result:      result,
		err:         stableError,
	})
}

func (o *recordingObserver) rpcResult(method string, result string, stableError string) int {
	count := 0
	for _, record := range o.rpcRecords {
		if record.method == method && record.result == result && record.err == stableError {
			count++
		}
	}
	return count
}

func (o *recordingObserver) controlResult(controlType string, result string, stableError string) int {
	count := 0
	for _, record := range o.controlRecords {
		if record.controlType == controlType && record.result == result && record.err == stableError {
			count++
		}
	}
	return count
}
