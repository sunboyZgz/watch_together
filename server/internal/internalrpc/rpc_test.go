package internalrpc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	"watch_together/server/internal/servicekit"
)

func TestUnaryClientAddsRequestIDAndAuthToken(t *testing.T) {
	var gotRequestID string
	mux := http.NewServeMux()
	path, handler := NewUnaryHandler(
		MediaProcedure("", MediaListTagsProcedure),
		"secret",
		func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			gotRequestID = servicekit.RequestIDFromContext(ctx)
			return Encode(map[string]any{"ok": true})
		},
	)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewUnaryClient(
		server.Client(),
		server.URL,
		MediaProcedure("", MediaListTagsProcedure),
		ClientConfig{Timeout: time.Second, AuthToken: "secret"},
	)
	var response map[string]any
	if err := client.Call(context.Background(), map[string]any{}, &response); err != nil {
		t.Fatalf("call rpc: %v", err)
	}
	if gotRequestID == "" {
		t.Fatalf("expected request id to be propagated")
	}
	if response["ok"] != true {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestUnaryHandlerRejectsInvalidAuthToken(t *testing.T) {
	mux := http.NewServeMux()
	path, handler := NewUnaryHandler(
		MediaProcedure("", MediaListTagsProcedure),
		"secret",
		func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			t.Fatal("handler should not be called")
			return nil, nil
		},
	)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewUnaryClient(
		server.Client(),
		server.URL,
		MediaProcedure("", MediaListTagsProcedure),
		ClientConfig{Timeout: time.Second, AuthToken: "wrong"},
	)
	err := client.Call(context.Background(), map[string]any{}, nil)
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated connect error, got %v", err)
	}
}

func TestUnaryClientTimeoutMapsToDeadlineExceeded(t *testing.T) {
	mux := http.NewServeMux()
	path, handler := NewUnaryHandler(
		TimelineProcedure("", TimelineRecordEventProcedure),
		"",
		func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)
	mux.Handle(path, handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewUnaryClient(
		server.Client(),
		server.URL,
		TimelineProcedure("", TimelineRecordEventProcedure),
		ClientConfig{Timeout: time.Millisecond},
	)
	err := client.Call(context.Background(), map[string]any{}, nil)
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeDeadlineExceeded {
		t.Fatalf("expected deadline exceeded connect error, got %v", err)
	}
}
