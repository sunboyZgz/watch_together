package app

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"watch_together/server/internal/room"
	"watch_together/server/internal/transport"
)

type Config struct {
	AppEnv    string
	Host      string
	Port      string
	LogLevel  string
	DebugSync bool
}

// LoadConfigFromEnv reads the minimum runtime config needed by the room server.
func LoadConfigFromEnv() Config {
	return Config{
		AppEnv:    envOrDefault("APP_ENV", "local"),
		Host:      envOrDefault("SERVER_HOST", "0.0.0.0"),
		Port:      envOrDefault("SERVER_PORT", "8080"),
		LogLevel:  envOrDefault("LOG_LEVEL", "debug"),
		DebugSync: strings.EqualFold(envOrDefault("DEBUG_SYNC", "true"), "true"),
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
	mux := http.NewServeMux()
	roomHTTPHandler := transport.NewRoomHTTPHandler(roomManager)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/rooms", roomHTTPHandler.CreateRoom)
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
