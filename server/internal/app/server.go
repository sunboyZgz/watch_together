package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"gorm.io/gorm"

	"watch_together/server/internal/auth"
	"watch_together/server/internal/authority"
	"watch_together/server/internal/cache"
	"watch_together/server/internal/eventbus"
	"watch_together/server/internal/home"
	"watch_together/server/internal/internalrpc"
	"watch_together/server/internal/media"
	"watch_together/server/internal/observability"
	"watch_together/server/internal/progress"
	"watch_together/server/internal/recovery"
	"watch_together/server/internal/room"
	"watch_together/server/internal/roomapi"
	"watch_together/server/internal/servicekit"
	"watch_together/server/internal/store"
	"watch_together/server/internal/telemetry"
	"watch_together/server/internal/timeline"
	"watch_together/server/internal/transport"
)

type Config struct {
	AppEnv              string
	Host                string
	Port                string
	LogLevel            string
	InstanceID          string
	RoomRuntimeMode     string
	DatabaseURL         string
	IdentityDatabaseURL string
	MediaDatabaseURL    string
	TimelineDatabaseURL string
	DebugSync           bool
	Auth                auth.TokenConfig
	Redis               cache.RedisConfig
	WebSocket           transport.WebSocketRuntimeConfig
	NATS                eventbus.NATSConfig
	Kafka               KafkaConfig
	AuthorityRecovery   AuthorityRecoveryConfig
	Observability       observability.Config
	Service             servicekit.Config
	InternalRPC         InternalRPCConfig
	ServiceClients      ServiceClientsConfig
	Telemetry           telemetry.Config
	Media               transport.MediaPlaybackConfig
}

type RedisConfig = cache.RedisConfig
type WebSocketRuntimeConfig = transport.WebSocketRuntimeConfig
type NATSConfig = eventbus.NATSConfig
type MediaPlaybackConfig = transport.MediaPlaybackConfig
type AuthTokenConfig = auth.TokenConfig
type ObservabilityConfig = observability.Config
type ServiceConfig = servicekit.Config
type TelemetryConfig = telemetry.Config

type InternalRPCConfig struct {
	Enabled    bool
	Addr       string
	PathPrefix string
	Timeout    time.Duration
	AuthToken  string
}

type ServiceClientsConfig struct {
	DiscoveryMode    string
	MediaMode        string
	MediaAddr        string
	IdentityMode     string
	IdentityAddr     string
	TimelineMode     string
	TimelineAddr     string
	AuthorityMode    string
	AuthorityAddr    string
	AuthorityLeaseID string
}

type KafkaConfig struct {
	Brokers                []string
	ClientID               string
	TopicRoomTimeline      string
	TopicRoomControlResult string
	TopicRoomMembership    string
	DerivedConsumerGroupID string
}

type AuthorityRecoveryConfig struct {
	RenewInterval        time.Duration
	TakeoverScanInterval time.Duration
	RecoveryTimeout      time.Duration
	KafkaReplayTimeout   time.Duration
}

type Server struct {
	config            Config
	httpServer        *http.Server
	roomManager       *room.Manager
	redis             *cache.RedisClient
	db                *gorm.DB
	identityDB        *gorm.DB
	mediaDB           *gorm.DB
	timelineDB        *gorm.DB
	cancel            context.CancelFunc
	eventBus          eventbus.RoomBroadcastBus
	controlBus        eventbus.RoomControlBus
	telemetryShutdown telemetry.ShutdownFunc
}

type accessTokenVerifier interface {
	VerifyAccessToken(rawToken string) (auth.TokenClaims, error)
}

const (
	roomRuntimeModeLocalProcess         = "local_process"
	roomRuntimeModeDistributedAuthority = "distributed_authority"
)

type runtimeBoundary struct {
	InstanceID      string
	RoomRuntimeMode string
}

