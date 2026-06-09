package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"watch_together/server/internal/authority"
	"watch_together/server/internal/cache"
	wtconfig "watch_together/server/internal/config"
	"watch_together/server/internal/eventbus"
	"watch_together/server/internal/internalrpc"
	"watch_together/server/internal/observability"
	"watch_together/server/internal/recovery"
	"watch_together/server/internal/room"
	"watch_together/server/internal/servicekit"
	"watch_together/server/internal/store"
	"watch_together/server/internal/telemetry"
	"watch_together/server/internal/timeline"
	"watch_together/server/internal/transport"
)

func main() {
	runtimeConfig, err := wtconfig.LoadServerRuntimeConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load server config: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(runtimeConfig.InstanceID) == "" {
		fmt.Fprintln(os.Stderr, "SERVER_INSTANCE_ID is required for roomauthorityservice")
		os.Exit(1)
	}
	if strings.TrimSpace(runtimeConfig.DatabaseURL) == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required for roomauthorityservice")
		os.Exit(1)
	}
	if strings.TrimSpace(runtimeConfig.Redis.Addr) == "" {
		fmt.Fprintln(os.Stderr, "REDIS_ADDR is required for roomauthorityservice")
		os.Exit(1)
	}
	if strings.TrimSpace(runtimeConfig.ServiceClients.TimelineAddr) == "" {
		fmt.Fprintln(os.Stderr, "TIMELINE_SERVICE_ADDR is required for roomauthorityservice")
		os.Exit(1)
	}
	if strings.EqualFold(strings.TrimSpace(runtimeConfig.AppEnv), "prod") &&
		strings.TrimSpace(runtimeConfig.InternalRPC.AuthToken) == "" {
		fmt.Fprintln(os.Stderr, "INTERNAL_RPC_AUTH_TOKEN is required for roomauthorityservice in prod")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serviceConfig := servicekit.Config{
		ServiceName:    serviceName(runtimeConfig.Service.Name, "watch-together-roomauthorityservice"),
		ServiceVersion: runtimeConfig.Service.Version,
		InstanceID:     runtimeConfig.InstanceID,
	}.Normalized("watch-together-roomauthorityservice")
	shutdownTelemetry, err := telemetry.Start(ctx, telemetry.Config{
		Enabled:      runtimeConfig.Telemetry.TracingEnabled,
		ServiceName:  runtimeConfig.Telemetry.ServiceName,
		OTLPEndpoint: runtimeConfig.Telemetry.OTLPEndpoint,
		SampleRatio:  runtimeConfig.Telemetry.TraceSampleRatio,
	}.Normalized(serviceConfig.ServiceName))
	if err != nil {
		log.Printf("failed to start telemetry; tracing disabled: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetry.Shutdown(shutdownCtx, shutdownTelemetry)
	}()

	db, err := store.OpenPostgres(ctx, runtimeConfig.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err == nil {
		defer sqlDB.Close()
	}
	roomStore := store.NewPostgresRoomStore(db)

	redisClient, err := cache.OpenRedis(ctx, cache.RedisConfig{
		Addr:       runtimeConfig.Redis.Addr,
		Username:   runtimeConfig.Redis.Username,
		Password:   runtimeConfig.Redis.Password,
		DB:         runtimeConfig.Redis.DB,
		TLSEnabled: runtimeConfig.Redis.TLSEnabled,
		Required:   true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect redis: %v\n", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	authorityRegistry := cache.NewRoomAuthorityRegistry(redisClient, 0)
	activeDeviceRegistry := cache.NewActiveDeviceRegistry(redisClient, 0)
	controlRequestRegistry := cache.NewControlRequestRegistry(
		redisClient,
		time.Duration(runtimeConfig.WebSocket.ControlIdempotencyTTLms)*time.Millisecond,
	)
	controlRateRegistry := cache.NewControlRateRegistry(redisClient)
	roomStateCache := cache.NewRoomStateCache(redisClient, 0)

	timelineClient := timeline.NewRPCClient(
		runtimeConfig.ServiceClients.TimelineAddr,
		internalRPCClientConfig(runtimeConfig, serviceConfig),
	)
	if timelineClient == nil {
		fmt.Fprintln(os.Stderr, "timeline rpc client is unavailable")
		os.Exit(1)
	}

	roomManager := room.NewManager()
	broadcastBus, err := eventbus.OpenNATSRoomBroadcastBus(eventbus.NATSConfig{
		URL:     runtimeConfig.NATS.URL,
		Name:    serviceConfig.ServiceName,
		Subject: runtimeConfig.NATS.SubjectRoomBroadcast,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect nats broadcast bus: %v\n", err)
		os.Exit(1)
	}
	defer broadcastBus.Close()

	handler := transport.NewWebSocketHandlerWithConfigAndRoomStateWriter(
		roomManager,
		runtimeConfig.DebugSync,
		transport.WebSocketRuntimeConfig{
			BroadcastConcurrencyLimit: runtimeConfig.WebSocket.BroadcastConcurrencyLimit,
			BroadcastTimeout:          time.Duration(runtimeConfig.WebSocket.BroadcastTimeoutMs) * time.Millisecond,
			BroadcastEnqueueTimeout:   time.Duration(runtimeConfig.WebSocket.BroadcastEnqueueTimeoutMs) * time.Millisecond,
			ClientOutboxCapacity:      runtimeConfig.WebSocket.ClientOutboxCapacity,
			SeekMinInterval:           time.Duration(runtimeConfig.WebSocket.SeekMinIntervalMs) * time.Millisecond,
			ControlIdempotencyTTL:     time.Duration(runtimeConfig.WebSocket.ControlIdempotencyTTLms) * time.Millisecond,
			PresenceLeaseTTL:          time.Duration(runtimeConfig.WebSocket.PresenceLeaseTTLms) * time.Millisecond,
			PresenceRefreshInterval:   time.Duration(runtimeConfig.WebSocket.PresenceRefreshIntervalMs) * time.Millisecond,
			CrossInstanceBroadcast:    true,
			EventBus:                  runtimeConfig.WebSocket.EventBus,
		},
		roomStateCache,
	)
	handler.SetRoomBroadcastBus(runtimeConfig.InstanceID, broadcastBus)
	handler.SetDistributedAuthorityRuntime(
		runtimeConfig.InstanceID,
		authorityRegistry,
		activeDeviceRegistry,
		eventbus.NewDisabledRoomControlBus(),
		timelineClient,
	)
	handler.SetDistributedControlHardening(controlRequestRegistry, nil, 0)
	handler.SetDistributedControlRateRegistry(controlRateRegistry)

	recoveryService := recovery.NewService(
		recovery.Config{
			InstanceID:      runtimeConfig.InstanceID,
			RecoveryTimeout: time.Duration(runtimeConfig.AuthorityRecovery.RecoveryTimeoutMs) * time.Millisecond,
		},
		authorityRegistry,
		roomManager,
		roomStore,
		timelineClient,
		roomStateCache,
	)
	handler.SetRoomAuthorityRecovery(recoveryService)
	go startAuthorityRenewLoop(ctx, time.Duration(runtimeConfig.AuthorityRecovery.RenewIntervalMs)*time.Millisecond, roomManager, authorityRegistry, runtimeConfig.InstanceID)
	go recoveryService.RunScanner(ctx, time.Duration(runtimeConfig.AuthorityRecovery.TakeoverScanIntervalMs)*time.Millisecond)

	metrics := observability.NewMetrics()
	mux := http.NewServeMux()
	installServiceEndpoints(mux, runtimeConfig, serviceConfig, metrics, sqlDB, redisClient, broadcastBus)
	authority.RegisterInternalRPCWithObserver(
		mux,
		runtimeConfig.InternalRPC.PathPrefix,
		runtimeConfig.InternalRPC.AuthToken,
		authorityRegistry,
		&loadingApplier{
			next:           handler,
			roomManager:    roomManager,
			roomStore:      roomStore,
			timelineReader: timelineClient,
			handler:        handler,
			instanceID:     runtimeConfig.InstanceID,
			authority:      authorityRegistry,
			recoverer:      recoveryService,
		},
		metrics,
	)

	addr := strings.TrimSpace(runtimeConfig.InternalRPC.Addr)
	if addr == "" {
		addr = ":8090"
	}
	server := &http.Server{Addr: addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("roomauthorityservice listening on %s", addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("roomauthorityservice graceful shutdown failed: %v", err)
		}
	}
}

type loadingApplier struct {
	next           authority.ControlApplier
	roomManager    *room.Manager
	roomStore      recovery.RoomDetailStore
	timelineReader timeline.RecoveryReader
	handler        *transport.WebSocketHandler
	instanceID     string
	authority      *cache.RoomAuthorityRegistry
	recoverer      *recovery.Service
}

func (a *loadingApplier) ApplyRoomControl(ctx context.Context, request authority.ApplyControlRequest) (authority.ApplyControlResponse, error) {
	if err := a.ensureAuthority(ctx, request.RoomID); err != nil {
		return authority.ApplyControlResponse{Error: authorityRecoveryMessage(err)}, nil
	}
	if _, ok := a.roomManager.Get(request.RoomID); !ok {
		if err := a.ensureRoomState(ctx, request.RoomID); err != nil {
			return authority.ApplyControlResponse{Error: "room authority unavailable"}, nil
		}
	}
	return a.next.ApplyRoomControl(ctx, request)
}

func (a *loadingApplier) ensureAuthority(ctx context.Context, roomID string) error {
	if a.authority == nil || a.recoverer == nil {
		return nil
	}
	lease, found, err := a.authority.GetAuthority(ctx, roomID)
	if err != nil {
		return err
	}
	if found && lease.IsActive() && !lease.ExpiredAt(time.Now()) {
		return nil
	}
	result, err := a.recoverer.TryRecoverRoomAuthority(ctx, roomID, "authority_rpc_control")
	if result.Recovered {
		a.handler.SeedRecoveredControlRequests(ctx, roomID, result.Lease.Epoch, result.Requests, result.RequestIDs)
	}
	if err != nil && !errors.Is(err, recovery.ErrAuthorityActive) {
		return err
	}
	return nil
}

func (a *loadingApplier) ensureRoomState(ctx context.Context, roomID string) error {
	detail, err := a.roomStore.GetRoomDetail(ctx, roomID)
	if err != nil {
		return err
	}
	base := recovery.BaseStateFromRoomDetail(detail, time.Now())
	events, err := a.timelineReader.ReadRoomRecoveryEvents(ctx, roomID)
	if err != nil {
		return err
	}
	state, requests, err := recovery.RecoverStateFromEvents(base, events)
	if err != nil {
		return err
	}
	a.roomManager.RegisterRecoveredRoom(state)
	lease, found, err := a.authority.GetAuthority(ctx, roomID)
	if err == nil && found {
		a.handler.SeedRecoveredControlRequests(ctx, roomID, lease.Epoch, requests, nil)
	}
	return nil
}

func authorityRecoveryMessage(err error) string {
	if errors.Is(err, recovery.ErrAuthorityRecovering) {
		return "room authority recovering"
	}
	return "room authority unavailable"
}

func installServiceEndpoints(
	mux *http.ServeMux,
	config wtconfig.ServerRuntimeConfig,
	service servicekit.Config,
	metrics *observability.Metrics,
	sqlDB *sql.DB,
	redisClient redisPinger,
	natsBus natsStatus,
) {
	metricsPath := strings.TrimSpace(config.Observability.MetricsPath)
	if metricsPath == "" {
		metricsPath = observability.DefaultMetricsPath
	}
	readinessPath := strings.TrimSpace(config.Observability.ReadinessPath)
	if readinessPath == "" {
		readinessPath = observability.DefaultReadinessPath
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Watch-Together-Service", service.ServiceName)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc(readinessPath, func(w http.ResponseWriter, r *http.Request) {
		readiness := roomauthorityReadiness(r.Context(), config, sqlDB, redisClient, natsBus)
		status := http.StatusOK
		if readiness.Status != "ready" {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(readiness)
	})
	if config.Observability.MetricsEnabled {
		mux.Handle(metricsPath, metrics.Handler())
	}
}

func roomauthorityReadiness(
	ctx context.Context,
	config wtconfig.ServerRuntimeConfig,
	sqlDB *sql.DB,
	redisClient redisPinger,
	natsBus natsStatus,
) observability.ReadinessSnapshot {
	return observability.NewReadinessSnapshot(
		config.AppEnv,
		config.InstanceID,
		"roomauthorityservice",
		[]observability.DependencyStatus{
			postgresDependency(ctx, sqlDB),
			redisDependency(ctx, redisClient),
			natsDependency(natsBus),
			timelineRPCDependency(ctx, config),
			{Name: "internal_rpc", Status: "ok", Required: true},
		},
	)
}

type redisPinger interface {
	Ping(ctx context.Context) error
}

type natsStatus interface {
	IsConnected() bool
}

func postgresDependency(ctx context.Context, sqlDB *sql.DB) observability.DependencyStatus {
	status := "unavailable"
	if sqlDB != nil {
		pingCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		if err := sqlDB.PingContext(pingCtx); err == nil {
			status = "ok"
		}
	}
	return observability.DependencyStatus{Name: "postgres", Status: status, Required: true}
}

func redisDependency(ctx context.Context, redisClient redisPinger) observability.DependencyStatus {
	status := "unavailable"
	if redisClient != nil {
		pingCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		if err := redisClient.Ping(pingCtx); err == nil {
			status = "ok"
		}
	}
	return observability.DependencyStatus{Name: "redis", Status: status, Required: true}
}

func natsDependency(natsBus natsStatus) observability.DependencyStatus {
	status := "unavailable"
	if natsBus != nil && natsBus.IsConnected() {
		status = "ok"
	}
	return observability.DependencyStatus{Name: "nats_broadcast", Status: status, Required: true}
}

func timelineRPCDependency(ctx context.Context, config wtconfig.ServerRuntimeConfig) observability.DependencyStatus {
	status := "unavailable"
	baseURL := internalrpc.NormalizeBaseURL(config.ServiceClients.TimelineAddr)
	if baseURL != "" {
		readinessPath := strings.TrimSpace(config.Observability.ReadinessPath)
		if readinessPath == "" {
			readinessPath = observability.DefaultReadinessPath
		}
		if !strings.HasPrefix(readinessPath, "/") {
			readinessPath = "/" + readinessPath
		}
		probeCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		request, err := http.NewRequestWithContext(
			probeCtx,
			http.MethodGet,
			strings.TrimRight(baseURL, "/")+readinessPath,
			nil,
		)
		if err == nil {
			response, err := http.DefaultClient.Do(request)
			if err == nil {
				defer response.Body.Close()
				if response.StatusCode >= 200 && response.StatusCode < 300 {
					var snapshot observability.ReadinessSnapshot
					if err := json.NewDecoder(response.Body).Decode(&snapshot); err == nil && snapshot.Status != "" {
						if snapshot.Status == "ready" {
							status = "ok"
						}
					} else {
						status = "ok"
					}
				}
			}
		}
	}
	return observability.DependencyStatus{Name: "timeline_rpc", Status: status, Required: true}
}

func internalRPCClientConfig(config wtconfig.ServerRuntimeConfig, service servicekit.Config) internalrpc.ClientConfig {
	return internalrpc.ClientConfig{
		PathPrefix: config.InternalRPC.PathPrefix,
		Timeout:    time.Duration(config.InternalRPC.TimeoutMs) * time.Millisecond,
		AuthToken:  config.InternalRPC.AuthToken,
		Service:    service,
	}
}

func serviceName(configured string, fallback string) string {
	if strings.TrimSpace(configured) != "" && configured != "watch-together-roomserver" {
		return configured
	}
	return fallback
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
