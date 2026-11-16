package apis

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	applogs "nexgestion/server/logs"
	"nexgestion/server/system"
)

// ssoErrorRateLimited is only produced at the HTTP layer; the other codes come
// from system.SSOLoginError.
const ssoErrorRateLimited = "rate_limited"

// RegisterSSORoutes wires docs/System/SSO.md "API Contract". The three
// /api/auth/sso endpoints are public by necessity (the user has no session
// yet) and are rate limited per source IP; everything under /api/admin/sso
// requires sso.read or sso.manage.
func RegisterSSORoutes(router *http.ServeMux, sso *system.SSOService, users *system.UserService, security *system.SecurityService, logService *applogs.Service, protected func(string, http.HandlerFunc) http.HandlerFunc) {
	limiter := newIPRateLimiter(20, time.Minute)

	router.HandleFunc("GET /api/auth/sso/connections", func(w http.ResponseWriter, r *http.Request) {
		connections, err := sso.ListPublicConnections(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": connections})
	})

	router.HandleFunc("POST /api/auth/sso/{connectionId}/start", func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow("start:" + clientIP(r)) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": ssoUserMessage(ssoErrorRateLimited), "code": ssoErrorRateLimited})
			return
		}
		var input struct {
			ReturnTo string `json:"return_to"`
		}
		if err := decodeOptionalJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		connectionID := r.PathValue("connectionId")
		authorizationURL, err := sso.StartLogin(r.Context(), connectionID, input.ReturnTo, ssoRequestContext(r))
		if err != nil {
			var loginErr *system.SSOLoginError
			if errors.As(err, &loginErr) {
				_ = logService.With(clientIP(r), "").Log("warning", "sso login start refused for connection "+connectionID+": "+loginErr.Code)
				writeJSON(w, http.StatusConflict, map[string]string{"error": ssoUserMessage(loginErr.Code), "code": loginErr.Code})
				return
			}
			_ = logService.With(clientIP(r), "").Log("error", "sso login start failed for connection "+connectionID+": "+err.Error())
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		_ = logService.With(clientIP(r), "").Log("info", "sso login started for connection "+connectionID)
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]string{"authorization_url": authorizationURL})
	})

	router.HandleFunc("GET /api/auth/sso/{connectionId}/callback", func(w http.ResponseWriter, r *http.Request) {
		// The callback URL carries the authorization code: never cache it and
		// never leak it through Referer on the next navigation.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		connectionID := r.PathValue("connectionId")
		if !limiter.allow("callback:" + clientIP(r)) {
			redirectSSOFailure(w, r, ssoErrorRateLimited)
			return
		}
		result, err := sso.CompleteLogin(r.Context(), connectionID, r.URL.Query(), ssoRequestContext(r))
		if err != nil {
			var loginErr *system.SSOLoginError
			if !errors.As(err, &loginErr) {
				_ = logService.With(clientIP(r), "").Log("error", "sso callback failed for connection "+connectionID+": "+err.Error())
				redirectSSOFailure(w, r, system.SSOErrorProviderUnavailable)
				return
			}
			_ = logService.With(clientIP(r), loginErr.UserID).Log("warning", "sso login failed for connection "+connectionID+": "+loginErr.Code)
			recordRef := loginErr.Email
			if recordRef == "" {
				recordRef = "sso:" + connectionID
			}
			_, _ = security.RecordEvent(r.Context(), system.SecurityEventInput{
				EventType: "login_failed", Severity: "warning", ActorUserID: loginErr.UserID, SourceIP: clientIP(r),
				Module: "sso", RecordRef: recordRef, Summary: "failed SSO login (" + loginErr.Code + ")",
			})
			redirectSSOFailure(w, r, loginErr.Code)
			return
		}
		_ = logService.With(clientIP(r), result.UserID).Log("info", "sso login succeeded via connection "+connectionID)
		privileged := false
		if keys, err := users.EffectivePermissionKeys(r.Context(), result.UserID); err == nil {
			for _, key := range keys {
				if isPrivilegedPermissionKey(key) {
					privileged = true
					break
				}
			}
		}
		_, _ = security.RecordEvent(r.Context(), system.SecurityEventInput{
			EventType: "login_succeeded", Severity: "info", ActorUserID: result.UserID, SourceIP: clientIP(r),
			Module: "sso", Summary: "SSO login succeeded", Privileged: privileged,
		})
		setRefreshCookie(w, r, result.Tokens.RefreshToken, int(system.RefreshTokenLifetime.Seconds()))
		// The SPA restores the session from the refresh cookie on load, exactly
		// as after a page reload following password login; no token travels in
		// the URL.
		http.Redirect(w, r, system.SanitizeSSOReturnTo(result.ReturnTo), http.StatusSeeOther)
	})

	handle := func(pattern, permission string, status int, fn func(*http.Request) (any, error)) {
		router.HandleFunc(pattern, protected(permission, func(w http.ResponseWriter, r *http.Request) {
			result, err := fn(r)
			if err != nil {
				writeSSOError(w, err)
				return
			}
			if r.Method != http.MethodGet {
				recordRequestLog(r, "info", "sso administration "+r.Method+" "+r.URL.Path)
			}
			if result == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			writeJSON(w, status, result)
		}))
	}

	handle("GET /api/admin/sso/connections", "sso.read", http.StatusOK, func(r *http.Request) (any, error) {
		connections, err := sso.ListConnections(r.Context())
		return map[string]any{"items": connections}, err
	})
	handle("GET /api/admin/sso/connections/{id}", "sso.read", http.StatusOK, func(r *http.Request) (any, error) {
		return sso.GetConnection(r.Context(), r.PathValue("id"))
	})
	handle("POST /api/admin/sso/connections", "sso.manage", http.StatusCreated, func(r *http.Request) (any, error) {
		var input system.SSOConnectionInput
		if err := decodeSSOJSON(r, &input); err != nil {
			return nil, err
		}
		return sso.CreateConnection(r.Context(), authenticatedUserID(r), input, ssoRequestContext(r))
	})
	handle("PATCH /api/admin/sso/connections/{id}", "sso.manage", http.StatusOK, func(r *http.Request) (any, error) {
		var input system.SSOConnectionInput
		if err := decodeSSOJSON(r, &input); err != nil {
			return nil, err
		}
		return sso.UpdateConnection(r.Context(), authenticatedUserID(r), r.PathValue("id"), input, ssoRequestContext(r))
	})
	handle("DELETE /api/admin/sso/connections/{id}", "sso.manage", http.StatusNoContent, func(r *http.Request) (any, error) {
		return nil, sso.DeleteConnection(r.Context(), authenticatedUserID(r), r.PathValue("id"), ssoRequestContext(r))
	})
	handle("POST /api/admin/sso/connections/{id}/test", "sso.manage", http.StatusOK, func(r *http.Request) (any, error) {
		return sso.TestConnection(r.Context(), authenticatedUserID(r), r.PathValue("id"))
	})

	handle("GET /api/admin/sso/identities", "sso.read", http.StatusOK, func(r *http.Request) (any, error) {
		query := r.URL.Query()
		identities, err := sso.ListIdentities(r.Context(), strings.TrimSpace(query.Get("user_id")), strings.TrimSpace(query.Get("connection_id")))
		return map[string]any{"items": identities}, err
	})
	handle("POST /api/admin/sso/identities", "sso.manage", http.StatusCreated, func(r *http.Request) (any, error) {
		var input system.LinkSSOIdentityInput
		if err := decodeSSOJSON(r, &input); err != nil {
			return nil, err
		}
		return sso.LinkIdentity(r.Context(), authenticatedUserID(r), input, ssoRequestContext(r))
	})
	handle("DELETE /api/admin/sso/identities/{id}", "sso.manage", http.StatusNoContent, func(r *http.Request) (any, error) {
		return nil, sso.UnlinkIdentity(r.Context(), authenticatedUserID(r), r.PathValue("id"), ssoRequestContext(r))
	})

	handle("GET /api/admin/sso/audit-events", "sso.read", http.StatusOK, func(r *http.Request) (any, error) {
		query := r.URL.Query()
		limit := 0
		if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 {
				return nil, &system.SSOValidationError{Message: "limit must be a positive integer"}
			}
			limit = parsed
		}
		events, err := sso.ListAuditEvents(r.Context(), system.SSOAuditQuery{
			EventType: strings.TrimSpace(query.Get("event_type")), ConnectionID: strings.TrimSpace(query.Get("connection_id")),
			UserID: strings.TrimSpace(query.Get("user_id")), Limit: limit,
		})
		return map[string]any{"items": events}, err
	})

	handle("GET /api/admin/sso/export", "sso.manage", http.StatusOK, func(r *http.Request) (any, error) {
		return sso.Export(r.Context())
	})
	handle("POST /api/admin/sso/import", "sso.manage", http.StatusOK, func(r *http.Request) (any, error) {
		var document system.SSOExport
		if err := decodeSSOJSON(r, &document); err != nil {
			return nil, err
		}
		return sso.Import(r.Context(), authenticatedUserID(r), document, ssoRequestContext(r))
	})
}