// NewServer assembles the in-memory room manager and the HTTP routes around it.
func NewServer(config Config) *Server {
	config.RoomRuntimeMode = normalizeRoomRuntimeMode(config.RoomRuntimeMode)
	distributedAuthority := config.RoomRuntimeMode == roomRuntimeModeDistributedAuthority
	if distributedAuthority && len(config.Kafka.Brokers) == 0 {
		log.Fatal("distributed_authority requires kafka brokers")
	}
	serverCtx, cancel := context.WithCancel(context.Background())
	roomManager := room.NewManager()
	tokenManager := auth.NewTokenManager(config.Auth)
	observabilityConfig := config.Observability.Normalized()
	serviceConfig := config.Service.Normalized("watch-together-roomserver")
	telemetryShutdown, err := telemetry.Start(serverCtx, config.Telemetry.Normalized(serviceConfig.ServiceName))
	if err != nil {
		log.Printf("failed to start telemetry; tracing disabled: %v", err)
	}
	metrics := observability.NewMetrics()
	if distributedAuthority {
		config.Redis.Required = true
	}
	redisClient := newRedisClient("room_state", cache.RoomStateRedisConfig(config.Redis))
	if distributedAuthority && redisClient == nil {
		log.Fatal("distributed_authority requires redis")
	}
	var authorityRegistry *cache.RoomAuthorityRegistry
	var activeDeviceRegistry *cache.ActiveDeviceRegistry
	var controlRequestRegistry *cache.ControlRequestRegistry
	var controlRateRegistry *cache.ControlRateRegistry
	var presenceRegistry *cache.PresenceRegistry
	var roomStateCache *cache.RoomStateCache
	if redisClient != nil {
		roomStateCache = cache.NewRoomStateCache(redisClient, 0)
		authorityRegistry = cache.NewRoomAuthorityRegistry(redisClient, 0)
		activeDeviceRegistry = cache.NewActiveDeviceRegistry(redisClient, 0)
		controlRequestRegistry = cache.NewControlRequestRegistry(redisClient, config.WebSocket.ControlIdempotencyTTL)
		controlRateRegistry = cache.NewControlRateRegistry(redisClient)
		presenceRegistry = cache.NewPresenceRegistry(redisClient, config.WebSocket.PresenceLeaseTTL)
	}
	db := newPostgresDB("postgres", config.DatabaseURL)
	if distributedAuthority && db == nil {
		log.Fatal("distributed_authority requires postgres")
	}
	var identityDB *gorm.DB
	if !isRPCMode(config.ServiceClients.IdentityMode) && strings.TrimSpace(config.IdentityDatabaseURL) != "" {
		identityDB = newPostgresDB("identity_postgres", config.IdentityDatabaseURL)
	}
	var mediaDB *gorm.DB
	if !isRPCMode(config.ServiceClients.MediaMode) && strings.TrimSpace(config.MediaDatabaseURL) != "" {
		mediaDB = newPostgresDB("media_postgres", config.MediaDatabaseURL)
	}
	var timelineDB *gorm.DB
	if !isRPCMode(config.ServiceClients.TimelineMode) && strings.TrimSpace(config.TimelineDatabaseURL) != "" {
		timelineDB = newPostgresDB("timeline_postgres", config.TimelineDatabaseURL)
	}
	mediaService := newMediaService(db, mediaDB, config, serviceConfig)
	identityService := newIdentityService(db, identityDB, config, serviceConfig, tokenManager)
	roomService, roomStore := newRoomService(db, mediaService)
	var timelineRecorder timeline.ResultRecorder = timeline.NoopRecorder{}
	var timelineRecoveryReader timeline.RecoveryReader
	var timelineOutboxStore *store.PostgresTimelineOutboxStore
	if distributedAuthority && !isRPCMode(config.ServiceClients.TimelineMode) {
		timelineOutboxStore = newTimelineOutboxStore(db, timelineDB, config)
	}
	if distributedAuthority && timelineOutboxStore != nil {
		timelineRecorder = timeline.NewService(timelineOutboxStore, nil, nil)
	} else if distributedAuthority && !isRPCMode(config.ServiceClients.TimelineMode) && strings.TrimSpace(config.TimelineDatabaseURL) != "" {
		unavailableTimeline := timeline.UnavailableStore{}
		timelineRecorder = unavailableTimeline
		timelineRecoveryReader = unavailableTimeline
	}
	if isRPCMode(config.ServiceClients.TimelineMode) {
		timelineClient := timeline.NewRPCClient(config.ServiceClients.TimelineAddr, internalRPCClientConfig(config, serviceConfig))
		if timelineClient != nil {
			timelineRecorder = timelineClient
			timelineRecoveryReader = timelineClient
		}
	}
	installRoomLifecycleHooks(roomManager, roomStore, roomStateCache)
	if roomStore != nil {
		now := time.Now()
		backfilled, err := roomStore.MarkAllActiveRoomsGracePeriod(
			context.Background(),
			now,
			now.Add(room.DefaultEmptyRoomGracePeriod()),
		)
		if err != nil {
			log.Printf("failed to backfill active rooms into grace_period: %v", err)
		} else if backfilled > 0 {
			log.Printf("backfilled %d active rooms into grace_period on startup", backfilled)
		}
	}
	go roomManager.StartCleanupLoop(serverCtx, room.DefaultCleanupInterval())
	if roomStore != nil {
		go startPersistentRoomCleanupLoop(serverCtx, room.DefaultCleanupInterval(), roomStore, roomStateCache)
	}
	roomHTTPHandler := transport.NewRoomHTTPHandlerWithTokenVerifierAndRoomStateWriter(
		roomManager,
		roomService,
		identityService,
		roomStateCache,
		config.Media,
	)
	broadcastInstanceID := roomBroadcastInstanceID(config.InstanceID)
	if distributedAuthority {
		roomHTTPHandler.SetRoomAuthorityClaimer(authorityClaimInstanceID(config, broadcastInstanceID), authorityRegistry)
	}
	var authorityRecovery *recovery.Service
	if distributedAuthority {
		if timelineRecoveryReader == nil {
			kafkaReader, err := timeline.NewKafkaRoomEventReader(
				config.Kafka.Brokers,
				config.Kafka.TopicRoomTimeline,
				config.AuthorityRecovery.KafkaReplayTimeout,
			)
			if err != nil {
				log.Fatalf("failed to open kafka room timeline reader: %v", err)
			}
			if timelineOutboxStore != nil {
				timelineRecoveryReader = timeline.NewService(timelineOutboxStore, kafkaReader, timelineOutboxStore)
			} else {
				timelineRecoveryReader = timeline.NewService(nil, kafkaReader, nil)
			}
		}
		authorityRecovery = recovery.NewService(
			recovery.Config{
				InstanceID:      broadcastInstanceID,
				RecoveryTimeout: config.AuthorityRecovery.RecoveryTimeout,
			},
			authorityRegistry,
			roomManager,
			roomStore,
			timelineRecoveryReader,
			roomStateCache,
		)
		roomHTTPHandler.SetRoomAuthorityRecovery(authorityRecovery)
		if !isRPCMode(config.ServiceClients.AuthorityMode) {
			go startAuthorityRenewLoop(
				serverCtx,
				config.AuthorityRecovery.RenewInterval,
				roomManager,
				authorityRegistry,
				broadcastInstanceID,
			)
		}
		go authorityRecovery.RunScanner(serverCtx, config.AuthorityRecovery.TakeoverScanInterval)
	}
	roomBroadcastBus := newRoomBroadcastBus(config.WebSocket, config.NATS, distributedAuthority)
	roomControlBus := newRoomControlBus(config.WebSocket, config.NATS, distributedAuthority)
	var authorityControl authority.ControlApplier
	if distributedAuthority && isRPCMode(config.ServiceClients.AuthorityMode) {
		authorityControl = authority.NewRPCClient(config.ServiceClients.AuthorityAddr, internalRPCClientConfig(config, serviceConfig))
	}
	authHTTPHandler := transport.NewAuthHTTPHandler(identityService)
	homeHTTPHandler := transport.NewHomeHTTPHandlerWithTokenVerifier(newHomeService(db, mediaService), identityService)
	mediaHTTPHandler := transport.NewMediaHTTPHandlerWithTokenVerifier(mediaService, identityService, config.Media)
	progressHTTPHandler := transport.NewProgressHTTPHandlerWithTokenVerifier(newProgressService(db, mediaService), identityService)
	router := newGinRouter(
		serverCtx,
		roomManager,
		config.DebugSync,
		config.WebSocket,
		roomStateCache,
		identityService,
		roomHTTPHandler,
		authHTTPHandler,
		homeHTTPHandler,
		mediaHTTPHandler,
		progressHTTPHandler,
		runtimeBoundaryFromConfig(config),
		func(ctx context.Context) observability.ReadinessSnapshot {
			return readinessSnapshotFromConfig(ctx, config, db, identityDB, mediaDB, timelineDB, redisClient, roomBroadcastBus, roomControlBus, timelineOutboxStore, authorityRecovery)
		},
		observabilityConfig,
		metrics,
		broadcastInstanceID,
		roomBroadcastBus,
		roomControlBus,
		authorityControl,
		authorityRegistry,
		activeDeviceRegistry,
		controlRequestRegistry,
		controlRateRegistry,
		presenceRegistry,
		timelineRecorder,
		authorityRecovery,
	)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", config.Host, config.Port),
		Handler: router,
	}

	return &Server{
		config:            config,
		httpServer:        httpServer,
		roomManager:       roomManager,
		redis:             redisClient,
		db:                db,
		identityDB:        identityDB,
		mediaDB:           mediaDB,
		timelineDB:        timelineDB,
		cancel:            cancel,
		eventBus:          roomBroadcastBus,
		controlBus:        roomControlBus,
		telemetryShutdown: telemetryShutdown,
	}
}

