package media

import (
	"context"
	"errors"
	"net/http"

	"connectrpc.com/connect"

	"watch_together/server/internal/internalrpc"
	internalv1 "watch_together/server/internal/rpcgen/v1"
	"watch_together/server/internal/rpcgen/v1/internalv1connect"
)

type RPCStore struct {
	client internalv1connect.MediaInternalServiceClient
	config internalrpc.ClientConfig
}

func NewRPCStore(baseURL string, config internalrpc.ClientConfig) *RPCStore {
	baseURL = internalrpc.NormalizeBaseURL(baseURL)
	if baseURL == "" {
		return nil
	}
	return &RPCStore{
		client: internalv1connect.NewMediaInternalServiceClient(
			http.DefaultClient,
			internalrpc.ClientBaseURL(baseURL, config.PathPrefix),
		),
		config: config,
	}
}

func (s *RPCStore) ListTags(ctx context.Context, allLimit int) (TagList, error) {
	if s == nil || s.client == nil {
		return TagList{}, errors.New("media rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.ListTagsRequest{
		AllLimit: int32(allLimit),
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		s.config,
		internalv1connect.MediaInternalServiceListTagsProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, s.config.Service, requestID)

	response, err := s.client.ListTags(ctx, request)
	if err != nil {
		return TagList{}, err
	}
	return tagListFromProto(response.Msg), nil
}

func (s *RPCStore) SearchItems(ctx context.Context, params StoreSearchParams) ([]Item, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("media rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.SearchItemsRequest{
		Params: mediaSearchParamsToProto(params),
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		s.config,
		internalv1connect.MediaInternalServiceSearchItemsProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, s.config.Service, requestID)

	response, err := s.client.SearchItems(ctx, request)
	if err != nil {
		return nil, err
	}
	return mediaItemsFromProto(response.Msg.GetItems()), nil
}

func (s *RPCStore) FindPlaybackItem(ctx context.Context, episodeID string) (PlaybackItem, error) {
	if s == nil || s.client == nil {
		return PlaybackItem{}, errors.New("media rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.GetPlaybackItemRequest{
		EpisodeId: episodeID,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		s.config,
		internalv1connect.MediaInternalServiceGetPlaybackItemProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, s.config.Service, requestID)

	response, err := s.client.GetPlaybackItem(ctx, request)
	if err != nil {
		var connectErr *connect.Error
		if errors.As(err, &connectErr) && connectErr.Code() == connect.CodeNotFound {
			return PlaybackItem{}, ErrMediaNotFound
		}
		return PlaybackItem{}, err
	}
	return playbackItemFromProto(response.Msg.GetItem()), nil
}

func (s *RPCStore) AuthorizePlayback(ctx context.Context, episodeID string, assetPath string) (bool, error) {
	if s == nil || s.client == nil {
		return false, errors.New("media rpc client is unavailable")
	}
	request := connect.NewRequest(&internalv1.AuthorizePlaybackRequest{
		EpisodeId: episodeID,
		AssetPath: assetPath,
	})
	ctx, cancel, requestID, span := internalrpc.PrepareClientRequest(
		ctx,
		s.config,
		internalv1connect.MediaInternalServiceAuthorizePlaybackProcedure,
		request.Header(),
	)
	defer cancel()
	defer span.End()
	request.Msg.Metadata = internalrpc.RequestMetadata(ctx, s.config.Service, requestID)

	response, err := s.client.AuthorizePlayback(ctx, request)
	if err != nil {
		return false, err
	}
	return response.Msg.GetAuthorized(), nil
}

func RegisterInternalRPC(mux *http.ServeMux, prefix string, authToken string, service *Service) {
	if mux == nil || service == nil {
		return
	}
	path, handler := internalv1connect.NewMediaInternalServiceHandler(&internalRPCHandler{
		authToken: authToken,
		service:   service,
	})
	mountPath, prefixed := internalrpc.PrefixedHandler(prefix, path, handler)
	mux.Handle(mountPath, prefixed)
}

type internalRPCHandler struct {
	authToken string
	service   *Service
}

func (h *internalRPCHandler) ListTags(
	ctx context.Context,
	request *connect.Request[internalv1.ListTagsRequest],
) (*connect.Response[internalv1.ListTagsResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.MediaInternalServiceListTagsProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	tags, err := h.service.store.ListTags(ctx, int(request.Msg.GetAllLimit()))
	if err != nil {
		return nil, internalrpc.ToConnectError(err)
	}
	return connect.NewResponse(tagListToProto(tags)), nil
}

func (h *internalRPCHandler) SearchItems(
	ctx context.Context,
	request *connect.Request[internalv1.SearchItemsRequest],
) (*connect.Response[internalv1.SearchItemsResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.MediaInternalServiceSearchItemsProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	items, err := h.service.store.SearchItems(ctx, storeSearchParamsFromProto(request.Msg.GetParams()))
	if err != nil {
		return nil, internalrpc.ToConnectError(err)
	}
	return connect.NewResponse(&internalv1.SearchItemsResponse{
		Items: mediaItemsToProto(items),
	}), nil
}

func (h *internalRPCHandler) GetPlaybackItem(
	ctx context.Context,
	request *connect.Request[internalv1.GetPlaybackItemRequest],
) (*connect.Response[internalv1.GetPlaybackItemResponse], error) {
	ctx, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.MediaInternalServiceGetPlaybackItemProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	item, err := h.service.store.FindPlaybackItem(ctx, request.Msg.GetEpisodeId())
	if err != nil {
		if errors.Is(err, ErrMediaNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, internalrpc.ToConnectError(err)
	}
	return connect.NewResponse(&internalv1.GetPlaybackItemResponse{
		Item: playbackItemToProto(item),
	}), nil
}

func (h *internalRPCHandler) AuthorizePlayback(
	ctx context.Context,
	request *connect.Request[internalv1.AuthorizePlaybackRequest],
) (*connect.Response[internalv1.AuthorizePlaybackResponse], error) {
	_, span, err := internalrpc.PrepareServerRequest(
		ctx,
		request.Header(),
		h.authToken,
		internalv1connect.MediaInternalServiceAuthorizePlaybackProcedure,
	)
	if err != nil {
		return nil, err
	}
	defer span.End()
	return connect.NewResponse(&internalv1.AuthorizePlaybackResponse{Authorized: true}), nil
}

func tagListToProto(tags TagList) *internalv1.ListTagsResponse {
	return &internalv1.ListTagsResponse{
		FeaturedTags: tagsToProto(tags.FeaturedTags),
		AllTags:      tagsToProto(tags.AllTags),
	}
}

func tagListFromProto(response *internalv1.ListTagsResponse) TagList {
	if response == nil {
		return TagList{}
	}
	return TagList{
		FeaturedTags: tagsFromProto(response.GetFeaturedTags()),
		AllTags:      tagsFromProto(response.GetAllTags()),
	}
}

func tagsToProto(tags []Tag) []*internalv1.MediaTag {
	out := make([]*internalv1.MediaTag, 0, len(tags))
	for _, tag := range tags {
		out = append(out, &internalv1.MediaTag{
			Id:   tag.ID,
			Slug: tag.Slug,
			Name: tag.Name,
		})
	}
	return out
}

func tagsFromProto(tags []*internalv1.MediaTag) []Tag {
	out := make([]Tag, 0, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		out = append(out, Tag{
			ID:   tag.GetId(),
			Slug: tag.GetSlug(),
			Name: tag.GetName(),
		})
	}
	return out
}

func mediaSearchParamsToProto(params StoreSearchParams) *internalv1.MediaSearchParams {
	return &internalv1.MediaSearchParams{
		Query:  params.Query,
		Tag:    params.Tag,
		Limit:  int32(params.Limit),
		Offset: int32(params.Offset),
	}
}

func storeSearchParamsFromProto(params *internalv1.MediaSearchParams) StoreSearchParams {
	if params == nil {
		return StoreSearchParams{}
	}
	return StoreSearchParams{
		Query:  params.GetQuery(),
		Tag:    params.GetTag(),
		Limit:  int(params.GetLimit()),
		Offset: int(params.GetOffset()),
	}
}

func mediaItemsToProto(items []Item) []*internalv1.MediaItem {
	out := make([]*internalv1.MediaItem, 0, len(items))
	for _, item := range items {
		out = append(out, mediaItemToProto(item))
	}
	return out
}

func mediaItemsFromProto(items []*internalv1.MediaItem) []Item {
	out := make([]Item, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		out = append(out, mediaItemFromProto(item))
	}
	return out
}

func mediaItemToProto(item Item) *internalv1.MediaItem {
	return &internalv1.MediaItem{
		Id:           item.ID,
		Title:        item.Title,
		Subtitle:     cloneString(item.Subtitle),
		Description:  cloneString(item.Description),
		CoverUrl:     cloneString(item.CoverURL),
		MediaUrl:     item.MediaURL,
		DurationMs:   cloneInt64(item.DurationMs),
		SeasonLabel:  cloneString(item.SeasonLabel),
		EpisodeLabel: cloneString(item.EpisodeLabel),
		Tags:         itemTagsToProto(item.Tags),
	}
}

func mediaItemFromProto(item *internalv1.MediaItem) Item {
	if item == nil {
		return Item{}
	}
	return Item{
		ID:           item.GetId(),
		Title:        item.GetTitle(),
		Subtitle:     cloneString(item.Subtitle),
		Description:  cloneString(item.Description),
		CoverURL:     cloneString(item.CoverUrl),
		MediaURL:     item.GetMediaUrl(),
		DurationMs:   cloneInt64(item.DurationMs),
		SeasonLabel:  cloneString(item.SeasonLabel),
		EpisodeLabel: cloneString(item.EpisodeLabel),
		Tags:         itemTagsFromProto(item.GetTags()),
	}
}

func itemTagsToProto(tags []ItemTag) []*internalv1.MediaItemTag {
	out := make([]*internalv1.MediaItemTag, 0, len(tags))
	for _, tag := range tags {
		out = append(out, &internalv1.MediaItemTag{
			Slug: tag.Slug,
			Name: tag.Name,
		})
	}
	return out
}

func itemTagsFromProto(tags []*internalv1.MediaItemTag) []ItemTag {
	out := make([]ItemTag, 0, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		out = append(out, ItemTag{
			Slug: tag.GetSlug(),
			Name: tag.GetName(),
		})
	}
	return out
}

func playbackItemToProto(item PlaybackItem) *internalv1.PlaybackItem {
	return &internalv1.PlaybackItem{
		Id:       item.ID,
		MediaUrl: item.MediaURL,
	}
}

func playbackItemFromProto(item *internalv1.PlaybackItem) PlaybackItem {
	if item == nil {
		return PlaybackItem{}
	}
	return PlaybackItem{
		ID:       item.GetId(),
		MediaURL: item.GetMediaUrl(),
	}
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
