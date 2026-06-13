package home

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"

	"watch_together/server/internal/internalrpc"
	internalv1 "watch_together/server/internal/rpcgen/v1"
	"watch_together/server/internal/rpcgen/v1/internalv1connect"
)

type RPCClient struct {
	client internalv1connect.HomeCompositionInternalServiceClient
	config internalrpc.ClientConfig
}

func NewRPCClient(baseURL string, config internalrpc.ClientConfig) *RPCClient {
	baseURL = internalrpc.NormalizeBaseURL(baseURL)
	if baseURL == "" {
		return nil
	}
	return &RPCClient{
		client: internalv1connect.NewHomeCompositionInternalServiceClient(
			http.DefaultClient,
			internalrpc.ClientBaseURL(baseURL, config.PathPrefix),
		),
		config: config,
	}
}

func (c *RPCClient) Summary(ctx context.Context, userID string) (Summary, error) {
	if c == nil || c.client == nil {
		return Summary{}, errors.New("home composition rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.GetHomeSummaryRequest{UserId: userID})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.HomeCompositionInternalServiceGetHomeSummaryProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.GetHomeSummary(ctx, request)
	if err != nil {
		return Summary{}, homeErrorFromConnect(err)
	}
	return summaryFromProto(response.Msg.GetSummary()), nil
}

func RegisterInternalRPC(mux *http.ServeMux, prefix string, authToken string, service BusinessService) {
	if mux == nil || service == nil {
		return
	}
	path, handler := internalv1connect.NewHomeCompositionInternalServiceHandler(&internalRPCHandler{
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

func (h *internalRPCHandler) GetHomeSummary(
	ctx context.Context,
	request *connect.Request[internalv1.GetHomeSummaryRequest],
) (*connect.Response[internalv1.GetHomeSummaryResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.HomeCompositionInternalServiceGetHomeSummaryProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	summary, err := h.service.Summary(ctx, request.Msg.GetUserId())
	if err != nil {
		return nil, homeErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.GetHomeSummaryResponse{Summary: summaryToProto(summary)}), nil
}

func homeErrorToConnect(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidUserID):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, ErrUserNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, ErrIdentityUnavailable), errors.Is(err, ErrProgressUnavailable), errors.Is(err, ErrMediaUnavailable):
		return connect.NewError(connect.CodeUnavailable, err)
	default:
		return internalrpc.ToConnectError(err)
	}
}

func homeErrorFromConnect(err error) error {
	if err == nil {
		return nil
	}
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return err
	}
	switch connectErr.Code() {
	case connect.CodeInvalidArgument:
		return ErrInvalidUserID
	case connect.CodeNotFound:
		return ErrUserNotFound
	case connect.CodeUnavailable:
		switch connectErr.Message() {
		case ErrIdentityUnavailable.Error():
			return ErrIdentityUnavailable
		case ErrProgressUnavailable.Error():
			return ErrProgressUnavailable
		default:
			return ErrMediaUnavailable
		}
	default:
		return err
	}
}

func summaryToProto(summary Summary) *internalv1.HomeSummary {
	return &internalv1.HomeSummary{
		User:             userSummaryToProto(summary.User),
		LastWatched:      watchProgressPtrToProto(summary.LastWatched),
		ContinueWatching: watchProgressListToProto(summary.ContinueWatching),
	}
}

func summaryFromProto(summary *internalv1.HomeSummary) Summary {
	if summary == nil {
		return Summary{}
	}
	return Summary{
		User:             userSummaryFromProto(summary.GetUser()),
		LastWatched:      watchProgressPtrFromProto(summary.LastWatched),
		ContinueWatching: watchProgressListFromProto(summary.GetContinueWatching()),
	}
}

func userSummaryToProto(user UserSummary) *internalv1.HomeUserSummary {
	return &internalv1.HomeUserSummary{
		Nickname:   user.Nickname,
		AvatarSeed: user.AvatarSeed,
		AvatarUrl:  cloneString(user.AvatarURL),
	}
}

func userSummaryFromProto(user *internalv1.HomeUserSummary) UserSummary {
	if user == nil {
		return UserSummary{}
	}
	return UserSummary{
		Nickname:   user.GetNickname(),
		AvatarSeed: user.GetAvatarSeed(),
		AvatarURL:  cloneString(user.AvatarUrl),
	}
}

func watchProgressPtrToProto(progress *WatchProgressSummary) *internalv1.HomeWatchProgressSummary {
	if progress == nil {
		return nil
	}
	return watchProgressToProto(*progress)
}

func watchProgressPtrFromProto(progress *internalv1.HomeWatchProgressSummary) *WatchProgressSummary {
	if progress == nil {
		return nil
	}
	item := watchProgressFromProto(progress)
	return &item
}

func watchProgressToProto(progress WatchProgressSummary) *internalv1.HomeWatchProgressSummary {
	return &internalv1.HomeWatchProgressSummary{
		MediaItemId:         progress.MediaItemID,
		Title:               progress.Title,
		CoverUrl:            cloneString(progress.CoverURL),
		LastPositionSeconds: int32(progress.LastPositionSeconds),
		DurationSeconds:     int32(progress.DurationSeconds),
	}
}

func watchProgressFromProto(progress *internalv1.HomeWatchProgressSummary) WatchProgressSummary {
	if progress == nil {
		return WatchProgressSummary{}
	}
	return WatchProgressSummary{
		MediaItemID:         progress.GetMediaItemId(),
		Title:               progress.GetTitle(),
		CoverURL:            cloneString(progress.CoverUrl),
		LastPositionSeconds: int(progress.GetLastPositionSeconds()),
		DurationSeconds:     int(progress.GetDurationSeconds()),
	}
}

func watchProgressListToProto(items []WatchProgressSummary) []*internalv1.HomeWatchProgressSummary {
	out := make([]*internalv1.HomeWatchProgressSummary, 0, len(items))
	for _, item := range items {
		out = append(out, watchProgressToProto(item))
	}
	return out
}

func watchProgressListFromProto(items []*internalv1.HomeWatchProgressSummary) []WatchProgressSummary {
	out := make([]WatchProgressSummary, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, watchProgressFromProto(item))
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
