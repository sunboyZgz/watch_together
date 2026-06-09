package authority

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

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

type Observer interface {
	RecordAuthorityRPC(method string, result string, stableError string, duration time.Duration)
	RecordAuthorityControlResult(controlType string, result string, stableError string)
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
	RegisterInternalRPCWithObserver(mux, prefix, authToken, registry, applier, nil)
}

func RegisterInternalRPCWithObserver(
	mux *http.ServeMux,
	prefix string,
	authToken string,
	registry LifecycleRegistry,
	applier ControlApplier,
	observer Observer,
) {
	if mux == nil {
		return
	}
	path, handler := internalv1connect.NewRoomAuthorityInternalServiceHandler(&internalRPCHandler{
		authToken: authToken,
		registry:  registry,
		applier:   NewSerialControlApplier(applier),
		observer:  observer,
	})
	mountPath, prefixed := internalrpc.PrefixedHandler(prefix, path, handler)
	mux.Handle(mountPath, prefixed)
}

type internalRPCHandler struct {
	authToken string
	registry  LifecycleRegistry
	applier   ControlApplier
	observer  Observer
}

func (h *internalRPCHandler) GetRoomAuthority(
	ctx context.Context,
	request *connect.Request[internalv1.GetRoomAuthorityRequest],
) (*connect.Response[internalv1.GetRoomAuthorityResponse], error) {
	start := time.Now()
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceGetRoomAuthorityProcedure)
	if err != nil {
		h.recordRPC("GetRoomAuthority", "error", stableErrorLabel(err), start)
		return nil, err
	}
	defer span.End()
	if h.registry == nil {
		err := connect.NewError(connect.CodeUnavailable, errors.New("authority registry unavailable"))
		h.recordRPC("GetRoomAuthority", "error", stableErrorLabel(err), start)
		return nil, err
	}
	lease, found, err := h.registry.GetAuthority(ctx, request.Msg.GetRoomId())
	if err != nil {
		err = internalrpc.ToConnectError(err)
		h.recordRPC("GetRoomAuthority", "error", stableErrorLabel(err), start)
		return nil, err
	}
	h.recordRPC("GetRoomAuthority", "ok", "none", start)
	return connect.NewResponse(&internalv1.GetRoomAuthorityResponse{Found: found, Lease: leaseToProto(lease)}), nil
}

func (h *internalRPCHandler) ClaimRoomAuthority(
	ctx context.Context,
	request *connect.Request[internalv1.ClaimRoomAuthorityRequest],
) (*connect.Response[internalv1.ClaimRoomAuthorityResponse], error) {
	start := time.Now()
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceClaimRoomAuthorityProcedure)
	if err != nil {
		h.recordRPC("ClaimRoomAuthority", "error", stableErrorLabel(err), start)
		return nil, err
	}
	defer span.End()
	if h.registry == nil {
		err := connect.NewError(connect.CodeUnavailable, errors.New("authority registry unavailable"))
		h.recordRPC("ClaimRoomAuthority", "error", stableErrorLabel(err), start)
		return nil, err
	}
	lease, claimed, err := h.registry.ClaimAuthority(ctx, request.Msg.GetRoomId(), request.Msg.GetInstanceId())
	if err != nil {
		err = internalrpc.ToConnectError(err)
		h.recordRPC("ClaimRoomAuthority", "error", stableErrorLabel(err), start)
		return nil, err
	}
	h.recordRPC("ClaimRoomAuthority", "ok", "none", start)
	return connect.NewResponse(&internalv1.ClaimRoomAuthorityResponse{Claimed: claimed, Lease: leaseToProto(lease)}), nil
}

func (h *internalRPCHandler) RenewRoomAuthority(
	ctx context.Context,
	request *connect.Request[internalv1.RenewRoomAuthorityRequest],
) (*connect.Response[internalv1.RenewRoomAuthorityResponse], error) {
	start := time.Now()
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceRenewRoomAuthorityProcedure)
	if err != nil {
		h.recordRPC("RenewRoomAuthority", "error", stableErrorLabel(err), start)
		return nil, err
	}
	defer span.End()
	if h.registry == nil {
		err := connect.NewError(connect.CodeUnavailable, errors.New("authority registry unavailable"))
		h.recordRPC("RenewRoomAuthority", "error", stableErrorLabel(err), start)
		return nil, err
	}
	lease, renewed, err := h.registry.RenewAuthorityEpoch(ctx, request.Msg.GetRoomId(), request.Msg.GetInstanceId(), request.Msg.GetEpoch())
	if err != nil {
		err = internalrpc.ToConnectError(err)
		h.recordRPC("RenewRoomAuthority", "error", stableErrorLabel(err), start)
		return nil, err
	}
	h.recordRPC("RenewRoomAuthority", "ok", "none", start)
	return connect.NewResponse(&internalv1.RenewRoomAuthorityResponse{Renewed: renewed, Lease: leaseToProto(lease)}), nil
}

