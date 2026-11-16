package system

// OIDC protocol code (SSO.md "Phase 1: OpenID Connect"). This file only speaks
// the protocol — discovery, JWKS, code exchange and ID token validation — and
// knows nothing about NexGestion users or sessions, so the core session system
// stays isolated from protocol details (SSO.md "Implementation Notes").

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	oidcHTTPTimeout      = 10 * time.Second
	oidcMaxResponseBytes = 1 << 20
	jwksCacheLifetime    = time.Hour
	jwksForcedRefreshGap = time.Minute
	idTokenClockLeeway   = time.Minute
)

// idTokenSigningMethods excludes "none" and every HMAC method: an ID token
// signed with the client secret could be forged by anyone holding the secret,
// and unsigned tokens are explicitly rejected (SSO.md "Rejected behavior").
var idTokenSigningMethods = []string{"RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA"}

var (
	errOIDCProviderUnavailable = errors.New("identity provider unavailable")
	errOIDCProviderRejected    = errors.New("identity provider rejected the request")
	errOIDCInvalidIDToken      = errors.New("invalid id token")
)

type oidcDiscoveryDocument struct {
	Issuer                        string   `json:"issuer"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	JWKSURI                       string   `json:"jwks_uri"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
}

type oidcTokenResponse struct {
	IDToken     string `json:"id_token"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

func oidcDiscoveryURL(issuer string) string {
	return strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
}

func (s *SSOService) fetchDiscovery(ctx context.Context, issuer string) (*oidcDiscoveryDocument, error) {
	var doc oidcDiscoveryDocument
	if err := s.getJSON(ctx, oidcDiscoveryURL(issuer), &doc); err != nil {
		return nil, err
	}
	// OpenID Connect Discovery 1.0 §4.3: the returned issuer must be identical
	// to the issuer the document was requested for.
	if doc.Issuer != issuer {
		return nil, fmt.Errorf("discovery issuer %q does not match configured issuer %q", doc.Issuer, issuer)
	}
	return &doc, nil
}

func (s *SSOService) getJSON(ctx context.Context, target string, destination any) error {
	ctx, cancel := context.WithTimeout(ctx, oidcHTTPTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := s.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", errOIDCProviderUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: GET %s returned HTTP %d", errOIDCProviderUnavailable, redactURL(target), response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, oidcMaxResponseBytes)).Decode(destination); err != nil {
		return fmt.Errorf("%w: decode %s: %v", errOIDCProviderUnavailable, redactURL(target), err)
	}
	return nil
}

// exchangeCode performs the backend-only code exchange (SSO.md "Access token
// exchange through the backend only"). Provider tokens never leave this
// function except the ID token, which is validated and then discarded.
func (s *SSOService) exchangeCode(ctx context.Context, connection *ssoConnectionRecord, clientSecret, code, codeVerifier string) (*oidcTokenResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {connection.RedirectURI},
		"code_verifier": {codeVerifier},
	}
	switch connection.TokenEndpointAuthMethod {
	case "client_secret_post":
		form.Set("client_id", connection.ClientID)
		form.Set("client_secret", clientSecret)
	case "none":
		form.Set("client_id", connection.ClientID)
	}
	ctx, cancel := context.WithTimeout(ctx, oidcHTTPTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, connection.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	if connection.TokenEndpointAuthMethod == "client_secret_basic" {
		// RFC 6749 §2.3.1 requires form-encoding both values before Basic auth.
		request.SetBasicAuth(url.QueryEscape(connection.ClientID), url.QueryEscape(clientSecret))
	}
	response, err := s.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: token endpoint: %v", errOIDCProviderUnavailable, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, oidcMaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: read token response: %v", errOIDCProviderUnavailable, err)
	}
	if response.StatusCode >= 500 {
		return nil, fmt.Errorf("%w: token endpoint returned HTTP %d", errOIDCProviderUnavailable, response.StatusCode)
	}
	if response.StatusCode != http.StatusOK {
		// Only the standard error code is kept; error_description may echo
		// request data and is never logged.
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &failure)
		return nil, fmt.Errorf("%w: token endpoint returned HTTP %d (%s)", errOIDCProviderRejected, response.StatusCode, safeOAuthErrorCode(failure.Error))
	}
	var tokens oidcTokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return nil, fmt.Errorf("%w: decode token response: %v", errOIDCProviderUnavailable, err)
	}
	if tokens.IDToken == "" {
		return nil, fmt.Errorf("%w: token response has no id_token", errOIDCInvalidIDToken)
	}
	return &tokens, nil
}

// verifyIDToken validates signature (via JWKS), issuer, audience, authorized
// party, expiry, issued-at and nonce, and returns the token's claims.
func (s *SSOService) verifyIDToken(ctx context.Context, connection *ssoConnectionRecord, rawToken, expectedNonce string) (jwt.MapClaims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods(idTokenSigningMethods),
		jwt.WithIssuer(connection.Issuer),
		jwt.WithAudience(connection.ClientID),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(idTokenClockLeeway),
	)
	claims := jwt.MapClaims{}
	token, err := parser.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		return s.jwks.key(ctx, s, connection.JWKSURI, kid, token.Method.Alg())
	})
	if err != nil {
		if errors.Is(err, errOIDCProviderUnavailable) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", errOIDCInvalidIDToken, err)
	}
	if !token.Valid {
		return nil, errOIDCInvalidIDToken
	}
	audience, _ := claims.GetAudience()
	authorizedParty, _ := claims["azp"].(string)
	if (len(audience) > 1 || authorizedParty != "") && authorizedParty != connection.ClientID {
		return nil, fmt.Errorf("%w: azp does not match client_id", errOIDCInvalidIDToken)
	}
	nonce, _ := claims["nonce"].(string)
	if subtle.ConstantTimeCompare([]byte(nonce), []byte(expectedNonce)) != 1 {
		return nil, fmt.Errorf("%w: nonce mismatch", errOIDCInvalidIDToken)
	}
	subject, _ := claims.GetSubject()
	if subject == "" || len(subject) > 255 {
		return nil, fmt.Errorf("%w: missing or oversized sub", errOIDCInvalidIDToken)
	}
	return claims, nil
}

// jwksCache keeps each provider's signing keys for an hour and refetches early
// when a token names an unknown kid, which is how providers roll keys (SSO.md
// "JWKS key rotation support"). Forced refreshes are spaced out so a flood of
// forged kids cannot turn NexGestion into a request amplifier.
type jwksCache struct {
	mu      sync.Mutex
	entries map[string]*jwksEntry
}

type jwksEntry struct {
	keys        []parsedJWK
	fetchedAt   time.Time
	lastForcing time.Time
}

type parsedJWK struct {
	kid string
	alg string
	key any
}

func newJWKSCache() *jwksCache { return &jwksCache{entries: map[string]*jwksEntry{}} }

func (c *jwksCache) key(ctx context.Context, s *SSOService, uri, kid, alg string) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := s.now()
	entry := c.entries[uri]
	if entry == nil || now.Sub(entry.fetchedAt) > jwksCacheLifetime {
		fresh, err := s.fetchJWKS(ctx, uri)
		if err != nil {
			return nil, err
		}
		entry = &jwksEntry{keys: fresh, fetchedAt: now}
		c.entries[uri] = entry
	}
	if key := selectJWK(entry.keys, kid, alg); key != nil {
		return key, nil
	}
	if now.Sub(entry.lastForcing) < jwksForcedRefreshGap {
		return nil, fmt.Errorf("no signing key matches kid %q", kid)
	}
	fresh, err := s.fetchJWKS(ctx, uri)
	if err != nil {
		return nil, err
	}
	entry.keys, entry.fetchedAt, entry.lastForcing = fresh, now, now
	if key := selectJWK(entry.keys, kid, alg); key != nil {
		return key, nil
	}
	return nil, fmt.Errorf("no signing key matches kid %q", kid)
}

func (c *jwksCache) forget(uri string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, uri)
}

// selectJWK picks the key named by kid. A token without kid is only accepted
// when exactly one published key fits the algorithm, so key choice is never
// ambiguous.
func selectJWK(keys []parsedJWK, kid, alg string) any {
	var candidates []parsedJWK
	for _, key := range keys {
		if key.alg != "" && key.alg != alg {
			continue
		}
		if !jwkFitsAlgorithm(key.key, alg) {
			continue
		}
		if kid != "" {
			if key.kid == kid {
				return key.key
			}
			continue
		}
		candidates = append(candidates, key)
	}
	if kid == "" && len(candidates) == 1 {
		return candidates[0].key
	}
	return nil
}

func jwkFitsAlgorithm(key any, alg string) bool {
	switch key.(type) {
	case *rsa.PublicKey:
		return strings.HasPrefix(alg, "RS") || strings.HasPrefix(alg, "PS")
	case *ecdsa.PublicKey:
		return strings.HasPrefix(alg, "ES")
	case ed25519.PublicKey:
		return alg == "EdDSA"
	}
	return false
}

func (s *SSOService) fetchJWKS(ctx context.Context, uri string) ([]parsedJWK, error) {
	var document struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := s.getJSON(ctx, uri, &document); err != nil {
		return nil, err
	}
	var keys []parsedJWK
	for _, raw := range document.Keys {
		if use, _ := raw["use"].(string); use != "" && use != "sig" {
			continue
		}
		key, err := parseJWK(raw)
		if err != nil {
			continue
		}
		kid, _ := raw["kid"].(string)
		alg, _ := raw["alg"].(string)
		keys = append(keys, parsedJWK{kid: kid, alg: alg, key: key})
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: JWKS at %s has no usable signing keys", errOIDCProviderUnavailable, redactURL(uri))
	}
	return keys, nil
}

func parseJWK(raw map[string]any) (any, error) {
	field := func(name string) ([]byte, error) {
		value, _ := raw[name].(string)
		if value == "" {
			return nil, fmt.Errorf("jwk missing %s", name)
		}
		return base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
	}
	kty, _ := raw["kty"].(string)
	switch kty {
	case "RSA":
		n, err := field("n")
		if err != nil {
			return nil, err
		}
		e, err := field("e")
		if err != nil {
			return nil, err
		}
		exponent := new(big.Int).SetBytes(e)
		if !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Int64() > 1<<31-1 {
			return nil, errors.New("jwk has invalid RSA exponent")
		}
		key := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exponent.Int64())}
		if key.N.BitLen() < 2048 {
			return nil, errors.New("jwk RSA modulus shorter than 2048 bits")
		}
		return key, nil
	case "EC":
		crv, _ := raw["crv"].(string)
		var curve elliptic.Curve
		switch crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("unsupported EC curve %q", crv)
		}
		x, err := field("x")
		if err != nil {
			return nil, err
		}
		y, err := field("y")
		if err != nil {
			return nil, err
		}
		key := &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !curve.IsOnCurve(key.X, key.Y) {
			return nil, errors.New("jwk EC point is not on curve")
		}
		return key, nil
	case "OKP":
		if crv, _ := raw["crv"].(string); crv != "Ed25519" {
			return nil, fmt.Errorf("unsupported OKP curve %q", crv)
		}
		x, err := field("x")
		if err != nil {
			return nil, err
		}
		if len(x) != ed25519.PublicKeySize {
			return nil, errors.New("jwk Ed25519 key has wrong size")
		}
		return ed25519.PublicKey(x), nil
	}
	return nil, fmt.Errorf("unsupported jwk kty %q", kty)
}

// redactURL drops query strings from URLs placed in error messages so
// diagnostics never carry codes or tokens (SSO.md "Error Handling").
func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "(invalid url)"
	}
	parsed.RawQuery, parsed.Fragment, parsed.User = "", "", nil
	return parsed.String()
}

// safeOAuthErrorCode keeps only RFC 6749 style error codes so a provider
// cannot smuggle arbitrary text into NexGestion's logs.
func safeOAuthErrorCode(code string) string {
	if code == "" || len(code) > 64 {
		return "unknown"
	}
	for _, r := range code {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return "unknown"
		}
	}
	return code
}
