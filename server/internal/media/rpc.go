package media

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	"watch_together/server/internal/internalrpc"
)

type RPCStore struct {
	listTags          *internalrpc.UnaryClient
	searchItems       *internalrpc.UnaryClient
	getPlaybackItem   *internalrpc.UnaryClient
	authorizePlayback *internalrpc.UnaryClient
}

func NewRPCStore(baseURL string, config internalrpc.ClientConfig) *RPCStore {
	baseURL = internalrpc.NormalizeBaseURL(baseURL)
	if baseURL == "" {
		return nil
	}
	prefix := config.PathPrefix
	return &RPCStore{
		listTags: internalrpc.NewUnaryClient(
			http.DefaultClient,
			baseURL,
			internalrpc.MediaProcedure(prefix, internalrpc.MediaListTagsProcedure),
			config,
		),
		searchItems: internalrpc.NewUnaryClient(
			http.DefaultClient,
			baseURL,
			internalrpc.MediaProcedure(prefix, internalrpc.MediaSearchItemsProcedure),
			config,
		),
		getPlaybackItem: internalrpc.NewUnaryClient(
			http.DefaultClient,
			baseURL,
			internalrpc.MediaProcedure(prefix, internalrpc.MediaGetPlaybackItemProcedure),
			config,
		),
		authorizePlayback: internalrpc.NewUnaryClient(
			http.DefaultClient,
			baseURL,
			internalrpc.MediaProcedure(prefix, internalrpc.MediaAuthorizePlaybackProcedure),
			config,
		),
	}
}

func (s *RPCStore) ListTags(ctx context.Context, allLimit int) (TagList, error) {
	var response tagListRPCResponse
	if err := s.listTags.Call(ctx, listTagsRPCRequest{AllLimit: allLimit}, &response); err != nil {
		return TagList{}, err
	}
	return response.Tags, nil
}

func (s *RPCStore) SearchItems(ctx context.Context, params StoreSearchParams) ([]Item, error) {
	var response searchItemsRPCResponse
	if err := s.searchItems.Call(ctx, searchItemsRPCRequest{Params: params}, &response); err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (s *RPCStore) FindPlaybackItem(ctx context.Context, episodeID string) (PlaybackItem, error) {
	var response playbackItemRPCResponse
	err := s.getPlaybackItem.Call(ctx, playbackItemRPCRequest{EpisodeID: episodeID}, &response)
	if err != nil {
		var connectErr *connect.Error
		if errors.As(err, &connectErr) && connectErr.Code() == connect.CodeNotFound {
			return PlaybackItem{}, ErrMediaNotFound
		}
		return PlaybackItem{}, err
	}
	return response.Item, nil
}

func RegisterInternalRPC(mux *http.ServeMux, prefix string, authToken string, service *Service) {
	if mux == nil || service == nil {
		return
	}
	register := func(path string, handler http.Handler) {
		mux.Handle(path, handler)
	}
	register(internalrpc.NewUnaryHandler(
		internalrpc.MediaProcedure(prefix, internalrpc.MediaListTagsProcedure),
		authToken,
		func(ctx context.Context, request *structpbStruct) (*structpbStruct, error) {
			var decoded listTagsRPCRequest
			if err := internalrpc.Decode(request, &decoded); err != nil {
				return nil, err
			}
			tags, err := service.store.ListTags(ctx, decoded.AllLimit)
			if err != nil {
				return nil, err
			}
			return internalrpc.Encode(tagListRPCResponse{Tags: tags})
		},
	))
	register(internalrpc.NewUnaryHandler(
		internalrpc.MediaProcedure(prefix, internalrpc.MediaSearchItemsProcedure),
		authToken,
		func(ctx context.Context, request *structpbStruct) (*structpbStruct, error) {
			var decoded searchItemsRPCRequest
			if err := internalrpc.Decode(request, &decoded); err != nil {
				return nil, err
			}
			items, err := service.store.SearchItems(ctx, decoded.Params)
			if err != nil {
				return nil, err
			}
			return internalrpc.Encode(searchItemsRPCResponse{Items: items})
		},
	))
	register(internalrpc.NewUnaryHandler(
		internalrpc.MediaProcedure(prefix, internalrpc.MediaGetPlaybackItemProcedure),
		authToken,
		func(ctx context.Context, request *structpbStruct) (*structpbStruct, error) {
			var decoded playbackItemRPCRequest
			if err := internalrpc.Decode(request, &decoded); err != nil {
				return nil, err
			}
			item, err := service.store.FindPlaybackItem(ctx, decoded.EpisodeID)
			if err != nil {
				if errors.Is(err, ErrMediaNotFound) {
					return nil, connect.NewError(connect.CodeNotFound, err)
				}
				return nil, err
			}
			return internalrpc.Encode(playbackItemRPCResponse{Item: item})
		},
	))
	register(internalrpc.NewUnaryHandler(
		internalrpc.MediaProcedure(prefix, internalrpc.MediaAuthorizePlaybackProcedure),
		authToken,
		func(ctx context.Context, request *structpbStruct) (*structpbStruct, error) {
			_ = ctx
			_ = request
			return internalrpc.Encode(authorizePlaybackRPCResponse{Authorized: true})
		},
	))
}

type structpbStruct = structpb.Struct

type listTagsRPCRequest struct {
	AllLimit int `json:"allLimit"`
}

type tagListRPCResponse struct {
	Tags TagList `json:"tags"`
}

type searchItemsRPCRequest struct {
	Params StoreSearchParams `json:"params"`
}

type searchItemsRPCResponse struct {
	Items []Item `json:"items"`
}

type playbackItemRPCRequest struct {
	EpisodeID string `json:"episodeId"`
}

type playbackItemRPCResponse struct {
	Item PlaybackItem `json:"item"`
}

type authorizePlaybackRPCResponse struct {
	Authorized bool `json:"authorized"`
}
