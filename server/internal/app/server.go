package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"watch_together/server/internal/auth"
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
}

// LoadConfigFromEnv reads the minimum runtime config needed by the room server.
func LoadConfigFromEnv() Config {
	return Config{
		AppEnv:      envOrDefault("APP_ENV", "local"),
		Host:        envOrDefault("SERVER_HOST", "0.0.0.0"),
		Port:        envOrDefault("SERVER_PORT", "8080"),
		LogLevel:    envOrDefault("LOG_LEVEL", "debug"),
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		DebugSync:   strings.EqualFold(envOrDefault("DEBUG_SYNC", "true"), "true"),
	}
}

// envOrDefault keeps startup simple by falling back to stable local defaults.
func envOrDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

type Server struct {
	config      Config
	httpServer  *http.Server
	roomManager *room.Manager
}

// NewServer assembles the in-memory room manager and the HTTP routes around it.
func NewServer(config Config) *Server {
	roomManager := room.NewManager()
	go roomManager.StartCleanupLoop(context.Background(), room.DefaultCleanupInterval())
	mux := http.NewServeMux()
	roomHTTPHandler := transport.NewRoomHTTPHandler(roomManager, newRoomService(config.DatabaseURL))
	authHTTPHandler := transport.NewAuthHTTPHandler(newAuthService(config.DatabaseURL))
	homeHTTPHandler := transport.NewHomeHTTPHandler(newHomeService(config.DatabaseURL))
	mediaHTTPHandler := transport.NewMediaHTTPHandler(newMediaService(config.DatabaseURL))
	progressHTTPHandler := transport.NewProgressHTTPHandler(newProgressService(config.DatabaseURL))

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/auth/login", authHTTPHandler.Login)
	mux.HandleFunc("/auth/register", authHTTPHandler.Register)
	mux.HandleFunc("/home/summary", homeHTTPHandler.Summary)
	mux.HandleFunc("/media/tags", mediaHTTPHandler.Tags)
	mux.HandleFunc("/media/items", mediaHTTPHandler.Items)
	mux.HandleFunc("/me/media-progress/", progressHTTPHandler.Update)
	mux.HandleFunc("/rooms", roomHTTPHandler.CreateRoom)
	mux.HandleFunc("/rooms/", roomHTTPHandler.RoomRoute)
	mux.Handle("/ws", transport.NewWebSocketHandler(roomManager, config.DebugSync))

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", config.Host, config.Port),
		Handler: mux,
	}

	return &Server{
		config:      config,
		httpServer:  httpServer,
		roomManager: roomManager,
	}
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
func newRoomService(databaseURL string) *roomapi.Service {
	if strings.TrimSpace(databaseURL) == "" {
		log.Print("DATABASE_URL is not set; room endpoints will return service unavailable")
		return nil
	}
	db, err := store.OpenPostgres(context.Background(), databaseURL)
	if err != nil {
		log.Printf("failed to connect database; room endpoints unavailable: %v", err)
		return nil
	}
	return roomapi.NewService(store.NewPostgresRoomStore(db))
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
