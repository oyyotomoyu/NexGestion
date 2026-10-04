package apis

import (
	"errors"
	"net/http"
	"strings"

	"nexgestion/server/system"
)

func listSecurityEvents(security *system.SecurityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		items, err := security.ListEvents(r.Context(), query)
		if err != nil {
			if writeListQueryError(w, err) {
				return
			}
			writeSecurityError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse("events", items))
	}
}

func listSecurityRules(security *system.SecurityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rules, err := security.ListRules(r.Context())
		if err != nil {
			writeSecurityError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
	}
}

func updateSecurityRule(security *system.SecurityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.SetSecurityRuleInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		rule, err := security.SetRule(r.Context(), authenticatedUserID(r), r.PathValue("key"), input)
		if err != nil {
			writeSecurityError(w, err)
			return
		}
		recordRequestLog(r, "warning", "updated security rule "+rule.RuleKey)
		writeJSON(w, http.StatusOK, rule)
	}
}

func getSecuritySettings(security *system.SecurityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := security.GetSettings(r.Context())
		if err != nil {
			writeSecurityError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

// updateSecuritySettings additionally records a security event on a posture
// change: SIEM.md Section 7.2 treats toggling lan_only/internet_exposed as a
// security-relevant configuration change in its own right, at the same
// severity as a network/interface change, since it redefines what "normal"
// means for every other rule.
func updateSecuritySettings(security *system.SecurityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.UpdateSecuritySettingsInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		before, err := security.GetSettings(r.Context())
		if err != nil {
			writeSecurityError(w, err)
			return
		}
		settings, err := security.SetSettings(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeSecurityError(w, err)
			return
		}
		recordRequestLog(r, "warning", "updated security settings")
		if input.DeploymentPosture != nil && before.DeploymentPosture != settings.DeploymentPosture {
			severity := "warning"
			if settings.DeploymentPosture == "internet_exposed" {
				severity = "critical"
			}
			_, _ = security.RecordEvent(r.Context(), system.SecurityEventInput{
				EventType:   "deployment_posture_changed",
				Severity:    severity,
				ActorUserID: authenticatedUserID(r),
				SourceIP:    clientIP(r),
				Module:      "security",
				RecordRef:   settings.DeploymentPosture,
				Summary:     "deployment posture changed from " + before.DeploymentPosture + " to " + settings.DeploymentPosture,
			})
		}
		writeJSON(w, http.StatusOK, settings)
	}
}

func listSecurityRecipients(security *system.SecurityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		recipients, err := security.ListRecipients(r.Context())
		if err != nil {
			writeSecurityError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"recipients": recipients})
	}
}

func addSecurityRecipient(security *system.SecurityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input system.AddSecurityAlertRecipientInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		recipient, err := security.AddRecipient(r.Context(), authenticatedUserID(r), input)
		if err != nil {
			writeSecurityError(w, err)
			return
		}
		recordRequestLog(r, "info", "added security alert recipient "+recipient.ID)
		writeJSON(w, http.StatusCreated, recipient)
	}
}

func removeSecurityRecipient(security *system.SecurityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := security.RemoveRecipient(r.Context(), id); err != nil {
			writeSecurityError(w, err)
			return
		}
		recordRequestLog(r, "info", "removed security alert recipient "+id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func sendSecurityTestAlert(security *system.SecurityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		event, err := security.SendTestAlert(r.Context(), authenticatedUserID(r), clientIP(r))
		if err != nil {
			writeSecurityError(w, err)
			return
		}
		recordRequestLog(r, "info", "sent security test alert")
		writeJSON(w, http.StatusOK, event)
	}
}

func writeSecurityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, system.ErrSecurityRuleNotFound), errors.Is(err, system.ErrSecurityRecipientNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	case errors.Is(err, system.ErrSecurityInvalid), errors.Is(err, system.ErrInvalidListQuery):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	case strings.Contains(err.Error(), "UNIQUE constraint failed"):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "data already exists"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}
