package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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
	Auth        auth.TokenConfig
	Redis       cache.RedisConfig
	WebSocket   transport.WebSocketRuntimeConfig
	Media       transport.MediaPlaybackConfig
}

type RedisConfig = cache.RedisConfig
type WebSocketRuntimeConfig = transport.WebSocketRuntimeConfig
type MediaPlaybackConfig = transport.MediaPlaybackConfig
type AuthTokenConfig = auth.TokenConfig

type Server struct {
	config      Config
	httpServer  *http.Server
	roomManager *room.Manager
	redis       *cache.RedisClient
	db          *gorm.DB
	cancel      context.CancelFunc
}

// NewServer assembles the in-memory room manager and the HTTP routes around it.
func NewServer(config Config) *Server {
	serverCtx, cancel := context.WithCancel(context.Background())
	roomManager := room.NewManager()
	tokenManager := auth.NewTokenManager(config.Auth)
	redisClient := newRedisClient("room_state", cache.RoomStateRedisConfig(config.Redis))
	var roomStateCache *cache.RoomStateCache
	if redisClient != nil {
		roomStateCache = cache.NewRoomStateCache(redisClient, 0)
	}
	db := newPostgresDB(config.DatabaseURL)
	roomService, roomStore := newRoomService(db)
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
	roomHTTPHandler := transport.NewRoomHTTPHandlerWithTokenVerifier(roomManager, roomService, tokenManager, config.Media)
	authHTTPHandler := transport.NewAuthHTTPHandler(newAuthService(db, tokenManager))
	homeHTTPHandler := transport.NewHomeHTTPHandlerWithTokenVerifier(newHomeService(db), tokenManager)
	mediaHTTPHandler := transport.NewMediaHTTPHandlerWithTokenVerifier(newMediaService(db), tokenManager, config.Media)
	progressHTTPHandler := transport.NewProgressHTTPHandlerWithTokenVerifier(newProgressService(db), tokenManager)
	router := newGinRouter(
		roomManager,
		config.DebugSync,
		config.WebSocket,
		roomStateCache,
		tokenManager,
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
		db:          db,
		cancel:      cancel,
	}
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

func newPostgresDB(databaseURL string) *gorm.DB {
	if strings.TrimSpace(databaseURL) == "" {
		log.Print("DATABASE_URL is not set; database-backed endpoints will return service unavailable")
		return nil
	}
	db, err := store.OpenPostgres(context.Background(), databaseURL)
	if err != nil {
		log.Printf("failed to connect database; database-backed endpoints unavailable: %v", err)
		return nil
	}
	return db
}

func newGinRouter(
	roomManager *room.Manager,
	debugSync bool,
	webSocketConfig transport.WebSocketRuntimeConfig,
	roomStateCache *cache.RoomStateCache,
	tokenManager *auth.TokenManager,
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
	router.Any("/media/internal/auth", gin.WrapF(mediaHTTPHandler.NginxAuth))
	router.Any("/media/playback/*playbackPath", gin.WrapF(mediaHTTPHandler.Playback))
	router.Any("/me/media-progress/*mediaPath", gin.WrapF(progressHTTPHandler.Update))
	router.Any("/rooms", gin.WrapF(roomHTTPHandler.CreateRoom))
	router.Any("/rooms/*roomPath", gin.WrapF(roomHTTPHandler.RoomRoute))
	router.Any("/ws", gin.WrapH(transport.NewWebSocketHandlerWithConfigAndRoomStateWriterAndTokenVerifierAndRoomLeaver(
		roomManager,
		debugSync,
		webSocketConfig,
		roomStateCache,
		tokenManager,
		roomHTTPHandler.RoomLeaver(),
	)))

	return router
}

// newAuthService connects the auth API to the shared PostgreSQL handle when available.
func newAuthService(db *gorm.DB, tokenManager *auth.TokenManager) *auth.Service {
	if db == nil {
		return nil
	}
	return auth.NewServiceWithTokenManager(store.NewPostgresUserStore(db), tokenManager)
}

// newHomeService connects home summary reads to the shared PostgreSQL handle when available.
func newHomeService(db *gorm.DB) *home.Service {
	if db == nil {
		return nil
	}
	return home.NewService(store.NewPostgresHomeStore(db))
}

// newMediaService connects media catalog APIs to the shared PostgreSQL handle when available.
func newMediaService(db *gorm.DB) *media.Service {
	if db == nil {
		return nil
	}
	return media.NewService(store.NewPostgresMediaStore(db))
}

// newRoomService connects room business APIs to the shared PostgreSQL handle when available.
func newRoomService(db *gorm.DB) (*roomapi.Service, *store.PostgresRoomStore) {
	if db == nil {
		return nil, nil
	}
	roomStore := store.NewPostgresRoomStore(db)
	return roomapi.NewService(roomStore), roomStore
}

// newProgressService connects media progress writes to the shared PostgreSQL handle when available.
func newProgressService(db *gorm.DB) *progress.Service {
	if db == nil {
		return nil
	}
	return progress.NewService(store.NewPostgresProgressStore(db))
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
	return closeErr
}

// RoomManager returns the shared in-memory manager used by transport handlers.
func (s *Server) RoomManager() *room.Manager {
	return s.roomManager
}

func (s *Server) RedisClient() *cache.RedisClient {
	return s.redis
}
