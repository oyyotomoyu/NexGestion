package apis

import (
	"net/http"

	applogs "nexgestion/server/logs"
	"nexgestion/server/system"
)

// InitRouter registers every API endpoint in one place.
//
// API handlers belong in this package and may call the system package to
// perform application operations. The router itself is only responsible for
// directing requests to the correct handler.
func InitRouter(router *http.ServeMux, users *system.UserService, auth *system.AuthService, logService *applogs.Service) {
	// Public endpoints.
	router.HandleFunc("GET /api/health", Health)
	router.HandleFunc("POST /api/auth/login", login(auth, logService))
	router.HandleFunc("POST /api/auth/refresh", refresh(auth, logService))

	// Protected endpoints. New business APIs must use requireAuth by default.
	protected := func(handler http.HandlerFunc) http.HandlerFunc {
		return requireAuth(auth, withRequestLogger(logService, handler))
	}
	router.HandleFunc("GET /api/auth/me", protected(me(users)))
	router.HandleFunc("POST /api/auth/logout", protected(logout(auth)))
	router.HandleFunc("GET /api/users", protected(listUsers(users)))
	router.HandleFunc("POST /api/users", protected(createUser(users)))
	router.HandleFunc("GET /api/users/{id}", protected(getUser(users)))
	router.HandleFunc("PUT /api/users/{id}", protected(updateUser(users)))
	router.HandleFunc("PATCH /api/users/{id}", protected(updateUser(users)))
	router.HandleFunc("DELETE /api/users/{id}", protected(deleteUser(users)))
	router.HandleFunc("GET /api/logs", protected(readLogs(logService)))

	// Keep unknown API paths inside the API layer instead of falling through to
	// the SPA handler.
	router.HandleFunc("/api/", NotFound)
}
