package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey string

const (
	UserContextKey    contextKey = "openleaf_user"
	SessionContextKey contextKey = "openleaf_session"
)

type User struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"emailVerified"`
	Image         *string   `json:"image,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Session struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	UserID    string    `json:"userId"`
	ExpiresAt time.Time `json:"expiresAt"`
	User      User      `json:"user"`
}

type Authenticator struct {
	db *pgxpool.Pool
}

func NewAuthenticator(db *pgxpool.Pool) *Authenticator {
	return &Authenticator{db: db}
}

// ExtractToken retrieves the session token from Authorization header, cookies, or query params
func ExtractToken(r *http.Request) string {
	// 1. Authorization: Bearer <token>
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	// 2. Query param (useful for WebSocket handshakes)
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}

	// 3. Better-Auth session cookie
	cookieNames := []string{
		"better-auth.session_token",
		"__Secure-better-auth.session_token",
	}

	for _, name := range cookieNames {
		if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" {
			val := cookie.Value
			// Better-Auth signed cookies may look like: "raw_token.signature"
			if idx := strings.Index(val, "."); idx != -1 {
				// Try the raw prefix
				return val[:idx]
			}
			return val
		}
	}

	return ""
}

// ValidateSession verifies a token against the Neon session table and joins the user
func (a *Authenticator) ValidateSession(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, errors.New("empty session token")
	}

	query := `
		SELECT 
			s.id, s.token, s.user_id, s.expires_at,
			u.id, u.name, u.email, u.email_verified, u.image, u.created_at
		FROM session s
		JOIN "user" u ON s.user_id = u.id
		WHERE s.token = $1 AND s.expires_at > NOW()
		LIMIT 1;
	`

	var sess Session
	var user User
	err := a.db.QueryRow(ctx, query, token).Scan(
		&sess.ID,
		&sess.Token,
		&sess.UserID,
		&sess.ExpiresAt,
		&user.ID,
		&user.Name,
		&user.Email,
		&user.EmailVerified,
		&user.Image,
		&user.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("invalid or expired session")
		}
		return nil, err
	}

	sess.User = user
	return &sess, nil
}

// Middleware enforces authentication on protected routes
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ExtractToken(r)
		if token == "" {
			http.Error(w, `{"error":"unauthorized","message":"missing session token"}`, http.StatusUnauthorized)
			return
		}

		sess, err := a.ValidateSession(r.Context(), token)
		if err != nil {
			http.Error(w, `{"error":"unauthorized","message":"invalid or expired session"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserContextKey, &sess.User)
		ctx = context.WithValue(ctx, SessionContextKey, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUser extracts the authenticated user from the context
func GetUser(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(UserContextKey).(*User)
	return u, ok
}

// GetSession extracts the authenticated session from the context
func GetSession(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(SessionContextKey).(*Session)
	return s, ok
}
