package apis

import (
	"net/http"

	"nexgestion/server/system"
)

func listGroups(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groups, err := users.ListGroups(r.Context())
		if err != nil {
			writeSystemError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
	}
}

func getGroup(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		group, err := users.GetGroup(r.Context(), r.PathValue("id"))
		if err != nil {
			writeSystemError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, group)
	}
}

func createGroup(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateGroupInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		group, err := users.CreateGroup(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeSystemError(w, err)
			return
		}
		w.Header().Set("Location", "/api/groups/"+group.ID)
		recordRequestLog(r, "info", "created group "+group.ID)
		writeJSON(w, http.StatusCreated, group)
	}
}

func updateGroup(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateGroupInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		group, err := users.UpdateGroup(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
		if err != nil {
			writeSystemError(w, err)
			return
		}
		recordRequestLog(r, "info", "updated group "+group.ID)
		writeJSON(w, http.StatusOK, group)
	}
}

func deleteGroup(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := users.DeleteGroup(r.Context(), authenticatedUserID(r), id); err != nil {
			writeSystemError(w, err)
			return
		}
		recordRequestLog(r, "info", "deleted group "+id)
		w.WriteHeader(http.StatusNoContent)
	}
}