func internalRPCClientConfig(config Config, service servicekit.Config) internalrpc.ClientConfig {
	return internalrpc.ClientConfig{
		PathPrefix: config.InternalRPC.PathPrefix,
		Timeout:    config.InternalRPC.Timeout,
		AuthToken:  config.InternalRPC.AuthToken,
		Service:    service,
	}
}

func isRPCMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), "rpc")
}

func authorityClaimInstanceID(config Config, fallback string) string {
	if isRPCMode(config.ServiceClients.AuthorityMode) && strings.TrimSpace(config.ServiceClients.AuthorityLeaseID) != "" {
		return strings.TrimSpace(config.ServiceClients.AuthorityLeaseID)
	}
	return fallback
}

func normalizeRoomRuntimeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return roomRuntimeModeLocalProcess
	}
	return mode
}

func runtimeBoundaryFromConfig(config Config) runtimeBoundary {
	return runtimeBoundary{
		InstanceID:      strings.TrimSpace(config.InstanceID),
		RoomRuntimeMode: normalizeRoomRuntimeMode(config.RoomRuntimeMode),
	}
}

func readinessSnapshotFromConfig(
	ctx context.Context,
	config Config,
	db *gorm.DB,
	identityDB *gorm.DB,
	mediaDB *gorm.DB,
	timelineDB *gorm.DB,
	redisClient *cache.RedisClient,
	roomBroadcastBus eventbus.RoomBroadcastBus,
	roomControlBus eventbus.RoomControlBus,
	timelineOutboxStore *store.PostgresTimelineOutboxStore,
	authorityRecovery *recovery.Service,
) observability.ReadinessSnapshot {
	distributedAuthority := normalizeRoomRuntimeMode(config.RoomRuntimeMode) == roomRuntimeModeDistributedAuthority
	return observability.NewReadinessSnapshot(
		config.AppEnv,
		strings.TrimSpace(config.InstanceID),
		normalizeRoomRuntimeMode(config.RoomRuntimeMode),
		[]observability.DependencyStatus{
			dependencyStatus("postgres", db != nil, distributedAuthority || strings.TrimSpace(config.DatabaseURL) != ""),
			dependencyStatus("identity_postgres", identityDB != nil, !isRPCMode(config.ServiceClients.IdentityMode) && strings.TrimSpace(config.IdentityDatabaseURL) != ""),
			dependencyStatus("media_postgres", mediaDB != nil, !isRPCMode(config.ServiceClients.MediaMode) && strings.TrimSpace(config.MediaDatabaseURL) != ""),
			dependencyStatus("timeline_postgres", timelineDB != nil, !isRPCMode(config.ServiceClients.TimelineMode) && strings.TrimSpace(config.TimelineDatabaseURL) != ""),
			dependencyStatus("redis", redisClient != nil, distributedAuthority || config.Redis.Enabled()),
			dependencyStatus("nats_broadcast", !isDisabledRoomBroadcastBus(roomBroadcastBus), distributedAuthority || config.WebSocket.CrossInstanceBroadcast),
			dependencyStatus("nats_control", !isDisabledRoomControlBus(roomControlBus), distributedAuthority),
			dependencyStatus("kafka", len(config.Kafka.Brokers) > 0, distributedAuthority),
			dependencyStatus("outbox", timelineOutboxStore != nil || isRPCMode(config.ServiceClients.TimelineMode), distributedAuthority),
			dependencyStatus("recovery", authorityRecovery != nil, distributedAuthority),
			rpcDependencyStatus(ctx, "identity_rpc", config.ServiceClients.IdentityAddr, config.Observability, isRPCMode(config.ServiceClients.IdentityMode)),
			rpcDependencyStatus(ctx, "media_rpc", config.ServiceClients.MediaAddr, config.Observability, isRPCMode(config.ServiceClients.MediaMode)),
			rpcDependencyStatus(ctx, "timeline_rpc", config.ServiceClients.TimelineAddr, config.Observability, isRPCMode(config.ServiceClients.TimelineMode)),
			rpcDependencyStatus(ctx, "authority_rpc", config.ServiceClients.AuthorityAddr, config.Observability, isRPCMode(config.ServiceClients.AuthorityMode)),
		},
	)
}

