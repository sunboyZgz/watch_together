package authority

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"connectrpc.com/connect"

	"watch_together/server/internal/cache"
	"watch_together/server/internal/internalrpc"
	internalv1 "watch_together/server/internal/rpcgen/v1"
	"watch_together/server/internal/rpcgen/v1/internalv1connect"
)

type RPCClient struct {
	client internalv1connect.RoomAuthorityInternalServiceClient
	config internalrpc.ClientConfig
}

func NewRPCClient(baseURL string, config internalrpc.ClientConfig) *RPCClient {
	baseURL = internalrpc.NormalizeBaseURL(baseURL)
	if baseURL == "" {
		return nil
	}
	return &RPCClient{
		client: internalv1connect.NewRoomAuthorityInternalServiceClient(
			http.DefaultClient,
			internalrpc.ClientBaseURL(baseURL, config.PathPrefix),
		),
		config: config,
	}
}

func (c *RPCClient) ApplyRoomControl(ctx context.Context, request ApplyControlRequest) (ApplyControlResponse, error) {
	if c == nil || c.client == nil {
		return ApplyControlResponse{}, errors.New("authority rpc client is unavailable")
	}
	rpcRequest := connect.NewRequest(&internalv1.ApplyRoomControlRequest{
		SourceInstanceId:       request.SourceInstanceID,
		RoomId:                 request.RoomID,
		UserId:                 request.UserID,
		DeviceId:               request.DeviceID,
		ConnectionId:           request.ConnectionID,
		Type:                   request.Type,
		Payload:                cloneBytes(request.Payload),
		RequestId:              request.RequestID,
		Seq:                    request.Seq,
		ExpectedAuthorityEpoch: request.ExpectedAuthorityEpoch,
		RequestedAtMs:          request.RequestedAtMs,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.RoomAuthorityInternalServiceApplyRoomControlProcedure,
		rpcRequest.Header(),
	)
	defer cancel()
	defer span.End()
	rpcRequest.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.ApplyRoomControl(ctx, rpcRequest)
	if err != nil {
		return ApplyControlResponse{}, err
	}
	return applyControlResponseFromProto(response.Msg), nil
}

func (c *RPCClient) GetAuthority(ctx context.Context, roomID string) (cache.RoomAuthorityLease, bool, error) {
	if c == nil || c.client == nil {
		return cache.RoomAuthorityLease{}, false, errors.New("authority rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.GetRoomAuthorityRequest{RoomId: roomID})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(ctx, c.config, internalv1connect.RoomAuthorityInternalServiceGetRoomAuthorityProcedure, request.Header())
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)
	response, err := c.client.GetRoomAuthority(ctx, request)
	if err != nil {
		return cache.RoomAuthorityLease{}, false, err
	}
	return leaseFromProto(response.Msg.GetLease()), response.Msg.GetFound(), nil
}

func (c *RPCClient) ClaimAuthority(ctx context.Context, roomID string, instanceID string) (cache.RoomAuthorityLease, bool, error) {
	if c == nil || c.client == nil {
		return cache.RoomAuthorityLease{}, false, errors.New("authority rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.ClaimRoomAuthorityRequest{RoomId: roomID, InstanceId: instanceID})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(ctx, c.config, internalv1connect.RoomAuthorityInternalServiceClaimRoomAuthorityProcedure, request.Header())
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)
	response, err := c.client.ClaimRoomAuthority(ctx, request)
	if err != nil {
		return cache.RoomAuthorityLease{}, false, err
	}
	return leaseFromProto(response.Msg.GetLease()), response.Msg.GetClaimed(), nil
}

func (c *RPCClient) RenewAuthorityEpoch(ctx context.Context, roomID string, instanceID string, epoch int64) (cache.RoomAuthorityLease, bool, error) {
	if c == nil || c.client == nil {
		return cache.RoomAuthorityLease{}, false, errors.New("authority rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.RenewRoomAuthorityRequest{RoomId: roomID, InstanceId: instanceID, Epoch: epoch})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(ctx, c.config, internalv1connect.RoomAuthorityInternalServiceRenewRoomAuthorityProcedure, request.Header())
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)
	response, err := c.client.RenewRoomAuthority(ctx, request)
	if err != nil {
		return cache.RoomAuthorityLease{}, false, err
	}
	return leaseFromProto(response.Msg.GetLease()), response.Msg.GetRenewed(), nil
}

func (c *RPCClient) BeginRecovery(ctx context.Context, roomID string, instanceID string) (cache.RoomAuthorityLease, bool, error) {
	if c == nil || c.client == nil {
		return cache.RoomAuthorityLease{}, false, errors.New("authority rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.BeginRoomRecoveryRequest{RoomId: roomID, InstanceId: instanceID})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(ctx, c.config, internalv1connect.RoomAuthorityInternalServiceBeginRoomRecoveryProcedure, request.Header())
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)
	response, err := c.client.BeginRoomRecovery(ctx, request)
	if err != nil {
		return cache.RoomAuthorityLease{}, false, err
	}
	return leaseFromProto(response.Msg.GetLease()), response.Msg.GetStarted(), nil
}

func (c *RPCClient) CompleteRecovery(ctx context.Context, roomID string, instanceID string, epoch int64) (cache.RoomAuthorityLease, bool, error) {
	if c == nil || c.client == nil {
		return cache.RoomAuthorityLease{}, false, errors.New("authority rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.CompleteRoomRecoveryRequest{RoomId: roomID, InstanceId: instanceID, Epoch: epoch})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(ctx, c.config, internalv1connect.RoomAuthorityInternalServiceCompleteRoomRecoveryProcedure, request.Header())
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)
	response, err := c.client.CompleteRoomRecovery(ctx, request)
	if err != nil {
		return cache.RoomAuthorityLease{}, false, err
	}
	return leaseFromProto(response.Msg.GetLease()), response.Msg.GetCompleted(), nil
}

func RegisterInternalRPC(
	mux *http.ServeMux,
	prefix string,
	authToken string,
	registry LifecycleRegistry,
	applier ControlApplier,
) {
	if mux == nil {
		return
	}
	path, handler := internalv1connect.NewRoomAuthorityInternalServiceHandler(&internalRPCHandler{
		authToken: authToken,
		registry:  registry,
		applier:   NewSerialControlApplier(applier),
	})
	mountPath, prefixed := internalrpc.PrefixedHandler(prefix, path, handler)
	mux.Handle(mountPath, prefixed)
}

type internalRPCHandler struct {
	authToken string
	registry  LifecycleRegistry
	applier   ControlApplier
}

func (h *internalRPCHandler) GetRoomAuthority(
	ctx context.Context,
	request *connect.Request[internalv1.GetRoomAuthorityRequest],
) (*connect.Response[internalv1.GetRoomAuthorityResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceGetRoomAuthorityProcedure)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.registry == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("authority registry unavailable"))
	}
	lease, found, err := h.registry.GetAuthority(ctx, request.Msg.GetRoomId())
	if err != nil {
		return nil, internalrpc.ToConnectError(err)
	}
	return connect.NewResponse(&internalv1.GetRoomAuthorityResponse{Found: found, Lease: leaseToProto(lease)}), nil
}

func (h *internalRPCHandler) ClaimRoomAuthority(
	ctx context.Context,
	request *connect.Request[internalv1.ClaimRoomAuthorityRequest],
) (*connect.Response[internalv1.ClaimRoomAuthorityResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceClaimRoomAuthorityProcedure)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.registry == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("authority registry unavailable"))
	}
	lease, claimed, err := h.registry.ClaimAuthority(ctx, request.Msg.GetRoomId(), request.Msg.GetInstanceId())
	if err != nil {
		return nil, internalrpc.ToConnectError(err)
	}
	return connect.NewResponse(&internalv1.ClaimRoomAuthorityResponse{Claimed: claimed, Lease: leaseToProto(lease)}), nil
}

func (h *internalRPCHandler) RenewRoomAuthority(
	ctx context.Context,
	request *connect.Request[internalv1.RenewRoomAuthorityRequest],
) (*connect.Response[internalv1.RenewRoomAuthorityResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceRenewRoomAuthorityProcedure)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.registry == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("authority registry unavailable"))
	}
	lease, renewed, err := h.registry.RenewAuthorityEpoch(ctx, request.Msg.GetRoomId(), request.Msg.GetInstanceId(), request.Msg.GetEpoch())
	if err != nil {
		return nil, internalrpc.ToConnectError(err)
	}
	return connect.NewResponse(&internalv1.RenewRoomAuthorityResponse{Renewed: renewed, Lease: leaseToProto(lease)}), nil
}

