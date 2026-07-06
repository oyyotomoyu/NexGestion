package system

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

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	AccessTokenLifetime  = 10 * time.Minute
	RefreshTokenLifetime = 30 * 24 * time.Hour
	maxLoginFailures     = 5
	loginLockDuration    = 15 * time.Minute
)

var (
	ErrInvalidCredentials = errors.New("email or password is incorrect")
	ErrAccountLocked      = errors.New("account is temporarily locked")
	ErrAccountInactive    = errors.New("account is not active")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

type AccessClaims struct {
	TokenType string `json:"typ"`
	jwt.RegisteredClaims
}

type AuthTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	UserID       string
}

type AuthService struct{ users *UserService }

func NewAuthService(users *UserService) *AuthService { return &AuthService{users: users} }

func (s *AuthService) Login(ctx context.Context, email, password, ipAddress, userAgent string) (*AuthTokens, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	db, err := s.users.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var id, passwordHash, status string
	var failed int
	var lockedUntil, deletedAt sql.NullString
	err = db.QueryRowContext(ctx, `SELECT id, password_hash, status, failed_login_count, locked_until, deleted_at
		FROM users WHERE email = ?`, email).Scan(&id, &passwordHash, &status, &failed, &lockedUntil, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if deletedAt.Valid || status == "disabled" || status == "pending" {
		return nil, ErrAccountInactive
	}
	if lockedUntil.Valid {
		if until, parseErr := time.Parse(time.RFC3339, lockedUntil.String); parseErr == nil && time.Now().UTC().Before(until) {
			return nil, ErrAccountLocked
		}
	}

	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		failed++
		var lock any
		if failed >= maxLoginFailures {
			lock = time.Now().UTC().Add(loginLockDuration).Format(time.RFC3339)
		}
		_, _ = db.ExecContext(ctx, `UPDATE users SET failed_login_count = ?, locked_until = ?, status = CASE WHEN ? IS NULL THEN status ELSE 'locked' END, updated_at = ? WHERE id = ?`, failed, lock, lock, time.Now().UTC().Format(time.RFC3339), id)
		return nil, ErrInvalidCredentials
	}
	if status == "locked" && !lockedUntil.Valid {
		return nil, ErrAccountLocked
	}

	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `UPDATE users SET failed_login_count = 0, locked_until = NULL,
		status = CASE WHEN status = 'locked' THEN 'active' ELSE status END, last_login_at = ?, updated_at = ? WHERE id = ?`, now.Format(time.RFC3339), now.Format(time.RFC3339), id); err != nil {
		return nil, err
	}
	return s.issueTokens(ctx, db, id, ipAddress, userAgent, now)
}

func (s *AuthService) Refresh(ctx context.Context, rawToken, ipAddress, userAgent string) (*AuthTokens, error) {
	if rawToken == "" {
		return nil, ErrInvalidToken
	}
	db, err := s.users.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	now := time.Now().UTC()
	var sessionID, userID, expiresAt string
	err = db.QueryRowContext(ctx, `SELECT s.id, s.user_id, s.expires_at FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.refresh_token_hash = ? AND s.revoked_at IS NULL AND u.deleted_at IS NULL AND u.status = 'active'`, tokenHash(rawToken)).Scan(&sessionID, &userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || !now.Before(expires) {
		return nil, ErrInvalidToken
	}
	result, err := db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ?, last_used_at = ? WHERE id = ? AND revoked_at IS NULL`, now.Format(time.RFC3339), now.Format(time.RFC3339), sessionID)
	if err != nil {
		return nil, err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return nil, ErrInvalidToken
	}
	return s.issueTokens(ctx, db, userID, ipAddress, userAgent, now)
}

func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	db, err := s.users.open()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE refresh_token_hash = ? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339), tokenHash(rawToken))
	return err
}

func (s *AuthService) VerifyAccessToken(ctx context.Context, rawToken string) (*AccessClaims, error) {
	secret, err := s.jwtSecret(ctx)
	if err != nil {
		return nil, err
	}
	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return secret, nil
	}, jwt.WithExpirationRequired(), jwt.WithIssuedAt())
	if err != nil || !token.Valid || claims.TokenType != "access" || claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	user, err := s.users.Get(ctx, claims.Subject)
	if err != nil || user.Status != "active" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func (s *AuthService) issueTokens(ctx context.Context, db *sql.DB, userID, ipAddress, userAgent string, now time.Time) (*AuthTokens, error) {
	secret, err := ensureJWTSecret(ctx, db)
	if err != nil {
		return nil, err
	}
	claims := AccessClaims{TokenType: "access", RegisteredClaims: jwt.RegisteredClaims{
		Subject: userID, ID: uuid.NewString(), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenLifetime)),
	}}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		return nil, err
	}
	refreshToken, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	_, err = db.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL AND id NOT IN (
		SELECT id FROM sessions WHERE user_id = ? AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 9
	)`, now.Format(time.RFC3339), userID, userID)
	if err != nil {
		return nil, err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO sessions (id, user_id, refresh_token_hash, expires_at, created_at, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), userID, tokenHash(refreshToken), now.Add(RefreshTokenLifetime).Format(time.RFC3339), now.Format(time.RFC3339), ipAddress, userAgent)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &AuthTokens{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresIn: int64(AccessTokenLifetime.Seconds()), UserID: userID}, nil
}

func (s *AuthService) jwtSecret(ctx context.Context) ([]byte, error) {
	db, err := s.users.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return ensureJWTSecret(ctx, db)
}

func ensureJWTSecret(ctx context.Context, db *sql.DB) ([]byte, error) {
	generated, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	_, err = db.ExecContext(ctx, `INSERT OR IGNORE INTO auth_secrets (name, secret, created_at) VALUES ('jwt_access', ?, ?)`, generated, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	var encoded string
	if err := db.QueryRowContext(ctx, `SELECT secret FROM auth_secrets WHERE name = 'jwt_access'`).Scan(&encoded); err != nil {
		return nil, err
	}
	return base64.RawURLEncoding.DecodeString(encoded)
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
