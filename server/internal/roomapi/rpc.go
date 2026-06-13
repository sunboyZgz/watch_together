package roomapi

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
	client internalv1connect.RoomInternalServiceClient
	config internalrpc.ClientConfig
}

func NewRPCClient(baseURL string, config internalrpc.ClientConfig) *RPCClient {
	baseURL = internalrpc.NormalizeBaseURL(baseURL)
	if baseURL == "" {
		return nil
	}
	return &RPCClient{
		client: internalv1connect.NewRoomInternalServiceClient(
			http.DefaultClient,
			internalrpc.ClientBaseURL(baseURL, config.PathPrefix),
		),
		config: config,
	}
}

func (c *RPCClient) CreateRoom(ctx context.Context, hostUserID string, mediaItemID string) (CreateRoomResult, error) {
	if c == nil || c.client == nil {
		return CreateRoomResult{}, errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.CreateRoomRequest{
		HostUserId:  hostUserID,
		MediaItemId: mediaItemID,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceCreateRoomProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.CreateRoom(ctx, request)
	if err != nil {
		return CreateRoomResult{}, roomErrorFromConnect(err)
	}
	return CreateRoomResult{
		Room:  roomFromProto(response.Msg.GetRoom()),
		Media: mediaFromProto(response.Msg.GetMedia()),
	}, nil
}

func (c *RPCClient) JoinRoomByCode(ctx context.Context, roomCode string, userID string) (JoinRoomResult, error) {
	if c == nil || c.client == nil {
		return JoinRoomResult{}, errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.JoinRoomRequest{
		RoomCode: roomCode,
		UserId:   userID,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceJoinRoomProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.JoinRoom(ctx, request)
	if err != nil {
		return JoinRoomResult{}, roomErrorFromConnect(err)
	}
	return JoinRoomResult{
		Room:   roomFromProto(response.Msg.GetRoom()),
		Media:  mediaFromProto(response.Msg.GetMedia()),
		Member: memberFromProto(response.Msg.GetMember()),
	}, nil
}

func (c *RPCClient) LeaveRoomByCode(ctx context.Context, roomCode string, userID string) error {
	if c == nil || c.client == nil {
		return errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.LeaveRoomRequest{
		RoomCode: roomCode,
		UserId:   userID,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceLeaveRoomProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	_, err := c.client.LeaveRoom(ctx, request)
	return roomErrorFromConnect(err)
}

func (c *RPCClient) DetailByCode(ctx context.Context, roomCode string) (DetailResult, error) {
	if c == nil || c.client == nil {
		return DetailResult{}, errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.GetRoomDetailRequest{
		RoomCode: roomCode,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceGetRoomDetailProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.GetRoomDetail(ctx, request)
	if err != nil {
		return DetailResult{}, roomErrorFromConnect(err)
	}
	return DetailResult{
		Room:    roomFromProto(response.Msg.GetRoom()),
		Media:   mediaFromProto(response.Msg.GetMedia()),
		Members: membersFromProto(response.Msg.GetMembers()),
	}, nil
}

func (c *RPCClient) IsActiveMemberByCode(ctx context.Context, roomCode string, userID string) (bool, error) {
	if c == nil || c.client == nil {
		return false, errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.IsActiveMemberRequest{
		RoomCode: roomCode,
		UserId:   userID,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceIsActiveMemberProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.IsActiveMember(ctx, request)
	if err != nil {
		return false, roomErrorFromConnect(err)
	}
	return response.Msg.GetActive(), nil
}

func (c *RPCClient) RuntimeBootstrapByCode(ctx context.Context, roomCode string) (RuntimeBootstrapResult, error) {
	if c == nil || c.client == nil {
		return RuntimeBootstrapResult{}, errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.GetRoomRuntimeBootstrapRequest{RoomCode: roomCode})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceGetRoomRuntimeBootstrapProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.GetRoomRuntimeBootstrap(ctx, request)
	if err != nil {
		return RuntimeBootstrapResult{}, roomErrorFromConnect(err)
	}
	return RuntimeBootstrapResult{
		Room:  roomFromProto(response.Msg.GetRoom()),
		Media: mediaFromProto(response.Msg.GetMedia()),
	}, nil
}

func (c *RPCClient) ListRecoverableRoomCodes(ctx context.Context, limit int) ([]string, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.ListRecoverableRoomsRequest{Limit: int32(limit)})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceListRecoverableRoomsProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.ListRecoverableRooms(ctx, request)
	if err != nil {
		return nil, roomErrorFromConnect(err)
	}
	return response.Msg.GetRoomCodes(), nil
}

func (c *RPCClient) MarkRoomGracePeriod(ctx context.Context, roomCode string, lastEmptyAt time.Time, destroyAfter time.Time) error {
	if c == nil || c.client == nil {
		return errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.MarkRoomGracePeriodRequest{
		RoomCode:       roomCode,
		LastEmptyAtMs:  lastEmptyAt.UnixMilli(),
		DestroyAfterMs: destroyAfter.UnixMilli(),
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceMarkRoomGracePeriodProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	_, err := c.client.MarkRoomGracePeriod(ctx, request)
	return roomErrorFromConnect(err)
}

func (c *RPCClient) MarkRoomActive(ctx context.Context, roomCode string) error {
	if c == nil || c.client == nil {
		return errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.MarkRoomActiveRequest{RoomCode: roomCode})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceMarkRoomActiveProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	_, err := c.client.MarkRoomActive(ctx, request)
	return roomErrorFromConnect(err)
}

func (c *RPCClient) DestroyRoom(ctx context.Context, roomCode string) error {
	if c == nil || c.client == nil {
		return errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.DestroyRoomRequest{RoomCode: roomCode})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceDestroyRoomProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	_, err := c.client.DestroyRoom(ctx, request)
	return roomErrorFromConnect(err)
}

func (c *RPCClient) MarkAllActiveRoomsGracePeriod(ctx context.Context, lastEmptyAt time.Time, destroyAfter time.Time) (int64, error) {
	if c == nil || c.client == nil {
		return 0, errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.MarkAllActiveRoomsGracePeriodRequest{
		LastEmptyAtMs:  lastEmptyAt.UnixMilli(),
		DestroyAfterMs: destroyAfter.UnixMilli(),
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceMarkAllActiveRoomsGracePeriodProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.MarkAllActiveRoomsGracePeriod(ctx, request)
	if err != nil {
		return 0, roomErrorFromConnect(err)
	}
	return response.Msg.GetUpdatedCount(), nil
}

func (c *RPCClient) CleanupExpiredRoomCodes(ctx context.Context, now time.Time) ([]string, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("room rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.CleanupExpiredRoomsRequest{NowMs: now.UnixMilli()})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomInternalServiceCleanupExpiredRoomsProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.CleanupExpiredRooms(ctx, request)
	if err != nil {
		return nil, roomErrorFromConnect(err)
	}
	return response.Msg.GetRoomCodes(), nil
}

func RegisterInternalRPC(mux *http.ServeMux, prefix string, authToken string, service BusinessService) {
	if mux == nil || service == nil {
		return
	}
	path, handler := internalv1connect.NewRoomInternalServiceHandler(&internalRPCHandler{
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

func (h *internalRPCHandler) CreateRoom(
	ctx context.Context,
	request *connect.Request[internalv1.CreateRoomRequest],
) (*connect.Response[internalv1.CreateRoomResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceCreateRoomProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	result, err := h.service.CreateRoom(ctx, request.Msg.GetHostUserId(), request.Msg.GetMediaItemId())
	if err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.CreateRoomResponse{
		Room:  roomToProto(result.Room),
		Media: mediaToProto(result.Media),
	}), nil
}

func (h *internalRPCHandler) JoinRoom(
	ctx context.Context,
	request *connect.Request[internalv1.JoinRoomRequest],
) (*connect.Response[internalv1.JoinRoomResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceJoinRoomProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	result, err := h.service.JoinRoomByCode(ctx, request.Msg.GetRoomCode(), request.Msg.GetUserId())
	if err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.JoinRoomResponse{
		Room:   roomToProto(result.Room),
		Media:  mediaToProto(result.Media),
		Member: memberToProto(result.Member),
	}), nil
}

func (h *internalRPCHandler) LeaveRoom(
	ctx context.Context,
	request *connect.Request[internalv1.LeaveRoomRequest],
) (*connect.Response[internalv1.LeaveRoomResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceLeaveRoomProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if err := h.service.LeaveRoomByCode(ctx, request.Msg.GetRoomCode(), request.Msg.GetUserId()); err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.LeaveRoomResponse{Left: true}), nil
}

func (h *internalRPCHandler) GetRoomDetail(
	ctx context.Context,
	request *connect.Request[internalv1.GetRoomDetailRequest],
) (*connect.Response[internalv1.GetRoomDetailResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceGetRoomDetailProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	result, err := h.service.DetailByCode(ctx, request.Msg.GetRoomCode())
	if err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.GetRoomDetailResponse{
		Room:    roomToProto(result.Room),
		Media:   mediaToProto(result.Media),
		Members: membersToProto(result.Members),
	}), nil
}

func (h *internalRPCHandler) IsActiveMember(
	ctx context.Context,
	request *connect.Request[internalv1.IsActiveMemberRequest],
) (*connect.Response[internalv1.IsActiveMemberResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceIsActiveMemberProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	active, err := h.service.IsActiveMemberByCode(ctx, request.Msg.GetRoomCode(), request.Msg.GetUserId())
	if err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.IsActiveMemberResponse{Active: active}), nil
}

func (h *internalRPCHandler) GetRoomRuntimeBootstrap(
	ctx context.Context,
	request *connect.Request[internalv1.GetRoomRuntimeBootstrapRequest],
) (*connect.Response[internalv1.GetRoomRuntimeBootstrapResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceGetRoomRuntimeBootstrapProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	result, err := h.service.RuntimeBootstrapByCode(ctx, request.Msg.GetRoomCode())
	if err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.GetRoomRuntimeBootstrapResponse{
		Room:  roomToProto(result.Room),
		Media: mediaToProto(result.Media),
	}), nil
}

func (h *internalRPCHandler) ListRecoverableRooms(
	ctx context.Context,
	request *connect.Request[internalv1.ListRecoverableRoomsRequest],
) (*connect.Response[internalv1.ListRecoverableRoomsResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceListRecoverableRoomsProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	roomCodes, err := h.service.ListRecoverableRoomCodes(ctx, int(request.Msg.GetLimit()))
	if err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.ListRecoverableRoomsResponse{RoomCodes: roomCodes}), nil
}

func (h *internalRPCHandler) MarkRoomGracePeriod(
	ctx context.Context,
	request *connect.Request[internalv1.MarkRoomGracePeriodRequest],
) (*connect.Response[internalv1.MarkRoomGracePeriodResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceMarkRoomGracePeriodProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if err := h.service.MarkRoomGracePeriod(
		ctx,
		request.Msg.GetRoomCode(),
		time.UnixMilli(request.Msg.GetLastEmptyAtMs()),
		time.UnixMilli(request.Msg.GetDestroyAfterMs()),
	); err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.MarkRoomGracePeriodResponse{Updated: true}), nil
}

func (h *internalRPCHandler) MarkRoomActive(
	ctx context.Context,
	request *connect.Request[internalv1.MarkRoomActiveRequest],
) (*connect.Response[internalv1.MarkRoomActiveResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceMarkRoomActiveProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if err := h.service.MarkRoomActive(ctx, request.Msg.GetRoomCode()); err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.MarkRoomActiveResponse{Updated: true}), nil
}

func (h *internalRPCHandler) DestroyRoom(
	ctx context.Context,
	request *connect.Request[internalv1.DestroyRoomRequest],
) (*connect.Response[internalv1.DestroyRoomResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceDestroyRoomProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if err := h.service.DestroyRoom(ctx, request.Msg.GetRoomCode()); err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.DestroyRoomResponse{Destroyed: true}), nil
}

func (h *internalRPCHandler) MarkAllActiveRoomsGracePeriod(
	ctx context.Context,
	request *connect.Request[internalv1.MarkAllActiveRoomsGracePeriodRequest],
) (*connect.Response[internalv1.MarkAllActiveRoomsGracePeriodResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceMarkAllActiveRoomsGracePeriodProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	count, err := h.service.MarkAllActiveRoomsGracePeriod(
		ctx,
		time.UnixMilli(request.Msg.GetLastEmptyAtMs()),
		time.UnixMilli(request.Msg.GetDestroyAfterMs()),
	)
	if err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.MarkAllActiveRoomsGracePeriodResponse{UpdatedCount: count}), nil
}

func (h *internalRPCHandler) CleanupExpiredRooms(
	ctx context.Context,
	request *connect.Request[internalv1.CleanupExpiredRoomsRequest],
) (*connect.Response[internalv1.CleanupExpiredRoomsResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.RoomInternalServiceCleanupExpiredRoomsProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	roomCodes, err := h.service.CleanupExpiredRoomCodes(ctx, time.UnixMilli(request.Msg.GetNowMs()))
	if err != nil {
		return nil, roomErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.CleanupExpiredRoomsResponse{RoomCodes: roomCodes}), nil
}

func roomErrorToConnect(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidInput):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, ErrRoomCodeExists), errors.Is(err, ErrUnableToCreate):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, ErrMediaNotFound), errors.Is(err, ErrRoomNotFound), errors.Is(err, ErrUserNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, ErrIdentityUnavailable):
		return connect.NewError(connect.CodeUnavailable, err)
	default:
		return internalrpc.ToConnectError(err)
	}
}

func roomErrorFromConnect(err error) error {
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
	case connect.CodeAlreadyExists:
		if errors.Is(connectErr.Unwrap(), ErrUnableToCreate) {
			return ErrUnableToCreate
		}
		return ErrRoomCodeExists
	case connect.CodeNotFound:
		message := connectErr.Message()
		switch message {
		case ErrMediaNotFound.Error():
			return ErrMediaNotFound
		case ErrUserNotFound.Error():
			return ErrUserNotFound
		default:
			return ErrRoomNotFound
		}
	case connect.CodeUnavailable:
		if connectErr.Message() == ErrIdentityUnavailable.Error() {
			return ErrIdentityUnavailable
		}
		return err
	default:
		return err
	}
}

func roomToProto(room Room) *internalv1.RoomBusinessRoom {
	return &internalv1.RoomBusinessRoom{
		Id:          room.ID,
		RoomCode:    room.RoomCode,
		HostUserId:  room.HostUserID,
		MediaItemId: room.MediaItemID,
		Status:      room.Status,
	}
}

func roomFromProto(room *internalv1.RoomBusinessRoom) Room {
	if room == nil {
		return Room{}
	}
	return Room{
		ID:          room.GetId(),
		RoomCode:    room.GetRoomCode(),
		HostUserID:  room.GetHostUserId(),
		MediaItemID: room.GetMediaItemId(),
		Status:      room.GetStatus(),
	}
}

func mediaToProto(media Media) *internalv1.RoomBusinessMedia {
	return &internalv1.RoomBusinessMedia{
		Id:           media.ID,
		Title:        media.Title,
		Subtitle:     cloneString(media.Subtitle),
		MediaUrl:     media.MediaURL,
		DurationMs:   cloneInt64(media.DurationMs),
		SeasonLabel:  cloneString(media.SeasonLabel),
		EpisodeLabel: cloneString(media.EpisodeLabel),
	}
}

func mediaFromProto(media *internalv1.RoomBusinessMedia) Media {
	if media == nil {
		return Media{}
	}
	return Media{
		ID:           media.GetId(),
		Title:        media.GetTitle(),
		Subtitle:     cloneString(media.Subtitle),
		MediaURL:     media.GetMediaUrl(),
		DurationMs:   cloneInt64(media.DurationMs),
		SeasonLabel:  cloneString(media.SeasonLabel),
		EpisodeLabel: cloneString(media.EpisodeLabel),
	}
}

func memberToProto(member Member) *internalv1.RoomBusinessMember {
	return &internalv1.RoomBusinessMember{
		UserId:     member.UserID,
		Nickname:   member.Nickname,
		AvatarSeed: member.AvatarSeed,
		AvatarUrl:  cloneString(member.AvatarURL),
		Role:       member.Role,
	}
}

func memberFromProto(member *internalv1.RoomBusinessMember) Member {
	if member == nil {
		return Member{}
	}
	return Member{
		UserID:     member.GetUserId(),
		Nickname:   member.GetNickname(),
		AvatarSeed: member.GetAvatarSeed(),
		AvatarURL:  cloneString(member.AvatarUrl),
		Role:       member.GetRole(),
	}
}

func membersToProto(members []Member) []*internalv1.RoomBusinessMember {
	out := make([]*internalv1.RoomBusinessMember, 0, len(members))
	for _, member := range members {
		out = append(out, memberToProto(member))
	}
	return out
}

func membersFromProto(members []*internalv1.RoomBusinessMember) []Member {
	out := make([]Member, 0, len(members))
	for _, member := range members {
		if member == nil {
			continue
		}
		out = append(out, memberFromProto(member))
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

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
