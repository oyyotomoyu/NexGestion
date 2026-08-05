package apis

import (
	"errors"
	"net/http"
	"strings"

	"nexgestion/server/system"
)

func listNotificationTypes(notifications *system.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		types, err := notifications.ListTypes(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeNotificationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("notification_types", types))
	}
}

func listMyNotifications(notifications *system.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := notifications.ListForUser(r.Context(), authenticatedUserID(r), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeNotificationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("notifications", items))
	}
}

func listAdminNotifications(notifications *system.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := notifications.ListAll(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeNotificationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("notifications", items))
	}
}

func createNotification(notifications *system.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.CreateNotificationInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		item, err := notifications.Create(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeNotificationError(w, err)
			return
		}
		recordRequestLog(r, "info", "created notification "+item.ID)
		writeJSON(w, http.StatusCreated, item)
	}
}

func updateNotification(notifications *system.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateNotificationInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "invalid_request", "error": err.Error()})
			return
		}
		item, err := notifications.Update(r.Context(), authenticatedUserID(r), r.PathValue("id"), input)
		if err != nil {
			writeNotificationError(w, err)
			return
		}
		recordRequestLog(r, "warning", "edited notification "+item.ID)
		writeJSON(w, http.StatusOK, item)
	}
}

func hideNotification(notifications *system.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := notifications.Hide(r.Context(), authenticatedUserID(r), r.PathValue("id")); err != nil {
			writeNotificationError(w, err)
			return
		}
		recordRequestLog(r, "warning", "hid notification "+r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	}
}

func exportNotificationsCSV(notifications *system.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		month := strings.TrimSpace(r.PathValue("month"))
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="notifications-`+month+`.csv"`)
		if err := notifications.ExportCSV(r.Context(), month, w); err != nil {
			writeNotificationError(w, err)
			return
		}
	}
}

func writeNotificationError(w http.ResponseWriter, err error) {
	code, status := "notification_internal_error", http.StatusInternalServerError
	switch {
	case errors.Is(err, system.ErrNotificationInvalid):
		code, status = "notification_invalid", http.StatusBadRequest
	case errors.Is(err, system.ErrNotificationNotFound):
		code, status = "notification_not_found", http.StatusNotFound
	case errors.Is(err, system.ErrNotificationPermission), errors.Is(err, system.ErrPermissionDenied):
		code, status = "notification_permission_denied", http.StatusForbidden
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		message = "internal server error"
	}
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
