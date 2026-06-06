package timeline

import (
	"context"
	"net/http"

	"google.golang.org/protobuf/types/known/structpb"

	"watch_together/server/internal/internalrpc"
)

type RPCClient struct {
	recordEvent     *internalrpc.UnaryClient
	listRoomEvents  *internalrpc.UnaryClient
	listUnpublished *internalrpc.UnaryClient
}

func NewRPCClient(baseURL string, config internalrpc.ClientConfig) *RPCClient {
	baseURL = internalrpc.NormalizeBaseURL(baseURL)
	if baseURL == "" {
		return nil
	}
	prefix := config.PathPrefix
	return &RPCClient{
		recordEvent: internalrpc.NewUnaryClient(
			http.DefaultClient,
			baseURL,
			internalrpc.TimelineProcedure(prefix, internalrpc.TimelineRecordEventProcedure),
			config,
		),
		listRoomEvents: internalrpc.NewUnaryClient(
			http.DefaultClient,
			baseURL,
			internalrpc.TimelineProcedure(prefix, internalrpc.TimelineListRoomEventsProcedure),
			config,
		),
		listUnpublished: internalrpc.NewUnaryClient(
			http.DefaultClient,
			baseURL,
			internalrpc.TimelineProcedure(prefix, internalrpc.TimelineListUnpublishedEventsProcedure),
			config,
		),
	}
}

func (c *RPCClient) RecordTimelineEvent(ctx context.Context, event Event) error {
	return c.recordEvent.Call(ctx, recordEventRPCRequest{Event: event}, nil)
}

func (c *RPCClient) ReadRoomEvents(ctx context.Context, roomID string) ([]Event, error) {
	var response listEventsRPCResponse
	if err := c.listRoomEvents.Call(ctx, listEventsRPCRequest{RoomID: roomID}, &response); err != nil {
		return nil, err
	}
	return response.Events, nil
}

func (c *RPCClient) ReadRoomUnpublishedTimelineEvents(ctx context.Context, roomID string) ([]Event, error) {
	var response listEventsRPCResponse
	if err := c.listUnpublished.Call(ctx, listEventsRPCRequest{RoomID: roomID}, &response); err != nil {
		return nil, err
	}
	return response.Events, nil
}

type RecordEventStore interface {
	RecordTimelineEvent(ctx context.Context, event Event) error
}

type UnpublishedReader interface {
	ReadRoomUnpublishedTimelineEvents(ctx context.Context, roomID string) ([]Event, error)
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
	register := func(path string, handler http.Handler) {
		mux.Handle(path, handler)
	}
	register(internalrpc.NewUnaryHandler(
		internalrpc.TimelineProcedure(prefix, internalrpc.TimelineRecordEventProcedure),
		authToken,
		func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			if recorder == nil {
				return nil, ErrTimelineUnavailable
			}
			var decoded recordEventRPCRequest
			if err := internalrpc.Decode(request, &decoded); err != nil {
				return nil, err
			}
			if err := recorder.RecordTimelineEvent(ctx, decoded.Event); err != nil {
				return nil, err
			}
			return internalrpc.Encode(recordEventRPCResponse{Recorded: true})
		},
	))
	register(internalrpc.NewUnaryHandler(
		internalrpc.TimelineProcedure(prefix, internalrpc.TimelineListRoomEventsProcedure),
		authToken,
		func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			if roomReader == nil {
				return nil, ErrTimelineUnavailable
			}
			var decoded listEventsRPCRequest
			if err := internalrpc.Decode(request, &decoded); err != nil {
				return nil, err
			}
			events, err := roomReader.ReadRoomEvents(ctx, decoded.RoomID)
			if err != nil {
				return nil, err
			}
			return internalrpc.Encode(listEventsRPCResponse{Events: events})
		},
	))
	register(internalrpc.NewUnaryHandler(
		internalrpc.TimelineProcedure(prefix, internalrpc.TimelineListUnpublishedEventsProcedure),
		authToken,
		func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			if unpublishedReader == nil {
				return nil, ErrTimelineUnavailable
			}
			var decoded listEventsRPCRequest
			if err := internalrpc.Decode(request, &decoded); err != nil {
				return nil, err
			}
			events, err := unpublishedReader.ReadRoomUnpublishedTimelineEvents(ctx, decoded.RoomID)
			if err != nil {
				return nil, err
			}
			return internalrpc.Encode(listEventsRPCResponse{Events: events})
		},
	))
}

type recordEventRPCRequest struct {
	Event Event `json:"event"`
}

type recordEventRPCResponse struct {
	Recorded bool `json:"recorded"`
}

type listEventsRPCRequest struct {
	RoomID string `json:"roomId"`
}

type listEventsRPCResponse struct {
	Events []Event `json:"events"`
}
