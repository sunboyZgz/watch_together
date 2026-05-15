package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"watch_together/server/internal/auth"
	"watch_together/server/internal/cache"
	"watch_together/server/internal/home"
	"watch_together/server/internal/media"
	"watch_together/server/internal/progress"
	"watch_together/server/internal/room"
	"watch_together/server/internal/roomapi"
	"watch_together/server/internal/store"
	"watch_together/server/internal/transport"
)

type Config struct {
	AppEnv      string
	Host        string
	Port        string
	LogLevel    string
	DatabaseURL string
	DebugSync   bool
	Redis       cache.RedisConfig
	WebSocket   transport.WebSocketRuntimeConfig
}

type RedisConfig = cache.RedisConfig
type WebSocketRuntimeConfig = transport.WebSocketRuntimeConfig

type Server struct {
	config      Config
	httpServer  *http.Server
	roomManager *room.Manager
	redis       *cache.RedisClient
}

// NewServer assembles the in-memory room manager and the HTTP routes around it.
func NewServer(config Config) *Server {
	roomManager := room.NewManager()
	redisClient := newRedisClient(config.Redis)
	roomService, roomStore := newRoomService(config.DatabaseURL)
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
		roomManager.SetLifecycleHooks(room.LifecycleHooks{
			OnRoomBecameEmpty: func(roomID string, emptySince time.Time, destroyAfter time.Time) {
				if err := roomStore.MarkRoomGracePeriod(context.Background(), roomID, emptySince, destroyAfter); err != nil {
					log.Printf("failed to mark room %s grace_period: %v", roomID, err)
				}
			},
			OnRoomReactivated: func(roomID string) {
				if err := roomStore.MarkRoomActive(context.Background(), roomID); err != nil {
					log.Printf("failed to reactivate room %s: %v", roomID, err)
				}
			},
			OnRoomDestroyed: func(roomID string) {
				if err := roomStore.DestroyRoom(context.Background(), roomID); err != nil {
					log.Printf("failed to destroy room %s: %v", roomID, err)
				}
			},
		})
	}
	go roomManager.StartCleanupLoop(context.Background(), room.DefaultCleanupInterval())
	if roomStore != nil {
		go startPersistentRoomCleanupLoop(context.Background(), room.DefaultCleanupInterval(), roomStore)
	}
	roomHTTPHandler := transport.NewRoomHTTPHandler(roomManager, roomService)
	authHTTPHandler := transport.NewAuthHTTPHandler(newAuthService(config.DatabaseURL))
	homeHTTPHandler := transport.NewHomeHTTPHandler(newHomeService(config.DatabaseURL))
	mediaHTTPHandler := transport.NewMediaHTTPHandler(newMediaService(config.DatabaseURL))
	progressHTTPHandler := transport.NewProgressHTTPHandler(newProgressService(config.DatabaseURL))
	router := newGinRouter(
		roomManager,
		config.DebugSync,
		config.WebSocket,
		roomHTTPHandler,
		authHTTPHandler,
		homeHTTPHandler,
		mediaHTTPHandler,
		progressHTTPHandler,
	)

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", config.Host, config.Port),
		Handler: router,
	}

	return &Server{
		config:      config,
		httpServer:  httpServer,
		roomManager: roomManager,
		redis:       redisClient,
	}
}

func newRedisClient(config cache.RedisConfig) *cache.RedisClient {
	if !config.Enabled() {
		log.Print("REDIS_ADDR is not set; redis-backed caches and runtime metadata are disabled")
		return nil
	}

	client, err := cache.OpenRedis(context.Background(), config)
	if err != nil {
		if config.Required {
			log.Fatalf("failed to connect required redis: %v", err)
		}
		log.Printf("failed to connect redis; redis-backed features disabled: %v", err)
		return nil
	}
	log.Printf("connected redis addr=%s db=%d tls=%t", config.Addr, config.DB, config.TLSEnabled)
	return client
}

