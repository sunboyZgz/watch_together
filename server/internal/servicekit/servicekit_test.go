package servicekit

import (
	"context"
	"net/http"
	"testing"
)

func TestEnsureRequestIDGeneratesAndPreservesID(t *testing.T) {
	ctx, requestID := EnsureRequestID(context.Background())
	if requestID == "" {
		t.Fatalf("expected generated request id")
	}
	ctx, second := EnsureRequestID(ctx)
	if second != requestID {
		t.Fatalf("expected request id to be preserved, got %q want %q", second, requestID)
	}
	if RequestIDFromContext(ctx) != requestID {
		t.Fatalf("expected request id in context")
	}
}

func TestInjectHeadersSetsServiceMetadata(t *testing.T) {
	headers := http.Header{}
	InjectHeaders(headers, Config{ServiceName: "svc", ServiceVersion: "v1"}, "req-1")
	if headers.Get(HeaderRequestID) != "req-1" ||
		headers.Get(HeaderServiceName) != "svc" ||
		headers.Get(HeaderServiceVersion) != "v1" {
		t.Fatalf("unexpected headers: %+v", headers)
	}
}

func TestBearerTokenHelpers(t *testing.T) {
	headers := http.Header{}
	SetBearerToken(headers, "secret")
	if got := BearerToken(headers); got != "secret" {
		t.Fatalf("expected bearer token, got %q", got)
	}
}