func dependencyStatus(name string, ok bool, required bool) observability.DependencyStatus {
	status := "disabled"
	if ok {
		status = "ok"
	} else if required {
		status = "unavailable"
	}
	return observability.DependencyStatus{Name: name, Status: status, Required: required}
}

func rpcDependencyStatus(ctx context.Context, name string, addr string, config observability.Config, required bool) observability.DependencyStatus {
	if !required {
		return observability.DependencyStatus{Name: name, Status: "disabled", Required: false}
	}
	baseURL := internalrpc.NormalizeBaseURL(addr)
	if baseURL == "" {
		return observability.DependencyStatus{Name: name, Status: "unavailable", Required: true}
	}
	readinessPath := strings.TrimSpace(config.Normalized().ReadinessPath)
	if !strings.HasPrefix(readinessPath, "/") {
		readinessPath = "/" + readinessPath
	}
	probeCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+readinessPath, nil)
	if err != nil {
		return observability.DependencyStatus{Name: name, Status: "unavailable", Required: true}
	}
	client := http.Client{Timeout: 300 * time.Millisecond}
	response, err := client.Do(request)
	if err != nil {
		return observability.DependencyStatus{Name: name, Status: "unavailable", Required: true}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return observability.DependencyStatus{Name: name, Status: "unavailable", Required: true}
	}
	var snapshot observability.ReadinessSnapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil || snapshot.Status != "ready" {
		return observability.DependencyStatus{Name: name, Status: "unavailable", Required: true}
	}
	return observability.DependencyStatus{Name: name, Status: "ok", Required: true}
}

