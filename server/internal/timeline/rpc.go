package timeline

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"connectrpc.com/connect"

	"watch_together/server/internal/internalrpc"
	internalv1 "watch_together/server/internal/rpcgen/v1"
	"watch_together/server/internal/rpcgen/v1/internalv1connect"
)

type RPCClient struct {
	client internalv1connect.TimelineInternalServiceClient
	config internalrpc.ClientConfig
}

func NewRPCClient(baseURL string, config internalrpc.ClientConfig) *RPCClient {
	baseURL = internalrpc.NormalizeBaseURL(baseURL)
	if baseURL == "" {
		return nil
	}
	return &RPCClient{
		client: internalv1connect.NewTimelineInternalServiceClient(
			http.DefaultClient,
			internalrpc.ClientBaseURL(baseURL, config.PathPrefix),
		),
		config: config,
	}
}

func (c *RPCClient) RecordTimelineEvent(ctx context.Context, event Event) error {
	if c == nil || c.client == nil {
		return errors.New("timeline rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.RecordTimelineEventRequest{
		Event: eventToProto(event),
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.TimelineInternalServiceRecordTimelineEventProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	_, err := c.client.RecordTimelineEvent(ctx, request)
	return err
}

func (c *RPCClient) RecordControlResult(ctx context.Context, result ControlResult) (Event, error) {
	if c == nil || c.client == nil {
		return Event{}, errors.New("timeline rpc client is unavailable")
	}
	payload, err := marshalPayload(result.Payload)
	if err != nil {
		return Event{}, err
	}
	request := connect.NewRequest(&internalv1.RecordControlResultRequest{
		RoomId:       result.RoomID,
		UserId:       result.UserID,
		DeviceId:     result.DeviceID,
		ConnectionId: result.ConnectionID,
		InstanceId:   result.InstanceID,
		ControlType:  result.ControlType,
		Seq:          result.Seq,
		Accepted:     result.Accepted,
		Payload:      cloneBytes(payload),
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.TimelineInternalServiceRecordControlResultProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.RecordControlResult(ctx, request)
	if err != nil {
		return Event{}, err
	}
	return eventFromProto(response.Msg.GetEvent()), nil
}

func (c *RPCClient) RecordMembershipResult(ctx context.Context, result MembershipResult) (Event, error) {
	if c == nil || c.client == nil {
		return Event{}, errors.New("timeline rpc client is unavailable")
	}
	payload, err := marshalPayload(result.Payload)
	if err != nil {
		return Event{}, err
	}
	request := connect.NewRequest(&internalv1.RecordMembershipResultRequest{
		RoomId:         result.RoomID,
		UserId:         result.UserID,
		DeviceId:       result.DeviceID,
		ConnectionId:   result.ConnectionID,
		InstanceId:     result.InstanceID,
		MembershipType: result.MembershipType,
		Payload:        cloneBytes(payload),
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.TimelineInternalServiceRecordMembershipResultProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.RecordMembershipResult(ctx, request)
	if err != nil {
		return Event{}, err
	}
	return eventFromProto(response.Msg.GetEvent()), nil
}

func (c *RPCClient) ReadRoomEvents(ctx context.Context, roomID string) ([]Event, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("timeline rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.ListRoomEventsRequest{
		RoomId: roomID,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.TimelineInternalServiceListRoomEventsProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.ListRoomEvents(ctx, request)
	if err != nil {
		return nil, err
	}
	return eventsFromProto(response.Msg.GetEvents()), nil
}

func (c *RPCClient) ReadRoomUnpublishedTimelineEvents(ctx context.Context, roomID string) ([]Event, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("timeline rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.ListUnpublishedRoomEventsRequest{
		RoomId: roomID,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.TimelineInternalServiceListUnpublishedRoomEventsProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.ListUnpublishedRoomEvents(ctx, request)
	if err != nil {
		return nil, err
	}
	return eventsFromProto(response.Msg.GetEvents()), nil
}

func (c *RPCClient) ReadRoomRecoveryEvents(ctx context.Context, roomID string) ([]Event, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("timeline rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.ListRoomRecoveryEventsRequest{
		RoomId: roomID,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.TimelineInternalServiceListRoomRecoveryEventsProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.ListRoomRecoveryEvents(ctx, request)
	if err != nil {
		return nil, err
	}
	return eventsFromProto(response.Msg.GetEvents()), nil
}

type RecordEventStore interface {
	RecordTimelineEvent(ctx context.Context, event Event) error
}

func RegisterInternalRPC(
	mux *http.ServeMux,
	prefix string,
	authToken string,
	recorder RecordEventStore,
	roomReader RoomEventReader,
	unpublishedReader UnpublishedReader,
) {
	if mux == nil {
		return
	}
	path, handler := internalv1connect.NewTimelineInternalServiceHandler(&internalRPCHandler{
		authToken:         authToken,
		recorder:          recorder,
		roomReader:        roomReader,
		unpublishedReader: unpublishedReader,
		service:           NewService(recorder, roomReader, unpublishedReader),
	})
	mountPath, prefixed := internalrpc.PrefixedHandler(prefix, path, handler)
	mux.Handle(mountPath, prefixed)
}

type internalRPCHandler struct {
	authToken         string
	recorder          RecordEventStore
	roomReader        RoomEventReader
	unpublishedReader UnpublishedReader
	service           *Service
}

func (h *internalRPCHandler) RecordTimelineEvent(
	ctx context.Context,
	request *connect.Request[internalv1.RecordTimelineEventRequest],
) (*connect.Response[internalv1.RecordTimelineEventResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.TimelineInternalServiceRecordTimelineEventProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.recorder == nil {
		return nil, connect.NewError(connect.CodeUnavailable, ErrTimelineUnavailable)
	}
	if err := h.recorder.RecordTimelineEvent(ctx, eventFromProto(request.Msg.GetEvent())); err != nil {
		return nil, timelineConnectError(err)
	}
	return connect.NewResponse(&internalv1.RecordTimelineEventResponse{Recorded: true}), nil
}

func (h *internalRPCHandler) RecordControlResult(
	ctx context.Context,
	request *connect.Request[internalv1.RecordControlResultRequest],
) (*connect.Response[internalv1.RecordControlResultResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.TimelineInternalServiceRecordControlResultProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.service == nil {
		return nil, connect.NewError(connect.CodeUnavailable, ErrTimelineUnavailable)
	}
	event, err := h.service.RecordControlResult(ctx, controlResultFromProto(request.Msg))
	if err != nil {
		return nil, timelineConnectError(err)
	}
	return connect.NewResponse(&internalv1.RecordControlResultResponse{
		Event: eventToProto(event),
	}), nil
}

func (h *internalRPCHandler) RecordMembershipResult(
	ctx context.Context,
	request *connect.Request[internalv1.RecordMembershipResultRequest],
) (*connect.Response[internalv1.RecordMembershipResultResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.TimelineInternalServiceRecordMembershipResultProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.service == nil {
		return nil, connect.NewError(connect.CodeUnavailable, ErrTimelineUnavailable)
	}
	event, err := h.service.RecordMembershipResult(ctx, membershipResultFromProto(request.Msg))
	if err != nil {
		return nil, timelineConnectError(err)
	}
	return connect.NewResponse(&internalv1.RecordMembershipResultResponse{
		Event: eventToProto(event),
	}), nil
}

func (h *internalRPCHandler) ListRoomEvents(
	ctx context.Context,
	request *connect.Request[internalv1.ListRoomEventsRequest],
) (*connect.Response[internalv1.ListRoomEventsResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.TimelineInternalServiceListRoomEventsProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.roomReader == nil {
		return nil, connect.NewError(connect.CodeUnavailable, ErrTimelineUnavailable)
	}
	events, err := h.roomReader.ReadRoomEvents(ctx, request.Msg.GetRoomId())
	if err != nil {
		return nil, timelineConnectError(err)
	}
	return connect.NewResponse(&internalv1.ListRoomEventsResponse{
		Events: eventsToProto(events),
	}), nil
}

func (h *internalRPCHandler) ListUnpublishedRoomEvents(
	ctx context.Context,
	request *connect.Request[internalv1.ListUnpublishedRoomEventsRequest],
) (*connect.Response[internalv1.ListUnpublishedRoomEventsResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.TimelineInternalServiceListUnpublishedRoomEventsProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.unpublishedReader == nil {
		return nil, connect.NewError(connect.CodeUnavailable, ErrTimelineUnavailable)
	}
	events, err := h.unpublishedReader.ReadRoomUnpublishedTimelineEvents(ctx, request.Msg.GetRoomId())
	if err != nil {
		return nil, timelineConnectError(err)
	}
	return connect.NewResponse(&internalv1.ListUnpublishedRoomEventsResponse{
		Events: eventsToProto(events),
	}), nil
}

func (h *internalRPCHandler) ListRoomRecoveryEvents(
	ctx context.Context,
	request *connect.Request[internalv1.ListRoomRecoveryEventsRequest],
) (*connect.Response[internalv1.ListRoomRecoveryEventsResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.TimelineInternalServiceListRoomRecoveryEventsProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.service == nil {
		return nil, connect.NewError(connect.CodeUnavailable, ErrTimelineUnavailable)
	}
	events, err := h.service.ReadRoomRecoveryEvents(ctx, request.Msg.GetRoomId())
	if err != nil {
		return nil, timelineConnectError(err)
	}
	return connect.NewResponse(&internalv1.ListRoomRecoveryEventsResponse{
		Events: eventsToProto(events),
	}), nil
}

func controlResultFromProto(request *internalv1.RecordControlResultRequest) ControlResult {
	if request == nil {
		return ControlResult{}
	}
	return ControlResult{
		RoomID:       request.GetRoomId(),
		UserID:       request.GetUserId(),
		DeviceID:     request.GetDeviceId(),
		ConnectionID: request.GetConnectionId(),
		InstanceID:   request.GetInstanceId(),
		ControlType:  request.GetControlType(),
		Seq:          request.GetSeq(),
		Accepted:     request.GetAccepted(),
		Payload:      json.RawMessage(cloneBytes(request.GetPayload())),
	}
}

func membershipResultFromProto(request *internalv1.RecordMembershipResultRequest) MembershipResult {
	if request == nil {
		return MembershipResult{}
	}
	return MembershipResult{
		RoomID:         request.GetRoomId(),
		UserID:         request.GetUserId(),
		DeviceID:       request.GetDeviceId(),
		ConnectionID:   request.GetConnectionId(),
		InstanceID:     request.GetInstanceId(),
		MembershipType: request.GetMembershipType(),
		Payload:        json.RawMessage(cloneBytes(request.GetPayload())),
	}
}

func timelineConnectError(err error) error {
	if errors.Is(err, ErrInvalidInput) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	if errors.Is(err, ErrTimelineUnavailable) {
		return connect.NewError(connect.CodeUnavailable, err)
	}
	return internalrpc.ToConnectError(err)
}

func eventsToProto(events []Event) []*internalv1.TimelineEvent {
	out := make([]*internalv1.TimelineEvent, 0, len(events))
	for _, event := range events {
		out = append(out, eventToProto(event))
	}
	return out
}

func eventsFromProto(events []*internalv1.TimelineEvent) []Event {
	out := make([]Event, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		out = append(out, eventFromProto(event))
	}
	return out
}

func eventToProto(event Event) *internalv1.TimelineEvent {
	return &internalv1.TimelineEvent{
		EventId:      event.EventID,
		EventVersion: int32(event.EventVersion),
		EventType:    event.EventType,
		RoomId:       event.RoomID,
		UserId:       event.UserID,
		DeviceId:     event.DeviceID,
		ConnectionId: event.ConnectionID,
		InstanceId:   event.InstanceID,
		ControlType:  event.ControlType,
		Seq:          event.Seq,
		OccurredAtMs: event.OccurredAtMs,
		Payload:      cloneBytes(event.Payload),
	}
}

func eventFromProto(event *internalv1.TimelineEvent) Event {
	if event == nil {
		return Event{}
	}
	return Event{
		EventID:      event.GetEventId(),
		EventVersion: int(event.GetEventVersion()),
		EventType:    event.GetEventType(),
		RoomID:       event.GetRoomId(),
		UserID:       event.GetUserId(),
		DeviceID:     event.GetDeviceId(),
		ConnectionID: event.GetConnectionId(),
		InstanceID:   event.GetInstanceId(),
		ControlType:  event.GetControlType(),
		Seq:          event.GetSeq(),
		OccurredAtMs: event.GetOccurredAtMs(),
		Payload:      json.RawMessage(cloneBytes(event.GetPayload())),
	}
}

func cloneBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	cloned := make([]byte, len(value))
	copy(cloned, value)
	return cloned
}
