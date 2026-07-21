package apis

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	applogs "nexgestion/server/logs"
	"nexgestion/server/system"
)

const maxJSONBodySize = 1 << 20

func listUsers(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := users.List(r.Context())
		if err != nil {
			writeSystemError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": result})
	}
}

func getUser(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := users.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeSystemError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func createUser(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateUserInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		result, err := users.Create(r.Context(), input)
		if err != nil {
			writeSystemError(w, err)
			return
		}
		w.Header().Set("Location", "/api/users/"+result.ID)
		recordRequestLog(r, "info", "created user "+result.ID)
		writeJSON(w, http.StatusCreated, result)
	}
}

func updateUser(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateUserInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		result, err := users.Update(r.Context(), r.PathValue("id"), input)
		if err != nil {
			writeSystemError(w, err)
			return
		}
		recordRequestLog(r, "info", "updated user "+result.ID)
		writeJSON(w, http.StatusOK, result)
	}
}

func deleteUser(users *system.UserService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := users.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeSystemError(w, err)
			return
		}
		recordRequestLog(r, "info", "deleted user "+r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	}
}

func recordRequestLog(r *http.Request, status, content string) {
	if logger := applogs.FromContext(r.Context()); logger != nil {
		_ = logger.Log(status, content)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return errors.New("invalid JSON body: " + err.Error())
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("JSON body must contain one object")
	}
	return nil
}

func writeSystemError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, system.ErrUserNotFound), errors.Is(err, system.ErrRoleNotFound), errors.Is(err, system.ErrGroupNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, system.ErrRoleAssigned), errors.Is(err, system.ErrGroupInUse):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, system.ErrRoleProtected), errors.Is(err, system.ErrAdminRequired):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	case strings.Contains(err.Error(), "UNIQUE constraint failed"):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "data already exists"})
	case strings.Contains(err.Error(), "protected user"):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, system.ErrInvalidParent), strings.Contains(err.Error(), "required"), strings.Contains(err.Error(), "invalid"),
		strings.Contains(err.Error(), "cannot be empty"), strings.Contains(err.Error(), "at least 12"):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