func (h *internalRPCHandler) BeginRoomRecovery(
	ctx context.Context,
	request *connect.Request[internalv1.BeginRoomRecoveryRequest],
) (*connect.Response[internalv1.BeginRoomRecoveryResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceBeginRoomRecoveryProcedure)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.registry == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("authority registry unavailable"))
	}
	lease, started, err := h.registry.BeginRecovery(ctx, request.Msg.GetRoomId(), request.Msg.GetInstanceId())
	if err != nil {
		return nil, internalrpc.ToConnectError(err)
	}
	return connect.NewResponse(&internalv1.BeginRoomRecoveryResponse{Started: started, Lease: leaseToProto(lease)}), nil
}

func (h *internalRPCHandler) CompleteRoomRecovery(
	ctx context.Context,
	request *connect.Request[internalv1.CompleteRoomRecoveryRequest],
) (*connect.Response[internalv1.CompleteRoomRecoveryResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceCompleteRoomRecoveryProcedure)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.registry == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("authority registry unavailable"))
	}
	lease, completed, err := h.registry.CompleteRecovery(ctx, request.Msg.GetRoomId(), request.Msg.GetInstanceId(), request.Msg.GetEpoch())
	if err != nil {
		return nil, internalrpc.ToConnectError(err)
	}
	return connect.NewResponse(&internalv1.CompleteRoomRecoveryResponse{Completed: completed, Lease: leaseToProto(lease)}), nil
}

