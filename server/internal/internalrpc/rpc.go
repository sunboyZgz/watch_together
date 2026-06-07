package internalrpc

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	internalv1 "watch_together/server/internal/rpcgen/v1"
	"watch_together/server/internal/servicekit"
)

const (
	DefaultPathPrefix = "/internal.rpc"
)

type ServerConfig struct {
	Enabled    bool
	Addr       string
	PathPrefix string
	AuthToken  string
	Service    servicekit.Config
}

type ClientConfig struct {
	Addr       string
	PathPrefix string
	Timeout    time.Duration
	AuthToken  string
	Service    servicekit.Config
}

func NormalizePathPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return DefaultPathPrefix
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	return strings.TrimRight(prefix, "/")
}

func NormalizeBaseURL(addr string) string {
	addr = strings.TrimRight(strings.TrimSpace(addr), "/")
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}

func PrefixedHandler(prefix string, servicePath string, handler http.Handler) (string, http.Handler) {
	prefix = NormalizePathPrefix(prefix)
	servicePath = "/" + strings.Trim(servicePath, "/") + "/"
	return prefix + servicePath, http.StripPrefix(prefix, handler)
}

func ClientBaseURL(baseURL string, prefix string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ""
	}
	return baseURL + NormalizePathPrefix(prefix)
}

func RequestMetadata(ctx context.Context, service servicekit.Config, requestID string) *internalv1.RequestMetadata {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return &internalv1.RequestMetadata{
		RequestId:      strings.TrimSpace(requestID),
		ServiceName:    strings.TrimSpace(service.ServiceName),
		ServiceVersion: strings.TrimSpace(service.ServiceVersion),
		Traceparent:    carrier.Get("traceparent"),
	}
}

func PrepareClientRequest(
	ctx context.Context,
	config ClientConfig,
	procedure string,
	header http.Header,
) (context.Context, context.CancelFunc, string, trace.Span) {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = time.Second
	}
	ctx, cancel := servicekit.WithTimeout(ctx, timeout)
	ctx, requestID := servicekit.EnsureRequestID(ctx)
	ctx, span := otel.Tracer("watch_together/internalrpc").Start(ctx, procedure)
	servicekit.InjectHeaders(header, config.Service, requestID)
	servicekit.SetBearerToken(header, config.AuthToken)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
	return ctx, cancel, requestID, span
}

func PrepareServerRequest(
	ctx context.Context,
	header http.Header,
	authToken string,
	procedure string,
) (context.Context, trace.Span, error) {
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(header))
	ctx, span := otel.Tracer("watch_together/internalrpc").Start(ctx, procedure)
	if strings.TrimSpace(authToken) != "" && servicekit.BearerToken(header) != strings.TrimSpace(authToken) {
		span.End()
		return ctx, trace.SpanFromContext(ctx), connect.NewError(connect.CodeUnauthenticated, servicekit.ErrUnauthorized)
	}
	if requestID := servicekit.RequestIDFromHeaders(header); requestID != "" {
		ctx = servicekit.ContextWithRequestID(ctx, requestID)
	} else {
		ctx, _ = servicekit.EnsureRequestID(ctx)
	}
	return ctx, span, nil
}

func ToConnectError(err error) error {
	if err == nil {
		return nil
	}
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	}
	if errors.Is(err, context.Canceled) {
		return connect.NewError(connect.CodeCanceled, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}
