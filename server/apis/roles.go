package apis

import (
	"net/http"

	"nexgestion/server/system"
)

func listRoles(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		roles, err := users.ListRoles(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeSystemError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("roles", roles))
	}
}

func getRole(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role, err := users.GetRole(r.Context(), r.PathValue("id"))
		if err != nil {
			writeSystemError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, role)
	}
}

func createRole(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateRoleInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		role, err := users.CreateRole(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeSystemError(w, err)
			return
		}
		w.Header().Set("Location", "/api/roles/"+role.ID)
		recordRequestLog(r, "info", "created role "+role.ID)
		writeJSON(w, http.StatusCreated, role)
	}
}

func updateRole(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateRoleInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		role, err := users.UpdateRole(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
		if err != nil {
			writeSystemError(w, err)
			return
		}
		recordRequestLog(r, "info", "updated role "+role.ID)
		writeJSON(w, http.StatusOK, role)
	}
}

func deleteRole(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := users.DeleteRole(r.Context(), authenticatedUserID(r), id); err != nil {
			writeSystemError(w, err)
			return
		}
		recordRequestLog(r, "info", "deleted role "+id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func authenticatedUserID(r *http.Request) string {
	claims, _ := r.Context().Value(authContextKey{}).(*system.AccessClaims)
	if claims == nil {
		return ""
	}
	return claims.Subject
}
