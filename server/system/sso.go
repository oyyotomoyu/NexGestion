package system

// SSO System (docs/System/SSO.md). SSO identifies the user; NexGestion
// authorizes the user. This file owns connections, linked identities, login
// state and the audit trail; sso_oidc.go owns the protocol itself.

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	ssoLoginStateLifetime = 10 * time.Minute
	ssoSecretKeyFileName  = "sso_secret.key"
	ssoSecretKeyEnv       = "NEXGESTION_SSO_SECRET_KEY"
	ssoAllowHTTPEnv       = "NEXGESTION_SSO_ALLOW_HTTP"
	ssoSecretPrefix       = "v1:"
	ssoDefaultReturnTo    = "/dashboard"
	ssoMaxAuditListLimit  = 200
)

var (
	ErrSSOConnectionNotFound = errors.New("sso connection not found")
	ErrSSOIdentityNotFound   = errors.New("sso identity not found")
	ErrSSOOrganizationAbsent = errors.New("organization not found")
)

// SSOValidationError is a 400-class configuration error shown to
// administrators. It never carries secret values.
type SSOValidationError struct{ Message string }

func (e *SSOValidationError) Error() string { return e.Message }

func ssoInvalid(format string, args ...any) error {
	return &SSOValidationError{Message: fmt.Sprintf(format, args...)}
}

// SSO login failure codes. Code is the only thing the browser ever sees; it
// maps to the short, safe messages in SSO.md "Error Handling". Reason is for
// the audit trail only and never contains tokens, codes, secrets or full
// claim payloads.
const (
	SSOErrorInvalidRequest      = "invalid_request"
	SSOErrorConnectionDisabled  = "connection_disabled"
	SSOErrorProviderError       = "provider_error"
	SSOErrorProviderUnavailable = "provider_unavailable"
	SSOErrorAccountNotAllowed   = "account_not_allowed"
	SSOErrorAccountPending      = "account_pending"
)

type SSOLoginError struct {
	Code           string
	Reason         string
	ConnectionID   string
	OrganizationID string
	UserID         string
	Email          string
}

func (e *SSOLoginError) Error() string { return e.Code + ": " + e.Reason }

type SSOClaimMapping struct {
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Name          string `json:"name"`
	Groups        string `json:"groups"`
}

func defaultSSOClaimMapping() SSOClaimMapping {
	return SSOClaimMapping{Email: "email", EmailVerified: "email_verified", Name: "name", Groups: "groups"}
}

// SSOConnection is the administrator view. The client secret is reported only
// as configured/not configured (SSO.md "The backend must never expose provider
// secrets through API responses").
type SSOConnection struct {
	ID                      string            `json:"id"`
	OrganizationID          string            `json:"organization_id"`
	Name                    string            `json:"name"`
	ProviderType            string            `json:"provider_type"`
	Protocol                string            `json:"protocol"`
	Issuer                  string            `json:"issuer"`
	ClientID                string            `json:"client_id"`
	ClientSecretConfigured  bool              `json:"client_secret_configured"`
	TokenEndpointAuthMethod string            `json:"token_endpoint_auth_method"`
	AuthorizationEndpoint   string            `json:"authorization_endpoint"`
	TokenEndpoint           string            `json:"token_endpoint"`
	JWKSURI                 string            `json:"jwks_uri"`
	RedirectURI             string            `json:"redirect_uri"`
	Scopes                  []string          `json:"scopes"`
	ClaimMapping            SSOClaimMapping   `json:"claim_mapping"`
	AllowedEmailDomains     []string          `json:"allowed_email_domains"`
	JITEnabled              bool              `json:"jit_enabled"`
	JITRequireApproval      bool              `json:"jit_require_approval"`
	JITDefaultRoleID        *string           `json:"jit_default_role_id"`
	JITDefaultGroupID       *string           `json:"jit_default_group_id"`
	GroupSyncEnabled        bool              `json:"group_sync_enabled"`
	GroupMapping            map[string]string `json:"group_mapping"`
	IsEnabled               bool              `json:"is_enabled"`
	LastTestedAt            *string           `json:"last_tested_at"`
	LastTestOK              *bool             `json:"last_test_ok"`
	LastTestError           *string           `json:"last_test_error"`
	CreatedAt               string            `json:"created_at"`
	UpdatedAt               string            `json:"updated_at"`
}

// ssoConnectionRecord adds the encrypted secret, which never leaves the
// system package.
type ssoConnectionRecord struct {
	SSOConnection
	clientSecretEncrypted string
}

// PublicSSOConnection is what the login page may see.
type PublicSSOConnection struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
}

// SSOConnectionInput is shared by create (missing fields take defaults or are
// rejected) and patch (nil means unchanged). An empty client_secret on patch
// is rejected rather than silently clearing the stored secret.
type SSOConnectionInput struct {
	OrganizationID          *string            `json:"organization_id"`
	Name                    *string            `json:"name"`
	ProviderType            *string            `json:"provider_type"`
	Protocol                *string            `json:"protocol"`
	Issuer                  *string            `json:"issuer"`
	ClientID                *string            `json:"client_id"`
	ClientSecret            *string            `json:"client_secret"`
	TokenEndpointAuthMethod *string            `json:"token_endpoint_auth_method"`
	AuthorizationEndpoint   *string            `json:"authorization_endpoint"`
	TokenEndpoint           *string            `json:"token_endpoint"`
	JWKSURI                 *string            `json:"jwks_uri"`
	RedirectURI             *string            `json:"redirect_uri"`
	Scopes                  *[]string          `json:"scopes"`
	ClaimMapping            *SSOClaimMapping   `json:"claim_mapping"`
	AllowedEmailDomains     *[]string          `json:"allowed_email_domains"`
	JITEnabled              *bool              `json:"jit_enabled"`
	JITRequireApproval      *bool              `json:"jit_require_approval"`
	JITDefaultRoleID        *string            `json:"jit_default_role_id"`
	JITDefaultGroupID       *string            `json:"jit_default_group_id"`
	GroupSyncEnabled        *bool              `json:"group_sync_enabled"`
	GroupMapping            *map[string]string `json:"group_mapping"`
	IsEnabled               *bool              `json:"is_enabled"`
}

type SSOIdentity struct {
	ID                    string  `json:"id"`
	UserID                string  `json:"user_id"`
	OrganizationID        string  `json:"organization_id"`
	SSOConnectionID       string  `json:"sso_connection_id"`
	ProviderSubject       string  `json:"provider_subject"`
	ProviderEmail         *string `json:"provider_email"`
	ProviderEmailVerified bool    `json:"provider_email_verified"`
	LastLoginAt           *string `json:"last_login_at"`
	CreatedAt             string  `json:"created_at"`
	UpdatedAt             string  `json:"updated_at"`
}

type LinkSSOIdentityInput struct {
	UserID          string `json:"user_id"`
	SSOConnectionID string `json:"sso_connection_id"`
	ProviderSubject string `json:"provider_subject"`
	ProviderEmail   string `json:"provider_email"`
}

type SSOAuditEvent struct {
	ID              string  `json:"id"`
	EventType       string  `json:"event_type"`
	OrganizationID  *string `json:"organization_id"`
	UserID          *string `json:"user_id"`
	SSOConnectionID *string `json:"sso_connection_id"`
	SourceIP        *string `json:"source_ip"`
	UserAgent       *string `json:"user_agent"`
	FailureReason   *string `json:"failure_reason"`
	Detail          *string `json:"detail"`
	OccurredAt      string  `json:"occurred_at"`
}

type SSOAuditQuery struct {
	EventType    string
	ConnectionID string
	UserID       string
	Limit        int
}

type SSOTestCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type SSOTestResult struct {
	OK       bool           `json:"ok"`
	Checks   []SSOTestCheck `json:"checks"`
	TestedAt string         `json:"tested_at"`
}

// SSOLoginResult is returned after a successful callback: the normal
// NexGestion session (SSO.md "SSO login must end with the same session model
// as password login").
type SSOLoginResult struct {
	Tokens       *AuthTokens
	ReturnTo     string
	UserID       string
	ConnectionID string
	Email        string
}

type SSORequestContext struct {
	IPAddress string
	UserAgent string
}

type SSOService struct {
	databasePath      string
	databaseDirectory string
	users             *UserService
	auth              *AuthService
	httpClient        *http.Client
	jwks              *jwksCache
	now               func() time.Time
	allowHTTP         bool

	keyOnce sync.Once
	key     []byte
	keyErr  error
}

