package auth

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"private-workspace/internal/httputil"
	"private-workspace/internal/security"
)

type contextKey string

const principalKey contextKey = "auth_principal"

type Principal struct {
	User    User
	Session Session
}

type Service struct {
	store          *Store
	cookieName     string
	cookieSecure   bool
	csrfHeaderName string
	limiter        *security.LoginLimiter
	logger         *log.Logger
}

type Options struct {
	CookieName     string
	CookieSecure   bool
	CSRFHeaderName string
	Limiter        *security.LoginLimiter
	Logger         *log.Logger
}

func NewService(store *Store, opts Options) *Service {
	cookieName := opts.CookieName
	if cookieName == "" {
		cookieName = "pw_session"
	}
	csrfHeader := opts.CSRFHeaderName
	if csrfHeader == "" {
		csrfHeader = "X-CSRF-Token"
	}
	limiter := opts.Limiter
	if limiter == nil {
		limiter = security.NewLoginLimiter(5, 15*time.Minute)
	}
	return &Service{
		store:          store,
		cookieName:     cookieName,
		cookieSecure:   opts.CookieSecure,
		csrfHeaderName: csrfHeader,
		limiter:        limiter,
		logger:         opts.Logger,
	}
}

func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey).(Principal)
	return principal, ok
}

func (s *Service) AttachSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(s.cookieName)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}

		session, err := s.store.SessionByToken(r.Context(), cookie.Value)
		if err == nil {
			ctx := context.WithValue(r.Context(), principalKey, Principal{User: session.User, Session: session})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if !errors.Is(err, ErrNotFound) && s.logger != nil {
			s.logger.Printf("session lookup failed request_id=%s error=%v", httputil.RequestID(r), err)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !security.IsUnsafeMethod(r.Method) || isLoginRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		principal, ok := FromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		if !security.ValidateCSRFToken(principal.Session.ID, principal.Session.CSRFSecret, r.Header.Get(s.csrfHeaderName)) {
			httputil.WriteError(w, http.StatusForbidden, "Invalid CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}

	email := normalizeEmail(req.Email)
	ip := httputil.ClientIP(r)
	if !s.limiter.Allow(ip, email) {
		httputil.WriteError(w, http.StatusTooManyRequests, "Too many login attempts. Try again later.")
		return
	}

	user, ok, err := s.store.VerifyAdminPassword(r.Context(), email, req.Password)
	if errors.Is(err, ErrNotFound) || (err == nil && !ok) {
		s.limiter.RecordFailure(ip, email)
		if s.logger != nil {
			s.logger.Printf("login failed request_id=%s email=%s ip=%s", httputil.RequestID(r), email, ip)
		}
		httputil.WriteError(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}
	if err != nil {
		if s.logger != nil {
			s.logger.Printf("login error request_id=%s email=%s ip=%s error=%v", httputil.RequestID(r), email, ip, err)
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	s.limiter.Reset(ip, email)
	_ = s.store.DeleteExpired(r.Context())
	session, rawToken, err := s.store.CreateSession(r.Context(), user.ID, r.UserAgent(), ip)
	if err != nil {
		if s.logger != nil {
			s.logger.Printf("create session failed request_id=%s user_id=%s error=%v", httputil.RequestID(r), user.ID, err)
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.setSessionCookie(w, rawToken, session.ExpiresAt)
	csrfToken, err := security.CSRFToken(session.ID, session.CSRFSecret)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"user":      user,
		"csrfToken": csrfToken,
	})
}

func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	principal, ok := FromContext(r.Context())
	if !ok {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthenticated")
		return
	}
	if err := s.store.DeleteSession(r.Context(), principal.Session.ID); err != nil && s.logger != nil {
		s.logger.Printf("logout delete session failed request_id=%s session_id=%s error=%v", httputil.RequestID(r), principal.Session.ID, err)
	}
	s.clearSessionCookie(w)
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Service) Session(w http.ResponseWriter, r *http.Request) {
	principal, ok := FromContext(r.Context())
	if !ok {
		httputil.WriteJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
		return
	}
	csrfToken, err := security.CSRFToken(principal.Session.ID, principal.Session.CSRFSecret)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"user":          principal.User,
		"csrfToken":     csrfToken,
	})
}

func (s *Service) setSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	})
}

func (s *Service) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
	})
}

func isLoginRequest(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == "/api/auth/login"
}