func (h *internalRPCHandler) BeginRoomRecovery(
	ctx context.Context,
	request *connect.Request[internalv1.BeginRoomRecoveryRequest],
) (*connect.Response[internalv1.BeginRoomRecoveryResponse], error) {
	start := time.Now()
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceBeginRoomRecoveryProcedure)
	if err != nil {
		h.recordRPC("BeginRoomRecovery", "error", stableErrorLabel(err), start)
		return nil, err
	}
	defer span.End()
	if h.registry == nil {
		err := connect.NewError(connect.CodeUnavailable, errors.New("authority registry unavailable"))
		h.recordRPC("BeginRoomRecovery", "error", stableErrorLabel(err), start)
		return nil, err
	}
	lease, started, err := h.registry.BeginRecovery(ctx, request.Msg.GetRoomId(), request.Msg.GetInstanceId())
	if err != nil {
		err = internalrpc.ToConnectError(err)
		h.recordRPC("BeginRoomRecovery", "error", stableErrorLabel(err), start)
		return nil, err
	}
	h.recordRPC("BeginRoomRecovery", "ok", "none", start)
	return connect.NewResponse(&internalv1.BeginRoomRecoveryResponse{Started: started, Lease: leaseToProto(lease)}), nil
}

func (h *internalRPCHandler) CompleteRoomRecovery(
	ctx context.Context,
	request *connect.Request[internalv1.CompleteRoomRecoveryRequest],
) (*connect.Response[internalv1.CompleteRoomRecoveryResponse], error) {
	start := time.Now()
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceCompleteRoomRecoveryProcedure)
	if err != nil {
		h.recordRPC("CompleteRoomRecovery", "error", stableErrorLabel(err), start)
		return nil, err
	}
	defer span.End()
	if h.registry == nil {
		err := connect.NewError(connect.CodeUnavailable, errors.New("authority registry unavailable"))
		h.recordRPC("CompleteRoomRecovery", "error", stableErrorLabel(err), start)
		return nil, err
	}
	lease, completed, err := h.registry.CompleteRecovery(ctx, request.Msg.GetRoomId(), request.Msg.GetInstanceId(), request.Msg.GetEpoch())
	if err != nil {
		err = internalrpc.ToConnectError(err)
		h.recordRPC("CompleteRoomRecovery", "error", stableErrorLabel(err), start)
		return nil, err
	}
	h.recordRPC("CompleteRoomRecovery", "ok", "none", start)
	return connect.NewResponse(&internalv1.CompleteRoomRecoveryResponse{Completed: completed, Lease: leaseToProto(lease)}), nil
}

func (h *internalRPCHandler) ApplyRoomControl(
	ctx context.Context,
	request *connect.Request[internalv1.ApplyRoomControlRequest],
) (*connect.Response[internalv1.ApplyRoomControlResponse], error) {
	start := time.Now()
	ctx, span, err := internalrpc.PrepareServerRequest(ctx, request.Header(), h.authToken, internalv1connect.RoomAuthorityInternalServiceApplyRoomControlProcedure)
	if err != nil {
		h.recordRPC("ApplyRoomControl", "error", stableErrorLabel(err), start)
		h.recordControl(request.Msg.GetType(), "error", stableErrorLabel(err))
		return nil, err
	}
	defer span.End()
	if h.applier == nil {
		err := connect.NewError(connect.CodeUnavailable, errors.New("authority control applier unavailable"))
		h.recordRPC("ApplyRoomControl", "error", stableErrorLabel(err), start)
		h.recordControl(request.Msg.GetType(), "error", stableErrorLabel(err))
		return nil, err
	}
	response, err := h.applier.ApplyRoomControl(ctx, applyControlRequestFromProto(request.Msg))
	if err != nil {
		err = internalrpc.ToConnectError(err)
		h.recordRPC("ApplyRoomControl", "error", stableErrorLabel(err), start)
		h.recordControl(request.Msg.GetType(), "error", stableErrorLabel(err))
		return nil, err
	}
	result := "rejected"
	stableError := response.Error
	if response.Accepted {
		result = "accepted"
		stableError = ""
	}
	h.recordRPC("ApplyRoomControl", result, stringOrNone(stableError), start)
	h.recordControl(request.Msg.GetType(), result, stringOrNone(stableError))
	return connect.NewResponse(applyControlResponseToProto(response)), nil
}

func (h *internalRPCHandler) recordRPC(method string, result string, stableError string, start time.Time) {
	if h == nil || h.observer == nil {
		return
	}
	h.observer.RecordAuthorityRPC(method, result, stableError, time.Since(start))
}

func (h *internalRPCHandler) recordControl(controlType string, result string, stableError string) {
	if h == nil || h.observer == nil {
		return
	}
	h.observer.RecordAuthorityControlResult(controlType, result, stableError)
}

func stableErrorLabel(err error) string {
	if err == nil {
		return "none"
	}
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectCodeLabel(connectErr.Code())
	}
	return "internal"
}

func connectCodeLabel(code connect.Code) string {
	switch code {
	case connect.CodeCanceled:
		return "canceled"
	case connect.CodeUnknown:
		return "unknown"
	case connect.CodeInvalidArgument:
		return "invalid_argument"
	case connect.CodeDeadlineExceeded:
		return "deadline_exceeded"
	case connect.CodeNotFound:
		return "not_found"
	case connect.CodeAlreadyExists:
		return "already_exists"
	case connect.CodePermissionDenied:
		return "permission_denied"
	case connect.CodeResourceExhausted:
		return "resource_exhausted"
	case connect.CodeFailedPrecondition:
		return "failed_precondition"
	case connect.CodeAborted:
		return "aborted"
	case connect.CodeOutOfRange:
		return "out_of_range"
	case connect.CodeUnimplemented:
		return "unimplemented"
	case connect.CodeInternal:
		return "internal"
	case connect.CodeUnavailable:
		return "unavailable"
	case connect.CodeDataLoss:
		return "data_loss"
	case connect.CodeUnauthenticated:
		return "unauthenticated"
	default:
		return "unknown"
	}
}

func stringOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
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
