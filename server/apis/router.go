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
	router.HandleFunc("GET /api/roles", protected(listRoles(users)))
	router.HandleFunc("GET /api/roles/{id}", protected(getRole(users)))
	router.HandleFunc("POST /api/roles", protected(createRole(users)))
	router.HandleFunc("PATCH /api/roles/{id}", protected(updateRole(users)))
	router.HandleFunc("DELETE /api/roles/{id}", protected(deleteRole(users)))
	router.HandleFunc("GET /api/roles/{id}/users", protected(listRoleUsers(users)))
	router.HandleFunc("PUT /api/roles/{id}/users/{userId}", protected(setRoleUser(users, true)))
	router.HandleFunc("DELETE /api/roles/{id}/users/{userId}", protected(setRoleUser(users, false)))
	router.HandleFunc("PUT /api/roles/{id}/permissions/{permissionId}", protected(setRolePermission(users, true)))
	router.HandleFunc("DELETE /api/roles/{id}/permissions/{permissionId}", protected(setRolePermission(users, false)))
	router.HandleFunc("GET /api/groups", protected(listGroups(users)))
	router.HandleFunc("GET /api/groups/{id}", protected(getGroup(users)))
	router.HandleFunc("POST /api/groups", protected(createGroup(users)))
	router.HandleFunc("PATCH /api/groups/{id}", protected(updateGroup(users)))
	router.HandleFunc("DELETE /api/groups/{id}", protected(deleteGroup(users)))
	router.HandleFunc("GET /api/groups/{id}/members", protected(listGroupMembers(users)))
	router.HandleFunc("PUT /api/groups/{id}/members/{userId}", protected(setGroupMember(users)))
	router.HandleFunc("DELETE /api/groups/{id}/members/{userId}", protected(removeGroupMember(users)))
	router.HandleFunc("PUT /api/groups/{id}/permissions/{permissionId}", protected(setGroupPermission(users, true)))
	router.HandleFunc("DELETE /api/groups/{id}/permissions/{permissionId}", protected(setGroupPermission(users, false)))
	router.HandleFunc("GET /api/permissions", protected(listPermissions(users)))
	router.HandleFunc("POST /api/permissions", protected(createPermission(users)))
	router.HandleFunc("PATCH /api/permissions/{id}", protected(updatePermission(users)))
	router.HandleFunc("GET /api/logs", protected(readLogs(logService)))

	// Keep unknown API paths inside the API layer instead of falling through to
	// the SPA handler.
	router.HandleFunc("/api/", NotFound)
}
