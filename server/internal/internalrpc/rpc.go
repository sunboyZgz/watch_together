package internalrpc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/protobuf/types/known/structpb"

	"watch_together/server/internal/servicekit"
)

const (
	DefaultPathPrefix = "/internal.rpc"

	MediaServiceName    = "watch_together.internal.v1.MediaInternalService"
	TimelineServiceName = "watch_together.internal.v1.TimelineInternalService"

	MediaListTagsProcedure          = "ListTags"
	MediaSearchItemsProcedure       = "SearchItems"
	MediaGetPlaybackItemProcedure   = "GetPlaybackItem"
	MediaAuthorizePlaybackProcedure = "AuthorizePlayback"

	TimelineRecordEventProcedure           = "RecordTimelineEvent"
	TimelineListRoomEventsProcedure        = "ListRoomEvents"
	TimelineListUnpublishedEventsProcedure = "ListUnpublishedRoomEvents"
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

func Procedure(prefix string, service string, method string) string {
	return NormalizePathPrefix(prefix) + "/" + strings.Trim(service, "/") + "/" + strings.Trim(method, "/")
}

func MediaProcedure(prefix string, method string) string {
	return Procedure(prefix, MediaServiceName, method)
}

func TimelineProcedure(prefix string, method string) string {
	return Procedure(prefix, TimelineServiceName, method)
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

func Encode(value any) (*structpb.Struct, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, err
	}
	return structpb.NewStruct(object)
}

func Decode(message *structpb.Struct, dest any) error {
	if message == nil {
		return errors.New("rpc message is empty")
	}
	payload, err := json.Marshal(message.AsMap())
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dest)
}

func NewUnaryHandler(
	procedure string,
	authToken string,
	handler func(context.Context, *structpb.Struct) (*structpb.Struct, error),
) (string, http.Handler) {
	connectHandler := connect.NewUnaryHandler(
		procedure,
		func(ctx context.Context, request *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(request.Header()))
			ctx, span := otel.Tracer("watch_together/internalrpc").Start(ctx, procedure)
			defer span.End()
			if strings.TrimSpace(authToken) != "" && servicekit.BearerToken(request.Header()) != strings.TrimSpace(authToken) {
				return nil, connect.NewError(connect.CodeUnauthenticated, servicekit.ErrUnauthorized)
			}
			if requestID := servicekit.RequestIDFromHeaders(request.Header()); requestID != "" {
				ctx = servicekit.ContextWithRequestID(ctx, requestID)
			} else {
				ctx, _ = servicekit.EnsureRequestID(ctx)
			}
			response, err := handler(ctx, request.Msg)
			if err != nil {
				return nil, toConnectError(err)
			}
			return connect.NewResponse(response), nil
		},
	)
	return procedure, connectHandler
}

type UnaryClient struct {
	client    *connect.Client[structpb.Struct, structpb.Struct]
	config    ClientConfig
	procedure string
	timeout   time.Duration
}

func NewUnaryClient(httpClient connect.HTTPClient, baseURL string, procedure string, config ClientConfig) *UnaryClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = time.Second
	}
	return &UnaryClient{
		client:    connect.NewClient[structpb.Struct, structpb.Struct](httpClient, baseURL+procedure),
		config:    config,
		procedure: procedure,
		timeout:   timeout,
	}
}

func (c *UnaryClient) Call(ctx context.Context, request any, response any) error {
	if c == nil || c.client == nil {
		return errors.New("internal rpc client is unavailable")
	}
	ctx, cancel := servicekit.WithTimeout(ctx, c.timeout)
	defer cancel()
	ctx, requestID := servicekit.EnsureRequestID(ctx)
	message, err := Encode(request)
	if err != nil {
		return err
	}
	rpcRequest := connect.NewRequest(message)
	servicekit.InjectHeaders(rpcRequest.Header(), c.config.Service, requestID)
	servicekit.SetBearerToken(rpcRequest.Header(), c.config.AuthToken)
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(rpcRequest.Header()))
	ctx, span := otel.Tracer("watch_together/internalrpc").Start(ctx, c.procedure)
	defer span.End()
	rpcResponse, err := c.client.CallUnary(ctx, rpcRequest)
	if err != nil {
		return err
	}
	if response == nil {
		return nil
	}
	return Decode(rpcResponse.Msg, response)
}

func toConnectError(err error) error {
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
