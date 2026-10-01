package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/pagefire/pagefire/internal/auth"
	"github.com/pagefire/pagefire/internal/homealerts"
	"github.com/pagefire/pagefire/internal/store"
)

func NewRouter(s store.Store, authSvc *auth.Service, home *homealerts.Service, frontendFS ...fs.FS) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(SecurityHeaders)
	r.Use(requestLogger)
	r.Use(chimw.Recoverer)

	// Session middleware (loads/saves session data on every request). Secure
	// cookies are retained for HTTPS (including TLS terminated by a trusted
	// reverse proxy), while plain HTTP LAN installs can still establish sessions.
	r.Use(func(next http.Handler) http.Handler {
		return sessionCookieSecurity(authSvc.SessionManager().LoadAndSave(next))
	})

	// Health check (no auth) — includes DB connectivity
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "degraded",
				"db":     err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"db":     "ok",
		})
	})

	// Auth endpoints (single mount — public + protected routes handled internally)
	authHandler := NewAuthHandler(authSvc, s.Users())
	authMiddleware := SessionAuth(authSvc)
	r.Mount("/api/v1/auth", authHandler.Routes(authMiddleware))

	integrationLimiter := NewRateLimiter(60, time.Minute)
	h := NewHomeAlertHandler(home)
	r.With(RateLimitMiddleware(integrationLimiter), EventIngestionAuth(authSvc, home, s.Users())).Post("/api/v1/events", h.CreateEvent)

	// Authenticated API routes
	apiLimiter := NewRateLimiter(1000, time.Minute)
	r.Group(func(r chi.Router) {
		r.Use(RateLimitMiddleware(apiLimiter))
		r.Use(authMiddleware)

		r.Route("/api/v1", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(RequireAdminForWrites)
				r.Mount("/users", NewUserHandler(s.Users()).Routes())
			})

			r.Get("/events", h.ListEvents)
			r.Get("/alert-events", h.Feed)
			r.Get("/events/{eventID}", h.GetEvent)
			r.Get("/home-alerts/active", h.ListActiveAlerts)
			r.Get("/home-alerts/{alertID}/events", h.ListAlertEvents)
			r.Get("/home-alerts/{alertID}", h.GetActiveAlert)
			r.Post("/home-alerts/{alertID}/close", h.ForceCloseAlert)
			r.Get("/home-stats", h.Stats)
			r.Group(func(r chi.Router) {
				r.Use(RequireAdminForWrites)
				r.Get("/event-ingestion-key", h.GetIngestionKey)
				r.Post("/event-ingestion-key", h.RotateIngestionKey)
				r.Delete("/event-ingestion-key", h.RevokeIngestionKey)
				r.Post("/event-ingestion-key/secret", h.RememberIngestionKey)
				r.With(RequireRole(store.RoleAdmin)).Get("/event-ingestion-key/secret", h.RevealIngestionKey)
				r.Get("/home-plugins", h.Plugins)
				r.Put("/home-plugins/{kind}", h.PutPlugin)
				r.Delete("/home-plugins/{kind}", h.DeletePlugin)
				r.Post("/home-plugins/{kind}/test", h.TestPlugin)
			})
		})
	})

	// Serve embedded frontend SPA (catch-all, must come after API routes)
	if len(frontendFS) > 0 && frontendFS[0] != nil {
		frontendHandler := spaHandler(frontendFS[0])
		r.NotFound(frontendHandler)
	}

	return r
}

// spaHandler serves static files from the embedded filesystem. If the requested
// file doesn't exist, it falls back to index.html for client-side routing.
func spaHandler(assets fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(assets))

	// Pre-read index.html for SPA fallback (avoids redirect loop)
	indexHTML, _ := fs.ReadFile(assets, "index.html")

	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// Relax CSP for frontend pages (override the strict API CSP)
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")

		// Try to serve the exact file (static assets like JS, CSS)
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if f, err := assets.Open(path); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// File not found or root — serve index.html for SPA client-side routing
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	}
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration", time.Since(start),
		)
	})
}