func newGinRouter(
	roomManager *room.Manager,
	debugSync bool,
	webSocketConfig transport.WebSocketRuntimeConfig,
	roomHTTPHandler *transport.RoomHTTPHandler,
	authHTTPHandler *transport.AuthHTTPHandler,
	homeHTTPHandler *transport.HomeHTTPHandler,
	mediaHTTPHandler *transport.MediaHTTPHandler,
	progressHTTPHandler *transport.ProgressHTTPHandler,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	router.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	router.Any("/auth/login", gin.WrapF(authHTTPHandler.Login))
	router.Any("/auth/register", gin.WrapF(authHTTPHandler.Register))
	router.Any("/home/summary", gin.WrapF(homeHTTPHandler.Summary))
	router.Any("/media/tags", gin.WrapF(mediaHTTPHandler.Tags))
	router.Any("/media/items", gin.WrapF(mediaHTTPHandler.Items))
	router.Any("/me/media-progress/*mediaPath", gin.WrapF(progressHTTPHandler.Update))
	router.Any("/rooms", gin.WrapF(roomHTTPHandler.CreateRoom))
	router.Any("/rooms/*roomPath", gin.WrapF(roomHTTPHandler.RoomRoute))
	router.Any("/ws", gin.WrapH(transport.NewWebSocketHandlerWithConfig(roomManager, debugSync, webSocketConfig)))

	return router
}

// newAuthService connects the auth API to PostgreSQL when DATABASE_URL is available.
func newAuthService(databaseURL string) *auth.Service {
	if strings.TrimSpace(databaseURL) == "" {
		log.Print("DATABASE_URL is not set; auth endpoints will return service unavailable")
		return nil
	}
	db, err := store.OpenPostgres(context.Background(), databaseURL)
	if err != nil {
		log.Printf("failed to connect database; auth endpoints unavailable: %v", err)
		return nil
	}
	return auth.NewService(store.NewPostgresUserStore(db))
}

// newHomeService connects home summary reads to PostgreSQL when DATABASE_URL is available.
func newHomeService(databaseURL string) *home.Service {
	if strings.TrimSpace(databaseURL) == "" {
		log.Print("DATABASE_URL is not set; home endpoints will return service unavailable")
		return nil
	}
	db, err := store.OpenPostgres(context.Background(), databaseURL)
	if err != nil {
		log.Printf("failed to connect database; home endpoints unavailable: %v", err)
		return nil
	}
	return home.NewService(store.NewPostgresHomeStore(db))
}

// newMediaService connects media catalog APIs to PostgreSQL when DATABASE_URL is available.
func newMediaService(databaseURL string) *media.Service {
	if strings.TrimSpace(databaseURL) == "" {
		log.Print("DATABASE_URL is not set; media endpoints will return service unavailable")
		return nil
	}
	db, err := store.OpenPostgres(context.Background(), databaseURL)
	if err != nil {
		log.Printf("failed to connect database; media endpoints unavailable: %v", err)
		return nil
	}
	return media.NewService(store.NewPostgresMediaStore(db))
}

// newRoomService connects room business APIs to PostgreSQL when DATABASE_URL is available.
func newRoomService(databaseURL string) (*roomapi.Service, *store.PostgresRoomStore) {
	if strings.TrimSpace(databaseURL) == "" {
		log.Print("DATABASE_URL is not set; room endpoints will return service unavailable")
		return nil, nil
	}
	db, err := store.OpenPostgres(context.Background(), databaseURL)
	if err != nil {
		log.Printf("failed to connect database; room endpoints unavailable: %v", err)
		return nil, nil
	}
	roomStore := store.NewPostgresRoomStore(db)
	return roomapi.NewService(roomStore), roomStore
}

// newProgressService connects media progress writes to PostgreSQL when DATABASE_URL is available.
func newProgressService(databaseURL string) *progress.Service {
	if strings.TrimSpace(databaseURL) == "" {
		log.Print("DATABASE_URL is not set; progress endpoints will return service unavailable")
		return nil
	}
	db, err := store.OpenPostgres(context.Background(), databaseURL)
	if err != nil {
		log.Printf("failed to connect database; progress endpoints unavailable: %v", err)
		return nil
	}
	return progress.NewService(store.NewPostgresProgressStore(db))
}

func startPersistentRoomCleanupLoop(
	ctx context.Context,
	interval time.Duration,
	roomStore *store.PostgresRoomStore,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed, err := roomStore.CleanupExpiredRooms(context.Background(), time.Now())
			if err != nil {
				log.Printf("failed to cleanup expired rooms in postgres: %v", err)
				continue
			}
			if removed > 0 {
				log.Printf("cleaned up %d expired rooms from postgres", removed)
			}
		}
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

// RoomManager returns the shared in-memory manager used by transport handlers.
func (s *Server) RoomManager() *room.Manager {
	return s.roomManager
}

func (s *Server) RedisClient() *cache.RedisClient {
	return s.redis
}
