package apis

import (
	"net/http"

	"nexgestion/server/system"
)

// InitRouter registers every API endpoint in one place.
//
// API handlers belong in this package and may call the system package to
// perform application operations. The router itself is only responsible for
// directing requests to the correct handler.
func InitRouter(router *http.ServeMux, users *system.UserService, auth *system.AuthService) {
	// Public endpoints.
	router.HandleFunc("GET /api/health", Health)
	router.HandleFunc("POST /api/auth/login", login(auth))
	router.HandleFunc("POST /api/auth/refresh", refresh(auth))

	// Protected endpoints. New business APIs must use requireAuth by default.
	router.HandleFunc("GET /api/auth/me", requireAuth(auth, me(users)))
	router.HandleFunc("POST /api/auth/logout", requireAuth(auth, logout(auth)))
	router.HandleFunc("GET /api/users", requireAuth(auth, listUsers(users)))
	router.HandleFunc("POST /api/users", requireAuth(auth, createUser(users)))
	router.HandleFunc("GET /api/users/{id}", requireAuth(auth, getUser(users)))
	router.HandleFunc("PUT /api/users/{id}", requireAuth(auth, updateUser(users)))
	router.HandleFunc("PATCH /api/users/{id}", requireAuth(auth, updateUser(users)))
	router.HandleFunc("DELETE /api/users/{id}", requireAuth(auth, deleteUser(users)))

	// Keep unknown API paths inside the API layer instead of falling through to
	// the SPA handler.
	router.HandleFunc("/api/", NotFound)
}