func isDisabledRoomBroadcastBus(bus eventbus.RoomBroadcastBus) bool {
	_, disabled := bus.(eventbus.DisabledRoomBroadcastBus)
	return disabled || bus == nil
}

func isDisabledRoomControlBus(bus eventbus.RoomControlBus) bool {
	_, disabled := bus.(eventbus.DisabledRoomControlBus)
	return disabled || bus == nil
}

func roomBroadcastInstanceID(instanceID string) string {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID != "" {
		return instanceID
	}
	return fmt.Sprintf("roomserver-%d", time.Now().UnixNano())
}

func newRoomBroadcastBus(
	webSocketConfig transport.WebSocketRuntimeConfig,
	natsConfig eventbus.NATSConfig,
	required bool,
) eventbus.RoomBroadcastBus {
	if !webSocketConfig.CrossInstanceBroadcast {
		if required {
			log.Fatal("distributed_authority requires cross-instance websocket broadcast")
		}
		return eventbus.NewDisabledRoomBroadcastBus()
	}
	eventBus, err := eventbus.NormalizeEventBus(webSocketConfig.EventBus)
	if err != nil {
		if required {
			log.Fatalf("unsupported websocket event bus: %v", err)
		}
		log.Printf("unsupported websocket event bus; cross-instance broadcast disabled: %v", err)
		return eventbus.NewDisabledRoomBroadcastBus()
	}
	if eventBus != eventbus.EventBusNATSCore {
		if required {
			log.Fatalf("unsupported websocket event bus %q", eventBus)
		}
		log.Printf("unsupported websocket event bus %q; cross-instance broadcast disabled", eventBus)
		return eventbus.NewDisabledRoomBroadcastBus()
	}
	bus, err := eventbus.OpenNATSRoomBroadcastBus(natsConfig)
	if err != nil {
		if required {
			log.Fatalf("failed to connect required NATS broadcast bus: %v", err)
		}
		log.Printf("failed to connect NATS event bus; cross-instance broadcast disabled: %v", err)
		return eventbus.NewDisabledRoomBroadcastBus()
	}
	normalized := eventbus.NormalizeNATSConfig(natsConfig)
	log.Printf("connected NATS event bus url=%s subject=%s name=%s", normalized.URL, normalized.Subject, normalized.Name)
	return bus
}

func newRoomControlBus(
	webSocketConfig transport.WebSocketRuntimeConfig,
	natsConfig eventbus.NATSConfig,
	required bool,
) eventbus.RoomControlBus {
	if !required {
		return eventbus.NewDisabledRoomControlBus()
	}
	if !webSocketConfig.CrossInstanceBroadcast {
		log.Fatal("distributed_authority requires cross-instance websocket broadcast")
	}
	bus, err := eventbus.OpenNATSRoomControlBus(natsConfig)
	if err != nil {
		log.Fatalf("failed to connect required NATS control bus: %v", err)
	}
	normalized := eventbus.NormalizeNATSConfig(natsConfig)
	log.Printf("connected NATS control bus url=%s subject=%s name=%s", normalized.URL, normalized.ControlSubject, normalized.Name)
	return bus
}

func newRedisClient(name string, config cache.RedisConfig) *cache.RedisClient {
	if !config.Enabled() {
		log.Printf("REDIS_ADDR is not set; redis-backed %s cache is disabled", name)
		return nil
	}

	client, err := cache.OpenRedis(context.Background(), config)
	if err != nil {
		if config.Required {
			log.Fatalf("failed to connect required redis for %s: %v", name, err)
		}
		log.Printf("failed to connect redis for %s; redis-backed feature disabled: %v", name, err)
		return nil
	}
	log.Printf("connected redis name=%s addr=%s db=%d tls=%t", name, config.Addr, config.DB, config.TLSEnabled)
	return client
}

