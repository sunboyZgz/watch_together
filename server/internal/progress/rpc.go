package progress

import (
	"context"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"watch_together/server/internal/internalrpc"
	internalv1 "watch_together/server/internal/rpcgen/v1"
	"watch_together/server/internal/rpcgen/v1/internalv1connect"
)

type RPCClient struct {
	client internalv1connect.ProgressInternalServiceClient
	config internalrpc.ClientConfig
}

func NewRPCClient(baseURL string, config internalrpc.ClientConfig) *RPCClient {
	baseURL = internalrpc.NormalizeBaseURL(baseURL)
	if baseURL == "" {
		return nil
	}
	return &RPCClient{
		client: internalv1connect.NewProgressInternalServiceClient(
			http.DefaultClient,
			internalrpc.ClientBaseURL(baseURL, config.PathPrefix),
		),
		config: config,
	}
}

func (c *RPCClient) Update(ctx context.Context, params UpdateParams) (Summary, error) {
	if c == nil || c.client == nil {
		return Summary{}, errors.New("progress rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.UpdateProgressRequest{
		UserId:              params.UserID,
		MediaItemId:         params.MediaItemID,
		LastPositionSeconds: int32(params.LastPositionSeconds),
		DurationSeconds:     int32(params.DurationSeconds),
		Completed:           params.Completed,
		CompletionSource:    cloneString(params.CompletionSource),
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.ProgressInternalServiceUpdateProgressProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.UpdateProgress(ctx, request)
	if err != nil {
		return Summary{}, progressErrorFromConnect(err)
	}
	return summaryFromProto(response.Msg.GetProgress()), nil
}

func (c *RPCClient) GetUserProgress(ctx context.Context, userID string, mediaItemID string) (Summary, bool, error) {
	if c == nil || c.client == nil {
		return Summary{}, false, errors.New("progress rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.GetUserProgressRequest{
		UserId:      userID,
		MediaItemId: mediaItemID,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.ProgressInternalServiceGetUserProgressProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.GetUserProgress(ctx, request)
	if err != nil {
		return Summary{}, false, progressErrorFromConnect(err)
	}
	if !response.Msg.GetFound() {
		return Summary{}, false, nil
	}
	return summaryFromProto(response.Msg.GetProgress()), true, nil
}

func (c *RPCClient) BatchGetUserProgress(ctx context.Context, userID string, mediaItemIDs []string) ([]Summary, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("progress rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.BatchGetUserProgressRequest{
		UserId:       userID,
		MediaItemIds: mediaItemIDs,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.ProgressInternalServiceBatchGetUserProgressProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.BatchGetUserProgress(ctx, request)
	if err != nil {
		return nil, progressErrorFromConnect(err)
	}
	return summariesFromProto(response.Msg.GetProgress()), nil
}

func (c *RPCClient) ListRecentUserProgress(ctx context.Context, params RecentParams) ([]Summary, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("progress rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.ListRecentUserProgressRequest{
		UserId:         params.UserID,
		Limit:          int32(params.Limit),
		IncompleteOnly: params.IncompleteOnly,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.ProgressInternalServiceListRecentUserProgressProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.ListRecentUserProgress(ctx, request)
	if err != nil {
		return nil, progressErrorFromConnect(err)
	}
	return summariesFromProto(response.Msg.GetProgress()), nil
}

func RegisterInternalRPC(mux *http.ServeMux, prefix string, authToken string, service BusinessService) {
	if mux == nil || service == nil {
		return
	}
	path, handler := internalv1connect.NewProgressInternalServiceHandler(&internalRPCHandler{
		authToken: authToken,
		service:   service,
	})
	mountPath, prefixed := internalrpc.PrefixedHandler(prefix, path, handler)
	mux.Handle(mountPath, prefixed)
}

type internalRPCHandler struct {
	authToken string
	service   BusinessService
}

func (h *internalRPCHandler) UpdateProgress(
	ctx context.Context,
	request *connect.Request[internalv1.UpdateProgressRequest],
) (*connect.Response[internalv1.UpdateProgressResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.ProgressInternalServiceUpdateProgressProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	summary, err := h.service.Update(ctx, UpdateParams{
		UserID:              request.Msg.GetUserId(),
		MediaItemID:         request.Msg.GetMediaItemId(),
		LastPositionSeconds: int(request.Msg.GetLastPositionSeconds()),
		DurationSeconds:     int(request.Msg.GetDurationSeconds()),
		Completed:           request.Msg.GetCompleted(),
		CompletionSource:    cloneString(request.Msg.CompletionSource),
	})
	if err != nil {
		return nil, progressErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.UpdateProgressResponse{Progress: summaryToProto(summary)}), nil
}

func (h *internalRPCHandler) GetUserProgress(
	ctx context.Context,
	request *connect.Request[internalv1.GetUserProgressRequest],
) (*connect.Response[internalv1.GetUserProgressResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.ProgressInternalServiceGetUserProgressProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	summary, found, err := h.service.GetUserProgress(ctx, request.Msg.GetUserId(), request.Msg.GetMediaItemId())
	if err != nil {
		return nil, progressErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.GetUserProgressResponse{
		Found:    found,
		Progress: summaryToProto(summary),
	}), nil
}

func (h *internalRPCHandler) BatchGetUserProgress(
	ctx context.Context,
	request *connect.Request[internalv1.BatchGetUserProgressRequest],
) (*connect.Response[internalv1.BatchGetUserProgressResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.ProgressInternalServiceBatchGetUserProgressProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	summaries, err := h.service.BatchGetUserProgress(ctx, request.Msg.GetUserId(), request.Msg.GetMediaItemIds())
	if err != nil {
		return nil, progressErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.BatchGetUserProgressResponse{
		Progress: summariesToProto(summaries),
	}), nil
}

func (h *internalRPCHandler) ListRecentUserProgress(
	ctx context.Context,
	request *connect.Request[internalv1.ListRecentUserProgressRequest],
) (*connect.Response[internalv1.ListRecentUserProgressResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.ProgressInternalServiceListRecentUserProgressProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	summaries, err := h.service.ListRecentUserProgress(ctx, RecentParams{
		UserID:         request.Msg.GetUserId(),
		Limit:          int(request.Msg.GetLimit()),
		IncompleteOnly: request.Msg.GetIncompleteOnly(),
	})
	if err != nil {
		return nil, progressErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.ListRecentUserProgressResponse{
		Progress: summariesToProto(summaries),
	}), nil
}

func progressErrorToConnect(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidInput):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, ErrMediaNotFound), errors.Is(err, ErrUserNotFound), errors.Is(err, ErrProgressNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	default:
		return internalrpc.ToConnectError(err)
	}
}

func progressErrorFromConnect(err error) error {
	if err == nil {
		return nil
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return err
	}
	switch connectErr.Code() {
	case connect.CodeInvalidArgument:
		return ErrInvalidInput
	case connect.CodeNotFound:
		switch connectErr.Message() {
		case ErrMediaNotFound.Error():
			return ErrMediaNotFound
		case ErrUserNotFound.Error():
			return ErrUserNotFound
		default:
			return ErrProgressNotFound
		}
	default:
		return err
	}
}

func summaryToProto(summary Summary) *internalv1.UserMediaProgress {
	return &internalv1.UserMediaProgress{
		MediaItemId:         summary.MediaItemID,
		LastPositionSeconds: int32(summary.LastPositionSeconds),
		DurationSeconds:     int32(summary.DurationSeconds),
		Completed:           summary.Completed,
		LastWatchedAtMs:     summary.LastWatchedAt.UTC().UnixMilli(),
	}
}

func summaryFromProto(summary *internalv1.UserMediaProgress) Summary {
	if summary == nil {
		return Summary{}
	}
	return Summary{
		MediaItemID:         summary.GetMediaItemId(),
		LastPositionSeconds: int(summary.GetLastPositionSeconds()),
		DurationSeconds:     int(summary.GetDurationSeconds()),
		Completed:           summary.GetCompleted(),
		LastWatchedAt:       time.UnixMilli(summary.GetLastWatchedAtMs()).UTC(),
	}
}

func summariesToProto(summaries []Summary) []*internalv1.UserMediaProgress {
	out := make([]*internalv1.UserMediaProgress, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, summaryToProto(summary))
	}
	return out
}

func summariesFromProto(summaries []*internalv1.UserMediaProgress) []Summary {
	out := make([]Summary, 0, len(summaries))
	for _, summary := range summaries {
		if summary == nil {
			continue
		}
		out = append(out, summaryFromProto(summary))
	}
	return out
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
