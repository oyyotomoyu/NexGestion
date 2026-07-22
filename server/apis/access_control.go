package apis

import (
	"net/http"
	"nexgestion/server/system"
)

func listPermissions(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := users.ListPermissions(r.Context())
		if err != nil {
			writeSystemError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"permissions": items})
	}
}
func createPermission(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreatePermissionInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := users.CreatePermission(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeSystemError(w, err)
			return
		}
		recordRequestLog(r, "info", "created permission "+item.ID)
		writeJSON(w, http.StatusCreated, item)
	}
}
func updatePermission(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdatePermissionInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		item, err := users.UpdatePermission(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
		if err != nil {
			writeSystemError(w, err)
			return
		}
		recordRequestLog(r, "info", "updated permission "+item.ID)
		writeJSON(w, http.StatusOK, item)
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
func setGroupPermission(users *system.UserService, grant bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := users.SetGroupPermission(r.Context(), authenticatedUserID(r), r.PathValue("id"), r.PathValue("permissionId"), grant); err != nil {
			writeSystemError(w, err)
			return
		}
		recordRequestLog(r, "info", "changed group permission "+r.PathValue("permissionId")+" on "+r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	}
}
func listRoleUsers(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := users.ListRoleUsers(r.Context(), r.PathValue("id"))
		if err != nil {
			writeSystemError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": items})
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
