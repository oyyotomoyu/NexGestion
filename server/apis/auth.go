package apis

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	applogs "nexgestion/server/logs"
	"nexgestion/server/system"
)

const refreshCookieName = "nexgestion_refresh_token"

type authContextKey struct{}

func login(auth *system.AuthService, logService *applogs.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		tokens, err := auth.Login(r.Context(), input.Email, input.Password, clientIP(r), r.UserAgent())
		if err != nil {
			_ = logService.With(clientIP(r), "").Log("warning", "login failed")
			writeAuthError(w, err)
			return
		}
		_ = logService.With(clientIP(r), tokens.UserID).Log("info", "login succeeded")
		setRefreshCookie(w, r, tokens.RefreshToken, int(system.RefreshTokenLifetime.Seconds()))
		writeTokenResponse(w, tokens)
	}
}

func refresh(auth *system.AuthService, logService *applogs.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(refreshCookieName)
		if err != nil {
			_ = logService.With(clientIP(r), "").Log("warning", "token refresh failed")
			writeAuthError(w, system.ErrInvalidToken)
			return
		}
		tokens, err := auth.Refresh(r.Context(), cookie.Value, clientIP(r), r.UserAgent())
		if err != nil {
			_ = logService.With(clientIP(r), "").Log("warning", "token refresh failed")
			clearRefreshCookie(w)
			writeAuthError(w, err)
			return
		}
		_ = logService.With(clientIP(r), tokens.UserID).Log("info", "token refreshed")
		setRefreshCookie(w, r, tokens.RefreshToken, int(system.RefreshTokenLifetime.Seconds()))
		writeTokenResponse(w, tokens)
	}
}

func logout(auth *system.AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(refreshCookieName); err == nil {
			_ = auth.Logout(r.Context(), cookie.Value)
		}
		recordRequestLog(r, "info", "logged out")
		clearRefreshCookie(w)
		w.WriteHeader(http.StatusNoContent)
	}
}

func withRequestLogger(service *applogs.Service, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, _ := r.Context().Value(authContextKey{}).(*system.AccessClaims)
		userID := ""
		if claims != nil {
			userID = claims.Subject
		}
		ctx := applogs.IntoContext(r.Context(), service.With(clientIP(r), userID))
		next(w, r.WithContext(ctx))
	}
}

func me(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(authContextKey{}).(*system.AccessClaims)
		if !ok {
			writeAuthError(w, system.ErrInvalidToken)
			return
		}
		user, err := users.Get(r.Context(), claims.Subject)
		if err != nil {
			writeAuthError(w, system.ErrInvalidToken)
			return
		}
		writeJSON(w, http.StatusOK, user)
	}
}

func requireAuth(auth *system.AuthService, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeAuthError(w, system.ErrInvalidToken)
			return
		}
		claims, err := auth.VerifyAccessToken(r.Context(), parts[1])
		if err != nil {
			writeAuthError(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), authContextKey{}, claims)
		next(w, r.WithContext(ctx))
	}
}

func writeTokenResponse(w http.ResponseWriter, tokens *system.AuthTokens) {
	writeJSON(w, http.StatusOK, map[string]any{"access_token": tokens.AccessToken, "token_type": "Bearer", "expires_in": tokens.ExpiresIn})
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, system.ErrInvalidCredentials), errors.Is(err, system.ErrInvalidToken):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
	case errors.Is(err, system.ErrAccountLocked), errors.Is(err, system.ErrAccountInactive):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}

func setRefreshCookie(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	secure := r.TLS != nil || strings.EqualFold(os.Getenv("NEXGESTION_SECURE_COOKIES"), "true")
	http.SetCookie(w, &http.Cookie{Name: refreshCookieName, Value: token, Path: "/api/auth", MaxAge: maxAge, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: secure})
}

func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: refreshCookieName, Value: "", Path: "/api/auth", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
