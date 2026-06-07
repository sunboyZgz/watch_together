package internalrpc

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"

	"watch_together/server/internal/rpcgen/v1/internalv1connect"
	"watch_together/server/internal/servicekit"
)

func TestPrepareClientAndServerRequestPropagateRequestIDAndAuth(t *testing.T) {
	header := http.Header{}
	baseCtx := servicekit.ContextWithRequestID(context.Background(), "req-existing")
	clientCtx, cancel, requestID, span := PrepareClientRequest(
		baseCtx,
		ClientConfig{
			Timeout:   time.Second,
			AuthToken: "secret",
			Service: servicekit.Config{
				ServiceName:    "roomserver",
				ServiceVersion: "test",
			},
		},
		internalv1connect.MediaInternalServiceListTagsProcedure,
		header,
	)
	defer cancel()
	defer span.End()

	if requestID != "req-existing" {
		t.Fatalf("expected existing request id to be preserved, got %q", requestID)
	}
	if servicekit.BearerToken(header) != "secret" {
		t.Fatalf("expected bearer token to be set")
	}
	if header.Get(servicekit.HeaderServiceName) != "roomserver" ||
		header.Get(servicekit.HeaderServiceVersion) != "test" {
		t.Fatalf("expected service metadata headers, got %+v", header)
	}
	if got := servicekit.RequestIDFromHeaders(header); got != requestID {
		t.Fatalf("expected request id header %q, got %q", requestID, got)
	}

	serverCtx, serverSpan, err := PrepareServerRequest(
		clientCtx,
		header,
		"secret",
		internalv1connect.MediaInternalServiceListTagsProcedure,
	)
	if err != nil {
		t.Fatalf("prepare server request: %v", err)
	}
	defer serverSpan.End()
	if got := servicekit.RequestIDFromContext(serverCtx); got != requestID {
		t.Fatalf("expected propagated request id %q, got %q", requestID, got)
	}
}

func TestPrepareServerRequestRejectsInvalidAuthToken(t *testing.T) {
	header := http.Header{}
	servicekit.SetBearerToken(header, "wrong")

	_, _, err := PrepareServerRequest(
		context.Background(),
		header,
		"secret",
		internalv1connect.MediaInternalServiceListTagsProcedure,
	)
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated connect error, got %v", err)
	}
}

func TestPrepareClientRequestDeadlineMapsToConnectDeadlineExceeded(t *testing.T) {
	header := http.Header{}
	ctx, cancel, _, span := PrepareClientRequest(
		context.Background(),
		ClientConfig{Timeout: time.Nanosecond},
		internalv1connect.TimelineInternalServiceRecordTimelineEventProcedure,
		header,
	)
	defer cancel()
	defer span.End()

	<-ctx.Done()
	err := ToConnectError(ctx.Err())
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeDeadlineExceeded {
		t.Fatalf("expected deadline exceeded connect error, got %v", err)
	}
}
