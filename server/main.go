// Command server relays a live camera feed from the Raspberry Pi to logged-in web viewers.
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed all:web/dist
var webFS embed.FS

type Config struct {
	Addr       string
	DataDir    string
	AdminUser  string
	AdminHash  string
	JWTSecret  string
	DeviceKey  string
	Secure     bool // Secure cookie flag; false only for local http development
	TrustProxy bool
	FPS        int // frame rate the Pi uploads at (used to encode clips)
	// Retention: 0 disables the limit.
	RetentionDays int
	MaxMediaBytes int64
	PreFrames     int
	PostFrames    int
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}

// envIntZero is like envInt but allows 0 (used to switch a limit off).
func envIntZero(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v >= 0 {
		return v
	}
	return def
}

func LoadConfig() (Config, error) {
	fps := envInt("STREAM_FPS", 8)
	c := Config{
		Addr:      env("ADDR", ":8080"),
		DataDir:   env("DATA_DIR", "./data"),
		AdminUser: os.Getenv("ADMIN_USER"),
		// Tolerate quotes: `docker run --env-file` keeps the single quotes that protect '$' in .env files.
		AdminHash:  strings.Trim(os.Getenv("ADMIN_PASSWORD_HASH"), `'"`),
		JWTSecret:  os.Getenv("JWT_SECRET"),
		DeviceKey:  os.Getenv("DEVICE_KEY"),
		Secure:     env("COOKIE_SECURE", "true") != "false",
		TrustProxy: env("TRUST_PROXY", "") == "1",
		FPS:        fps,
		PreFrames:  fps * envInt("CLIP_PRE_SECONDS", 3),
		PostFrames: fps * envInt("CLIP_POST_SECONDS", 7),

		RetentionDays: envIntZero("RETENTION_DAYS", 14),
		MaxMediaBytes: int64(envIntZero("MAX_MEDIA_MB", 2000)) << 20,
	}
	var missing []string
	for k, v := range map[string]string{"ADMIN_USER": c.AdminUser, "ADMIN_PASSWORD_HASH": c.AdminHash,
		"JWT_SECRET": c.JWTSecret, "DEVICE_KEY": c.DeviceKey} {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("missing required env vars: %s (see .env.example)", strings.Join(missing, ", "))
	}
	if len(c.JWTSecret) < 32 {
		return c, errors.New("JWT_SECRET must be at least 32 characters")
	}
	if len(c.DeviceKey) < 16 {
		return c, errors.New("DEVICE_KEY must be at least 16 characters")
	}
	return c, nil
}

type Server struct {
	cfg      Config
	hub      *Hub
	broker   *Broker
	store    *Store
	auth     *Auth
	mediaDir string
	ffmpeg   string

	ingestMu     sync.Mutex
	ingestCancel context.CancelFunc
	ingestGen    int
}

func NewServer(cfg Config) (*Server, error) {
	mediaDir := filepath.Join(cfg.DataDir, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return nil, err
	}
	store, err := OpenStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	ff, _ := exec.LookPath("ffmpeg")
	if ff == "" {
		log.Printf("ffmpeg not found: alert clips are disabled (snapshots still work)")
	}
	return &Server{
		cfg:      cfg,
		hub:      NewHub(cfg.PreFrames),
		broker:   NewBroker(),
		store:    store,
		auth:     NewAuth(cfg.AdminUser, cfg.AdminHash, []byte(cfg.JWTSecret), cfg.Secure, cfg.TrustProxy),
		mediaDir: mediaDir,
		ffmpeg:   ff,
	}, nil
}

func (s *Server) statusView() StatusView { return s.hub.View(s.store.LastAlertTime()) }

func (s *Server) publishStatus() {
	s.broker.Publish(map[string]any{"type": "status", "status": s.statusView()})
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 200 {
		limit = n
	}
	alerts, err := s.store.List(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
		return
	}
	writeJSON(w, http.StatusOK, alerts)
}

// noListing hides directory listings for the media file server.
func noListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// spa serves the embedded React build, falling back to index.html for client routes.
func spa() http.Handler {
	sub, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			r.URL.Path = "/" // client-side route
		}
		files.ServeHTTP(w, r)
	})
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; media-src 'self'; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.auth.Login)
	mux.HandleFunc("POST /api/logout", s.auth.Logout)
	mux.Handle("GET /api/me", s.auth.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"user": s.auth.User(r)})
	})))
	mux.Handle("GET /api/status", s.auth.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.statusView())
	})))
	mux.Handle("GET /api/alerts", s.auth.Require(http.HandlerFunc(s.handleAlerts)))
	mux.Handle("GET /api/events", s.auth.Require(http.HandlerFunc(s.handleEvents)))
	mux.Handle("GET /stream.mjpg", s.auth.Require(http.HandlerFunc(s.handleStream)))
	mux.Handle("GET /media/", s.auth.Require(http.StripPrefix("/media/", noListing(http.FileServer(http.Dir(s.mediaDir))))))
	mux.HandleFunc("GET /ingest", s.handleIngest)
	mux.Handle("/", spa())
	return secureHeaders(mux)
}

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	s, err := NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer s.store.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Periodic status for the dashboard (viewer count, uptime of the Pi, etc.).
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.publishStatus()
			}
		}
	}()

	// Delete old alerts and enforce the storage cap: at startup, then hourly.
	go func() {
		s.prune(time.Now())
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				s.prune(now)
			}
		}
	}()

	srv := &http.Server{Addr: cfg.Addr, Handler: s.Routes(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("listening on %s", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