func NewSSOService(databaseDirectory string, users *UserService, auth *AuthService) *SSOService {
	if strings.TrimSpace(databaseDirectory) == "" {
		databaseDirectory = defaultDatabaseDirectory
	}
	return &SSOService{
		databasePath:      filepath.Join(databaseDirectory, "sso.db"),
		databaseDirectory: databaseDirectory,
		users:             users,
		auth:              auth,
		httpClient:        &http.Client{Timeout: oidcHTTPTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		jwks:              newJWKSCache(),
		now:               func() time.Time { return time.Now().UTC() },
		allowHTTP:         strings.EqualFold(strings.TrimSpace(os.Getenv(ssoAllowHTTPEnv)), "true"),
	}
}

func (s *SSOService) open() (*sql.DB, error) {
	db, err := sql.Open("sqlite", s.databasePath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{`PRAGMA busy_timeout = 5000`, `PRAGMA foreign_keys = ON`} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

func (s *SSOService) timestamp() string { return s.now().UTC().Format(time.RFC3339) }

// ---------------------------------------------------------------------------
// Client secret encryption (SSO.md "client_secret must never be stored in
// plaintext"). AES-256-GCM with the connection id as associated data, so a
// ciphertext copied onto another row fails to decrypt. The key comes from
// NEXGESTION_SSO_SECRET_KEY when set, otherwise from a 0600 key file generated
// beside the databases on first use.
// ---------------------------------------------------------------------------

func (s *SSOService) secretKey() ([]byte, error) {
	s.keyOnce.Do(func() { s.key, s.keyErr = loadSSOSecretKey(s.databaseDirectory) })
	return s.key, s.keyErr
}

func loadSSOSecretKey(directory string) ([]byte, error) {
	decode := func(encoded string) ([]byte, error) {
		encoded = strings.TrimSpace(encoded)
		for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
			if key, err := encoding.DecodeString(encoded); err == nil && len(key) == 32 {
				return key, nil
			}
		}
		return nil, errors.New("sso secret key must be 32 bytes, base64 encoded")
	}
	if fromEnv := os.Getenv(ssoSecretKeyEnv); strings.TrimSpace(fromEnv) != "" {
		return decode(fromEnv)
	}
	path := filepath.Join(directory, ssoSecretKeyFileName)
	if content, err := os.ReadFile(path); err == nil {
		return decode(string(content))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read sso secret key: %w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, readErr
		}
		return decode(string(content))
	}
	if err != nil {
		return nil, fmt.Errorf("create sso secret key: %w", err)
	}
	defer file.Close()
	if _, err := file.WriteString(base64.StdEncoding.EncodeToString(key) + "\n"); err != nil {
		return nil, fmt.Errorf("write sso secret key: %w", err)
	}
	return key, nil
}

func (s *SSOService) encryptSecret(connectionID, plaintext string) (string, error) {
	key, err := s.secretKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, []byte(plaintext), []byte(connectionID))
	return ssoSecretPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (s *SSOService) decryptSecret(connectionID, encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}
	if !strings.HasPrefix(encrypted, ssoSecretPrefix) {
		return "", errors.New("unrecognized client secret format")
	}
	sealed, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(encrypted, ssoSecretPrefix))
	if err != nil {
		return "", errors.New("client secret is corrupt")
	}
	key, err := s.secretKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < aead.NonceSize() {
		return "", errors.New("client secret is corrupt")
	}
	plaintext, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(connectionID))
	if err != nil {
		return "", errors.New("client secret cannot be decrypted; the SSO secret key may have changed")
	}
	return string(plaintext), nil
}

// ---------------------------------------------------------------------------
// Audit
// ---------------------------------------------------------------------------

