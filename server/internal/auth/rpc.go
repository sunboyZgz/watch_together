package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"watch_together/server/internal/internalrpc"
	internalv1 "watch_together/server/internal/rpcgen/v1"
	"watch_together/server/internal/rpcgen/v1/internalv1connect"
	"watch_together/server/internal/servicekit"
)

type RPCClient struct {
	client internalv1connect.IdentityInternalServiceClient
	config internalrpc.ClientConfig
}

func NewRPCClient(baseURL string, config internalrpc.ClientConfig) *RPCClient {
	baseURL = internalrpc.NormalizeBaseURL(baseURL)
	if baseURL == "" {
		return nil
	}
	return &RPCClient{
		client: internalv1connect.NewIdentityInternalServiceClient(
			http.DefaultClient,
			internalrpc.ClientBaseURL(baseURL, config.PathPrefix),
		),
		config: config,
	}
}

func (c *RPCClient) Register(ctx context.Context, account string, password string, nickname string) (AuthResult, error) {
	if c == nil || c.client == nil {
		return AuthResult{}, errors.New("identity rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.RegisterRequest{
		Account:  account,
		Password: password,
		Nickname: nickname,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.IdentityInternalServiceRegisterProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.Register(ctx, request)
	if err != nil {
		return AuthResult{}, authErrorFromConnect(err)
	}
	return authResultFromRegisterProto(response.Msg), nil
}

func (c *RPCClient) Login(ctx context.Context, account string, password string) (AuthResult, error) {
	if c == nil || c.client == nil {
		return AuthResult{}, errors.New("identity rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.LoginRequest{
		Account:  account,
		Password: password,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.IdentityInternalServiceLoginProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.Login(ctx, request)
	if err != nil {
		return AuthResult{}, authErrorFromConnect(err)
	}
	return authResultFromLoginProto(response.Msg), nil
}

func (c *RPCClient) VerifyAccessToken(rawToken string) (TokenClaims, error) {
	if c == nil || c.client == nil {
		return TokenClaims{}, errors.New("identity rpc client is unavailable")
	}
	ctx := context.Background()
	request := connect.NewRequest(&internalv1.VerifyAccessTokenRequest{
		AccessToken: rawToken,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.IdentityInternalServiceVerifyAccessTokenProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.VerifyAccessToken(ctx, request)
	if err != nil {
		if isConnectCode(err, connect.CodeUnauthenticated) {
			return TokenClaims{}, ErrInvalidToken
		}
		return TokenClaims{}, authErrorFromConnect(err)
	}
	userID := strings.TrimSpace(response.Msg.GetUserId())
	if userID == "" {
		return TokenClaims{}, ErrInvalidToken
	}
	return TokenClaims{UserID: userID}, nil
}

func (c *RPCClient) GetUserProfile(ctx context.Context, userID string) (User, error) {
	if c == nil || c.client == nil {
		return User{}, errors.New("identity rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.GetUserProfileRequest{
		UserId: userID,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		c.config,
		internalv1connect.IdentityInternalServiceGetUserProfileProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, c.config.Service, requestID)

	response, err := c.client.GetUserProfile(ctx, request)
	if err != nil {
		return User{}, authErrorFromConnect(err)
	}
	return userFromProto(response.Msg.GetUser()), nil
}

func RegisterInternalRPC(mux *http.ServeMux, prefix string, authToken string, service IdentityService) {
	if mux == nil || service == nil {
		return
	}
	path, handler := internalv1connect.NewIdentityInternalServiceHandler(&internalRPCHandler{
		authToken: authToken,
		service:   service,
	})
	mountPath, prefixed := internalrpc.PrefixedHandler(prefix, path, handler)
	mux.Handle(mountPath, prefixed)
}

type internalRPCHandler struct {
	authToken string
	service   IdentityService
}

func (h *internalRPCHandler) Register(
	ctx context.Context,
	request *connect.Request[internalv1.RegisterRequest],
) (*connect.Response[internalv1.RegisterResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.IdentityInternalServiceRegisterProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	result, err := h.service.Register(ctx, request.Msg.GetAccount(), request.Msg.GetPassword(), request.Msg.GetNickname())
	if err != nil {
		return nil, authErrorToConnect(err)
	}
	return connect.NewResponse(registerResponseToProto(result)), nil
}

func (h *internalRPCHandler) Login(
	ctx context.Context,
	request *connect.Request[internalv1.LoginRequest],
) (*connect.Response[internalv1.LoginResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.IdentityInternalServiceLoginProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	result, err := h.service.Login(ctx, request.Msg.GetAccount(), request.Msg.GetPassword())
	if err != nil {
		return nil, authErrorToConnect(err)
	}
	return connect.NewResponse(loginResponseToProto(result)), nil
}

func (h *internalRPCHandler) VerifyAccessToken(
	ctx context.Context,
	request *connect.Request[internalv1.VerifyAccessTokenRequest],
) (*connect.Response[internalv1.VerifyAccessTokenResponse], error) {
	_, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.IdentityInternalServiceVerifyAccessTokenProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	claims, err := h.service.VerifyAccessToken(request.Msg.GetAccessToken())
	if err != nil {
		return nil, authErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.VerifyAccessTokenResponse{UserId: claims.UserID}), nil
}

func (h *internalRPCHandler) GetUserProfile(
	ctx context.Context,
	request *connect.Request[internalv1.GetUserProfileRequest],
) (*connect.Response[internalv1.GetUserProfileResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.IdentityInternalServiceGetUserProfileProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	user, err := h.service.GetUserProfile(ctx, request.Msg.GetUserId())
	if err != nil {
		return nil, authErrorToConnect(err)
	}
	return connect.NewResponse(&internalv1.GetUserProfileResponse{User: userToProto(user)}), nil
}

func authErrorToConnect(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidInput):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, ErrAccountExists):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrInvalidToken):
		return connect.NewError(connect.CodeUnauthenticated, err)
	case errors.Is(err, ErrUserNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	default:
		return internalrpc.ToConnectError(err)
	}
}

func authErrorFromConnect(err error) error {
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return err
	}
	switch connectErr.Code() {
	case connect.CodeInvalidArgument:
		return ErrInvalidInput
	case connect.CodeAlreadyExists:
		return ErrAccountExists
	case connect.CodeUnauthenticated:
		if strings.Contains(connectErr.Message(), servicekit.ErrUnauthorized.Error()) {
			return err
		}
		return ErrInvalidCredentials
	case connect.CodeNotFound:
		return ErrUserNotFound
	default:
		return err
	}
}

func isConnectCode(err error, code connect.Code) bool {
	var connectErr *connect.Error
	return errors.As(err, &connectErr) && connectErr.Code() == code
}

func registerResponseToProto(result AuthResult) *internalv1.RegisterResponse {
	return &internalv1.RegisterResponse{
		User:        userToProto(result.User),
		AccessToken: result.AccessToken,
	}
}

func loginResponseToProto(result AuthResult) *internalv1.LoginResponse {
	return &internalv1.LoginResponse{
		User:        userToProto(result.User),
		AccessToken: result.AccessToken,
	}
}

func authResultFromRegisterProto(result *internalv1.RegisterResponse) AuthResult {
	if result == nil {
		return AuthResult{}
	}
	return AuthResult{
		User:        userFromProto(result.GetUser()),
		AccessToken: result.GetAccessToken(),
	}
}

func authResultFromLoginProto(result *internalv1.LoginResponse) AuthResult {
	if result == nil {
		return AuthResult{}
	}
	return AuthResult{
		User:        userFromProto(result.GetUser()),
		AccessToken: result.GetAccessToken(),
	}
}

func userToProto(user User) *internalv1.IdentityUser {
	return &internalv1.IdentityUser{
		Id:         user.ID,
		Account:    user.Account,
		Nickname:   user.Nickname,
		AvatarSeed: user.AvatarSeed,
		AvatarUrl:  cloneString(user.AvatarURL),
		Bio:        cloneString(user.Bio),
	}
}

func userFromProto(user *internalv1.IdentityUser) User {
	if user == nil {
		return User{}
	}
	return User{
		ID:         user.GetId(),
		Account:    user.GetAccount(),
		Nickname:   user.GetNickname(),
		AvatarSeed: user.GetAvatarSeed(),
		AvatarURL:  cloneString(user.AvatarUrl),
		Bio:        cloneString(user.Bio),
	}
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