func newPostgresDB(name string, databaseURL string) *gorm.DB {
	if strings.TrimSpace(databaseURL) == "" {
		log.Printf("%s URL is not set; database-backed endpoints may return service unavailable", name)
		return nil
	}
	db, err := store.OpenPostgres(context.Background(), databaseURL)
	if err != nil {
		log.Printf("failed to connect %s; database-backed endpoints unavailable: %v", name, err)
		return nil
	}
	return db
}

func newGinRouter(
	ctx context.Context,
	roomManager *room.Manager,
	debugSync bool,
	webSocketConfig transport.WebSocketRuntimeConfig,
	roomStateCache *cache.RoomStateCache,
	tokenVerifier accessTokenVerifier,
	roomHTTPHandler *transport.RoomHTTPHandler,
	authHTTPHandler *transport.AuthHTTPHandler,
	homeHTTPHandler *transport.HomeHTTPHandler,
	mediaHTTPHandler *transport.MediaHTTPHandler,
	progressHTTPHandler *transport.ProgressHTTPHandler,
	runtime runtimeBoundary,
	readinessProvider func(context.Context) observability.ReadinessSnapshot,
	observabilityConfig observability.Config,
	metrics *observability.Metrics,
	broadcastInstanceID string,
	roomBroadcastBus eventbus.RoomBroadcastBus,
	roomControlBus eventbus.RoomControlBus,
	authorityControl authority.ControlApplier,
	authorityRegistry *cache.RoomAuthorityRegistry,
	activeDeviceRegistry *cache.ActiveDeviceRegistry,
	controlRequestRegistry *cache.ControlRequestRegistry,
	controlRateRegistry *cache.ControlRateRegistry,
	presenceRegistry *cache.PresenceRegistry,
	timelineRecorder timeline.ResultRecorder,
	authorityRecovery *recovery.Service,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) {
		spanName := c.Request.Method + " " + c.Request.URL.Path
		ctx, span := otel.Tracer("watch_together/http").Start(c.Request.Context(), spanName)
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	router.GET("/healthz", func(c *gin.Context) {
		if runtime.InstanceID != "" {
			c.Header("X-Watch-Together-Instance-ID", runtime.InstanceID)
		}
		c.Header("X-Watch-Together-Room-Runtime", normalizeRoomRuntimeMode(runtime.RoomRuntimeMode))
		c.String(http.StatusOK, "ok")
	})
	router.GET(observabilityConfig.ReadinessPath, func(c *gin.Context) {
		readiness := observability.ReadinessSnapshot{Status: "ready"}
		if readinessProvider != nil {
			readiness = readinessProvider(c.Request.Context())
		}
		status := http.StatusOK
		if readiness.Status != "ready" {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, readiness)
	})
	if observabilityConfig.MetricsEnabled {
		router.GET(observabilityConfig.MetricsPath, gin.WrapH(metrics.Handler()))
	}
	router.Any("/auth/login", gin.WrapF(authHTTPHandler.Login))
	router.Any("/auth/register", gin.WrapF(authHTTPHandler.Register))
	router.Any("/home/summary", gin.WrapF(homeHTTPHandler.Summary))
	router.Any("/media/tags", gin.WrapF(mediaHTTPHandler.Tags))
	router.Any("/media/items", gin.WrapF(mediaHTTPHandler.Items))
	router.Any("/media/internal/auth", gin.WrapF(mediaHTTPHandler.NginxAuth))
	router.Any("/media/playback/*playbackPath", gin.WrapF(mediaHTTPHandler.Playback))
	router.Any("/me/media-progress/*mediaPath", gin.WrapF(progressHTTPHandler.Update))
	router.Any("/rooms", gin.WrapF(roomHTTPHandler.CreateRoom))
	router.Any("/rooms/*roomPath", gin.WrapF(roomHTTPHandler.RoomRoute))
	webSocketHandler := transport.NewWebSocketHandlerWithConfigAndRoomStateWriterAndTokenVerifierAndRoomLeaver(
		roomManager,
		debugSync,
		webSocketConfig,
		roomStateCache,
		tokenVerifier,
		roomHTTPHandler.RoomLeaver(),
	)
	webSocketHandler.SetMetrics(metrics)
	webSocketHandler.SetRoomBroadcastBus(broadcastInstanceID, roomBroadcastBus)
	webSocketHandler.SetAuthorityControlApplier(authorityControl)
	if normalizeRoomRuntimeMode(runtime.RoomRuntimeMode) == roomRuntimeModeDistributedAuthority {
		webSocketHandler.SetDistributedAuthorityRuntime(
			broadcastInstanceID,
			authorityRegistry,
			activeDeviceRegistry,
			roomControlBus,
			timelineRecorder,
		)
		webSocketHandler.SetDistributedControlHardening(
			controlRequestRegistry,
			presenceRegistry,
			webSocketConfig.PresenceRefreshInterval,
		)
		webSocketHandler.SetDistributedControlRateRegistry(controlRateRegistry)
		webSocketHandler.SetRoomAuthorityRecovery(authorityRecovery)
	}
	if err := webSocketHandler.SubscribeRoomBroadcasts(ctx); err != nil {
		log.Printf("failed to subscribe room broadcasts; cross-instance broadcast disabled: %v", err)
	}
	if err := webSocketHandler.SubscribeRoomControls(ctx); err != nil {
		log.Printf("failed to subscribe room control requests: %v", err)
	}
	router.Any("/ws", gin.WrapH(webSocketHandler))

	return router
}

