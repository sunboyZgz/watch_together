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
