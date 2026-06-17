package main

import (
	"context"
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
	"watch_together/server/internal/roomapi"
	"watch_together/server/internal/servicekit"
	"watch_together/server/internal/telemetry"
	"watch_together/server/internal/timeline"
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
	if strings.TrimSpace(runtimeConfig.Redis.Addr) == "" {
		fmt.Fprintln(os.Stderr, "REDIS_ADDR is required for roomauthorityservice")
		os.Exit(1)
	}
	if strings.TrimSpace(runtimeConfig.ServiceClients.RoomAddr) == "" {
		fmt.Fprintln(os.Stderr, "ROOM_SERVICE_ADDR is required for roomauthorityservice")
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
	roomClient := roomapi.NewRPCClient(
		runtimeConfig.ServiceClients.RoomAddr,
		internalRPCClientConfig(runtimeConfig, serviceConfig),
	)
	if roomClient == nil {
		fmt.Fprintln(os.Stderr, "room rpc client is unavailable")
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

	recoveryService := recovery.NewService(
		recovery.Config{
			InstanceID:      runtimeConfig.InstanceID,
			RecoveryTimeout: time.Duration(runtimeConfig.AuthorityRecovery.RecoveryTimeoutMs) * time.Millisecond,
		},
		authorityRegistry,
		roomManager,
		roomClient,
		timelineClient,
		roomStateCache,
	)
	go startAuthorityRenewLoop(ctx, time.Duration(runtimeConfig.AuthorityRecovery.RenewIntervalMs)*time.Millisecond, roomManager, authorityRegistry, runtimeConfig.InstanceID)
	go recoveryService.RunScanner(ctx, time.Duration(runtimeConfig.AuthorityRecovery.TakeoverScanIntervalMs)*time.Millisecond)
	internalRPCTimeout := time.Duration(runtimeConfig.InternalRPC.TimeoutMs) * time.Millisecond
	engine := authority.NewEngine(
		authority.EngineConfig{
			InstanceID:      runtimeConfig.InstanceID,
			SeekMinInterval: time.Duration(runtimeConfig.WebSocket.SeekMinIntervalMs) * time.Millisecond,
			RecordTimeout:   internalRPCTimeout,
			PublishTimeout:  internalRPCTimeout,
			DebugSync:       runtimeConfig.DebugSync,
		},
		roomManager,
		authorityRegistry,
		activeDeviceRegistry,
		controlRequestRegistry,
		controlRateRegistry,
		roomClient,
		timelineClient,
		roomStateCache,
		broadcastBus,
		recoveryService,
	)

	metrics := observability.NewMetrics()
	mux := http.NewServeMux()
	installServiceEndpoints(mux, runtimeConfig, serviceConfig, metrics, redisClient, broadcastBus)
	authority.RegisterInternalRPCWithObserver(
		mux,
		runtimeConfig.InternalRPC.PathPrefix,
		runtimeConfig.InternalRPC.AuthToken,
		authorityRegistry,
		engine,
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

func installServiceEndpoints(
	mux *http.ServeMux,
	config wtconfig.ServerRuntimeConfig,
	service servicekit.Config,
	metrics *observability.Metrics,
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
		readiness := roomauthorityReadiness(r.Context(), config, redisClient, natsBus)
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
	redisClient redisPinger,
	natsBus natsStatus,
) observability.ReadinessSnapshot {
	return observability.NewReadinessSnapshot(
		config.AppEnv,
		config.InstanceID,
		"roomauthorityservice",
		[]observability.DependencyStatus{
			redisDependency(ctx, redisClient),
			natsDependency(natsBus),
			roomRPCDependency(ctx, config),
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

func roomRPCDependency(ctx context.Context, config wtconfig.ServerRuntimeConfig) observability.DependencyStatus {
	return rpcReadinessDependency(ctx, "room_rpc", config.ServiceClients.RoomAddr, config)
}

func timelineRPCDependency(ctx context.Context, config wtconfig.ServerRuntimeConfig) observability.DependencyStatus {
	return rpcReadinessDependency(ctx, "timeline_rpc", config.ServiceClients.TimelineAddr, config)
}

func rpcReadinessDependency(ctx context.Context, name string, addr string, config wtconfig.ServerRuntimeConfig) observability.DependencyStatus {
	status := "unavailable"
	baseURL := internalrpc.NormalizeBaseURL(addr)
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
	return observability.DependencyStatus{Name: name, Status: status, Required: true}
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