type ssoAuditEntry struct {
	EventType      string
	OrganizationID string
	UserID         string
	ConnectionID   string
	Request        SSORequestContext
	FailureReason  string
	Detail         string
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *SSOService) recordAudit(ctx context.Context, db sqlExecer, entry ssoAuditEntry) error {
	_, err := db.ExecContext(ctx, `INSERT INTO sso_audit_events
		(id, event_type, organization_id, user_id, sso_connection_id, source_ip, user_agent, failure_reason, detail, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), entry.EventType, nullIfEmpty(entry.OrganizationID), nullIfEmpty(entry.UserID), nullIfEmpty(entry.ConnectionID),
		nullIfEmpty(entry.Request.IPAddress), nullIfEmpty(truncate(entry.Request.UserAgent, 512)), nullIfEmpty(truncate(entry.FailureReason, 500)),
		nullIfEmpty(truncate(entry.Detail, 2000)), s.timestamp())
	return err
}

func (s *SSOService) ListAuditEvents(ctx context.Context, query SSOAuditQuery) ([]SSOAuditEvent, error) {
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > ssoMaxAuditListLimit {
		query.Limit = ssoMaxAuditListLimit
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	clauses, args := []string{"1 = 1"}, []any{}
	if query.EventType != "" {
		clauses, args = append(clauses, "event_type = ?"), append(args, query.EventType)
	}
	if query.ConnectionID != "" {
		clauses, args = append(clauses, "sso_connection_id = ?"), append(args, query.ConnectionID)
	}
	if query.UserID != "" {
		clauses, args = append(clauses, "user_id = ?"), append(args, query.UserID)
	}
	args = append(args, query.Limit)
	rows, err := db.QueryContext(ctx, `SELECT id, event_type, organization_id, user_id, sso_connection_id, source_ip, user_agent,
		failure_reason, detail, occurred_at FROM sso_audit_events WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY occurred_at DESC, rowid DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []SSOAuditEvent{}
	for rows.Next() {
		var event SSOAuditEvent
		if err := rows.Scan(&event.ID, &event.EventType, &event.OrganizationID, &event.UserID, &event.SSOConnectionID,
			&event.SourceIP, &event.UserAgent, &event.FailureReason, &event.Detail, &event.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// ---------------------------------------------------------------------------
// Connections
// ---------------------------------------------------------------------------

const ssoConnectionColumns = `id, organization_id, name, provider_type, protocol, issuer, client_id, client_secret_encrypted,
	token_endpoint_auth_method, authorization_endpoint, token_endpoint, jwks_uri, redirect_uri, scopes, claim_mapping,
	allowed_email_domains, jit_enabled, jit_require_approval, jit_default_role_id, jit_default_group_id, group_sync_enabled,
	group_mapping, is_enabled, last_tested_at, last_test_ok, last_test_error, created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanSSOConnection(row rowScanner) (*ssoConnectionRecord, error) {
	var record ssoConnectionRecord
	var secret sql.NullString
	var scopes, claimMapping, domains, groupMapping string
	var jitEnabled, jitApproval, groupSync, enabled int
	var lastTestOK sql.NullInt64
	err := row.Scan(&record.ID, &record.OrganizationID, &record.Name, &record.ProviderType, &record.Protocol, &record.Issuer,
		&record.ClientID, &secret, &record.TokenEndpointAuthMethod, &record.AuthorizationEndpoint, &record.TokenEndpoint,
		&record.JWKSURI, &record.RedirectURI, &scopes, &claimMapping, &domains, &jitEnabled, &jitApproval,
		&record.JITDefaultRoleID, &record.JITDefaultGroupID, &groupSync, &groupMapping, &enabled, &record.LastTestedAt,
		&lastTestOK, &record.LastTestError, &record.CreatedAt, &record.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSSOConnectionNotFound
	}
	if err != nil {
		return nil, err
	}
	record.clientSecretEncrypted = secret.String
	record.ClientSecretConfigured = secret.String != ""
	record.Scopes = strings.Fields(scopes)
	record.ClaimMapping = defaultSSOClaimMapping()
	_ = json.Unmarshal([]byte(claimMapping), &record.ClaimMapping)
	record.AllowedEmailDomains = []string{}
	_ = json.Unmarshal([]byte(domains), &record.AllowedEmailDomains)
	record.GroupMapping = map[string]string{}
	_ = json.Unmarshal([]byte(groupMapping), &record.GroupMapping)
	record.JITEnabled, record.JITRequireApproval = jitEnabled == 1, jitApproval == 1
	record.GroupSyncEnabled, record.IsEnabled = groupSync == 1, enabled == 1
	if lastTestOK.Valid {
		ok := lastTestOK.Int64 == 1
		record.LastTestOK = &ok
	}
	return &record, nil
}

func (s *SSOService) loadConnection(ctx context.Context, db *sql.DB, id string) (*ssoConnectionRecord, error) {
	return scanSSOConnection(db.QueryRowContext(ctx, `SELECT `+ssoConnectionColumns+` FROM sso_connections WHERE id = ?`, id))
}

func (s *SSOService) ListConnections(ctx context.Context) ([]SSOConnection, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT `+ssoConnectionColumns+` FROM sso_connections ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	connections := []SSOConnection{}
	for rows.Next() {
		record, err := scanSSOConnection(rows)
		if err != nil {
			return nil, err
		}
		connections = append(connections, record.SSOConnection)
	}
	return connections, rows.Err()
}

func (s *SSOService) GetConnection(ctx context.Context, id string) (*SSOConnection, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	record, err := s.loadConnection(ctx, db, id)
	if err != nil {
		return nil, err
	}
	return &record.SSOConnection, nil
}

// ListPublicConnections returns only enabled connections of active
// organizations, with nothing beyond what a login button needs.
func (s *SSOService) ListPublicConnections(ctx context.Context) ([]PublicSSOConnection, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT c.id, c.name, c.provider_type FROM sso_connections c
		JOIN organizations o ON o.id = c.organization_id
		WHERE c.is_enabled = 1 AND o.status = 'active' ORDER BY c.name COLLATE NOCASE, c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	connections := []PublicSSOConnection{}
	for rows.Next() {
		var connection PublicSSOConnection
		if err := rows.Scan(&connection.ID, &connection.Name, &connection.ProviderType); err != nil {
			return nil, err
		}
		connections = append(connections, connection)
	}
	return connections, rows.Err()
}

func (s *SSOService) CreateConnection(ctx context.Context, actorUserID string, input SSOConnectionInput, request SSORequestContext) (*SSOConnection, error) {
	id := uuid.NewString()
	record := &ssoConnectionRecord{SSOConnection: SSOConnection{
		ID: id, OrganizationID: DefaultOrganizationID, ProviderType: "generic", Protocol: "oidc",
		TokenEndpointAuthMethod: "client_secret_basic", Scopes: []string{"openid", "email", "profile"},
		ClaimMapping: defaultSSOClaimMapping(), AllowedEmailDomains: []string{}, GroupMapping: map[string]string{},
	}}
	if input.Name == nil || input.Issuer == nil || input.ClientID == nil || input.RedirectURI == nil {
		return nil, ssoInvalid("name, issuer, client_id and redirect_uri are required")
	}
	changed, err := s.applyConnectionInput(ctx, record, input, true)
	if err != nil {
		return nil, err
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := s.requireOrganization(ctx, db, record.OrganizationID); err != nil {
		return nil, err
	}
	now := s.timestamp()
	record.CreatedAt, record.UpdatedAt = now, now
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := insertSSOConnection(ctx, tx, record); err != nil {
		return nil, err
	}
	if err := s.recordAudit(ctx, tx, ssoAuditEntry{EventType: "sso.connection.created", OrganizationID: record.OrganizationID,
		UserID: actorUserID, ConnectionID: id, Request: request, Detail: "fields: " + strings.Join(changed, ",")}); err != nil {
		return nil, err
	}
	if record.GroupSyncEnabled || len(record.GroupMapping) > 0 {
		if err := s.recordAudit(ctx, tx, ssoAuditEntry{EventType: "sso.group_mapping.updated", OrganizationID: record.OrganizationID,
			UserID: actorUserID, ConnectionID: id, Request: request, Detail: groupMappingDetail(record)}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &record.SSOConnection, nil
}

func insertSSOConnection(ctx context.Context, db sqlExecer, record *ssoConnectionRecord) error {
	claimMapping, _ := json.Marshal(record.ClaimMapping)
	domains, _ := json.Marshal(record.AllowedEmailDomains)
	groupMapping, _ := json.Marshal(record.GroupMapping)
	_, err := db.ExecContext(ctx, `INSERT INTO sso_connections (`+ssoConnectionColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.OrganizationID, record.Name, record.ProviderType, record.Protocol, record.Issuer, record.ClientID,
		nullIfEmpty(record.clientSecretEncrypted), record.TokenEndpointAuthMethod, record.AuthorizationEndpoint,
		record.TokenEndpoint, record.JWKSURI, record.RedirectURI, strings.Join(record.Scopes, " "), string(claimMapping),
		string(domains), boolInt(record.JITEnabled), boolInt(record.JITRequireApproval), record.JITDefaultRoleID,
		record.JITDefaultGroupID, boolInt(record.GroupSyncEnabled), string(groupMapping), boolInt(record.IsEnabled),
		record.LastTestedAt, nullableBoolInt(record.LastTestOK), record.LastTestError, record.CreatedAt, record.UpdatedAt)
	return err
}

func (s *SSOService) UpdateConnection(ctx context.Context, actorUserID, id string, input SSOConnectionInput, request SSORequestContext) (*SSOConnection, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	record, err := s.loadConnection(ctx, db, id)
	if err != nil {
		return nil, err
	}
	previousGroupSync, previousMapping := record.GroupSyncEnabled, groupMappingDetail(record)
	previousJWKS := record.JWKSURI
	changed, err := s.applyConnectionInput(ctx, record, input, false)
	if err != nil {
		return nil, err
	}
	if len(changed) == 0 {
		return &record.SSOConnection, nil
	}
	if err := s.requireOrganization(ctx, db, record.OrganizationID); err != nil {
		return nil, err
	}
	record.UpdatedAt = s.timestamp()
	claimMapping, _ := json.Marshal(record.ClaimMapping)
	domains, _ := json.Marshal(record.AllowedEmailDomains)
	groupMapping, _ := json.Marshal(record.GroupMapping)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE sso_connections SET organization_id = ?, name = ?, provider_type = ?, protocol = ?,
		issuer = ?, client_id = ?, client_secret_encrypted = ?, token_endpoint_auth_method = ?, authorization_endpoint = ?,
		token_endpoint = ?, jwks_uri = ?, redirect_uri = ?, scopes = ?, claim_mapping = ?, allowed_email_domains = ?,
		jit_enabled = ?, jit_require_approval = ?, jit_default_role_id = ?, jit_default_group_id = ?, group_sync_enabled = ?,
		group_mapping = ?, is_enabled = ?, updated_at = ? WHERE id = ?`,
		record.OrganizationID, record.Name, record.ProviderType, record.Protocol, record.Issuer, record.ClientID,
		nullIfEmpty(record.clientSecretEncrypted), record.TokenEndpointAuthMethod, record.AuthorizationEndpoint,
		record.TokenEndpoint, record.JWKSURI, record.RedirectURI, strings.Join(record.Scopes, " "), string(claimMapping),
		string(domains), boolInt(record.JITEnabled), boolInt(record.JITRequireApproval), record.JITDefaultRoleID,
		record.JITDefaultGroupID, boolInt(record.GroupSyncEnabled), string(groupMapping), boolInt(record.IsEnabled),
		record.UpdatedAt, record.ID)
	if err != nil {
		return nil, err
	}
	if err := s.recordAudit(ctx, tx, ssoAuditEntry{EventType: "sso.connection.updated", OrganizationID: record.OrganizationID,
		UserID: actorUserID, ConnectionID: id, Request: request, Detail: "fields: " + strings.Join(changed, ",")}); err != nil {
		return nil, err
	}
	if previousGroupSync != record.GroupSyncEnabled || previousMapping != groupMappingDetail(record) {
		if err := s.recordAudit(ctx, tx, ssoAuditEntry{EventType: "sso.group_mapping.updated", OrganizationID: record.OrganizationID,
			UserID: actorUserID, ConnectionID: id, Request: request, Detail: groupMappingDetail(record)}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if previousJWKS != record.JWKSURI {
		s.jwks.forget(previousJWKS)
	}
	return &record.SSOConnection, nil
}

func (s *SSOService) DeleteConnection(ctx context.Context, actorUserID, id string, request SSORequestContext) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	record, err := s.loadConnection(ctx, db, id)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var linked int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sso_user_identities WHERE sso_connection_id = ?`, id).Scan(&linked); err != nil {
		return err
	}
	// Linked identities and pending login states cascade with the connection.
	if _, err := tx.ExecContext(ctx, `DELETE FROM sso_connections WHERE id = ?`, id); err != nil {
		return err
	}
	if err := s.recordAudit(ctx, tx, ssoAuditEntry{EventType: "sso.connection.deleted", OrganizationID: record.OrganizationID,
		UserID: actorUserID, ConnectionID: id, Request: request,
		Detail: fmt.Sprintf("name=%q issuer=%s linked_identities_removed=%d", record.Name, record.Issuer, linked)}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.jwks.forget(record.JWKSURI)
	return nil
}

func (s *SSOService) requireOrganization(ctx context.Context, db *sql.DB, id string) error {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM organizations WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ssoInvalid("organization %q does not exist", id)
	}
	return err
}

var (
	ssoScopeTokenPattern = regexp.MustCompile(`^[\x21\x23-\x5B\x5D-\x7E]+$`)
	ssoDomainPattern     = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	ssoClaimNamePattern  = regexp.MustCompile(`^[A-Za-z0-9_:/.\-]{1,200}$`)
)

// applyConnectionInput merges input into record, validates the result, runs
// provider discovery when endpoints are missing, and encrypts a new secret.
// It returns the names of the fields that changed (never their values, so the
// audit detail can never contain the secret).
func (s *SSOService) applyConnectionInput(ctx context.Context, record *ssoConnectionRecord, input SSOConnectionInput, creating bool) ([]string, error) {
	var changed []string
	setString := func(name string, target *string, value *string) {
		if value == nil {
			return
		}
		trimmed := strings.TrimSpace(*value)
		if trimmed != *target {
			*target = trimmed
			changed = append(changed, name)
		}
	}
	setBool := func(name string, target *bool, value *bool) {
		if value != nil && *value != *target {
			*target = *value
			changed = append(changed, name)
		}
	}
	setOptionalID := func(name string, target **string, value *string) {
		if value == nil {
			return
		}
		trimmed := strings.TrimSpace(*value)
		current := ""
		if *target != nil {
			current = **target
		}
		if trimmed == current {
			return
		}
		if trimmed == "" {
			*target = nil
		} else {
			*target = &trimmed
		}
		changed = append(changed, name)
	}

	issuerBefore := record.Issuer
	setString("organization_id", &record.OrganizationID, input.OrganizationID)
	setString("name", &record.Name, input.Name)
	setString("provider_type", &record.ProviderType, input.ProviderType)
	setString("protocol", &record.Protocol, input.Protocol)
	setString("issuer", &record.Issuer, input.Issuer)
	setString("client_id", &record.ClientID, input.ClientID)
	setString("token_endpoint_auth_method", &record.TokenEndpointAuthMethod, input.TokenEndpointAuthMethod)
	setString("authorization_endpoint", &record.AuthorizationEndpoint, input.AuthorizationEndpoint)
	setString("token_endpoint", &record.TokenEndpoint, input.TokenEndpoint)
	setString("jwks_uri", &record.JWKSURI, input.JWKSURI)
	if input.RedirectURI != nil {
		redirect := strings.ReplaceAll(strings.TrimSpace(*input.RedirectURI), "{id}", record.ID)
		setString("redirect_uri", &record.RedirectURI, &redirect)
	}
	if input.Scopes != nil {
		scopes := normalizeSSOScopes(*input.Scopes)
		if strings.Join(scopes, " ") != strings.Join(record.Scopes, " ") {
			record.Scopes = scopes
			changed = append(changed, "scopes")
		}
	}
	if input.ClaimMapping != nil {
		mapping := *input.ClaimMapping
		defaults := defaultSSOClaimMapping()
		for _, field := range []struct{ value, fallback *string }{
			{&mapping.Email, &defaults.Email}, {&mapping.EmailVerified, &defaults.EmailVerified},
			{&mapping.Name, &defaults.Name}, {&mapping.Groups, &defaults.Groups},
		} {
			if *field.value = strings.TrimSpace(*field.value); *field.value == "" {
				*field.value = *field.fallback
			}
		}
		if mapping != record.ClaimMapping {
			record.ClaimMapping = mapping
			changed = append(changed, "claim_mapping")
		}
	}
	if input.AllowedEmailDomains != nil {
		domains, err := normalizeSSODomains(*input.AllowedEmailDomains)
		if err != nil {
			return nil, err
		}
		if strings.Join(domains, ",") != strings.Join(record.AllowedEmailDomains, ",") {
			record.AllowedEmailDomains = domains
			changed = append(changed, "allowed_email_domains")
		}
	}
	setBool("jit_enabled", &record.JITEnabled, input.JITEnabled)
	setBool("jit_require_approval", &record.JITRequireApproval, input.JITRequireApproval)
	setOptionalID("jit_default_role_id", &record.JITDefaultRoleID, input.JITDefaultRoleID)
	setOptionalID("jit_default_group_id", &record.JITDefaultGroupID, input.JITDefaultGroupID)
	setBool("group_sync_enabled", &record.GroupSyncEnabled, input.GroupSyncEnabled)
	if input.GroupMapping != nil {
		mapping := map[string]string{}
		for providerGroup, groupID := range *input.GroupMapping {
			providerGroup, groupID = strings.TrimSpace(providerGroup), strings.TrimSpace(groupID)
			if providerGroup == "" || len(providerGroup) > 200 || groupID == "" {
				return nil, ssoInvalid("group_mapping entries need a provider group (max 200 characters) and a NexGestion group id")
			}
			mapping[providerGroup] = groupID
		}
		before, _ := json.Marshal(record.GroupMapping)
		after, _ := json.Marshal(mapping)
		if string(before) != string(after) {
			record.GroupMapping = mapping
			changed = append(changed, "group_mapping")
		}
	}
	setBool("is_enabled", &record.IsEnabled, input.IsEnabled)

	if err := s.validateConnection(ctx, record); err != nil {
		return nil, err
	}

	// Endpoints come from discovery unless the administrator supplied all
	// three (providers without discovery). A changed issuer re-runs discovery
	// for any endpoint not explicitly given in the same request.
	issuerChanged := !creating && issuerBefore != record.Issuer
	if issuerChanged {
		if input.AuthorizationEndpoint == nil {
			record.AuthorizationEndpoint = ""
		}
		if input.TokenEndpoint == nil {
			record.TokenEndpoint = ""
		}
		if input.JWKSURI == nil {
			record.JWKSURI = ""
		}
	}
	if record.AuthorizationEndpoint == "" || record.TokenEndpoint == "" || record.JWKSURI == "" {
		doc, err := s.fetchDiscovery(ctx, record.Issuer)
		if err != nil {
			return nil, ssoInvalid("provider discovery failed (%v); supply authorization_endpoint, token_endpoint and jwks_uri explicitly", err)
		}
		for _, endpoint := range []struct {
			name   string
			target *string
			value  string
		}{
			{"authorization_endpoint", &record.AuthorizationEndpoint, doc.AuthorizationEndpoint},
			{"token_endpoint", &record.TokenEndpoint, doc.TokenEndpoint},
			{"jwks_uri", &record.JWKSURI, doc.JWKSURI},
		} {
			if *endpoint.target == "" {
				*endpoint.target = strings.TrimSpace(endpoint.value)
				changed = append(changed, endpoint.name)
			}
		}
	}
	for _, endpoint := range []struct{ name, value string }{
		{"authorization_endpoint", record.AuthorizationEndpoint},
		{"token_endpoint", record.TokenEndpoint},
		{"jwks_uri", record.JWKSURI},
	} {
		if err := s.validateProviderURL(endpoint.name, endpoint.value, true); err != nil {
			return nil, err
		}
	}

	if input.ClientSecret != nil {
		secret := strings.TrimSpace(*input.ClientSecret)
		if secret == "" {
			return nil, ssoInvalid("client_secret cannot be empty; set token_endpoint_auth_method to \"none\" for a public client")
		}
		if len(secret) > 4096 {
			return nil, ssoInvalid("client_secret is too long")
		}
		encrypted, err := s.encryptSecret(record.ID, secret)
		if err != nil {
			return nil, err
		}
		record.clientSecretEncrypted = encrypted
		record.ClientSecretConfigured = true
		changed = append(changed, "client_secret")
	}
	if record.TokenEndpointAuthMethod == "none" {
		if record.clientSecretEncrypted != "" {
			record.clientSecretEncrypted, record.ClientSecretConfigured = "", false
			changed = append(changed, "client_secret")
		}
	} else if record.clientSecretEncrypted == "" {
		return nil, ssoInvalid("client_secret is required for token_endpoint_auth_method %q", record.TokenEndpointAuthMethod)
	}
	sort.Strings(changed)
	return dedupeStrings(changed), nil
}

func (s *SSOService) validateConnection(ctx context.Context, record *ssoConnectionRecord) error {
	if record.Name == "" || len(record.Name) > 100 {
		return ssoInvalid("name is required and must be at most 100 characters")
	}
	switch record.ProviderType {
	case "google", "microsoft", "okta", "auth0", "keycloak", "generic":
	default:
		return ssoInvalid("provider_type must be google, microsoft, okta, auth0, keycloak or generic")
	}
	if record.Protocol == "saml" {
		return ssoInvalid("SAML 2.0 is not supported yet; only oidc connections can be created")
	}
	if record.Protocol != "oidc" {
		return ssoInvalid("protocol must be oidc")
	}
	if err := s.validateProviderURL("issuer", record.Issuer, false); err != nil {
		return err
	}
	if record.ClientID == "" || len(record.ClientID) > 500 {
		return ssoInvalid("client_id is required and must be at most 500 characters")
	}
	switch record.TokenEndpointAuthMethod {
	case "client_secret_basic", "client_secret_post", "none":
	default:
		return ssoInvalid("token_endpoint_auth_method must be client_secret_basic, client_secret_post or none")
	}
	if err := s.validateRedirectURI(record.ID, record.RedirectURI); err != nil {
		return err
	}
	if len(record.Scopes) == 0 || record.Scopes[0] != "openid" {
		return ssoInvalid("scopes must include openid")
	}
	for _, scope := range record.Scopes {
		if !ssoScopeTokenPattern.MatchString(scope) {
			return ssoInvalid("scope %q contains invalid characters", scope)
		}
	}
	for _, claim := range []string{record.ClaimMapping.Email, record.ClaimMapping.EmailVerified, record.ClaimMapping.Name, record.ClaimMapping.Groups} {
		if !ssoClaimNamePattern.MatchString(claim) {
			return ssoInvalid("claim_mapping contains an invalid claim name %q", claim)
		}
	}
	if record.JITEnabled && len(record.AllowedEmailDomains) == 0 {
		return ssoInvalid("just-in-time provisioning requires at least one allowed email domain")
	}
	if record.JITDefaultRoleID != nil {
		if err := s.validateProvisioningRole(ctx, *record.JITDefaultRoleID); err != nil {
			return err
		}
	}
	if record.JITDefaultGroupID != nil {
		if err := s.validateActiveGroup(ctx, *record.JITDefaultGroupID); err != nil {
			return err
		}
	}
	for _, groupID := range record.GroupMapping {
		if err := s.validateActiveGroup(ctx, groupID); err != nil {
			return err
		}
	}
	return nil
}

// validateProvisioningRole enforces SSO.md "Administrator roles must require
// explicit local assignment": neither the built-in Admin role nor any role
// that grants all permissions can be handed out automatically.
func (s *SSOService) validateProvisioningRole(ctx context.Context, roleID string) error {
	db, err := s.users.open()
	if err != nil {
		return err
	}
	defer db.Close()
	var grantsAll int
	err = db.QueryRowContext(ctx, `SELECT grants_all_permissions FROM roles WHERE id = ?`, roleID).Scan(&grantsAll)
	if errors.Is(err, sql.ErrNoRows) {
		return ssoInvalid("jit_default_role_id %q does not exist", roleID)
	}
	if err != nil {
		return err
	}
	if roleID == adminRoleID || grantsAll == 1 {
		return ssoInvalid("administrator roles cannot be assigned automatically; assign them locally after provisioning")
	}
	return nil
}

func (s *SSOService) validateActiveGroup(ctx context.Context, groupID string) error {
	db, err := s.users.open()
	if err != nil {
		return err
	}
	defer db.Close()
	var status string
	err = db.QueryRowContext(ctx, `SELECT status FROM groups WHERE id = ?`, groupID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ssoInvalid("group %q does not exist", groupID)
	}
	if err != nil {
		return err
	}
	if status != "active" {
		return ssoInvalid("group %q is not active", groupID)
	}
	return nil
}

// validateProviderURL requires absolute https URLs. Plain http is accepted
// only for loopback hosts (local development) or when the deployment opts in
// with NEXGESTION_SSO_ALLOW_HTTP=true for an identity provider on its own
// private network (SSO.md "Local-First Deployment Rules").
func (s *SSOService) validateProviderURL(name, raw string, allowQuery bool) error {
	parsed, err := url.Parse(raw)
	if err != nil || raw == "" || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return ssoInvalid("%s must be an absolute URL without credentials or fragment", name)
	}
	if !allowQuery && parsed.RawQuery != "" {
		return ssoInvalid("%s must not contain a query string", name)
	}
	if len(raw) > 2000 {
		return ssoInvalid("%s is too long", name)
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if s.allowHTTP || isLoopbackHost(parsed.Hostname()) {
			return nil
		}
		return ssoInvalid("%s must use https (set %s=true to allow http on a private network)", name, ssoAllowHTTPEnv)
	}
	return ssoInvalid("%s must use https", name)
}

// validateRedirectURI pins the redirect URI to this connection's own callback
// route, so a provider can only ever send the code back to the endpoint that
// expects it (SSO.md "Strict redirect URI validation").
func (s *SSOService) validateRedirectURI(connectionID, raw string) error {
	if err := s.validateProviderURL("redirect_uri", raw, false); err != nil {
		return err
	}
	parsed, _ := url.Parse(raw)
	expected := "/api/auth/sso/" + connectionID + "/callback"
	if parsed.Path != expected {
		return ssoInvalid("redirect_uri path must be %s (use {id} as a placeholder when creating)", expected)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeSSOScopes(scopes []string) []string {
	result := []string{"openid"}
	seen := map[string]bool{"openid": true}
	for _, scope := range scopes {
		for _, token := range strings.Fields(scope) {
			if !seen[token] {
				seen[token] = true
				result = append(result, token)
			}
		}
	}
	return result
}

func normalizeSSODomains(domains []string) ([]string, error) {
	result := []string{}
	seen := map[string]bool{}
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(domain), "@"))
		if domain == "" || seen[domain] {
			continue
		}
		if !ssoDomainPattern.MatchString(domain) || len(domain) > 253 {
			return nil, ssoInvalid("allowed_email_domains contains an invalid domain %q", domain)
		}
		seen[domain] = true
		result = append(result, domain)
	}
	sort.Strings(result)
	return result, nil
}

func groupMappingDetail(record *ssoConnectionRecord) string {
	encoded, _ := json.Marshal(record.GroupMapping)
	return fmt.Sprintf("group_sync_enabled=%t group_mapping=%s", record.GroupSyncEnabled, encoded)
}

// ---------------------------------------------------------------------------
// Connection test (SSO.md "Test a connection before enabling it")
// ---------------------------------------------------------------------------

func (s *SSOService) TestConnection(ctx context.Context, actorUserID, id string) (*SSOTestResult, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	record, err := s.loadConnection(ctx, db, id)
	if err != nil {
		return nil, err
	}
	result := &SSOTestResult{OK: true, TestedAt: s.timestamp()}
	check := func(name string, err error, success string) {
		item := SSOTestCheck{Name: name, OK: err == nil, Message: success}
		if err != nil {
			item.Message = err.Error()
			result.OK = false
		}
		result.Checks = append(result.Checks, item)
	}

	doc, discoveryErr := s.fetchDiscovery(ctx, record.Issuer)
	switch {
	case discoveryErr == nil:
		check("discovery", nil, "discovery document found and issuer matches")
		mismatch := []string{}
		if doc.AuthorizationEndpoint != record.AuthorizationEndpoint {
			mismatch = append(mismatch, "authorization_endpoint")
		}
		if doc.TokenEndpoint != record.TokenEndpoint {
			mismatch = append(mismatch, "token_endpoint")
		}
		if doc.JWKSURI != record.JWKSURI {
			mismatch = append(mismatch, "jwks_uri")
		}
		if len(mismatch) > 0 {
			check("endpoints", fmt.Errorf("configured %s differ from the discovery document", strings.Join(mismatch, ", ")), "")
		} else {
			check("endpoints", nil, "configured endpoints match the discovery document")
		}
		if len(doc.CodeChallengeMethodsSupported) > 0 && !containsString(doc.CodeChallengeMethodsSupported, "S256") {
			check("pkce", errors.New("provider does not advertise PKCE S256 support"), "")
		} else {
			check("pkce", nil, "PKCE S256 supported")
		}
	case errors.Is(discoveryErr, errOIDCProviderUnavailable):
		// Discovery is optional for providers configured by hand.
		result.Checks = append(result.Checks, SSOTestCheck{Name: "discovery", OK: true, Message: "no discovery document; using configured endpoints"})
	default:
		check("discovery", discoveryErr, "")
	}

	keys, jwksErr := s.fetchJWKS(ctx, record.JWKSURI)
	check("jwks", jwksErr, fmt.Sprintf("%d signing key(s) available", len(keys)))
	if jwksErr == nil {
		s.jwks.forget(record.JWKSURI)
	}

	if record.TokenEndpointAuthMethod == "none" {
		check("client_secret", nil, "public client; no secret required")
	} else {
		_, secretErr := s.decryptSecret(record.ID, record.clientSecretEncrypted)
		check("client_secret", secretErr, "client secret is stored encrypted and can be decrypted")
	}
	check("redirect_uri", s.validateRedirectURI(record.ID, record.RedirectURI), "redirect URI is valid: "+record.RedirectURI)

	var failure any
	if !result.OK {
		messages := []string{}
		for _, item := range result.Checks {
			if !item.OK {
				messages = append(messages, item.Name+": "+item.Message)
			}
		}
		failure = truncate(strings.Join(messages, "; "), 1000)
	}
	_, err = db.ExecContext(ctx, `UPDATE sso_connections SET last_tested_at = ?, last_test_ok = ?, last_test_error = ? WHERE id = ?`,
		result.TestedAt, boolInt(result.OK), failure, id)
	return result, err
}

// ---------------------------------------------------------------------------
// Identities
// ---------------------------------------------------------------------------

const ssoIdentityColumns = `id, user_id, organization_id, sso_connection_id, provider_subject, provider_email,
	provider_email_verified, last_login_at, created_at, updated_at`

func scanSSOIdentity(row rowScanner) (*SSOIdentity, error) {
	var identity SSOIdentity
	var verified int
	err := row.Scan(&identity.ID, &identity.UserID, &identity.OrganizationID, &identity.SSOConnectionID, &identity.ProviderSubject,
		&identity.ProviderEmail, &verified, &identity.LastLoginAt, &identity.CreatedAt, &identity.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSSOIdentityNotFound
	}
	identity.ProviderEmailVerified = verified == 1
	return &identity, err
}

func (s *SSOService) ListIdentities(ctx context.Context, userID, connectionID string) ([]SSOIdentity, error) {
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	clauses, args := []string{"1 = 1"}, []any{}
	if userID != "" {
		clauses, args = append(clauses, "user_id = ?"), append(args, userID)
	}
	if connectionID != "" {
		clauses, args = append(clauses, "sso_connection_id = ?"), append(args, connectionID)
	}
	rows, err := db.QueryContext(ctx, `SELECT `+ssoIdentityColumns+` FROM sso_user_identities WHERE `+
		strings.Join(clauses, " AND ")+` ORDER BY created_at DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	identities := []SSOIdentity{}
	for rows.Next() {
		identity, err := scanSSOIdentity(rows)
		if err != nil {
			return nil, err
		}
		identities = append(identities, *identity)
	}
	return identities, rows.Err()
}

// LinkIdentity is the administrator pre-provisioning path (SSO.md "Account
// Linking": "Administrator pre-provisions the user and assigns the SSO
// identity"). It is the only way to link a protected or all-permission user.
func (s *SSOService) LinkIdentity(ctx context.Context, actorUserID string, input LinkSSOIdentityInput, request SSORequestContext) (*SSOIdentity, error) {
	input.UserID, input.SSOConnectionID = strings.TrimSpace(input.UserID), strings.TrimSpace(input.SSOConnectionID)
	input.ProviderSubject, input.ProviderEmail = strings.TrimSpace(input.ProviderSubject), strings.TrimSpace(input.ProviderEmail)
	if input.UserID == "" || input.SSOConnectionID == "" || input.ProviderSubject == "" {
		return nil, ssoInvalid("user_id, sso_connection_id and provider_subject are required")
	}
	if len(input.ProviderSubject) > 255 {
		return nil, ssoInvalid("provider_subject must be at most 255 characters")
	}
	user, err := s.users.Get(ctx, input.UserID)
	if err != nil {
		return nil, err
	}
	if user.DeletedAt != nil {
		return nil, ErrUserNotFound
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	connection, err := s.loadConnection(ctx, db, input.SSOConnectionID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	identity, err := s.insertIdentity(ctx, tx, connection, input.UserID, input.ProviderSubject, input.ProviderEmail, false)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, ssoInvalid("this provider subject is already linked, or the user already has an identity on this connection")
		}
		return nil, err
	}
	if err := s.recordAudit(ctx, tx, ssoAuditEntry{EventType: "sso.identity.linked", OrganizationID: connection.OrganizationID,
		UserID: input.UserID, ConnectionID: connection.ID, Request: request,
		Detail: "strategy=administrator actor=" + actorUserID}); err != nil {
		return nil, err
	}
	return identity, tx.Commit()
}

func (s *SSOService) UnlinkIdentity(ctx context.Context, actorUserID, identityID string, request SSORequestContext) error {
	db, err := s.open()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	identity, err := scanSSOIdentity(tx.QueryRowContext(ctx, `SELECT `+ssoIdentityColumns+` FROM sso_user_identities WHERE id = ?`, identityID))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sso_user_identities WHERE id = ?`, identityID); err != nil {
		return err
	}
	if err := s.recordAudit(ctx, tx, ssoAuditEntry{EventType: "sso.identity.unlinked", OrganizationID: identity.OrganizationID,
		UserID: identity.UserID, ConnectionID: identity.SSOConnectionID, Request: request, Detail: "actor=" + actorUserID}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SSOService) insertIdentity(ctx context.Context, db sqlExecer, connection *ssoConnectionRecord, userID, subject, email string, verified bool) (*SSOIdentity, error) {
	now := s.timestamp()
	identity := &SSOIdentity{ID: uuid.NewString(), UserID: userID, OrganizationID: connection.OrganizationID,
		SSOConnectionID: connection.ID, ProviderSubject: subject, ProviderEmailVerified: verified, CreatedAt: now, UpdatedAt: now}
	if email != "" {
		identity.ProviderEmail = &email
	}
	_, err := db.ExecContext(ctx, `INSERT INTO sso_user_identities (`+ssoIdentityColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		identity.ID, userID, connection.OrganizationID, connection.ID, subject, identity.ProviderEmail, boolInt(verified), nil, now, now)
	if err != nil {
		return nil, err
	}
	return identity, nil
}

// ---------------------------------------------------------------------------
// Login flow (SSO.md "Login Flow")
// ---------------------------------------------------------------------------

// StartLogin creates the one-time state, nonce and PKCE verifier and returns
// the provider authorization URL (steps 3–5).
func (s *SSOService) StartLogin(ctx context.Context, connectionID, returnTo string, request SSORequestContext) (string, error) {
	db, err := s.open()
	if err != nil {
		return "", err
	}
	defer db.Close()
	connection, err := s.loadConnection(ctx, db, connectionID)
	if errors.Is(err, ErrSSOConnectionNotFound) {
		return "", &SSOLoginError{Code: SSOErrorConnectionDisabled, Reason: "unknown connection", ConnectionID: connectionID}
	}
	if err != nil {
		return "", err
	}
	if err := s.requireUsableConnection(ctx, db, connection); err != nil {
		return "", err
	}
	state, err := randomToken(32)
	if err != nil {
		return "", err
	}
	nonce, err := randomToken(32)
	if err != nil {
		return "", err
	}
	verifier, err := randomToken(32)
	if err != nil {
		return "", err
	}
	challenge := sha256.Sum256([]byte(verifier))
	now := s.now()
	if _, err := db.ExecContext(ctx, `DELETE FROM sso_login_states WHERE expires_at < ?`, now.Format(time.RFC3339)); err != nil {
		return "", err
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sso_login_states (state_hash, sso_connection_id, code_verifier, nonce, return_to,
		ip_address, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, tokenHash(state), connection.ID, verifier, nonce,
		SanitizeSSOReturnTo(returnTo), nullIfEmpty(request.IPAddress), now.Format(time.RFC3339),
		now.Add(ssoLoginStateLifetime).Format(time.RFC3339)); err != nil {
		return "", err
	}
	authorizationURL, err := url.Parse(connection.AuthorizationEndpoint)
	if err != nil {
		return "", err
	}
	query := authorizationURL.Query()
	query.Set("response_type", "code")
	query.Set("client_id", connection.ClientID)
	query.Set("redirect_uri", connection.RedirectURI)
	query.Set("scope", strings.Join(connection.Scopes, " "))
	query.Set("state", state)
	query.Set("nonce", nonce)
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	query.Set("code_challenge_method", "S256")
	authorizationURL.RawQuery = query.Encode()
	if err := s.recordAudit(ctx, db, ssoAuditEntry{EventType: "sso.login.started", OrganizationID: connection.OrganizationID,
		ConnectionID: connection.ID, Request: request}); err != nil {
		return "", err
	}
	return authorizationURL.String(), nil
}

func (s *SSOService) requireUsableConnection(ctx context.Context, db *sql.DB, connection *ssoConnectionRecord) error {
	if !connection.IsEnabled {
		return &SSOLoginError{Code: SSOErrorConnectionDisabled, Reason: "connection is disabled",
			ConnectionID: connection.ID, OrganizationID: connection.OrganizationID}
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM organizations WHERE id = ?`, connection.OrganizationID).Scan(&status); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if status != "active" {
		return &SSOLoginError{Code: SSOErrorConnectionDisabled, Reason: "organization is not active",
			ConnectionID: connection.ID, OrganizationID: connection.OrganizationID}
	}
	return nil
}

// SanitizeSSOReturnTo only accepts same-origin paths, so the post-login
// redirect cannot be turned into an open redirect.
func SanitizeSSOReturnTo(returnTo string) string {
	returnTo = strings.TrimSpace(returnTo)
	if returnTo == "" || len(returnTo) > 500 || !strings.HasPrefix(returnTo, "/") ||
		strings.HasPrefix(returnTo, "//") || strings.HasPrefix(returnTo, "/\\") || strings.ContainsAny(returnTo, "\\\r\n\t") {
		return ssoDefaultReturnTo
	}
	parsed, err := url.Parse(returnTo)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || strings.HasPrefix(parsed.Path, "/login") || strings.HasPrefix(parsed.Path, "/api/") {
		return ssoDefaultReturnTo
	}
	return returnTo
}

type ssoLoginState struct {
	connectionID string
	codeVerifier string
	nonce        string
	returnTo     string
	expiresAt    string
}

// CompleteLogin handles the provider callback (steps 7–11). The state row is
// consumed before anything else, so every state value is single-use even when
// the callback fails.
func (s *SSOService) CompleteLogin(ctx context.Context, connectionID string, params url.Values, request SSORequestContext) (*SSOLoginResult, error) {
	fail := func(code, reason string, connection *ssoConnectionRecord, userID, email string) error {
		loginErr := &SSOLoginError{Code: code, Reason: reason, ConnectionID: connectionID, UserID: userID, Email: email}
		if connection != nil {
			loginErr.OrganizationID = connection.OrganizationID
		}
		return loginErr
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	stateValue := params.Get("state")
	if stateValue == "" || len(stateValue) > 512 {
		return nil, s.auditFailure(ctx, db, fail(SSOErrorInvalidRequest, "missing state", nil, "", ""), request)
	}
	var state ssoLoginState
	err = db.QueryRowContext(ctx, `DELETE FROM sso_login_states WHERE state_hash = ?
		RETURNING sso_connection_id, code_verifier, nonce, return_to, expires_at`, tokenHash(stateValue)).
		Scan(&state.connectionID, &state.codeVerifier, &state.nonce, &state.returnTo, &state.expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, s.auditFailure(ctx, db, fail(SSOErrorInvalidRequest, "unknown or already used state", nil, "", ""), request)
	}
	if err != nil {
		return nil, err
	}
	if state.connectionID != connectionID {
		return nil, s.auditFailure(ctx, db, fail(SSOErrorInvalidRequest, "state belongs to another connection", nil, "", ""), request)
	}
	if expires, err := time.Parse(time.RFC3339, state.expiresAt); err != nil || !s.now().Before(expires) {
		return nil, s.auditFailure(ctx, db, fail(SSOErrorInvalidRequest, "state expired", nil, "", ""), request)
	}

	connection, err := s.loadConnection(ctx, db, connectionID)
	if errors.Is(err, ErrSSOConnectionNotFound) {
		return nil, s.auditFailure(ctx, db, fail(SSOErrorConnectionDisabled, "connection was deleted", nil, "", ""), request)
	}
	if err != nil {
		return nil, err
	}
	if err := s.requireUsableConnection(ctx, db, connection); err != nil {
		return nil, s.auditFailure(ctx, db, err, request)
	}
	if providerError := params.Get("error"); providerError != "" {
		return nil, s.auditFailure(ctx, db, fail(SSOErrorProviderError, "provider returned error "+safeOAuthErrorCode(providerError), connection, "", ""), request)
	}
	code := params.Get("code")
	if code == "" || len(code) > 4096 {
		return nil, s.auditFailure(ctx, db, fail(SSOErrorInvalidRequest, "missing authorization code", connection, "", ""), request)
	}

	secret, err := s.decryptSecret(connection.ID, connection.clientSecretEncrypted)
	if err != nil {
		return nil, s.auditFailure(ctx, db, fail(SSOErrorProviderUnavailable, "client secret unavailable: "+err.Error(), connection, "", ""), request)
	}
	tokens, err := s.exchangeCode(ctx, connection, secret, code, state.codeVerifier)
	if err != nil {
		return nil, s.auditFailure(ctx, db, fail(providerFailureCode(err), err.Error(), connection, "", ""), request)
	}
	claims, err := s.verifyIDToken(ctx, connection, tokens.IDToken, state.nonce)
	if err != nil {
		return nil, s.auditFailure(ctx, db, fail(providerFailureCode(err), err.Error(), connection, "", ""), request)
	}
	profile := readSSOProfile(claims, connection.ClaimMapping)

	userID, err := s.resolveUser(ctx, db, connection, profile, request)
	if err != nil {
		var loginErr *SSOLoginError
		if errors.As(err, &loginErr) {
			loginErr.ConnectionID, loginErr.OrganizationID, loginErr.Email = connection.ID, connection.OrganizationID, profile.email
			return nil, s.auditFailure(ctx, db, loginErr, request)
		}
		return nil, err
	}
	sessionTokens, err := s.auth.IssueSSOSession(ctx, userID, request.IPAddress, request.UserAgent)
	if err != nil {
		code := SSOErrorAccountNotAllowed
		if errors.Is(err, ErrAccountPending) {
			code = SSOErrorAccountPending
		} else if !errors.Is(err, ErrAccountInactive) && !errors.Is(err, ErrAccountLocked) && !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
		return nil, s.auditFailure(ctx, db, fail(code, "session refused: "+err.Error(), connection, userID, profile.email), request)
	}
	now := s.timestamp()
	if _, err := db.ExecContext(ctx, `UPDATE sso_user_identities SET last_login_at = ?, provider_email = ?, provider_email_verified = ?,
		updated_at = ? WHERE sso_connection_id = ? AND provider_subject = ?`, now, nullIfEmpty(profile.email),
		boolInt(profile.emailVerified), now, connection.ID, profile.subject); err != nil {
		return nil, err
	}
	synced, err := s.syncGroups(ctx, connection, userID, profile.groups)
	if err != nil {
		return nil, err
	}
	detail := ""
	if len(synced) > 0 {
		detail = "groups_added=" + strings.Join(synced, ",")
	}
	if err := s.recordAudit(ctx, db, ssoAuditEntry{EventType: "sso.login.succeeded", OrganizationID: connection.OrganizationID,
		UserID: userID, ConnectionID: connection.ID, Request: request, Detail: detail}); err != nil {
		return nil, err
	}
	return &SSOLoginResult{Tokens: sessionTokens, ReturnTo: state.returnTo, UserID: userID, ConnectionID: connection.ID, Email: profile.email}, nil
}

func providerFailureCode(err error) string {
	switch {
	case errors.Is(err, errOIDCProviderUnavailable):
		return SSOErrorProviderUnavailable
	default:
		return SSOErrorProviderError
	}
}

func (s *SSOService) auditFailure(ctx context.Context, db *sql.DB, err error, request SSORequestContext) error {
	var loginErr *SSOLoginError
	if errors.As(err, &loginErr) {
		detail := ""
		if loginErr.Email != "" {
			detail = "provider_email=" + loginErr.Email
		}
		_ = s.recordAudit(ctx, db, ssoAuditEntry{EventType: "sso.login.failed", OrganizationID: loginErr.OrganizationID,
			UserID: loginErr.UserID, ConnectionID: loginErr.ConnectionID, Request: request,
			FailureReason: loginErr.Code + ": " + loginErr.Reason, Detail: detail})
	}
	return err
}

type ssoProfile struct {
	subject       string
	email         string
	emailVerified bool
	name          string
	groups        []string
}

func readSSOProfile(claims jwt.MapClaims, mapping SSOClaimMapping) ssoProfile {
	profile := ssoProfile{}
	profile.subject, _ = claims.GetSubject()
	if email, ok := claims[mapping.Email].(string); ok {
		if parsed, err := mail.ParseAddress(strings.TrimSpace(email)); err == nil && parsed.Address == strings.TrimSpace(email) {
			profile.email = strings.ToLower(parsed.Address)
		}
	}
	switch verified := claims[mapping.EmailVerified].(type) {
	case bool:
		profile.emailVerified = verified
	case string:
		profile.emailVerified = strings.EqualFold(verified, "true")
	}
	if name, ok := claims[mapping.Name].(string); ok {
		profile.name = truncate(strings.TrimSpace(name), 100)
	}
	switch groups := claims[mapping.Groups].(type) {
	case []any:
		for _, group := range groups {
			if value, ok := group.(string); ok && value != "" {
				profile.groups = append(profile.groups, value)
			}
		}
	case string:
		if groups != "" {
			profile.groups = []string{groups}
		}
	}
	return profile
}

// resolveUser applies SSO.md "Account Linking" in order:
//  1. an identity already linked to (connection, subject);
//  2. exactly one existing user whose email matches a verified provider email;
//  3. just-in-time provisioning when the connection allows it;
//
// otherwise login is denied.
func (s *SSOService) resolveUser(ctx context.Context, db *sql.DB, connection *ssoConnectionRecord, profile ssoProfile, request SSORequestContext) (string, error) {
	deny := func(code, reason string) error { return &SSOLoginError{Code: code, Reason: reason} }
	var userID string
	err := db.QueryRowContext(ctx, `SELECT user_id FROM sso_user_identities WHERE sso_connection_id = ? AND provider_subject = ?`,
		connection.ID, profile.subject).Scan(&userID)
	if err == nil {
		return userID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	if profile.email == "" {
		return "", deny(SSOErrorAccountNotAllowed, "provider returned no usable email and no identity is linked")
	}
	if !profile.emailVerified {
		return "", deny(SSOErrorAccountNotAllowed, "provider email is not verified; refusing to link by email")
	}
	if !connection.emailDomainAllowed(profile.email) {
		return "", deny(SSOErrorAccountNotAllowed, "email domain is not allowed for this connection")
	}

	candidates, err := s.usersByEmail(ctx, profile.email)
	if err != nil {
		return "", err
	}
	switch {
	case len(candidates) > 1:
		return "", deny(SSOErrorAccountNotAllowed, "multiple users match the provider email; administrator action required")
	case len(candidates) == 1:
		candidate := candidates[0]
		if candidate.privileged {
			return "", deny(SSOErrorAccountNotAllowed, "administrator accounts must be linked explicitly by an administrator")
		}
		var existing int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sso_user_identities WHERE sso_connection_id = ? AND user_id = ?`,
			connection.ID, candidate.id).Scan(&existing); err != nil {
			return "", err
		}
		if existing > 0 {
			return "", deny(SSOErrorAccountNotAllowed, "user is already linked to a different identity on this connection")
		}
		if err := s.linkAutomatically(ctx, db, connection, candidate.id, profile, "verified_email", request); err != nil {
			return "", err
		}
		return candidate.id, nil
	}

	if !connection.JITEnabled {
		return "", deny(SSOErrorAccountNotAllowed, "no matching user and just-in-time provisioning is disabled")
	}
	return s.provisionUser(ctx, db, connection, profile, request)
}

func (c *ssoConnectionRecord) emailDomainAllowed(email string) bool {
	if len(c.AllowedEmailDomains) == 0 {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	return containsString(c.AllowedEmailDomains, strings.ToLower(email[at+1:]))
}

type ssoUserCandidate struct {
	id         string
	privileged bool
}

// usersByEmail matches login emails and, as a secondary source, work emails
// on employee profiles, so "multiple users match" can genuinely happen and is
// then refused instead of guessed.
func (s *SSOService) usersByEmail(ctx context.Context, email string) ([]ssoUserCandidate, error) {
	db, err := s.users.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT u.id, u.is_protected,
			EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
				WHERE ur.user_id = u.id AND (r.grants_all_permissions = 1 OR r.id = ?))
		FROM users u LEFT JOIN employee_profiles p ON p.user_id = u.id
		WHERE u.deleted_at IS NULL AND (u.email = ? COLLATE NOCASE OR p.work_email = ? COLLATE NOCASE)`, adminRoleID, email, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candidates []ssoUserCandidate
	for rows.Next() {
		var candidate ssoUserCandidate
		var protected, admin int
		if err := rows.Scan(&candidate.id, &protected, &admin); err != nil {
			return nil, err
		}
		candidate.privileged = protected == 1 || admin == 1
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (s *SSOService) linkAutomatically(ctx context.Context, db *sql.DB, connection *ssoConnectionRecord, userID string, profile ssoProfile, strategy string, request SSORequestContext) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.insertIdentity(ctx, tx, connection, userID, profile.subject, profile.email, profile.emailVerified); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return &SSOLoginError{Code: SSOErrorAccountNotAllowed, Reason: "identity was linked concurrently; retry sign-in"}
		}
		return err
	}
	if err := s.recordAudit(ctx, tx, ssoAuditEntry{EventType: "sso.identity.linked", OrganizationID: connection.OrganizationID,
		UserID: userID, ConnectionID: connection.ID, Request: request, Detail: "strategy=" + strategy}); err != nil {
		return err
	}
	return tx.Commit()
}

// provisionUser implements SSO.md "Just-In-Time Provisioning". The account
// gets an unusable random password (SSO users never manage a NexGestion
// password), the connection's default role/group, and status pending when
// approval is required before first access.
func (s *SSOService) provisionUser(ctx context.Context, db *sql.DB, connection *ssoConnectionRecord, profile ssoProfile, request SSORequestContext) (string, error) {
	password, err := randomToken(32)
	if err != nil {
		return "", err
	}
	status := "active"
	if connection.JITRequireApproval {
		status = "pending"
	}
	displayName := profile.name
	if displayName == "" {
		displayName = profile.email[:strings.LastIndex(profile.email, "@")]
	}
	mustChange := false
	user, err := s.users.Create(ctx, "", CreateUserInput{DisplayName: displayName, Email: profile.email, Password: password,
		Status: status, MustChangePassword: &mustChange})
	if err != nil {
		return "", err
	}
	if err := s.assignProvisioningDefaults(ctx, connection, user.ID); err != nil {
		return "", err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := s.insertIdentity(ctx, tx, connection, user.ID, profile.subject, profile.email, profile.emailVerified); err != nil {
		return "", err
	}
	detail := "status=" + status
	if connection.JITDefaultRoleID != nil {
		detail += " role=" + *connection.JITDefaultRoleID
	}
	if connection.JITDefaultGroupID != nil {
		detail += " group=" + *connection.JITDefaultGroupID
	}
	for _, entry := range []ssoAuditEntry{
		{EventType: "sso.user.provisioned", Detail: detail},
		{EventType: "sso.identity.linked", Detail: "strategy=jit"},
	} {
		entry.OrganizationID, entry.UserID, entry.ConnectionID, entry.Request = connection.OrganizationID, user.ID, connection.ID, request
		if err := s.recordAudit(ctx, tx, entry); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	if status == "pending" {
		return "", &SSOLoginError{Code: SSOErrorAccountPending, Reason: "provisioned user awaits administrator approval", UserID: user.ID}
	}
	return user.ID, nil
}

func (s *SSOService) assignProvisioningDefaults(ctx context.Context, connection *ssoConnectionRecord, userID string) error {
	if connection.JITDefaultRoleID == nil && connection.JITDefaultGroupID == nil {
		return nil
	}
	// Re-check at use time: the role may have been changed to grant all
	// permissions after the connection was saved.
	if connection.JITDefaultRoleID != nil {
		if err := s.validateProvisioningRole(ctx, *connection.JITDefaultRoleID); err != nil {
			return err
		}
	}
	db, err := s.users.open()
	if err != nil {
		return err
	}
	defer db.Close()
	now := s.timestamp()
	if connection.JITDefaultRoleID != nil {
		if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO user_roles (user_id, role_id, created_at) VALUES (?, ?, ?)`,
			userID, *connection.JITDefaultRoleID, now); err != nil {
			return err
		}
	}
	if connection.JITDefaultGroupID != nil {
		if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO user_groups (user_id, group_id, joined_at, created_at)
			SELECT ?, id, ?, ? FROM groups WHERE id = ? AND status = 'active'`, userID, now[:10], now, *connection.JITDefaultGroupID); err != nil {
			return err
		}
	}
	return nil
}

// syncGroups is additive only and opt-in per connection (SSO.md
// "Authorization"): mapped provider groups add memberships; a group removed
// at the provider never removes the user or their memberships here.
func (s *SSOService) syncGroups(ctx context.Context, connection *ssoConnectionRecord, userID string, providerGroups []string) ([]string, error) {
	if !connection.GroupSyncEnabled || len(connection.GroupMapping) == 0 || len(providerGroups) == 0 {
		return nil, nil
	}
	db, err := s.users.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	now := s.timestamp()
	var added []string
	for _, providerGroup := range providerGroups {
		groupID, ok := connection.GroupMapping[providerGroup]
		if !ok {
			continue
		}
		result, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO user_groups (user_id, group_id, joined_at, created_at)
			SELECT ?, id, ?, ? FROM groups WHERE id = ? AND status = 'active'`, userID, now[:10], now, groupID)
		if err != nil {
			return nil, err
		}
		if count, _ := result.RowsAffected(); count > 0 {
			added = append(added, groupID)
		}
	}
	return added, nil
}

// ---------------------------------------------------------------------------
// Export / import with secrets redacted (SSO.md "Local-First Deployment
// Rules").
// ---------------------------------------------------------------------------

type SSOExport struct {
	Version     int             `json:"version"`
	ExportedAt  string          `json:"exported_at"`
	Connections []SSOConnection `json:"connections"`
}

type SSOImportResult struct {
	Imported []string           `json:"imported"`
	Skipped  []SSOImportSkipped `json:"skipped"`
}

type SSOImportSkipped struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

func (s *SSOService) Export(ctx context.Context) (*SSOExport, error) {
	connections, err := s.ListConnections(ctx)
	if err != nil {
		return nil, err
	}
	for index := range connections {
		connections[index].ClientSecretConfigured = false
		connections[index].LastTestedAt, connections[index].LastTestOK, connections[index].LastTestError = nil, nil, nil
	}
	return &SSOExport{Version: 1, ExportedAt: s.timestamp(), Connections: connections}, nil
}

// Import recreates connections under their original ids (so the redirect URIs
// already registered at the providers stay valid). Every imported connection
// is disabled and secretless until an administrator sets the secret, tests it
// and enables it. Existing ids are skipped, never overwritten.
func (s *SSOService) Import(ctx context.Context, actorUserID string, document SSOExport, request SSORequestContext) (*SSOImportResult, error) {
	if document.Version != 1 {
		return nil, ssoInvalid("unsupported export version %d", document.Version)
	}
	db, err := s.open()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	result := &SSOImportResult{Imported: []string{}, Skipped: []SSOImportSkipped{}}
	for _, item := range document.Connections {
		if _, err := uuid.Parse(item.ID); err != nil {
			result.Skipped = append(result.Skipped, SSOImportSkipped{ID: item.ID, Reason: "id is not a valid uuid"})
			continue
		}
		if _, err := s.loadConnection(ctx, db, item.ID); err == nil {
			result.Skipped = append(result.Skipped, SSOImportSkipped{ID: item.ID, Reason: "a connection with this id already exists"})
			continue
		} else if !errors.Is(err, ErrSSOConnectionNotFound) {
			return nil, err
		}
		record := &ssoConnectionRecord{SSOConnection: item}
		record.IsEnabled, record.ClientSecretConfigured, record.clientSecretEncrypted = false, false, ""
		record.LastTestedAt, record.LastTestOK, record.LastTestError = nil, nil, nil
		if record.OrganizationID == "" {
			record.OrganizationID = DefaultOrganizationID
		}
		record.Scopes = normalizeSSOScopes(record.Scopes)
		if record.AllowedEmailDomains == nil {
			record.AllowedEmailDomains = []string{}
		}
		if record.GroupMapping == nil {
			record.GroupMapping = map[string]string{}
		}
		if domains, err := normalizeSSODomains(record.AllowedEmailDomains); err != nil {
			result.Skipped = append(result.Skipped, SSOImportSkipped{ID: item.ID, Reason: err.Error()})
			continue
		} else {
			record.AllowedEmailDomains = domains
		}
		if err := s.validateConnection(ctx, record); err != nil {
			result.Skipped = append(result.Skipped, SSOImportSkipped{ID: item.ID, Reason: err.Error()})
			continue
		}
		if err := s.requireOrganization(ctx, db, record.OrganizationID); err != nil {
			result.Skipped = append(result.Skipped, SSOImportSkipped{ID: item.ID, Reason: err.Error()})
			continue
		}
		valid := true
		for _, endpoint := range []struct{ name, value string }{
			{"authorization_endpoint", record.AuthorizationEndpoint}, {"token_endpoint", record.TokenEndpoint}, {"jwks_uri", record.JWKSURI},
		} {
			if err := s.validateProviderURL(endpoint.name, endpoint.value, true); err != nil {
				result.Skipped = append(result.Skipped, SSOImportSkipped{ID: item.ID, Reason: err.Error()})
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		now := s.timestamp()
		record.CreatedAt, record.UpdatedAt = now, now
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		if err := insertSSOConnection(ctx, tx, record); err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := s.recordAudit(ctx, tx, ssoAuditEntry{EventType: "sso.connection.created", OrganizationID: record.OrganizationID,
			UserID: actorUserID, ConnectionID: record.ID, Request: request, Detail: "source=import"}); err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		result.Imported = append(result.Imported, record.ID)
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// small helpers
// ---------------------------------------------------------------------------

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableBoolInt(value *bool) any {
	if value == nil {
		return nil
	}
	return boolInt(*value)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func dedupeStrings(sorted []string) []string {
	result := []string{}
	for index, value := range sorted {
		if index == 0 || value != sorted[index-1] {
			result = append(result, value)
		}
	}
	return result
}
