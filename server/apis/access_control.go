package apis

import (
	"context"
	"net/http"
	"nexgestion/server/system"
)

// requirePermission authorizes a request using the union of permissions from
// every role assigned to the authenticated user. Authentication must run
// before this middleware so the access-token claims are available.
func requirePermission(users *system.UserService, permission string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := r.Context().Value(authContextKey{}).(*system.AccessClaims)
		if !ok || claims.Subject == "" {
			writeAuthError(w, system.ErrInvalidToken)
			return
		}
		allowed, err := userHasPermission(r.Context(), users, claims.Subject, permission)
		if err != nil {
			recordRequestLog(r, "error", "permission check failed: "+r.Method+" "+r.URL.Path+" requires "+permission)
			writeSystemError(w, err)
			return
		}
		if !allowed {
			recordRequestLog(r, "warning", "permission denied: "+r.Method+" "+r.URL.Path+" requires "+permission)
			writeSystemError(w, system.ErrPermissionDenied)
			return
		}
		next(w, r)
	}
}

// userHasPermission is the API authorization decision point. Effective
// permissions are the union of all roles assigned to the user.
func userHasPermission(ctx context.Context, users *system.UserService, userID, required string) (bool, error) {
	keys, err := users.EffectivePermissionKeys(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, key := range keys {
		if key == required {
			return true, nil
		}
	}
	return false, nil
}

func listPermissions(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := users.ListPermissions(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeSystemError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("permissions", items))
	}
}
func setRolePermission(users *system.UserService, grant bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := users.SetRolePermission(r.Context(), authenticatedUserID(r), r.PathValue("id"), r.PathValue("permissionId"), grant); err != nil {
			writeSystemError(w, err)
			return
		}
		recordRequestLog(r, "info", "changed role permission "+r.PathValue("permissionId")+" on "+r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	}
}
func listRoleUsers(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := users.ListRoleUsers(r.Context(), r.PathValue("id"), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeSystemError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("users", items))
	}
}
func setRoleUser(users *system.UserService, assign bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := users.SetRoleUser(r.Context(), authenticatedUserID(r), r.PathValue("id"), r.PathValue("userId"), assign); err != nil {
			writeSystemError(w, err)
			return
		}
		recordRequestLog(r, "info", "changed role user "+r.PathValue("userId")+" on "+r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	}
}