// newIdentityService connects identity APIs to a local PostgreSQL store or the identity RPC boundary.
func newIdentityService(
	db *gorm.DB,
	identityDB *gorm.DB,
	config Config,
	service servicekit.Config,
	tokenManager *auth.TokenManager,
) auth.IdentityService {
	if isRPCMode(config.ServiceClients.IdentityMode) {
		return auth.NewRPCClient(config.ServiceClients.IdentityAddr, internalRPCClientConfig(config, service))
	}
	var storeDB *gorm.DB
	if strings.TrimSpace(config.IdentityDatabaseURL) != "" {
		storeDB = identityDB
	} else {
		storeDB = db
	}
	if storeDB == nil {
		return nil
	}
	return auth.NewServiceWithTokenManager(store.NewPostgresUserStore(storeDB), tokenManager)
}

// newHomeService connects home summary reads to the shared PostgreSQL handle when available.
func newHomeService(db *gorm.DB, mediaService *media.Service) *home.Service {
	if db == nil || mediaService == nil {
		return nil
	}
	return home.NewServiceWithMediaSummaries(store.NewPostgresHomeStore(db), mediaService)
}

// newMediaService connects media catalog APIs to the shared PostgreSQL handle when available.
func newMediaService(db *gorm.DB, mediaDB *gorm.DB, config Config, service servicekit.Config) *media.Service {
	if isRPCMode(config.ServiceClients.MediaMode) {
		rpcStore := media.NewRPCStore(config.ServiceClients.MediaAddr, internalRPCClientConfig(config, service))
		if rpcStore == nil {
			return nil
		}
		return media.NewService(rpcStore)
	}
	var storeDB *gorm.DB
	if strings.TrimSpace(config.MediaDatabaseURL) != "" {
		storeDB = mediaDB
	} else {
		storeDB = db
	}
	if storeDB == nil {
		return nil
	}
	return media.NewService(store.NewPostgresMediaStore(storeDB))
}

func newTimelineOutboxStore(db *gorm.DB, timelineDB *gorm.DB, config Config) *store.PostgresTimelineOutboxStore {
	var storeDB *gorm.DB
	if strings.TrimSpace(config.TimelineDatabaseURL) != "" {
		storeDB = timelineDB
	} else {
		storeDB = db
	}
	if storeDB == nil {
		return nil
	}
	return store.NewPostgresTimelineOutboxStore(storeDB, config.Kafka.TopicRoomTimeline)
}

// newRoomService connects room business APIs to the shared PostgreSQL handle when available.
func newRoomService(db *gorm.DB, mediaService *media.Service) (*roomapi.Service, *store.PostgresRoomStore) {
	if db == nil {
		return nil, nil
	}
	roomStore := store.NewPostgresRoomStore(db)
	if mediaService == nil {
		return nil, roomStore
	}
	var mediaLookup roomapi.MediaDetailLookup
	mediaLookup = mediaService
	return roomapi.NewServiceWithMediaLookup(roomStore, mediaLookup), roomStore
}

// newProgressService connects media progress writes to the shared PostgreSQL handle when available.
func newProgressService(db *gorm.DB, mediaService *media.Service) *progress.Service {
	if db == nil || mediaService == nil {
		return nil
	}
	var mediaValidator progress.MediaValidator
	mediaValidator = mediaService
	return progress.NewServiceWithMediaValidator(store.NewPostgresProgressStore(db), mediaValidator)
}

func startPersistentRoomCleanupLoop(
	ctx context.Context,
	interval time.Duration,
	roomStore *store.PostgresRoomStore,
	roomStateCache *cache.RoomStateCache,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			roomCodes, err := roomStore.CleanupExpiredRoomCodes(context.Background(), time.Now())
			if err != nil {
				log.Printf("failed to cleanup expired rooms in postgres: %v", err)
				continue
			}
			for _, roomCode := range roomCodes {
				deleteRoomStateCache(context.Background(), roomStateCache, roomCode)
			}
			if len(roomCodes) > 0 {
				log.Printf("cleaned up %d expired rooms from postgres", len(roomCodes))
			}
		}
	}
}

