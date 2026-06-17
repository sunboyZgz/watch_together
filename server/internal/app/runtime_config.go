package app

import (
	"time"

	wtconfig "watch_together/server/internal/config"
)

// ConfigFromRuntime maps env/file runtime config into the app server assembly
// config used by the roomserver and apigateway binaries.
func ConfigFromRuntime(runtimeConfig wtconfig.ServerRuntimeConfig) Config {
	return Config{
		AppEnv:              runtimeConfig.AppEnv,
		Host:                runtimeConfig.Host,
		Port:                runtimeConfig.Port,
		LogLevel:            runtimeConfig.LogLevel,
		InstanceID:          runtimeConfig.InstanceID,
		EdgeMode:            runtimeConfig.EdgeMode,
		RoomRuntimeMode:     runtimeConfig.RoomRuntimeMode,
		DatabaseURL:         runtimeConfig.DatabaseURL,
		IdentityDatabaseURL: runtimeConfig.IdentityDatabaseURL,
		RoomDatabaseURL:     runtimeConfig.RoomDatabaseURL,
		MediaDatabaseURL:    runtimeConfig.MediaDatabaseURL,
		ProgressDatabaseURL: runtimeConfig.ProgressDatabaseURL,
		TimelineDatabaseURL: runtimeConfig.TimelineDatabaseURL,
		DebugSync:           runtimeConfig.DebugSync,
		Auth: AuthTokenConfig{
			JWTSecret:      runtimeConfig.Auth.JWTSecret,
			AccessTokenTTL: time.Duration(runtimeConfig.Auth.AccessTokenTTLHours) * time.Hour,
		},
		Redis: RedisConfig{
			Addr:       runtimeConfig.Redis.Addr,
			Username:   runtimeConfig.Redis.Username,
			Password:   runtimeConfig.Redis.Password,
			DB:         runtimeConfig.Redis.DB,
			TLSEnabled: runtimeConfig.Redis.TLSEnabled,
			Required:   runtimeConfig.Redis.Required,
		},
		WebSocket: WebSocketRuntimeConfig{
			BroadcastConcurrencyLimit: runtimeConfig.WebSocket.BroadcastConcurrencyLimit,
			BroadcastTimeout:          time.Duration(runtimeConfig.WebSocket.BroadcastTimeoutMs) * time.Millisecond,
			BroadcastEnqueueTimeout:   time.Duration(runtimeConfig.WebSocket.BroadcastEnqueueTimeoutMs) * time.Millisecond,
			ClientOutboxCapacity:      runtimeConfig.WebSocket.ClientOutboxCapacity,
			MaxConnections:            runtimeConfig.WebSocket.MaxConnections,
			MaxRoomClients:            runtimeConfig.WebSocket.MaxRoomClients,
			SeekMinInterval:           time.Duration(runtimeConfig.WebSocket.SeekMinIntervalMs) * time.Millisecond,
			ControlIdempotencyTTL:     time.Duration(runtimeConfig.WebSocket.ControlIdempotencyTTLms) * time.Millisecond,
			PresenceLeaseTTL:          time.Duration(runtimeConfig.WebSocket.PresenceLeaseTTLms) * time.Millisecond,
			PresenceRefreshInterval:   time.Duration(runtimeConfig.WebSocket.PresenceRefreshIntervalMs) * time.Millisecond,
			ControlRequestTimeout:     time.Duration(runtimeConfig.WebSocket.ControlRequestTimeoutMs) * time.Millisecond,
			DrainGrace:                time.Duration(runtimeConfig.WebSocket.DrainGraceMs) * time.Millisecond,
			CrossInstanceBroadcast:    runtimeConfig.WebSocket.CrossInstanceBroadcast,
			EventBus:                  runtimeConfig.WebSocket.EventBus,
		},
		NATS: NATSConfig{
			URL:            runtimeConfig.NATS.URL,
			Name:           runtimeConfig.NATS.Name,
			Subject:        runtimeConfig.NATS.SubjectRoomBroadcast,
			ControlSubject: runtimeConfig.NATS.SubjectRoomControl,
		},
		Kafka: KafkaConfig{
			Brokers:                runtimeConfig.Kafka.Brokers,
			ClientID:               runtimeConfig.Kafka.ClientID,
			TopicRoomTimeline:      runtimeConfig.Kafka.TopicRoomTimeline,
			TopicRoomControlResult: runtimeConfig.Kafka.TopicRoomControlResult,
			TopicRoomMembership:    runtimeConfig.Kafka.TopicRoomMembership,
			DerivedConsumerGroupID: runtimeConfig.Kafka.DerivedConsumerGroupID,
		},
		AuthorityRecovery: AuthorityRecoveryConfig{
			RenewInterval:        time.Duration(runtimeConfig.AuthorityRecovery.RenewIntervalMs) * time.Millisecond,
			TakeoverScanInterval: time.Duration(runtimeConfig.AuthorityRecovery.TakeoverScanIntervalMs) * time.Millisecond,
			RecoveryTimeout:      time.Duration(runtimeConfig.AuthorityRecovery.RecoveryTimeoutMs) * time.Millisecond,
			KafkaReplayTimeout:   time.Duration(runtimeConfig.AuthorityRecovery.KafkaReplayTimeoutMs) * time.Millisecond,
		},
		Observability: ObservabilityConfig{
			MetricsEnabled: runtimeConfig.Observability.MetricsEnabled,
			MetricsAddr:    runtimeConfig.Observability.MetricsAddr,
			MetricsPath:    runtimeConfig.Observability.MetricsPath,
			ReadinessPath:  runtimeConfig.Observability.ReadinessPath,
		},
		Service: ServiceConfig{
			ServiceName:    runtimeConfig.Service.Name,
			ServiceVersion: runtimeConfig.Service.Version,
			InstanceID:     runtimeConfig.InstanceID,
		},
		InternalRPC: InternalRPCConfig{
			Enabled:    runtimeConfig.InternalRPC.Enabled,
			Addr:       runtimeConfig.InternalRPC.Addr,
			PathPrefix: runtimeConfig.InternalRPC.PathPrefix,
			Timeout:    time.Duration(runtimeConfig.InternalRPC.TimeoutMs) * time.Millisecond,
			AuthToken:  runtimeConfig.InternalRPC.AuthToken,
		},
		ServiceClients: ServiceClientsConfig{
			DiscoveryMode:    runtimeConfig.ServiceClients.DiscoveryMode,
			IdentityMode:     runtimeConfig.ServiceClients.IdentityMode,
			IdentityAddr:     runtimeConfig.ServiceClients.IdentityAddr,
			RoomMode:         runtimeConfig.ServiceClients.RoomMode,
			RoomAddr:         runtimeConfig.ServiceClients.RoomAddr,
			MediaMode:        runtimeConfig.ServiceClients.MediaMode,
			MediaAddr:        runtimeConfig.ServiceClients.MediaAddr,
			ProgressMode:     runtimeConfig.ServiceClients.ProgressMode,
			ProgressAddr:     runtimeConfig.ServiceClients.ProgressAddr,
			HomeMode:         runtimeConfig.ServiceClients.HomeMode,
			HomeAddr:         runtimeConfig.ServiceClients.HomeAddr,
			TimelineMode:     runtimeConfig.ServiceClients.TimelineMode,
			TimelineAddr:     runtimeConfig.ServiceClients.TimelineAddr,
			AuthorityMode:    runtimeConfig.ServiceClients.AuthorityMode,
			AuthorityAddr:    runtimeConfig.ServiceClients.AuthorityAddr,
			AuthorityLeaseID: runtimeConfig.ServiceClients.AuthorityLeaseID,
		},
		Telemetry: TelemetryConfig{
			Enabled:      runtimeConfig.Telemetry.TracingEnabled,
			ServiceName:  runtimeConfig.Telemetry.ServiceName,
			OTLPEndpoint: runtimeConfig.Telemetry.OTLPEndpoint,
			SampleRatio:  runtimeConfig.Telemetry.TraceSampleRatio,
		},
		Media: MediaPlaybackConfig{
			DeliveryMode:           runtimeConfig.Media.DeliveryMode,
			SigningSecret:          runtimeConfig.Media.SigningSecret,
			URLTTL:                 time.Duration(runtimeConfig.Media.URLTTLSeconds) * time.Second,
			PublicBaseURL:          runtimeConfig.Media.PublicBaseURL,
			InternalBaseURL:        runtimeConfig.Media.InternalBaseURL,
			StorageEndpoint:        runtimeConfig.Media.StorageEndpoint,
			StorageBucket:          runtimeConfig.Media.StorageBucket,
			StorageRegion:          runtimeConfig.Media.StorageRegion,
			StorageAccessKeyID:     runtimeConfig.Media.StorageAccessKeyID,
			StorageSecretAccessKey: runtimeConfig.Media.StorageSecretAccessKey,
			StorageForcePathStyle:  runtimeConfig.Media.StorageForcePathStyle,
		},
	}
}