func (h *internalRPCHandler) ApplyRoomControl(
	ctx context.Context,
	request *connect.Request[internalv1.ApplyRoomControlRequest],
) (*connect.Response[internalv1.ApplyRoomControlResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceApplyRoomControlProcedure)
	if err != nil {
		return nil, err
	}
	defer span.End()
	if h.applier == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("authority control applier unavailable"))
	}
	response, err := h.applier.ApplyRoomControl(ctx, applyControlRequestFromProto(request.Msg))
	if err != nil {
		return nil, internalrpc.ToConnectError(err)
	}
	return connect.NewResponse(applyControlResponseToProto(response)), nil
}

func leaseToProto(lease cache.RoomAuthorityLease) *internalv1.AuthorityLease {
	return &internalv1.AuthorityLease{
		InstanceId:   lease.InstanceID,
		Epoch:        lease.Epoch,
		Status:       lease.Status,
		LeaseUntilMs: lease.LeaseUntilMs,
	}
}

func leaseFromProto(lease *internalv1.AuthorityLease) cache.RoomAuthorityLease {
	if lease == nil {
		return cache.RoomAuthorityLease{}
	}
	return cache.RoomAuthorityLease{
		InstanceID:   lease.GetInstanceId(),
		Epoch:        lease.GetEpoch(),
		Status:       lease.GetStatus(),
		LeaseUntilMs: lease.GetLeaseUntilMs(),
	}
}

func applyControlRequestFromProto(request *internalv1.ApplyRoomControlRequest) ApplyControlRequest {
	if request == nil {
		return ApplyControlRequest{}
	}
	return ApplyControlRequest{
		SourceInstanceID:       request.GetSourceInstanceId(),
		RoomID:                 request.GetRoomId(),
		UserID:                 request.GetUserId(),
		DeviceID:               request.GetDeviceId(),
		ConnectionID:           request.GetConnectionId(),
		Type:                   request.GetType(),
		Payload:                json.RawMessage(cloneBytes(request.GetPayload())),
		RequestID:              request.GetRequestId(),
		Seq:                    request.GetSeq(),
		ExpectedAuthorityEpoch: request.GetExpectedAuthorityEpoch(),
		RequestedAtMs:          request.GetRequestedAtMs(),
	}
}

func applyControlResponseToProto(response ApplyControlResponse) *internalv1.ApplyRoomControlResponse {
	return &internalv1.ApplyRoomControlResponse{
		Accepted:       response.Accepted,
		Type:           response.Type,
		Payload:        cloneBytes(response.Payload),
		Seq:            response.Seq,
		AuthorityEpoch: response.AuthorityEpoch,
		Error:          response.Error,
	}
}

func applyControlResponseFromProto(response *internalv1.ApplyRoomControlResponse) ApplyControlResponse {
	if response == nil {
		return ApplyControlResponse{}
	}
	return ApplyControlResponse{
		Accepted:       response.GetAccepted(),
		Type:           response.GetType(),
		Payload:        json.RawMessage(cloneBytes(response.GetPayload())),
		Seq:            response.GetSeq(),
		AuthorityEpoch: response.GetAuthorityEpoch(),
		Error:          response.GetError(),
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
