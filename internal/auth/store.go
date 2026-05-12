package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"private-workspace/internal/db"
)

var ErrNotFound = errors.New("not found")

const dbTimeFormat = "2006-01-02T15:04:05.000000000Z07:00"

var dummyPasswordHash = "argon2id$v=19$m=65536,t=3,p=1$XzaeJEmk9z5UU9FlhM1GUQ$WCt+yJPQ4DcvPCALy76Yxw1wDrbFC2CSOdB+g3GTGag"

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type Session struct {
	ID         string
	User       User
	CSRFSecret string
	ExpiresAt  time.Time
}

type Store struct {
	db  *db.DB
	ttl time.Duration
	now func() time.Time
}

func NewStore(database *db.DB, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 168 * time.Hour
	}
	return &Store{
		db:  database,
		ttl: ttl,
		now: time.Now,
	}
}

func (s *Store) BootstrapAdmin(ctx context.Context, email string, passwordHash string) (User, error) {
	email = normalizeEmail(email)
	if email == "" {
		return User{}, errors.New("admin email is required")
	}
	if err := ValidatePasswordHash(passwordHash); err != nil {
		return User{}, fmt.Errorf("admin password hash is invalid: %w", err)
	}

	existing, err := s.AdminByEmail(ctx, email)
	if err == nil {
		if err := s.updateAdminHashIfNeeded(ctx, existing.ID, passwordHash); err != nil {
			return User{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return User{}, err
	}

	id, err := randomID()
	if err != nil {
		return User{}, err
	}
	now := formatTime(s.now().UTC())
	if _, err := s.db.ExecContext(ctx, "INSERT INTO admin_users (id, email, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", id, email, passwordHash, now, now); err != nil {
		return User{}, fmt.Errorf("insert admin user: %w", err)
	}
	return User{ID: id, Email: email}, nil
}

func (s *Store) AdminByEmail(ctx context.Context, email string) (User, error) {
	email = normalizeEmail(email)
	var user User
	err := s.db.QueryRowContext(ctx, "SELECT id, email FROM admin_users WHERE email = ?", email).Scan(&user.ID, &user.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get admin user: %w", err)
	}
	return user, nil
}

func (s *Store) VerifyAdminPassword(ctx context.Context, email string, password string) (User, bool, error) {
	email = normalizeEmail(email)
	var user User
	var passwordHash string
	err := s.db.QueryRowContext(ctx, "SELECT id, email, password_hash FROM admin_users WHERE email = ?", email).Scan(&user.ID, &user.Email, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		if _, verifyErr := VerifyPassword(password, dummyPasswordHash); verifyErr != nil {
			return User{}, false, fmt.Errorf("verify dummy admin password: %w", verifyErr)
		}
		return User{}, false, ErrNotFound
	}
	if err != nil {
		return User{}, false, fmt.Errorf("get admin credentials: %w", err)
	}
	ok, err := VerifyPassword(password, passwordHash)
	if err != nil {
		return User{}, false, fmt.Errorf("verify admin password: %w", err)
	}
	return user, ok, nil
}

func (s *Store) CreateSession(ctx context.Context, userID string, userAgent string, ipAddress string) (Session, string, error) {
	id, err := randomID()
	if err != nil {
		return Session{}, "", err
	}
	rawToken, err := randomToken(32)
	if err != nil {
		return Session{}, "", err
	}
	csrfSecret, err := randomToken(32)
	if err != nil {
		return Session{}, "", err
	}

	now := s.now().UTC()
	expiresAt := now.Add(s.ttl)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO sessions
		(id, user_id, token_hash, csrf_secret, expires_at, created_at, last_seen_at, user_agent, ip_address)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id,
		userID,
		HashSessionToken(rawToken),
		csrfSecret,
		formatTime(expiresAt),
		formatTime(now),
		formatTime(now),
		limitString(userAgent, 500),
		limitString(ipAddress, 100),
	); err != nil {
		return Session{}, "", fmt.Errorf("insert session: %w", err)
	}

	user, err := s.userByID(ctx, userID)
	if err != nil {
		return Session{}, "", err
	}
	return Session{ID: id, User: user, CSRFSecret: csrfSecret, ExpiresAt: expiresAt}, rawToken, nil
}

func (s *Store) SessionByToken(ctx context.Context, rawToken string) (Session, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return Session{}, ErrNotFound
	}

	var session Session
	var expiresAt string
	err := s.db.QueryRowContext(ctx, `SELECT sessions.id, sessions.csrf_secret, sessions.expires_at, admin_users.id, admin_users.email
		FROM sessions
		JOIN admin_users ON admin_users.id = sessions.user_id
		WHERE sessions.token_hash = ?`, HashSessionToken(rawToken)).
		Scan(&session.ID, &session.CSRFSecret, &expiresAt, &session.User.ID, &session.User.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("get session: %w", err)
	}

	parsedExpiresAt, err := parseTime(expiresAt)
	if err != nil {
		return Session{}, fmt.Errorf("parse session expiry: %w", err)
	}
	if !s.now().UTC().Before(parsedExpiresAt) {
		_ = s.DeleteSession(ctx, session.ID)
		return Session{}, ErrNotFound
	}
	session.ExpiresAt = parsedExpiresAt
	_ = s.TouchSession(ctx, session.ID)
	return session, nil
}

func (s *Store) TouchSession(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE sessions SET last_seen_at = ? WHERE id = ?", formatTime(s.now().UTC()), sessionID)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE id = ?", sessionID)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Store) DeleteExpired(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", formatTime(s.now().UTC()))
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

func HashSessionToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func (s *Store) updateAdminHashIfNeeded(ctx context.Context, userID string, passwordHash string) error {
	var existing string
	if err := s.db.QueryRowContext(ctx, "SELECT password_hash FROM admin_users WHERE id = ?", userID).Scan(&existing); err != nil {
		return fmt.Errorf("get admin password hash: %w", err)
	}
	if existing == passwordHash {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, "UPDATE admin_users SET password_hash = ?, updated_at = ? WHERE id = ?", passwordHash, formatTime(s.now().UTC()), userID); err != nil {
		return fmt.Errorf("update admin password hash: %w", err)
	}
	return nil
}

func (s *Store) userByID(ctx context.Context, userID string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, "SELECT id, email FROM admin_users WHERE id = ?", userID).Scan(&user.ID, &user.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("get admin user: %w", err)
	}
	return user, nil
}

func randomID() (string, error) {
	return randomToken(16)
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func formatTime(t time.Time) string {
	return t.UTC().Format(dbTimeFormat)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func limitString(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}