func startAuthorityRenewLoop(
	ctx context.Context,
	interval time.Duration,
	roomManager *room.Manager,
	authorityRegistry *cache.RoomAuthorityRegistry,
	instanceID string,
) {
	if interval <= 0 || roomManager == nil || authorityRegistry == nil || instanceID == "" {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, roomID := range roomManager.RoomIDs() {
				if _, renewed, err := authorityRegistry.RenewAuthority(ctx, roomID, instanceID); err != nil {
					log.Printf("room authority renew failed room=%s instance=%s err=%v", roomID, instanceID, err)
				} else if !renewed {
					log.Printf("room authority renew skipped room=%s instance=%s", roomID, instanceID)
				}
			}
		}
	}
}

func installRoomLifecycleHooks(
	roomManager *room.Manager,
	roomStore *store.PostgresRoomStore,
	roomStateCache *cache.RoomStateCache,
) {
	if roomManager == nil {
		return
	}
	roomManager.SetLifecycleHooks(room.LifecycleHooks{
		OnRoomBecameEmpty: func(roomID string, emptySince time.Time, destroyAfter time.Time) {
			if roomStore != nil {
				if err := roomStore.MarkRoomGracePeriod(context.Background(), roomID, emptySince, destroyAfter); err != nil {
					log.Printf("failed to mark room %s grace_period: %v", roomID, err)
				}
			}
		},
		OnRoomReactivated: func(roomID string) {
			if roomStore != nil {
				if err := roomStore.MarkRoomActive(context.Background(), roomID); err != nil {
					log.Printf("failed to reactivate room %s: %v", roomID, err)
				}
			}
		},
		OnRoomDestroyed: func(roomID string) {
			if roomStore != nil {
				if err := roomStore.DestroyRoom(context.Background(), roomID); err != nil {
					log.Printf("failed to destroy room %s: %v", roomID, err)
				}
			}
			deleteRoomStateCache(context.Background(), roomStateCache, roomID)
		},
	})
}

func deleteRoomStateCache(ctx context.Context, roomStateCache *cache.RoomStateCache, roomID string) {
	if roomStateCache == nil || strings.TrimSpace(roomID) == "" {
		return
	}
	if err := roomStateCache.DeleteRoomState(ctx, roomID); err != nil && !errors.Is(err, cache.ErrRedisDisabled) {
		log.Printf("failed to delete room_state cache room=%s err=%v", roomID, err)
	}
}

// Address exposes the listen address mainly for logging and local verification.
func (s *Server) Address() string {
	return s.httpServer.Addr
}

// ListenAndServe delegates the actual HTTP serving to net/http.
func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops HTTP serving and closes infrastructure resources.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	var shutdownErr error
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			shutdownErr = err
		}
	}
	if err := s.Close(); err != nil && shutdownErr == nil {
		shutdownErr = err
	}
	return shutdownErr
}

// Close releases shared Redis and PostgreSQL resources.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	var closeErr error
	if s.httpServer != nil {
		if err := s.httpServer.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			closeErr = err
		}
	}
	if s.redis != nil {
		if err := s.redis.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if s.eventBus != nil {
		if err := s.eventBus.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if s.controlBus != nil {
		if err := s.controlBus.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if s.db != nil {
		sqlDB, err := s.db.DB()
		if err != nil {
			if closeErr == nil {
				closeErr = err
			}
		} else if err := sqlDB.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if s.identityDB != nil && s.identityDB != s.db {
		sqlDB, err := s.identityDB.DB()
		if err != nil {
			if closeErr == nil {
				closeErr = err
			}
		} else if err := sqlDB.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if s.mediaDB != nil && s.mediaDB != s.db && s.mediaDB != s.identityDB {
		sqlDB, err := s.mediaDB.DB()
		if err != nil {
			if closeErr == nil {
				closeErr = err
			}
		} else if err := sqlDB.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if s.timelineDB != nil && s.timelineDB != s.db && s.timelineDB != s.identityDB && s.timelineDB != s.mediaDB {
		sqlDB, err := s.timelineDB.DB()
		if err != nil {
			if closeErr == nil {
				closeErr = err
			}
		} else if err := sqlDB.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	if s.telemetryShutdown != nil {
		if err := telemetry.Shutdown(context.Background(), s.telemetryShutdown); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

// RoomManager returns the shared in-memory manager used by transport handlers.
func (s *Server) RoomManager() *room.Manager {
	return s.roomManager
}

func (s *Server) RedisClient() *cache.RedisClient {
	return s.redis
}