func ssoRequestContext(r *http.Request) system.SSORequestContext {
	return system.SSORequestContext{IPAddress: clientIP(r), UserAgent: r.UserAgent()}
}

// redirectSSOFailure sends the browser back to the login page with only a
// short failure code; the page maps it to a safe message.
func redirectSSOFailure(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?sso_error="+url.QueryEscape(code), http.StatusSeeOther)
}

// ssoUserMessage holds the user-facing wording from SSO.md "Error Handling".
func ssoUserMessage(code string) string {
	switch code {
	case system.SSOErrorConnectionDisabled:
		return "This SSO connection is currently disabled."
	case system.SSOErrorAccountNotAllowed:
		return "Your account is not allowed to access this organization."
	case system.SSOErrorAccountPending:
		return "Your account is awaiting administrator approval."
	case ssoErrorRateLimited:
		return "Too many sign-in attempts. Please wait a minute and try again."
	default:
		return "Unable to sign in with this provider."
	}
}

func writeSSOError(w http.ResponseWriter, err error) {
	var validation *system.SSOValidationError
	switch {
	case errors.As(err, &validation):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": validation.Message})
	case errors.Is(err, system.ErrSSOConnectionNotFound), errors.Is(err, system.ErrSSOIdentityNotFound), errors.Is(err, system.ErrUserNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "UNIQUE constraint failed"):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "data already exists"})
	default:
		writeSystemError(w, err)
	}
}

func decodeSSOJSON(r *http.Request, destination any) error {
	if err := decodeJSON(nil, r, destination); err != nil {
		return &system.SSOValidationError{Message: err.Error()}
	}
	return nil
}

// decodeOptionalJSON accepts an empty body as "no input".
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		return errors.New("request body is too large")
	}
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	return decodeJSON(w, r, destination)
}

// ipRateLimiter is a fixed-window limiter keyed by source IP and action
// (SSO.md "Login rate limiting"). State is in memory: a restart resets it,
// which is acceptable for a single-process, self-hosted server.
type ipRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*rateBucket
	swept   time.Time
}

type rateBucket struct {
	start time.Time
	count int
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{limit: limit, window: window, buckets: map[string]*rateBucket{}}
}

func (l *ipRateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.Sub(l.swept) > l.window {
		for bucketKey, bucket := range l.buckets {
			if now.Sub(bucket.start) > l.window {
				delete(l.buckets, bucketKey)
			}
		}
		l.swept = now
	}
	bucket := l.buckets[key]
	if bucket == nil || now.Sub(bucket.start) > l.window {
		l.buckets[key] = &rateBucket{start: now, count: 1}
		return true
	}
	bucket.count++
	return bucket.count <= l.limit
}
