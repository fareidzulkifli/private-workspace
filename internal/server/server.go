package server

import (
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"private-workspace/internal/ai"
	"private-workspace/internal/auth"
	"private-workspace/internal/dashboard"
	"private-workspace/internal/db"
	"private-workspace/internal/gitnote"
	"private-workspace/internal/holidays"
	"private-workspace/internal/httputil"
	"private-workspace/internal/orgs"
	"private-workspace/internal/projects"
	"private-workspace/internal/prompts"
	"private-workspace/internal/security"
	"private-workspace/internal/share"
	"private-workspace/internal/tasks"
	"private-workspace/internal/uploads"
	"private-workspace/internal/wallet"
	"private-workspace/internal/web"
)

type R2Client interface {
	tasks.ObjectStore
	uploads.Presigner
}

type Config struct {
	DB           *db.DB
	Auth         *auth.Service
	Logger       *log.Logger
	Web          *web.Handler
	R2           R2Client
	GitNote      gitnote.Client
	Holidays     holidays.Client
	HolidayState string
	AI           ai.Client
}

func NewRouter(cfg Config) http.Handler {
	webHandler := cfg.Web
	if webHandler == nil {
		webHandler = web.New(web.Options{})
	}

	r := chi.NewRouter()
	r.Use(httputil.RequestIDMiddleware)
	r.Use(httputil.RealIP)
	r.Use(httputil.RequestLogger(cfg.Logger))
	r.Use(httputil.Recoverer(cfg.Logger))
	r.Use(httputil.BodyLimit(1 << 20))
	r.Use(security.Headers)
	if cfg.Auth != nil {
		r.Use(cfg.Auth.AttachSession)
		r.Use(protectRoutes(webHandler))
		r.Use(cfg.Auth.CSRFMiddleware)
	}

	r.Get("/api/healthz", healthz)
	r.Get("/api/readyz", readyz(cfg.DB))
	if cfg.Auth != nil {
		r.Post("/api/auth/login", cfg.Auth.Login)
		r.Post("/api/auth/logout", cfg.Auth.Logout)
		r.Get("/api/auth/session", cfg.Auth.Session)
	}
	if cfg.DB != nil {
		orgs.NewHandler(cfg.DB).RegisterRoutes(r)
		projects.NewHandler(cfg.DB).RegisterRoutes(r)
		tasks.NewHandler(cfg.DB, cfg.R2).RegisterRoutes(r)
		uploads.NewHandler(cfg.R2).RegisterRoutes(r)
		prompts.NewHandler(cfg.DB).RegisterRoutes(r)
		gitnote.NewHandler(cfg.GitNote).RegisterRoutes(r)
		holidays.NewHandler(cfg.Holidays, cfg.HolidayState).RegisterRoutes(r)
		ai.NewHandler(cfg.AI).RegisterRoutes(r)
		dashboard.NewHandler(cfg.DB, cfg.Holidays, cfg.HolidayState).RegisterRoutes(r)
		wallet.NewHandler(cfg.DB).RegisterRoutes(r)
		share.NewHandler(cfg.DB, cfg.GitNote).RegisterRoutes(r)
	}

	r.MethodNotAllowed(httputil.MethodNotAllowed)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			httputil.NotFound(w, r)
			return
		}
		webHandler.ServeHTTP(w, r)
	})

	return r
}

func healthz(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func readyz(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if database == nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "Not ready")
			return
		}
		if err := database.Ready(r.Context()); err != nil {
			httputil.WriteError(w, http.StatusServiceUnavailable, "Not ready")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func protectRoutes(webHandler *web.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isLoginPage(r) {
				if _, ok := auth.FromContext(r.Context()); ok {
					http.Redirect(w, r, "/dashboard", http.StatusFound)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if IsPublicRequest(r) || isPublicAssetRequest(r, webHandler) {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := auth.FromContext(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				httputil.WriteError(w, http.StatusUnauthorized, "Unauthenticated")
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
		})
	}
}

func IsPublicRequest(r *http.Request) bool {
	path := r.URL.Path
	switch {
	case r.Method == http.MethodGet && path == "/api/healthz":
		return true
	case r.Method == http.MethodGet && path == "/api/readyz":
		return true
	case r.Method == http.MethodGet && path == "/api/auth/session":
		return true
	case r.Method == http.MethodPost && path == "/api/auth/login":
		return true
	case r.Method == http.MethodGet && (path == "/api/share/gitnote" || strings.HasPrefix(path, "/api/share/gitnote/")):
		return true
	case isLoginPage(r):
		return true
	case isSharePage(r):
		return true
	default:
		return false
	}
}

func isPublicAssetRequest(r *http.Request, webHandler *web.Handler) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/api/") || web.IsClientRoute(r.URL.Path) || !web.IsConcreteAsset(r.URL.Path) {
		return false
	}
	return webHandler != nil
}

func isLoginPage(r *http.Request) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.URL.Path == "/login"
}

func isSharePage(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	return r.URL.Path == "/share" || strings.HasPrefix(r.URL.Path, "/share/")
}
